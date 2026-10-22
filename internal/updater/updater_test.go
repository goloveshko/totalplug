package updater

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/goloveshko/totalplug/internal/config"
	"github.com/goloveshko/totalplug/internal/domain"
)

// TestMain keeps every update test's download inside the test's own scratch
// folder, and its backups inside a data folder of its own, so the package
// neither writes to nor sweeps the machine's %TEMP% nor touches the data folder
// of whatever install happens to be running.
func TestMain(m *testing.M) {
	scratch, err := os.MkdirTemp("", "totalplug_scratch_test_*")
	if err != nil {
		panic(err)
	}
	if err := os.Setenv(config.ScratchEnv, filepath.Join(scratch, "scratch")); err != nil {
		panic(err)
	}
	if err := os.Setenv(config.DataDirEnv, filepath.Join(scratch, "data")); err != nil {
		panic(err)
	}
	code := m.Run()
	_ = os.RemoveAll(scratch)
	os.Exit(code)
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mkdir for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func readTestFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func buildTestZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		f, err := zw.Create(name)
		if err != nil {
			t.Fatalf("zip create %s: %v", name, err)
		}
		if _, err := io.WriteString(f, content); err != nil {
			t.Fatalf("zip write %s: %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	return buf.Bytes()
}

func serveTestZip(t *testing.T, data []byte) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/zip")
		_, _ = w.Write(data)
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func TestBackupPluginCopiesTreeKeepingOriginal(t *testing.T) {
	root := t.TempDir()
	pluginDir := filepath.Join(root, "wcx", "demo")
	writeTestFile(t, filepath.Join(pluginDir, "demo.wdx"), "binary")
	writeTestFile(t, filepath.Join(pluginDir, "readme.txt"), "readme")
	writeTestFile(t, filepath.Join(pluginDir, "sub", "data.bin"), "data")

	backupDir, err := NewUpdater().BackupPlugin(pluginDir)
	if err != nil {
		t.Fatalf("BackupPlugin: %v", err)
	}

	if _, err := os.Stat(pluginDir); err != nil {
		t.Fatalf("original plugin dir must stay in place: %v", err)
	}
	if !strings.HasPrefix(filepath.Base(backupDir), "demo.bak_") {
		t.Errorf("backup dir name = %q, want prefix demo.bak_", filepath.Base(backupDir))
	}

	// The copy belongs to the data folder, not to the Total Commander tree: a
	// dead copy left beside the plugin is pollution in the user's own directory.
	if want := filepath.Join(config.BackupSpacePath(), "wcx"); filepath.Dir(backupDir) != want {
		t.Errorf("backup dir = %q, want it inside %q", filepath.Dir(backupDir), want)
	}
	if siblings, _ := filepath.Glob(filepath.Join(filepath.Dir(pluginDir), "demo.bak_*")); len(siblings) != 0 {
		t.Errorf("no backup may be written next to the plugin, found %v", siblings)
	}

	for rel, want := range map[string]string{
		"demo.wdx":                       "binary",
		"readme.txt":                     "readme",
		filepath.Join("sub", "data.bin"): "data",
	} {
		if got := readTestFile(t, filepath.Join(backupDir, rel)); got != want {
			t.Errorf("backup content %q = %q, want %q", rel, got, want)
		}
	}
}

// The version that wrote backups beside the plugin left them there, and updating
// that plugin is what moves them out — retention cannot reach a folder it does
// not own.
func TestBackupPluginMovesLegacyCopiesIntoTheSpace(t *testing.T) {
	root := t.TempDir()
	pluginDir := filepath.Join(root, "Legacy", "demo")
	writeTestFile(t, filepath.Join(pluginDir, "demo.wdx"), "binary")
	space := backupSpaceFor(t, pluginDir)

	legacy := filepath.Join(root, "Legacy", "demo.bak_"+strconv.FormatInt(time.Now().Add(-24*time.Hour).Unix(), 10))
	writeTestFile(t, filepath.Join(legacy, "demo.wdx"), "ancient")
	notOurs := filepath.Join(root, "Legacy", "demo.bak_manual")
	writeTestFile(t, filepath.Join(notOurs, "keep.txt"), "mine")

	if _, err := NewUpdater().BackupPlugin(pluginDir); err != nil {
		t.Fatalf("BackupPlugin: %v", err)
	}

	if _, err := os.Stat(filepath.Join(space, filepath.Base(legacy), "demo.wdx")); err != nil {
		t.Errorf("old copy should be readable in %s: %v", space, err)
	}
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Error("the Total Commander tree should be rid of the old copy")
	}
	if _, err := os.Stat(notOurs); err != nil {
		t.Errorf("a copy this tool did not write must stay where it is: %v", err)
	}
}

func TestUpdatePluginSuccessKeepsBackup(t *testing.T) {
	root := t.TempDir()
	pluginDir := filepath.Join(root, "wdx", "demo")
	dllPath := filepath.Join(pluginDir, "demo.dll")
	writeTestFile(t, dllPath, "old-dll")
	writeTestFile(t, filepath.Join(pluginDir, "keep.txt"), "extra-file")

	downloadURL := serveTestZip(t, buildTestZip(t, map[string]string{
		"demo.dll":      "new-dll",
		"docs/info.txt": "release-notes",
	}))

	outcome, err := NewUpdater().UpdatePlugin(context.Background(), domain.Plugin{
		Path:        dllPath,
		DownloadURL: downloadURL,
	})
	if err != nil {
		t.Fatalf("UpdatePlugin: %v", err)
	}

	if got := readTestFile(t, dllPath); got != "new-dll" {
		t.Errorf("plugin.dll = %q, want new-dll", got)
	}
	if got := readTestFile(t, filepath.Join(pluginDir, "docs", "info.txt")); got != "release-notes" {
		t.Errorf("docs/info.txt = %q, want release-notes", got)
	}
	if got := readTestFile(t, filepath.Join(outcome.BackupPath, "demo.dll")); got != "old-dll" {
		t.Errorf("backup demo.dll = %q, want old-dll", got)
	}
	// A rollback may take only these out of the folder, so keep.txt — the file the
	// user put there himself — must not be on the list.
	if strings.Join(outcome.AppliedFiles, ",") != "demo.dll,"+filepath.Join("docs", "info.txt") {
		t.Errorf("AppliedFiles = %v, want the two files the archive carried", outcome.AppliedFiles)
	}
}

func TestUpdatePluginRollsBackOnApplyFailure(t *testing.T) {
	root := t.TempDir()
	pluginDir := filepath.Join(root, "wdx", "demo")
	dllPath := filepath.Join(pluginDir, "demo.dll")
	writeTestFile(t, dllPath, "old-dll")
	writeTestFile(t, filepath.Join(pluginDir, "a.txt"), "a-old")

	// Lexicographic walk order in applyTree is a.txt, b.txt, demo.dll, new.txt,
	// so the read-only b.txt fails after a.txt was applied but before demo.dll.
	lockedPath := filepath.Join(pluginDir, "b.txt")
	writeTestFile(t, lockedPath, "b-old")
	if err := os.Chmod(lockedPath, 0444); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(lockedPath, 0644) })

	downloadURL := serveTestZip(t, buildTestZip(t, map[string]string{
		"a.txt":    "a-new",
		"b.txt":    "b-new",
		"demo.dll": "new-dll",
		"new.txt":  "created",
	}))

	_, err := NewUpdater().UpdatePlugin(context.Background(), domain.Plugin{
		Path:        dllPath,
		DownloadURL: downloadURL,
	})
	if err == nil {
		t.Fatal("expected apply failure, got nil")
	}
	if !strings.Contains(err.Error(), "rolled back") {
		t.Errorf("error should mention rollback: %v", err)
	}
	// A read-only (access denied) target must surface as a lockable-file error.
	if !errors.Is(err, ErrFileLocked) {
		t.Errorf("error should wrap ErrFileLocked for a locked target: %v", err)
	}

	if got := readTestFile(t, filepath.Join(pluginDir, "a.txt")); got != "a-old" {
		t.Errorf("a.txt after rollback = %q, want a-old", got)
	}
	if got := readTestFile(t, dllPath); got != "old-dll" {
		t.Errorf("demo.dll after rollback = %q, want old-dll", got)
	}
	if _, err := os.Stat(filepath.Join(pluginDir, "new.txt")); !os.IsNotExist(err) {
		t.Errorf("new.txt should have been removed by rollback")
	}

	space, err := backupNamespace(pluginDir)
	if err != nil {
		t.Fatalf("backupNamespace: %v", err)
	}
	leftovers, _ := filepath.Glob(filepath.Join(space, "*"))
	if len(leftovers) != 0 {
		t.Errorf("backup dir should be removed after successful rollback, found %v", leftovers)
	}
}

