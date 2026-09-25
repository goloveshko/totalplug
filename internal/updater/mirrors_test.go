package updater

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"totalplug/internal/domain"
)

// Excerpt of the real https://totalcmd.net/plugring/fileinfo.html markup
// (fetched 2026-09-25), trimmed to the download/mirror anchors.
const fileinfoPageHTML = `<!DOCTYPE html><html><body>
<a href="/download.php?id=fileinfo" style="text-decoration: none;" title="http://st.free.fr/phpmyvisites.php?url=http%3A//fg.tcplugins.free.fr/wlx_fileinfo223.zip&id=253868&pagename=FILE:fileinfo223">Download (x32, x64)</a>
<a href="https://wincmd.ru/files/1140597/mirror/wlx_fileinfo_2.23.zip" style="text-decoration: none;" title="https://wincmd.ru/files/1140597/mirror/wlx_fileinfo_2.23.zip">Mirror</a>
<a href="https://evil.example/mirror.zip">Mirror</a>
<a href="http://wincmd.ru/plain.zip">Mirror</a>
</body></html>`

func overrideMirrorGlobals(t *testing.T, pageFormat string, suffixes []string) {
	t.Helper()
	oldFormat, oldSuffixes := totalcmdPluginPageURLFormat, trustedMirrorSuffixes
	totalcmdPluginPageURLFormat, trustedMirrorSuffixes = pageFormat, suffixes
	t.Cleanup(func() {
		totalcmdPluginPageURLFormat, trustedMirrorSuffixes = oldFormat, oldSuffixes
	})
}

func TestTotalcmdPluginID(t *testing.T) {
	tests := []struct {
		url, want string
	}{
		{"https://totalcmd.net/download.php?id=fileinfo", "fileinfo"},
		{"https://totalcmd.net/download.php?id=fi&x=1", "fi"},
		{"https://totalcmd.net/download.php", ""},
		{"https://evil.example/download.php?id=x", ""},
		{"https://github.com/o/r/releases/download/a.zip", ""},
	}
	for _, tt := range tests {
		if got := totalcmdPluginID(tt.url); got != tt.want {
			t.Errorf("totalcmdPluginID(%q) = %q, want %q", tt.url, got, tt.want)
		}
	}
}

func TestIsTrustedMirrorHost(t *testing.T) {
	tests := []struct {
		url  string
		want bool
	}{
		{"https://wincmd.ru/files/1/m.zip", true},
		{"https://dl.wincmd.ru/m.zip", true},
		{"https://wincmd.ru.evil.example/m.zip", false},
		{"https://evil.example/m.zip", false},
	}
	for _, tt := range tests {
		if got := isTrustedMirrorHost(tt.url); got != tt.want {
			t.Errorf("isTrustedMirrorHost(%q) = %v, want %v", tt.url, got, tt.want)
		}
	}
}

func TestResolveTotalCmdMirrors(t *testing.T) {
	pageSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/plugring/fileinfo.html" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(fileinfoPageHTML))
	}))
	defer pageSrv.Close()

	overrideMirrorGlobals(t, pageSrv.URL+"/plugring/%s.html", []string{"wincmd.ru"})

	upd := NewUpdater()
	got := upd.resolveTotalCmdMirrors(context.Background(), "https://totalcmd.net/download.php?id=fileinfo")

	want := "https://wincmd.ru/files/1140597/mirror/wlx_fileinfo_2.23.zip"
	if len(got) != 1 || got[0] != want {
		t.Fatalf("resolveTotalCmdMirrors() = %v, want [%s]", got, want)
	}

	// Non-totalcmd sources never reach the page fetch.
	if mirrors := upd.resolveTotalCmdMirrors(context.Background(), "https://github.com/x/y/releases/download/a.zip"); mirrors != nil {
		t.Errorf("github URL should resolve no mirrors, got %v", mirrors)
	}
}

func TestUpdatePluginFallsBackToTotalCmdMirror(t *testing.T) {
	dir := t.TempDir()
	pluginFile := filepath.Join(dir, "fi.wlx")
	writeTestFile(t, pluginFile, "original")

	zipBody := buildTestZip(t, map[string]string{"fi.wlx": "via-mirror"})

	// Mirrors must be https (regex + validateDownloadURL), so serve the
	// package from a TLS test server whose cert the updater client trusts.
	mirrorSrv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/zip")
		_, _ = w.Write(zipBody)
	}))
	defer mirrorSrv.Close()

	pageSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<a href="` + mirrorSrv.URL + `/mirror/wlx_fi_2.23.zip">Mirror</a>`))
	}))
	defer pageSrv.Close()

	overrideMirrorGlobals(t, pageSrv.URL+"/plugring/%s.html", []string{"127.0.0.1"})

	upd := NewUpdater()
	upd.client = mirrorSrv.Client()

	seen := map[string]bool{}
	upd.SetProgressListener(func(p domain.UpdateProgress) {
		if p.Phase == "download" {
			seen[p.URL] = true
		}
	})

	// Catalog candidates are exhausted (dead loopback port); the mirror must
	// be discovered from the plugin page and used to complete the update.
	dead := "https://127.0.0.1:9/package.zip"
	if _, err := upd.UpdatePlugin(context.Background(), domain.Plugin{
		Path:         pluginFile,
		DownloadURL:  "https://totalcmd.net/download.php?id=fi",
		DownloadURLs: []string{dead},
	}); err != nil {
		t.Fatalf("totalcmd mirror fallback should succeed: %v", err)
	}

	if readTestFile(t, pluginFile) != "via-mirror" {
		t.Error("plugin file should contain the mirrored package")
	}
	if !seen[dead] || !seen[mirrorSrv.URL+"/mirror/wlx_fi_2.23.zip"] {
		t.Errorf("progress should report both attempts, seen: %v", seen)
	}
	upd.SetProgressListener(nil)
}
