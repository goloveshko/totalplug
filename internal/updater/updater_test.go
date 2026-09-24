package updater

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"totalplug/internal/domain"
)

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

	for rel, want := range map[string]string{
		"demo.wdx":         "binary",
		"readme.txt":       "readme",
		filepath.Join("sub", "data.bin"): "data",
	} {
		if got := readTestFile(t, filepath.Join(backupDir, rel)); got != want {
			t.Errorf("backup content %q = %q, want %q", rel, got, want)
		}
	}
}

func TestUpdatePluginSuccessKeepsBackup(t *testing.T) {
	root := t.TempDir()
	pluginDir := filepath.Join(root, "wdx", "demo")
	dllPath := filepath.Join(pluginDir, "demo.dll")
	writeTestFile(t, dllPath, "old-dll")
	writeTestFile(t, filepath.Join(pluginDir, "keep.txt"), "extra-file")

	downloadURL := serveTestZip(t, buildTestZip(t, map[string]string{
		"demo.dll":        "new-dll",
		"docs/info.txt":   "release-notes",
	}))

	backupDir, err := NewUpdater().UpdatePlugin(context.Background(), domain.Plugin{
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
	if got := readTestFile(t, filepath.Join(backupDir, "demo.dll")); got != "old-dll" {
		t.Errorf("backup demo.dll = %q, want old-dll", got)
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

	leftovers, _ := filepath.Glob(pluginDir + ".bak_*")
	if len(leftovers) != 0 {
		t.Errorf("backup dir should be removed after successful rollback, found %v", leftovers)
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
		err := validateDownloadURL(tc.url)
		if tc.wantErr && !errors.Is(err, ErrInsecureScheme) {
			t.Errorf("validateDownloadURL(%q) = %v, want ErrInsecureScheme", tc.url, err)
		}
		if !tc.wantErr && err != nil {
			t.Errorf("validateDownloadURL(%q) = %v, want nil", tc.url, err)
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