// serveCountedZip is serveTestZip with a tally of how often the host was asked,
// which is how the tests below tell a reused package from a re-downloaded one.
func serveCountedZip(t *testing.T, data []byte, hits *int32) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(hits, 1)
		w.Header().Set("Content-Type", "application/zip")
		_, _ = w.Write(data)
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

// keptFor reports whether this run's scratch holds a package back for one URL. It
// asks about a single slot rather than listing the folder because every test in
// this package shares the one run folder the process was given, and a failed
// update in another test is legitimately sitting there.
func keptFor(t *testing.T, rawURL string) bool {
	t.Helper()
	runDir, err := config.RunScratch()
	if err != nil {
		t.Fatalf("RunScratch: %v", err)
	}
	_, err = os.Stat(filepath.Join(runDir, keptSpace, packageKey(rawURL)))
	switch {
	case err == nil:
		return true
	case os.IsNotExist(err):
		return false
	default:
		t.Fatalf("reading the kept package for %s: %v", rawURL, err)
		return false
	}
}

// The locked-retry path is the one a user reaches for: Total Commander holds the
// plugin, the window says so, and closing it and asking again should not have to
// fetch the same package a second time.
func TestARetriedUpdateReusesThePackageItKept(t *testing.T) {
	root := t.TempDir()
	pluginDir := filepath.Join(root, "wdx", "demo")
	dllPath := filepath.Join(pluginDir, "demo.dll")
	writeTestFile(t, dllPath, "old-dll")
	writeTestFile(t, filepath.Join(pluginDir, "a.txt"), "a-old")

	lockedPath := filepath.Join(pluginDir, "b.txt")
	writeTestFile(t, lockedPath, "b-old")
	if err := os.Chmod(lockedPath, 0444); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(lockedPath, 0644) })

	var hits int32
	downloadURL := serveCountedZip(t, buildTestZip(t, map[string]string{
		"a.txt":    "a-new",
		"b.txt":    "b-new",
		"demo.dll": "new-dll",
	}), &hits)

	u := NewUpdater()
	plugin := domain.Plugin{Path: dllPath, DownloadURL: downloadURL}
	if _, err := u.UpdatePlugin(context.Background(), plugin); err == nil {
		t.Fatal("expected the blocked apply to fail")
	}
	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Fatalf("the first attempt asked the host %d times; want 1", got)
	}
	if !keptFor(t, downloadURL) {
		t.Fatal("the package that could not be applied should be kept for the retry")
	}

	// Total Commander is closed: the same ask now goes through on the bytes that
	// are already on disk.
	if err := os.Chmod(lockedPath, 0644); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	if _, err := u.UpdatePlugin(context.Background(), plugin); err != nil {
		t.Fatalf("second UpdatePlugin: %v", err)
	}
	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Errorf("the retry asked the host %d times; want the kept package reused", got)
	}
	if got := readTestFile(t, dllPath); got != "new-dll" {
		t.Errorf("demo.dll = %q, want new-dll", got)
	}
	if keptFor(t, downloadURL) {
		t.Error("the retry applied the kept package, so it should be gone")
	}
}

func TestKeptHTTPPackageRequiresConsentForEachPlugin(t *testing.T) {
	for _, redirected := range []bool{false, true} {
		t.Run(fmt.Sprintf("redirect=%t", redirected), func(t *testing.T) {
			var hits int32
			payload := buildTestZip(t, map[string]string{"demo.dll": "new", "b.txt": "new"})
			plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				atomic.AddInt32(&hits, 1)
				_, _ = w.Write(payload)
			}))
			defer plain.Close()
			plainURL := "http://plugins.example.com/consent.zip"
			source := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Redirect(w, r, plainURL, http.StatusFound)
			}))
			defer source.Close()

			transport := source.Client().Transport.(*http.Transport).Clone()
			transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
				if addr == "plugins.example.com:80" {
					addr = plain.Listener.Addr().String()
				}
				return (&net.Dialer{}).DialContext(ctx, network, addr)
			}
			defer transport.CloseIdleConnections()
			u := NewUpdater()
			u.client.Transport = transport
			downloadURL := plainURL
			if redirected {
				downloadURL = source.URL + "/package.zip"
			}

			root := t.TempDir()
			a := domain.Plugin{Path: filepath.Join(root, "a", "demo.dll"), DownloadURL: downloadURL}
			b := domain.Plugin{Path: filepath.Join(root, "b", "demo.dll"), DownloadURL: downloadURL}
			for _, p := range []domain.Plugin{a, b} {
				writeTestFile(t, p.Path, "old")
				writeTestFile(t, filepath.Join(filepath.Dir(p.Path), "b.txt"), "old")
			}
			locked := filepath.Join(filepath.Dir(a.Path), "b.txt")
			if err := os.Chmod(locked, 0444); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.Chmod(locked, 0644) })
			if _, err := u.UpdatePluginInsecure(context.Background(), a); err == nil {
				t.Fatal("expected the first plugin's apply to fail")
			}
			if got := atomic.LoadInt32(&hits); got != 1 {
				t.Fatalf("first download hits = %d, want 1", got)
			}

			for _, p := range []domain.Plugin{b, a} {
				if _, err := u.UpdatePlugin(context.Background(), p); !errors.Is(err, ErrPlainHTTPSource) {
					t.Fatalf("strict update of %s = %v, want HTTP consent", p.Path, err)
				}
				if got := readTestFile(t, p.Path); got != "old" {
					t.Fatalf("strict update changed %s to %q", p.Path, got)
				}
			}
			if err := os.Chmod(locked, 0644); err != nil {
				t.Fatal(err)
			}
			if _, err := u.UpdatePluginInsecure(context.Background(), a); err != nil {
				t.Fatalf("consented retry: %v", err)
			}
			if got := atomic.LoadInt32(&hits); got != 1 {
				t.Fatalf("consented retry downloaded again, hits = %d", got)
			}
			if _, err := u.UpdatePluginInsecure(context.Background(), b); err != nil {
				t.Fatalf("second plugin with its own consent: %v", err)
			}
			if got := atomic.LoadInt32(&hits); got != 2 {
				t.Errorf("second plugin reused another plugin's consent cache, hits = %d", got)
			}
		})
	}
}

func TestHTTPConsentSurvivesOtherCandidateFailures(t *testing.T) {
	payload := buildTestZip(t, map[string]string{"demo.dll": "new"})
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(payload)
	}))
	defer plain.Close()
	mirror := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/good.zip" {
			_, _ = w.Write(payload)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer mirror.Close()
	plainURL := "http://plugins.example.com/fallback.zip"
	for _, plainFirst := range []bool{true, false} {
		t.Run(fmt.Sprintf("plain-first=%t", plainFirst), func(t *testing.T) {
			transport := mirror.Client().Transport.(*http.Transport).Clone()
			transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
				if addr == "plugins.example.com:80" {
					addr = plain.Listener.Addr().String()
				}
				return (&net.Dialer{}).DialContext(ctx, network, addr)
			}
			defer transport.CloseIdleConnections()
			u := NewUpdater()
			u.client.Transport = transport
			urls := []string{plainURL, mirror.URL + "/dead.zip"}
			if !plainFirst {
				urls[0], urls[1] = urls[1], urls[0]
			}
			p := domain.Plugin{Path: filepath.Join(t.TempDir(), "demo", "demo.dll"), DownloadURLs: urls}
			writeTestFile(t, p.Path, "old")
			if _, err := u.UpdatePlugin(context.Background(), p); !errors.Is(err, ErrPlainHTTPSource) || !strings.Contains(err.Error(), "404") {
				t.Fatalf("candidate errors = %v, want both HTTP consent and 404", err)
			}
			if got := readTestFile(t, p.Path); got != "old" {
				t.Fatalf("refused update changed plugin to %q", got)
			}
			if _, err := u.UpdatePluginInsecure(context.Background(), p); err != nil {
				t.Fatalf("consented update: %v", err)
			}
		})
	}
	t.Run("usable-https-needs-no-consent", func(t *testing.T) {
		u := NewUpdater()
		u.client.Transport = mirror.Client().Transport
		p := domain.Plugin{
			Path:         filepath.Join(t.TempDir(), "demo", "demo.dll"),
			DownloadURLs: []string{plainURL, mirror.URL + "/good.zip"},
		}
		writeTestFile(t, p.Path, "old")
		if _, err := u.UpdatePlugin(context.Background(), p); err != nil {
			t.Fatalf("usable HTTPS fallback: %v", err)
		}
	})
}

// Magic bytes alone do not make a package readable, so one that fails to unpack
// must not be held back: a retry has to be free to download another copy.
func TestAPackageThatWillNotUnpackIsNotKept(t *testing.T) {
	root := t.TempDir()
	pluginDir := filepath.Join(root, "wdx", "demo")
	dllPath := filepath.Join(pluginDir, "demo.dll")
	writeTestFile(t, dllPath, "old-dll")

	whole := buildTestZip(t, map[string]string{"demo.dll": "new-dll"})
	truncated := whole[:len(whole)/2]

	var hits int32
	downloadURL := serveCountedZip(t, truncated, &hits)

	u := NewUpdater()
	plugin := domain.Plugin{Path: dllPath, DownloadURL: downloadURL}
	for attempt := 1; attempt <= 2; attempt++ {
		if _, err := u.UpdatePlugin(context.Background(), plugin); err == nil {
			t.Fatalf("attempt %d: expected the truncated package to fail", attempt)
		}
		if got := atomic.LoadInt32(&hits); got != int32(attempt) {
			t.Errorf("attempt %d asked the host %d times; want a fresh download", attempt, got)
		}
	}
	if keptFor(t, downloadURL) {
		t.Error("an unreadable package should not be kept")
	}
}

// A package that has been written into the plugin folder is spent: the kept folder
// holds only what a retry still needs, and the run's scratch goes with the run.
func TestASuccessfulUpdateKeepsNothingBehind(t *testing.T) {
	root := t.TempDir()
	pluginDir := filepath.Join(root, "wdx", "demo")
	dllPath := filepath.Join(pluginDir, "demo.dll")
	writeTestFile(t, dllPath, "old-dll")

	downloadURL := serveTestZip(t, buildTestZip(t, map[string]string{"demo.dll": "new-dll"}))

	if _, err := NewUpdater().UpdatePlugin(context.Background(), domain.Plugin{
		Path:        dllPath,
		DownloadURL: downloadURL,
	}); err != nil {
		t.Fatalf("UpdatePlugin: %v", err)
	}
	if keptFor(t, downloadURL) {
		t.Error("an applied package should be forgotten")
	}
}

// copyFile renames into place, so a target it could not replace is left
// byte-intact. Truncating in place instead used to leave a damaged file that
// applyTree had not recorded, so rollback skipped it and UpdatePlugin then
// deleted the backup holding the only good copy.
func TestCopyFileFailureLeavesTargetIntact(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src", "b.txt")
	dst := filepath.Join(dir, "dst", "b.txt")
	writeTestFile(t, src, "b-new")
	writeTestFile(t, dst, "b-old")

	if err := os.Chmod(dst, 0444); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dst, 0644) })

	err := copyFile(src, dst, 0644)
	if err == nil {
		t.Fatal("expected copyFile to fail on a read-only target")
	}
	if !isLockedErr(err) {
		t.Errorf("a target that denies writes should surface as lockable: %v", err)
	}
	if got := readTestFile(t, dst); got != "b-old" {
		t.Errorf("target after a failed copy = %q, want the untouched b-old", got)
	}

	leftovers, _ := filepath.Glob(filepath.Join(dir, "dst", ".totalplug-copy-*"))
	if len(leftovers) != 0 {
		t.Errorf("a failed copy left temp files behind: %v", leftovers)
	}
}

func TestUpdatePluginErrorPaths(t *testing.T) {
	upd := NewUpdater()

	if _, err := upd.UpdatePlugin(context.Background(), domain.Plugin{}); !errors.Is(err, ErrEmptyDownloadURL) {
		t.Errorf("empty URL error = %v, want ErrEmptyDownloadURL", err)
	}

	missingDirPlugin := filepath.Join(t.TempDir(), "absent", "p.dll")
	_, err := upd.UpdatePlugin(context.Background(), domain.Plugin{
		Path:        missingDirPlugin,
		DownloadURL: "https://example.invalid/package.zip",
	})
	if !errors.Is(err, ErrPluginNotFound) {
		t.Errorf("missing dir error = %v, want ErrPluginNotFound", err)
	}
}

func TestValidateDownloadURL(t *testing.T) {
	cases := []struct {
		url     string
		wantErr bool
	}{
		{"https://github.com/x/y.zip", false},
		{"HTTPS://Example.COM/a.zip", false},
		{"http://localhost:8080/a.zip", false},
		{"http://127.0.0.1:9/a.zip", false},
		{"http://[::1]:9/a.zip", false},
		{"http://plugins.example.com/a.zip", true},
		{"ftp://example.com/a.zip", true},
		{"file:///C:/Windows/x.zip", true},
		{"/relative/path.zip", true},
	}
	for _, tc := range cases {
		err := validateDownloadURL(tc.url, false)
		if tc.wantErr && !errors.Is(err, ErrInsecureScheme) {
			t.Errorf("validateDownloadURL(%q) = %v, want ErrInsecureScheme", tc.url, err)
		}
		if !tc.wantErr && err != nil {
			t.Errorf("validateDownloadURL(%q) = %v, want nil", tc.url, err)
		}
	}
}

func TestValidateDownloadURLInsecureOptIn(t *testing.T) {
	// The GUI asks the user before passing this flag; the opt-in must not also
	// open non-network schemes such as a local file.
	if err := validateDownloadURL("http://plugins.example.com/a.zip", true); err != nil {
		t.Errorf("opted-in http: %v", err)
	}
	for _, raw := range []string{"ftp://example.com/a.zip", "file:///C:/Windows/x.zip"} {
		if err := validateDownloadURL(raw, true); !errors.Is(err, ErrInsecureScheme) {
			t.Errorf("validateDownloadURL(%q, true) = %v, want ErrInsecureScheme", raw, err)
		}
	}
}

// The GUI reads ErrPlainHTTPSource to decide whether to offer a retry over plain
// HTTP, so only the source the opt-in would actually accept may carry it.
func TestInsecureRefusalIsRetryableOnlyForPlainHTTP(t *testing.T) {
	if err := validateDownloadURL("http://plugins.example.com/a.zip", false); !errors.Is(err, ErrPlainHTTPSource) {
		t.Errorf("http refusal = %v, want ErrPlainHTTPSource", err)
	}
	for _, raw := range []string{"ftp://example.com/a.zip", "file:///C:/Windows/x.zip", "/relative/path.zip"} {
		if err := validateDownloadURL(raw, false); errors.Is(err, ErrPlainHTTPSource) {
			t.Errorf("validateDownloadURL(%q) = %v, must not be offered for a retry", raw, err)
		}
	}
}

func TestDownloadFileRejectsInsecureURLBeforeNetwork(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "out.zip")
	err := NewUpdater().DownloadFile(context.Background(), "http://plugins.example.com/pkg.zip", dest)
	if !errors.Is(err, ErrInsecureScheme) {
		t.Fatalf("error = %v, want ErrInsecureScheme", err)
	}
	if _, e := os.Stat(dest); !os.IsNotExist(e) {
		t.Error("destination file must not be created for rejected URLs")
	}
}

func TestDownloadFileRejectsInsecureRedirect(t *testing.T) {
	// validateDownloadURL only ever sees the URL it was handed; the redirect the
	// server answers with used to be followed silently, so an https endpoint could
	// point the download at plaintext http on a host we never vetted.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://plugins.example.com/pkg.zip", http.StatusFound)
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "out.zip")
	err := NewUpdater().DownloadFile(context.Background(), srv.URL+"/pkg.zip", dest)
	if !errors.Is(err, ErrInsecureScheme) {
		t.Fatalf("error = %v, want ErrInsecureScheme", err)
	}
	// This redirect is the shape a totalcmd.net download link has: the catalog
	// hands out https and the author's server answers over plaintext. The GUI
	// offers the user a retry only when the refusal carries this signal.
	if !errors.Is(err, ErrPlainHTTPSource) {
		t.Errorf("error = %v, want the plain-HTTP signal a retry could clear", err)
	}
	if _, e := os.Stat(dest); !os.IsNotExist(e) {
		t.Error("a download refused mid-redirect must not leave a destination file")
	}

	// The guard must not cost the legitimate case: mirrors are a hop between two
	// hosts the caller already trusts, so a redirect that stays allowed downloads.
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("mirror payload"))
	}))
	defer target.Close()

	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/mirror.zip", http.StatusFound)
	}))
	defer source.Close()

	if err := NewUpdater().DownloadFile(context.Background(), source.URL+"/pkg.zip", dest); err != nil {
		t.Fatalf("an allowed redirect failed: %v", err)
	}
	body, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "mirror payload" {
		t.Errorf("downloaded %q, want the payload behind the redirect", body)
	}
}

// The retry the GUI offers after a plain-HTTP refusal must get past the policy
// instead of refusing again: the failure it ends in is a network one, so the
// user sees the real reason the author's server did not answer.
func TestDownloadFileInsecureOptInDoesNotRefusePlainHTTP(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	dest := filepath.Join(t.TempDir(), "out.zip")
	err := NewUpdater().downloadFile(ctx, "http://totalplug.invalid/pkg.zip", dest, true)
	if errors.Is(err, ErrInsecureScheme) {
		t.Fatalf("the opt-in refused a plain HTTP source: %v", err)
	}
	if err == nil {
		t.Fatal("a host that cannot exist must not download")
	}
}

func TestDownloadFileEnforcesSizeCap(t *testing.T) {
	// Advertised length path: handler writes a small body in one shot, so the
	// Go server sets Content-Length automatically.
	sized := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(bytes.Repeat([]byte("x"), 4096))
	}))
	defer sized.Close()

	// Streaming path: chunked body hides the real size from Content-Length.
	streamed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		chunk := bytes.Repeat([]byte("y"), 512)
		for i := 0; i < 64; i++ {
			_, _ = w.Write(chunk)
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		}
	}))
	defer streamed.Close()

	upd := NewUpdater()
	upd.maxDownloadBytes = 1024

	for _, url := range []string{sized.URL, streamed.URL} {
		dest := filepath.Join(t.TempDir(), "out.zip")
		err := upd.DownloadFile(context.Background(), url, dest)
		if !errors.Is(err, ErrDownloadTooLarge) {
			t.Errorf("DownloadFile(%s) error = %v, want ErrDownloadTooLarge", url, err)
		}
	}
}

// backupSpaceFor returns the namespace folder the updater writes pluginDir's
// copies into, emptied so a test starts from the copies it names. The package
// pins TOTALPLUG_DATA_DIR in TestMain, so this is a folder inside the test's own
// temp tree.
func backupSpaceFor(t *testing.T, pluginDir string) string {
	t.Helper()
	space, err := backupNamespace(pluginDir)
	if err != nil {
		t.Fatalf("backupNamespace: %v", err)
	}
	if err := os.RemoveAll(space); err != nil {
		t.Fatalf("clearing %s: %v", space, err)
	}
	return space
}

func TestPruneBackupSpaceDropsOldCopiesBeyondTheFloor(t *testing.T) {
	pluginDir := filepath.Join(t.TempDir(), "MyGroup", "MyPlugin")
	space := backupSpaceFor(t, pluginDir)

	names := []string{
		"MyPlugin.bak_1001", "MyPlugin.bak_1003", "MyPlugin.bak_1002",
		"MyPlugin.bak_manual", "Other.bak_9999", "MyPlugin.bak_plain",
	}
	for _, n := range names {
		if err := os.MkdirAll(filepath.Join(space, n), 0755); err != nil {
			t.Fatal(err)
		}
	}
	writeTestFile(t, filepath.Join(pluginDir, "plugin.dll"), "core")

	// Every seeded copy is from 1970, so all of them are past a 30-day limit and
	// only the floor of two protects the newest.
	removed, err := pruneBackupSpace(pluginDir, config.BackupPolicy{KeepNewest: 2, MaxAgeDays: 30})
	if err != nil {
		t.Fatalf("pruneBackupSpace: %v", err)
	}
	if len(removed) != 1 || filepath.Base(removed[0]) != "MyPlugin.bak_1001" {
		t.Errorf("removed = %v, want only MyPlugin.bak_1001", removed)
	}

	for _, keep := range []string{"MyPlugin.bak_1003", "MyPlugin.bak_1002", "MyPlugin.bak_manual", "Other.bak_9999", "MyPlugin.bak_plain"} {
		if _, err := os.Stat(filepath.Join(space, keep)); err != nil {
			t.Errorf("%s must survive pruning: %v", keep, err)
		}
	}
	if _, err := os.Stat(filepath.Join(pluginDir, "plugin.dll")); err != nil {
		t.Errorf("plugin dir must stay untouched: %v", err)
	}
}

func TestPruneBackupSpaceKeepsYoungCopies(t *testing.T) {
	pluginDir := filepath.Join(t.TempDir(), "YoungGroup", "P")
	space := backupSpaceFor(t, pluginDir)

	ago := func(d time.Duration) string {
		return fmt.Sprintf("P.bak_%d", time.Now().Add(-d).Unix())
	}
	recent1, recent2, stale := ago(time.Hour), ago(2*time.Hour), ago(40*24*time.Hour)
	for _, n := range []string{recent1, recent2, stale} {
		if err := os.MkdirAll(filepath.Join(space, n), 0755); err != nil {
			t.Fatal(err)
		}
	}

	// A one-copy floor plus a 30-day limit: the second copy is past the floor but
	// an hour old, so it stays. Age is what deletes, the floor is what protects.
	removed, err := pruneBackupSpace(pluginDir, config.BackupPolicy{KeepNewest: 1, MaxAgeDays: 30})
	if err != nil {
		t.Fatalf("pruneBackupSpace: %v", err)
	}
	if len(removed) != 1 || filepath.Base(removed[0]) != filepath.Base(stale) {
		t.Errorf("removed = %v, want only the 40-day copy", removed)
	}
	for _, n := range []string{recent1, recent2} {
		if _, err := os.Stat(filepath.Join(space, n)); err != nil {
			t.Errorf("%s must survive: %v", n, err)
		}
	}
}

func TestPruneBackupSpaceWithoutAgeLimitDeletesNothing(t *testing.T) {
	pluginDir := filepath.Join(t.TempDir(), "NoLimit", "P")
	space := backupSpaceFor(t, pluginDir)

	for _, n := range []string{"P.bak_1", "P.bak_2", "P.bak_3"} {
		if err := os.MkdirAll(filepath.Join(space, n), 0755); err != nil {
			t.Fatal(err)
		}
	}

	removed, err := pruneBackupSpace(pluginDir, config.BackupPolicy{KeepNewest: 2, MaxAgeDays: 0})
	if err != nil {
		t.Fatalf("pruneBackupSpace: %v", err)
	}
	if removed != nil {
		t.Errorf("removed = %v, want nothing deleted without an age limit", removed)
	}
}

func TestUpdatePluginPrunesOldBackups(t *testing.T) {
	pluginDir := filepath.Join(t.TempDir(), "WCX", "test")
	pluginFile := filepath.Join(pluginDir, "test.wcx")
	writeTestFile(t, pluginFile, "old")
	space := backupSpaceFor(t, pluginDir)

	// The copies the policy judges live in the data folder, not beside the
	// plugin; these timestamps are decades old, so the default 30-day limit
	// reaches the oldest of them while the floor of three keeps the rest.
	for _, n := range []string{"test.bak_1001", "test.bak_1002", "test.bak_1003"} {
		if err := os.MkdirAll(filepath.Join(space, n), 0755); err != nil {
			t.Fatal(err)
		}
	}

	archive := buildTestZip(t, map[string]string{"test.wcx": "new"})
	outcome, err := NewUpdater().UpdatePlugin(context.Background(), domain.Plugin{
		Path:        pluginFile,
		DownloadURL: serveTestZip(t, archive),
	})
	if err != nil {
		t.Fatalf("UpdatePlugin: %v", err)
	}

	// Three old copies plus the fresh one are four, so the oldest goes and the
	// backup returned to the caller survives.
	if _, err := os.Stat(outcome.BackupPath); err != nil {
		t.Errorf("fresh backup must survive pruning: %v", err)
	}
	if _, err := os.Stat(filepath.Join(space, "test.bak_1001")); !os.IsNotExist(err) {
		t.Error("oldest seeded backup should have been pruned")
	}
	for _, n := range []string{"test.bak_1002", "test.bak_1003"} {
		if _, err := os.Stat(filepath.Join(space, n)); err != nil {
			t.Errorf("%s must survive pruning: %v", n, err)
		}
	}
	if readTestFile(t, pluginFile) != "new" {
		t.Error("plugin file was not updated")
	}
}

func TestDetectArchiveFormat(t *testing.T) {
	rarHeader := append([]byte("Rar!\x1a\x07\x01"), bytes.Repeat([]byte{0}, 32)...)
	sevenZip := []byte{0x37, 0x7A, 0xBC, 0xAF, 0x27, 0x1C, 0x00, 0x04}
	zipBytes := buildTestZip(t, map[string]string{"a.txt": "x"})

	tests := []struct {
		name    string
		data    []byte
		want    string
		wantErr bool
	}{
		{"zip", zipBytes, "zip", false},
		{"rar", rarHeader, "rar", false},
		{"7z", sevenZip, "7z", false},
		{"gzip", []byte{0x1f, 0x8b, 8, 0}, "gzip", false},
		{"html page", []byte("<!DOCTYPE html><html>"), "", true},
		{"empty file", []byte{}, "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "pkg.bin")
			if err := os.WriteFile(path, tt.data, 0644); err != nil {
				t.Fatal(err)
			}
			got, err := detectArchiveFormat(path)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr = %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("format = %q, want %q", got, tt.want)
			}
			if err != nil && !errors.Is(err, ErrUnsupportedArchive) {
				t.Errorf("error must wrap ErrUnsupportedArchive, got %v", err)
			}
		})
	}
}

func TestUpdatePluginRejectsUnsupportedFormatBeforeTouchingPlugin(t *testing.T) {
	dir := t.TempDir()
	pluginFile := filepath.Join(dir, "WCX", "nl", "nl.wdx")
	writeTestFile(t, pluginFile, "original")

	gzipBody := []byte{0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/gzip")
		_, _ = w.Write(gzipBody)
	}))
	defer srv.Close()

	p := domain.Plugin{Path: pluginFile, DownloadURL: srv.URL}
	_, err := NewUpdater().UpdatePlugin(context.Background(), p)
	if !errors.Is(err, ErrUnsupportedArchive) {
		t.Fatalf("error = %v, want ErrUnsupportedArchive", err)
	}
	if readTestFile(t, pluginFile) != "original" {
		t.Error("plugin file must be untouched")
	}
	entries, _ := os.ReadDir(filepath.Dir(pluginFile))
	for _, e := range entries {
		if strings.Contains(e.Name(), ".bak_") {
			t.Errorf("no backup must be created for rejected format, found %s", e.Name())
		}
	}
}

// A wincmd.ini entry may point straight at a plugin file rather than into a
// folder of its own. Backup and unpack both take the whole parent directory, so
// such an update would replace everything else living beside that file.
func TestUpdatePluginRefusesFolderItDoesNotOwn(t *testing.T) {
	tests := []struct {
		name   string
		layout func(dir string) string
	}{
		{
			name: "Total Commander install root",
			layout: func(dir string) string {
				return filepath.Join(dir, "loose.wcx")
			},
		},
		{
			name: "folder shared by every plugin of a type",
			layout: func(dir string) string {
				return filepath.Join(dir, "Plugins", "loose.wcx")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			writeTestFile(t, filepath.Join(dir, "wincmd.exe"), "exe")
			pluginFile := tt.layout(dir)
			writeTestFile(t, pluginFile, "original")

			_, err := NewUpdater().UpdatePlugin(context.Background(), domain.Plugin{
				Path:        pluginFile,
				DownloadURL: "https://example.invalid/loose.zip",
			})
			if !errors.Is(err, ErrPluginNeedsOwnFolder) {
				t.Fatalf("error = %v, want ErrPluginNeedsOwnFolder", err)
			}
			if readTestFile(t, pluginFile) != "original" {
				t.Error("plugin file must be untouched")
			}
			entries, _ := os.ReadDir(filepath.Dir(pluginFile))
			for _, e := range entries {
				if strings.Contains(e.Name(), ".bak_") {
					t.Errorf("no backup must be created for a refused folder, found %s", e.Name())
				}
			}
		})
	}
}

func TestUpdatePluginAppliesRar(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("testdata", "nl_info_1.20.rar"))
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "WDX", "NL_Info")
	pluginFile := filepath.Join(pluginDir, "NL_Info.wdx")
	writeTestFile(t, pluginFile, "old-wdx")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rar")
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	outcome, err := NewUpdater().UpdatePlugin(context.Background(), domain.Plugin{
		Path:        pluginFile,
		DownloadURL: srv.URL,
	})
	if err != nil {
		t.Fatalf("UpdatePlugin: %v", err)
	}

	// The real archive ships both 32- and 64-bit binaries plus a Descript dir.
	info, err := os.Stat(filepath.Join(pluginDir, "NL_Info.wdx64"))
	if err != nil {
		t.Fatalf("NL_Info.wdx64 not extracted: %v", err)
	}
	if info.Size() != 47104 {
		t.Errorf("NL_Info.wdx64 size = %d, want 47104", info.Size())
	}
	if got := readTestFile(t, filepath.Join(pluginDir, "Descript", "ReadMe_eng.txt")); !strings.Contains(got, "NL_Info") {
		t.Error("nested rar entry content missing")
	}
	if readTestFile(t, pluginFile) == "old-wdx" {
		t.Error("NL_Info.wdx was not replaced")
	}
	if _, err := os.Stat(filepath.Join(outcome.BackupPath, "NL_Info.wdx")); err != nil {
		t.Errorf("backup must contain the pre-update wdx: %v", err)
	}
}

func TestUpdatePluginApplies7z(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("testdata", "test_plugin.7z"))
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "WCX", "demo")
	pluginFile := filepath.Join(pluginDir, "demo.dll")
	writeTestFile(t, pluginFile, "old-dll")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-7z-compressed")
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	if _, err := NewUpdater().UpdatePlugin(context.Background(), domain.Plugin{
		Path:        pluginFile,
		DownloadURL: srv.URL,
	}); err != nil {
		t.Fatalf("UpdatePlugin: %v", err)
	}

	if got := readTestFile(t, pluginFile); got != "sevenzip-payload" {
		t.Errorf("demo.dll = %q, want sevenzip-payload", got)
	}
	if got := readTestFile(t, filepath.Join(pluginDir, "docs", "readme.txt")); got != "hello from 7z" {
		t.Errorf("docs/readme.txt = %q", got)
	}
}

func TestUpdatePluginDownloadFailurePointsAtPluginPage(t *testing.T) {
	dir := t.TempDir()
	pluginFile := filepath.Join(dir, "WCX", "fi", "fi.wlx")
	writeTestFile(t, pluginFile, "original")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	_, err := NewUpdater().UpdatePlugin(context.Background(), domain.Plugin{
		Path:        pluginFile,
		DownloadURL: srv.URL,
		WebURL:      "https://totalcmd.net/plugring/fileinfo.html",
	})
	if err == nil || !strings.Contains(err.Error(), "plugin page: https://totalcmd.net/plugring/fileinfo.html") {
		t.Fatalf("error should point at the plugin page, got: %v", err)
	}
	if readTestFile(t, pluginFile) != "original" {
		t.Error("plugin file must be untouched")
	}
}

func TestUpdatePluginFallsBackToMirrorSource(t *testing.T) {
	dir := t.TempDir()
	pluginFile := filepath.Join(dir, "WLX", "fi", "fi.wlx")
	writeTestFile(t, pluginFile, "original")

	zipBody := buildTestZip(t, map[string]string{"fi.wlx": "updated"})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/mirror/") {
			w.Header().Set("Content-Type", "application/zip")
			_, _ = w.Write(zipBody)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	dead := srv.URL + "/download.php?id=fi"
	mirror := srv.URL + "/mirror/wlx_fi_2.23.zip"

	upd := NewUpdater()
	seen := map[string]bool{}
	upd.SetProgressListener(func(p domain.UpdateProgress) {
		if p.Phase == "download" {
			seen[p.URL] = true
		}
	})

	if _, err := upd.UpdatePlugin(context.Background(), domain.Plugin{
		Path:         pluginFile,
		DownloadURL:  dead,
		DownloadURLs: []string{dead, mirror},
	}); err != nil {
		t.Fatalf("mirror fallback should succeed: %v", err)
	}

	if readTestFile(t, pluginFile) != "updated" {
		t.Error("plugin file should contain the mirrored package")
	}
	if !seen[dead] || !seen[mirror] {
		t.Errorf("progress should report both attempts, seen: %v", seen)
	}
	upd.SetProgressListener(nil)
}

func TestSanitizeEntryPath(t *testing.T) {
	target := filepath.Join("C:", "tc", "plugins")
	for _, bad := range []string{"..\\evil.dll", "../../evil.dll", filepath.Join(target, "abs.dll"), ".."} {
		if _, err := sanitizeEntryPath(target, bad); err == nil {
			t.Errorf("sanitizeEntryPath(%q) must reject traversal", bad)
		}
	}
	ok, err := sanitizeEntryPath(target, "sub/inner.dll")
	if err != nil {
		t.Fatalf("legal name rejected: %v", err)
	}
	if ok != filepath.Join(target, "sub", "inner.dll") {
		t.Errorf("resolved path = %q", ok)
	}
}

func TestExtractZipEnforcesDecompressedCap(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "bomb.zip")
	// 64 KiB of highly compressible payload; the archive on the wire stays tiny,
	// so only the decompressed cap can stop it.
	payload := strings.Repeat("A", 64<<10)
	if err := os.WriteFile(zipPath, buildTestZip(t, map[string]string{"big.bin": payload}), 0644); err != nil {
		t.Fatalf("write zip: %v", err)
	}

	upd := NewUpdater()
	upd.maxExtractBytes = 1024 // far below the 64 KiB the entry expands to

	target := filepath.Join(dir, "out")
	err := upd.ExtractZip(zipPath, target)
	if !errors.Is(err, ErrExtractTooLarge) {
		t.Fatalf("ExtractZip over cap = %v, want ErrExtractTooLarge", err)
	}
}

func TestExtractZipWithinCapSucceeds(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "ok.zip")
	content := "plugin payload"
	if err := os.WriteFile(zipPath, buildTestZip(t, map[string]string{"plugin.txt": content}), 0644); err != nil {
		t.Fatalf("write zip: %v", err)
	}

	upd := NewUpdater() // default 1 GiB decompressed cap
	target := filepath.Join(dir, "out")
	if err := upd.ExtractZip(zipPath, target); err != nil {
		t.Fatalf("ExtractZip within cap = %v, want success", err)
	}
	if got := readTestFile(t, filepath.Join(target, "plugin.txt")); got != content {
		t.Errorf("extracted content = %q, want %q", got, content)
	}
}
