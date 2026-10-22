package updater

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/goloveshko/totalplug/internal/domain"
)

const (
	peviewerENPage  = "https://totalcmd.net/plugins/PEViewer3"
	peviewerRUPage  = "https://wincmd.ru/plugins/PEViewer3"
	peviewerMain    = "https://wincmd.ru/download/PEViewer3/main"
	peviewerArchive = "https://wincmd.ru/mirror/files/9924391/wlx_peviewer_3.0.18.rar"
)

// These excerpts retain the live header, primary download panel, screenshots,
// and a related plugin's version from the public pages fetched 2026-10-22.
func mirrorFixture(t *testing.T, language string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("testdata", "peviewer3_"+language+".html"))
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

type mirrorTestRoundTripper func(*http.Request) (*http.Response, error)

func (f mirrorTestRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

// Rewrite at the transport boundary to exercise redirects without weakening production allowlists.
func mirrorTestUpdater(t *testing.T, handler http.HandlerFunc) (*Updater, *[]string) {
	t.Helper()
	srv := httptest.NewTLSServer(handler)
	t.Cleanup(srv.Close)
	target, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	transport := srv.Client().Transport
	var requests []string
	upd := NewUpdater()
	upd.client.Transport = mirrorTestRoundTripper(func(req *http.Request) (*http.Response, error) {
		requests = append(requests, req.Method+" "+req.URL.String())
		local := req.Clone(req.Context())
		local.Host = req.URL.Host
		local.URL.Scheme, local.URL.Host = target.Scheme, target.Host
		resp, err := transport.RoundTrip(local)
		if resp != nil {
			resp.Request = req
		}
		return resp, err
	})
	return upd, &requests
}

func mirrorPageHandler(en, ru, archive string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Host + r.URL.Path {
		case "totalcmd.net/plugins/PEViewer3":
			_, _ = w.Write([]byte(en))
		case "wincmd.ru/plugins/PEViewer3":
			_, _ = w.Write([]byte(ru))
		case "wincmd.ru/download/PEViewer3/main":
			if r.Method != http.MethodHead {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			http.Redirect(w, r, archive, http.StatusFound)
		default:
			if r.Method != http.MethodHead {
				w.WriteHeader(http.StatusNotFound)
			}
		}
	}
}

func TestTotalcmdPluginID(t *testing.T) {
	tests := []struct {
		url, want string
	}{
		{"https://totalcmd.net/download.php?id=fileinfo", "fileinfo"},
		{"https://totalcmd.net/download.php?id=fi&x=1", "fi"},
		{"https://totalcmd.net/download.php?id=13954", "13954"},
		{"https://totalcmd.net/download.php?id=CVSBrowser_1.0.5", "CVSBrowser_1.0.5"},
		{"https://totalcmd.net/download/PEViewer3/main", "PEViewer3"},
		{"https://TOTALCMD.NET:443/download/PEViewer3/main", "PEViewer3"},
		{"https://totalcmd.net/download.php", ""},
		{"https://totalcmd.net/download.php?id=", ""},
		{"https://totalcmd.net/download.php?id=fi&id=other", ""},
		{"https://totalcmd.net/download.php?id=fi&x=%ZZ", ""},
		{"https://totalcmd.net/other/download.php?id=fi", ""},
		{"https://totalcmd.net/notdownload.php?id=fi", ""},
		{"https://totalcmd.net/download.php/extra?id=fi", ""},
		{"https://totalcmd.net/download.php?id=../fi", ""},
		{"https://totalcmd.net/download.php?id=%2F%2Fevil.example", ""},
		{"https://totalcmd.net/download.php?id=..", ""},
		{"https://totalcmd.net/download/PEViewer3/other", ""},
		{"https://totalcmd.net/download/PEViewer3/main/extra", ""},
		{"https://totalcmd.net/download/PEViewer3/main/", ""},
		{"https://totalcmd.net/download/PEViewer3/main?id=other", ""},
		{"https://totalcmd.net/download/PEViewer3/main?", ""},
		{"https://totalcmd.net/download/PEViewer3/main#other", ""},
		{"https://totalcmd.net/download/../main", ""},
		{"https://totalcmd.net/download/%2E%2E/main", ""},
		{"https://totalcmd.net/download/PEViewer3%2Fother/main", ""},
		{"https://totalcmd.net/%64ownload/PEViewer3/main", ""},
		{"https://totalcmd.net/plugins/PEViewer3", ""},
		{"https://totalcmd.net/mirror/files/1/plugin.zip", ""},
		{"http://totalcmd.net/download.php?id=fi", ""},
		{"http://totalcmd.net/download/PEViewer3/main", ""},
		{"//totalcmd.net/download/PEViewer3/main", ""},
		{"ftp://totalcmd.net/download/PEViewer3/main", ""},
		{"https://evil.example/download.php?id=x", ""},
		{"https://totalcmd.net.evil.example/download/PEViewer3/main", ""},
		{"https://evil.totalcmd.net/download/PEViewer3/main", ""},
		{"https://totalcmd.net@evil.example/download/PEViewer3/main", ""},
		{"https://evil.example@totalcmd.net/download/PEViewer3/main", ""},
		{"https://totalcmd.net:444/download/PEViewer3/main", ""},
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
		{"https://wincmd.ru/mirror/files/1/m.zip", true},
		{"https://dl.wincmd.ru/m.zip", true},
		{"https://WINCMD.RU:443/m.zip", true},
		{"https://wincmd.ru.evil.example/m.zip", false},
		{"https://evilwincmd.ru/m.zip", false},
		{"https://evil.example/m.zip", false},
		{"https://totalcmd.net/mirror/m.zip", false},
		{"https://evil.example@wincmd.ru/m.zip", false},
		{"https://wincmd.ru@evil.example/m.zip", false},
		{"https://wincmd.ru:444/m.zip", false},
		{"http://wincmd.ru/m.zip", false},
		{"http://127.0.0.1/m.zip", false},
		{"//wincmd.ru/m.zip", false},
		{"file:///wincmd.ru/m.zip", false},
	}
	for _, tt := range tests {
		if got := isTrustedMirrorHost(tt.url); got != tt.want {
			t.Errorf("isTrustedMirrorHost(%q) = %v, want %v", tt.url, got, tt.want)
		}
	}
}

func TestResolveTotalCmdMirrors(t *testing.T) {
	en, ru := mirrorFixture(t, "en"), mirrorFixture(t, "ru")
	for _, primary := range []string{"https://totalcmd.net/download.php?id=PEViewer3", "https://totalcmd.net/download/PEViewer3/main"} {
		t.Run(primary, func(t *testing.T) {
			upd, requests := mirrorTestUpdater(t, mirrorPageHandler(en, ru, peviewerArchive))
			got := upd.resolveTotalCmdMirrors(context.Background(), primary, "3.0.18")
			if !reflect.DeepEqual(got, []string{peviewerArchive}) {
				t.Fatalf("resolveTotalCmdMirrors() = %v, want [%s]", got, peviewerArchive)
			}
			wantRequests := []string{"GET " + peviewerENPage, "GET " + peviewerRUPage, "HEAD " + peviewerMain, "HEAD " + peviewerArchive}
			if !reflect.DeepEqual(*requests, wantRequests) {
				t.Fatalf("requests = %v, want %v (never fetch screenshot mirrors)", *requests, wantRequests)
			}
		})
	}
}

func TestResolveTotalCmdMirrorsRequiresMatchingRelease(t *testing.T) {
	en, ru := mirrorFixture(t, "en"), mirrorFixture(t, "ru")
	for _, tt := range []struct {
		name, expected, page string
		requests             int
	}{
		{"older catalog release", "3.0.17", ru, 2},
		{"newer catalog release", "3.0.19", ru, 2},
		{"related plugin version", "2.23", ru, 2},
		{"unknown expected release", "", ru, 0},
		{"blank expected release", " \t ", ru, 0},
		{"Russian release differs from English", "3.0.18", strings.Replace(ru, `<span class="ver">3.0.18</span>`, `<span class="ver">3.0.19</span>`, 1), 2},
		{"missing heading release", "3.0.18", strings.Replace(ru, `<span class="ver">3.0.18</span>`, `<span>3.0.18</span>`, 1), 2},
		{"missing plugin header", "3.0.18", strings.Replace(ru, `class="plugin-head"`, `class="other"`, 1), 2},
	} {
		t.Run(tt.name, func(t *testing.T) {
			upd, requests := mirrorTestUpdater(t, mirrorPageHandler(en, tt.page, peviewerArchive))
			if got := upd.resolveTotalCmdMirrors(context.Background(), "https://totalcmd.net/download/PEViewer3/main", tt.expected); got != nil {
				t.Fatalf("release must not be guessed: %v", got)
			}
			if len(*requests) != tt.requests {
				t.Fatalf("must stop before probing an unverified archive: requests = %v", *requests)
			}
		})
	}
}

func TestResolveTotalCmdMirrorsRejectsUntrustedLinks(t *testing.T) {
	en, ru := mirrorFixture(t, "en"), mirrorFixture(t, "ru")
	for _, tt := range []struct {
		name, original, replacement string
		counterpart                 bool
	}{
		{"counterpart hostname", peviewerRUPage, "https://wincmd.ru.evil.example/plugins/PEViewer3", true},
		{"counterpart subdomain", peviewerRUPage, "https://dl.wincmd.ru/plugins/PEViewer3", true},
		{"counterpart plaintext", peviewerRUPage, "http://wincmd.ru/plugins/PEViewer3", true},
		{"counterpart other plugin", peviewerRUPage, "https://wincmd.ru/plugins/fileinfo", true},
		{"counterpart arbitrary path", peviewerRUPage, "https://wincmd.ru/other/plugins/PEViewer3", true},
		{"counterpart encoded path", peviewerRUPage, "https://wincmd.ru/plugins/%50EViewer3", true},
		{"counterpart credentials", peviewerRUPage, "https://evil.example@wincmd.ru/plugins/PEViewer3", true},
		{"counterpart port", peviewerRUPage, "https://wincmd.ru:444/plugins/PEViewer3", true},
		{"counterpart query", peviewerRUPage, peviewerRUPage + "?id=fileinfo", true},
		{"counterpart fragment", peviewerRUPage, peviewerRUPage + "#other", true},
		{"counterpart absent", `hreflang="ru"`, `hreflang="de"`, true},
		{"main hostname", peviewerMain, "https://wincmd.ru.evil.example/download/PEViewer3/main", false},
		{"main subdomain", peviewerMain, "https://dl.wincmd.ru/download/PEViewer3/main", false},
		{"main plaintext", peviewerMain, "http://wincmd.ru/download/PEViewer3/main", false},
		{"main other plugin", peviewerMain, "https://wincmd.ru/download/fileinfo/main", false},
		{"main alternate download", peviewerMain, "https://wincmd.ru/download/PEViewer3/other", false},
		{"main arbitrary path", peviewerMain, "https://wincmd.ru/other/download/PEViewer3/main", false},
		{"main encoded path", peviewerMain, "https://wincmd.ru/download/PEViewer3%2Fother/main", false},
		{"main credentials", peviewerMain, "https://evil.example@wincmd.ru/download/PEViewer3/main", false},
		{"main port", peviewerMain, "https://wincmd.ru:444/download/PEViewer3/main", false},
		{"main query", peviewerMain, peviewerMain + "?id=fileinfo", false},
		{"screenshot instead of main", peviewerMain, "https://wincmd.ru/mirror/files/9924391/MainDM_N1.png", false},
		{"no primary button", `class="btn btn-primary btn-block"`, `class="btn"`, false},
		{"no download panel", `class="plugin-side"`, `class="other"`, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			pageEN, pageRU := en, ru
			wantRequests := 2
			if tt.counterpart {
				pageEN = strings.ReplaceAll(en, tt.original, tt.replacement)
				wantRequests = 1
			} else {
				pageRU = strings.ReplaceAll(ru, tt.original, tt.replacement)
			}
			upd, requests := mirrorTestUpdater(t, mirrorPageHandler(pageEN, pageRU, peviewerArchive))
			if got := upd.resolveTotalCmdMirrors(context.Background(), "https://totalcmd.net/download/PEViewer3/main", "3.0.18"); got != nil {
				t.Fatalf("untrusted link returned: %v", got)
			}
			if len(*requests) != wantRequests {
				t.Fatalf("invalid links must be rejected before fetching: %v", *requests)
			}
		})
	}
}

func TestResolveTotalCmdMirrorsParsesRelativeMainHref(t *testing.T) {
	en, ru := mirrorFixture(t, "en"), mirrorFixture(t, "ru")
	ru = strings.Replace(ru, `class="btn btn-primary btn-block"`, `class='btn-block btn-primary btn'`, 1)
	ru = strings.Replace(ru, `href="`+peviewerMain+`"`, `href='/download/PEViewer3/main'`, 1)
	upd, _ := mirrorTestUpdater(t, mirrorPageHandler(en, ru, peviewerArchive))
	if got := upd.resolveTotalCmdMirrors(context.Background(), "https://totalcmd.net/download/PEViewer3/main", "3.0.18"); !reflect.DeepEqual(got, []string{peviewerArchive}) {
		t.Fatalf("actual relative main href should resolve: %v", got)
	}
}

func TestResolveTotalCmdMirrorsRedirectPolicy(t *testing.T) {
	en, ru := mirrorFixture(t, "en"), mirrorFixture(t, "ru")
	for _, tt := range []struct {
		name, from, to string
		blocked        bool
	}{
		{"English page offsite", peviewerENPage, "https://evil.example/plugins/PEViewer3", true},
		{"English page plaintext", peviewerENPage, "http://totalcmd.net/plugins/PEViewer3", true},
		{"English page wrong plugin", peviewerENPage, "https://totalcmd.net/plugins/fileinfo", true},
		{"Russian page offsite", peviewerRUPage, "https://wincmd.ru.evil.example/plugins/PEViewer3", true},
		{"Russian page plaintext", peviewerRUPage, "http://wincmd.ru/plugins/PEViewer3", true},
		{"Russian page wrong plugin", peviewerRUPage, "https://wincmd.ru/plugins/fileinfo", true},
		{"main offsite", peviewerMain, "https://evil.example/pkg.zip", true},
		{"main deceptive host", peviewerMain, "https://wincmd.ru.evil.example/pkg.zip", true},
		{"main plaintext", peviewerMain, "http://wincmd.ru/mirror/pkg.zip", true},
		{"main loopback plaintext", peviewerMain, "http://127.0.0.1/pkg.zip", true},
		{"main credentials", peviewerMain, "https://evil.example@wincmd.ru/pkg.zip", true},
		{"archive offsite second hop", peviewerArchive, "https://evil.example/pkg.zip", true},
		{"archive plaintext second hop", peviewerArchive, "http://wincmd.ru/pkg.zip", true},
		{"main screenshot", peviewerMain, "https://wincmd.ru/mirror/files/9924391/MainDM_N1.png", false},
		{"main HTML page", peviewerMain, peviewerRUPage, false},
		{"redirect loop", peviewerMain, peviewerMain, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			base := mirrorPageHandler(en, ru, peviewerArchive)
			upd, requests := mirrorTestUpdater(t, func(w http.ResponseWriter, r *http.Request) {
				if "https://"+r.Host+r.URL.Path == tt.from {
					http.Redirect(w, r, tt.to, http.StatusFound)
					return
				}
				base(w, r)
			})
			if got := upd.resolveTotalCmdMirrors(context.Background(), "https://totalcmd.net/download/PEViewer3/main", "3.0.18"); got != nil {
				t.Fatalf("invalid redirect chain returned %v", got)
			}
			if tt.blocked {
				for _, request := range *requests {
					if strings.HasSuffix(request, " "+tt.to) {
						t.Fatalf("untrusted redirect reached network: %v", *requests)
					}
				}
			}
			if len(*requests) > 12 {
				t.Fatalf("redirects must be bounded: %v", *requests)
			}
		})
	}
}

func TestResolveTotalCmdMirrorsTrustedArchiveRedirects(t *testing.T) {
	en, ru := mirrorFixture(t, "en"), mirrorFixture(t, "ru")
	for _, archive := range []string{peviewerArchive, "https://dl.wincmd.ru/mirror/plugin.zip", "https://wincmd.ru/mirror/plugin.7z"} {
		t.Run(archive, func(t *testing.T) {
			upd, _ := mirrorTestUpdater(t, mirrorPageHandler(en, ru, archive))
			if got := upd.resolveTotalCmdMirrors(context.Background(), "https://totalcmd.net/download/PEViewer3/main", "3.0.18"); !reflect.DeepEqual(got, []string{archive}) {
				t.Fatalf("trusted main archive should resolve: %v", got)
			}
		})
	}
}

func TestResolveTotalCmdMirrorsFailureIsBestEffort(t *testing.T) {
	en, ru := mirrorFixture(t, "en"), mirrorFixture(t, "ru")
	for _, failedURL := range []string{peviewerENPage, peviewerRUPage, peviewerMain, peviewerArchive} {
		t.Run(failedURL, func(t *testing.T) {
			base := mirrorPageHandler(en, ru, peviewerArchive)
			upd, _ := mirrorTestUpdater(t, func(w http.ResponseWriter, r *http.Request) {
				if "https://"+r.Host+r.URL.Path == failedURL {
					w.WriteHeader(http.StatusNotFound)
					return
				}
				base(w, r)
			})
			if got := upd.resolveTotalCmdMirrors(context.Background(), "https://totalcmd.net/download/PEViewer3/main", "3.0.18"); got != nil {
				t.Fatalf("failed fetch returned %v", got)
			}
		})
	}
	upd, requests := mirrorTestUpdater(t, mirrorPageHandler(en, strings.Repeat(" ", 2<<20)+ru, peviewerArchive))
	if got := upd.resolveTotalCmdMirrors(context.Background(), "https://totalcmd.net/download/PEViewer3/main", "3.0.18"); got != nil || len(*requests) != 2 {
		t.Fatalf("oversized page must not yield a candidate: %v, requests %v", got, *requests)
	}
	upd.client.Transport = mirrorTestRoundTripper(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("network unavailable")
	})
	if got := upd.resolveTotalCmdMirrors(context.Background(), "https://totalcmd.net/download/PEViewer3/main", "3.0.18"); got != nil {
		t.Fatalf("transport failure returned %v", got)
	}
}

func TestResolveTotalCmdMirrorsRejectsOtherPrimarySources(t *testing.T) {
	upd, requests := mirrorTestUpdater(t, mirrorPageHandler("", "", peviewerArchive))
	for _, primary := range []string{
		"https://github.com/x/y/releases/download/a.zip",
		"https://totalcmd.net.evil.example/download/PEViewer3/main",
		"https://totalcmd.net/other/download.php?id=PEViewer3",
		"https://totalcmd.net/download.php?id=../PEViewer3",
		"http://totalcmd.net/download/PEViewer3/main",
	} {
		if got := upd.resolveTotalCmdMirrors(context.Background(), primary, "3.0.18"); got != nil {
			t.Errorf("invalid primary %q returned %v", primary, got)
		}
	}
	if len(*requests) != 0 {
		t.Fatalf("invalid primary must never fetch a page: %v", *requests)
	}
}

func TestResolveTotalCmdMirrorsFileInfoDeadCounter(t *testing.T) {
	// This measured counter is neither a hosted mirror nor the author's newer release.
	en := `<div class="lang-switch"><a href="https://wincmd.ru/plugins/fileinfo" hreflang="ru" lang="ru">RUS</a></div>`
	ru := `<article class="wrap plugin-page">
<div class="plugin-main"><header class="plugin-head"><h1>FileInfo <span class="ver">2.23</span></h1></header>
<figure class="shot"><a href="https://wincmd.ru/mirror/images/fileinfo.gif"><img src="https://wincmd.ru/mirror/images/fileinfo.gif"></a></figure></div>
<aside class="plugin-side"><div class="panel">
<a class="btn btn-primary btn-block" href="https://wincmd.ru/download/fileinfo/main" rel="nofollow">Скачать <small>1004 КБ</small></a>
<dl class="facts"><dt>Версия</dt><dd>2.23</dd></dl>
</div></aside></article>`
	counter := "http://st.free.fr/phpmyvisites.php?url=http%3A//fg.tcplugins.free.fr/wlx_fileinfo223.zip&id=253868&pagename=FILE:fileinfo223"
	for _, expected := range []string{"2.23", "2.24"} {
		t.Run(expected, func(t *testing.T) {
			upd, requests := mirrorTestUpdater(t, func(w http.ResponseWriter, r *http.Request) {
				switch r.Host + r.URL.Path {
				case "totalcmd.net/plugins/fileinfo":
					_, _ = w.Write([]byte(en))
				case "wincmd.ru/plugins/fileinfo":
					_, _ = w.Write([]byte(ru))
				case "wincmd.ru/download/fileinfo/main":
					http.Redirect(w, r, counter, http.StatusFound)
				default:
					w.WriteHeader(http.StatusNotFound)
				}
			})
			if got := upd.resolveTotalCmdMirrors(context.Background(), "https://totalcmd.net/download/fileinfo/main", expected); got != nil {
				t.Fatalf("dead offsite counter must not be presented as an archive: %v", got)
			}
			want := 2
			if expected == "2.23" {
				want = 3
			}
			if len(*requests) != want {
				t.Fatalf("must stop at release check or before dead counter: %v", *requests)
			}
		})
	}
}

func TestTryTotalCmdMirrorsRedirectPolicy(t *testing.T) {
	zipBody := buildTestZip(t, map[string]string{"peviewer.wlx": "via-mirror"})
	for _, tt := range []struct {
		name, target string
		allowed      bool
	}{
		{"trusted archive GET redirect", "https://dl.wincmd.ru/mirror/plugin.zip", true},
		{"offsite archive GET redirect", "https://evil.example/plugin.zip", false},
		{"deceptive archive GET host", "https://wincmd.ru.evil.example/plugin.zip", false},
		{"plaintext archive GET redirect", "http://wincmd.ru/mirror/plugin.zip", false},
		{"loopback plaintext archive GET", "http://127.0.0.1/plugin.zip", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			upd, requests := mirrorTestUpdater(t, func(w http.ResponseWriter, r *http.Request) {
				if "https://"+r.Host+r.URL.Path == peviewerArchive {
					http.Redirect(w, r, tt.target, http.StatusFound)
					return
				}
				_, _ = w.Write(zipBody)
			})
			client := upd.client
			pkg, err := upd.tryTotalCmdMirrors(context.Background(), []string{peviewerArchive}, filepath.Join(t.TempDir(), "package.archive"), t.TempDir())
			if tt.allowed {
				if err != nil || pkg.format != "zip" {
					t.Fatalf("trusted mirror GET should succeed: %v, %v", pkg, err)
				}
			} else {
				if err == nil || len(*requests) != 1 {
					t.Fatalf("untrusted archive GET redirect must stop before network: err %v, requests %v", err, *requests)
				}
			}
			if upd.client != client {
				t.Fatal("mirror policy must not replace the normal updater client")
			}
		})
	}
	upd, requests := mirrorTestUpdater(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(zipBody) })
	if _, err := upd.tryTotalCmdMirrors(context.Background(), []string{"http://wincmd.ru/plugin.zip"}, filepath.Join(t.TempDir(), "package.archive"), t.TempDir()); err == nil || len(*requests) != 0 {
		t.Fatalf("insecure mirror must be rejected before network: err %v, requests %v", err, *requests)
	}
}

func TestUpdatePluginFallsBackToTotalCmdMirror(t *testing.T) {
	dir := t.TempDir()
	pluginFile := filepath.Join(dir, "peviewer.wlx")
	writeTestFile(t, pluginFile, "original")
	zipBody := buildTestZip(t, map[string]string{"peviewer.wlx": "via-mirror"})
	en, ru := mirrorFixture(t, "en"), mirrorFixture(t, "ru")
	archive := strings.TrimSuffix(peviewerArchive, ".rar") + ".zip"
	base := mirrorPageHandler(en, ru, archive)
	upd, requests := mirrorTestUpdater(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Host == "totalcmd.net" && r.URL.Path == "/download/PEViewer3/main" {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		if "https://"+r.Host+r.URL.Path == archive {
			w.Header().Set("Content-Type", "application/zip")
			if r.Method == http.MethodGet {
				_, _ = w.Write(zipBody)
			}
			return
		}
		base(w, r)
	})
	var seen []string
	upd.SetProgressListener(func(p domain.UpdateProgress) {
		if p.Phase == "download" && (len(seen) == 0 || seen[len(seen)-1] != p.URL) {
			seen = append(seen, p.URL)
		}
	})
	primary := "https://totalcmd.net/download/PEViewer3/main"
	if _, err := upd.UpdatePlugin(context.Background(), domain.Plugin{
		Path:          pluginFile,
		DownloadURL:   primary,
		LatestVersion: "3.0.18",
	}); err != nil {
		t.Fatalf("primary failure should use the current main mirror: %v", err)
	}
	if readTestFile(t, pluginFile) != "via-mirror" {
		t.Error("plugin file should contain the mirrored package")
	}
	if !reflect.DeepEqual(seen, []string{primary, archive}) {
		t.Errorf("progress should report primary and verified archive attempts: %v", seen)
	}
	wantRequests := []string{"GET " + primary, "GET " + peviewerENPage, "GET " + peviewerRUPage, "HEAD " + peviewerMain, "HEAD " + archive, "GET " + archive}
	if !reflect.DeepEqual(*requests, wantRequests) {
		t.Errorf("fallback requests = %v, want %v", *requests, wantRequests)
	}
}

func TestUpdatePluginMirrorRejectsChangedGETRedirect(t *testing.T) {
	en, ru := mirrorFixture(t, "en"), mirrorFixture(t, "ru")
	zipBody := buildTestZip(t, map[string]string{"peviewer.wlx": "untrusted"})
	for _, tt := range []struct {
		name, target string
		insecure     bool
	}{
		{"HTTPS offsite GET", "https://evil.example/plugin.zip", false},
		{"insecure primary does not relax mirror GET", "http://wincmd.ru/mirror/plain.zip", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			pluginFile := filepath.Join(t.TempDir(), "peviewer.wlx")
			writeTestFile(t, pluginFile, "original")
			// Unique sources keep this regression independent of cached packages.
			archive := "https://wincmd.ru/mirror/" + url.PathEscape(tt.name) + ".zip"
			base := mirrorPageHandler(en, ru, archive)
			upd, requests := mirrorTestUpdater(t, func(w http.ResponseWriter, r *http.Request) {
				if "https://"+r.Host+r.URL.EscapedPath() == archive {
					if r.Method == http.MethodGet {
						http.Redirect(w, r, tt.target, http.StatusFound)
					}
					return
				}
				if r.Host == "evil.example" || r.URL.Path == "/mirror/plain.zip" {
					_, _ = w.Write(zipBody)
					return
				}
				base(w, r)
			})
			plugin := domain.Plugin{Path: pluginFile, DownloadURL: "https://totalcmd.net/download/PEViewer3/main", LatestVersion: "3.0.18"}
			var err error
			if tt.insecure {
				_, err = upd.UpdatePluginInsecure(context.Background(), plugin)
			} else {
				_, err = upd.UpdatePlugin(context.Background(), plugin)
			}
			if err == nil || readTestFile(t, pluginFile) != "original" {
				t.Fatalf("mirror GET must use the discovery allowlist: err %v, requests %v", err, *requests)
			}
			for _, request := range *requests {
				if strings.HasSuffix(request, " "+tt.target) {
					t.Fatalf("untrusted GET redirect reached network: %v", *requests)
				}
			}
		})
	}
}

func TestUpdatePluginMirrorDoesNotReuseUnrestrictedCandidateCache(t *testing.T) {
	en, ru := mirrorFixture(t, "en"), mirrorFixture(t, "ru")
	zipBody := buildTestZip(t, map[string]string{"peviewer.wlx": "untrusted", "b.txt": "new"})
	for _, tt := range []struct {
		name, target string
		insecure     bool
	}{
		{"HTTPS offsite cache", "https://evil.example/plugin.zip", false},
		{"plaintext cache", "http://wincmd.ru/mirror/plain.zip", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			pluginFile := filepath.Join(t.TempDir(), "peviewer.wlx")
			writeTestFile(t, pluginFile, "original")
			locked := filepath.Join(filepath.Dir(pluginFile), "b.txt")
			writeTestFile(t, locked, "old")
			if err := os.Chmod(locked, 0444); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.Chmod(locked, 0644) })
			archive := "https://wincmd.ru/mirror/" + url.PathEscape(tt.name) + ".zip"
			base := mirrorPageHandler(en, ru, archive)
			var targetHits atomic.Int32
			upd, requests := mirrorTestUpdater(t, func(w http.ResponseWriter, r *http.Request) {
				if "https://"+r.Host+r.URL.EscapedPath() == archive {
					if r.Method == http.MethodGet {
						http.Redirect(w, r, tt.target, http.StatusFound)
					}
					return
				}
				if r.Host == "evil.example" || r.URL.Path == "/mirror/plain.zip" {
					targetHits.Add(1)
					_, _ = w.Write(zipBody)
					return
				}
				base(w, r)
			})
			plugin := domain.Plugin{Path: pluginFile, DownloadURL: archive, LatestVersion: "3.0.18"}
			var err error
			if tt.insecure {
				_, err = upd.UpdatePluginInsecure(context.Background(), plugin)
			} else {
				_, err = upd.UpdatePlugin(context.Background(), plugin)
			}
			if err == nil || targetHits.Load() != 1 {
				t.Fatalf("initial apply must retain the ordinary candidate: err %v, requests %v", err, *requests)
			}
			if err := os.Chmod(locked, 0644); err != nil {
				t.Fatal(err)
			}
			plugin.DownloadURL = "https://totalcmd.net/download/PEViewer3/main"
			if tt.insecure {
				_, err = upd.UpdatePluginInsecure(context.Background(), plugin)
			} else {
				_, err = upd.UpdatePlugin(context.Background(), plugin)
			}
			if err == nil || readTestFile(t, pluginFile) != "original" {
				t.Fatalf("mirror must not reuse a less-restricted candidate: err %v, requests %v", err, *requests)
			}
			if targetHits.Load() != 1 {
				t.Fatalf("mirror retry reached the untrusted target: %v", *requests)
			}
		})
	}
}

func TestRetriedMirrorUpdateReusesItsOwnKeptPackage(t *testing.T) {
	en, ru := mirrorFixture(t, "en"), mirrorFixture(t, "ru")
	zipBody := buildTestZip(t, map[string]string{"peviewer.wlx": "via-mirror", "b.txt": "new"})
	for _, insecure := range []bool{false, true} {
		name := "strict"
		if insecure {
			name = "consented"
		}
		t.Run(name, func(t *testing.T) {
			pluginFile := filepath.Join(t.TempDir(), "peviewer.wlx")
			writeTestFile(t, pluginFile, "original")
			locked := filepath.Join(filepath.Dir(pluginFile), "b.txt")
			writeTestFile(t, locked, "old")
			if err := os.Chmod(locked, 0444); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.Chmod(locked, 0644) })
			archive := "https://wincmd.ru/mirror/retry-" + name + ".zip"
			base := mirrorPageHandler(en, ru, archive)
			var downloads atomic.Int32
			upd, _ := mirrorTestUpdater(t, func(w http.ResponseWriter, r *http.Request) {
				if "https://"+r.Host+r.URL.Path == archive {
					if r.Method == http.MethodGet {
						downloads.Add(1)
						_, _ = w.Write(zipBody)
					}
					return
				}
				base(w, r)
			})
			plugin := domain.Plugin{Path: pluginFile, DownloadURL: "https://totalcmd.net/download/PEViewer3/main", LatestVersion: "3.0.18"}
			update := upd.UpdatePlugin
			if insecure {
				update = upd.UpdatePluginInsecure
			}
			if _, err := update(context.Background(), plugin); err == nil || downloads.Load() != 1 {
				t.Fatalf("first mirror apply must fail after one download: %v, downloads %d", err, downloads.Load())
			}
			if err := os.Chmod(locked, 0644); err != nil {
				t.Fatal(err)
			}
			if _, err := update(context.Background(), plugin); err != nil {
				t.Fatalf("mirror retry: %v", err)
			}
			if downloads.Load() != 1 || readTestFile(t, pluginFile) != "via-mirror" {
				t.Fatalf("mirror retry must install the retained package: downloads %d", downloads.Load())
			}
			if _, err := update(context.Background(), plugin); err != nil {
				t.Fatalf("update after success: %v", err)
			}
			if downloads.Load() != 2 {
				t.Fatalf("successful mirror apply must forget its package: downloads %d", downloads.Load())
			}
		})
	}
}

func TestUpdatePluginDoesNotFallbackToDifferentMirrorVersion(t *testing.T) {
	en, ru := mirrorFixture(t, "en"), mirrorFixture(t, "ru")
	for _, expected := range []string{"3.0.17", ""} {
		t.Run("expected="+expected, func(t *testing.T) {
			pluginFile := filepath.Join(t.TempDir(), "peviewer.wlx")
			writeTestFile(t, pluginFile, "original")
			upd, requests := mirrorTestUpdater(t, mirrorPageHandler(en, ru, peviewerArchive))
			if _, err := upd.UpdatePlugin(context.Background(), domain.Plugin{
				Path:          pluginFile,
				DownloadURL:   "https://totalcmd.net/download/PEViewer3/main",
				LatestVersion: expected,
			}); err == nil {
				t.Fatal("unverified mirror release must not be installed")
			}
			if readTestFile(t, pluginFile) != "original" {
				t.Fatal("plugin changed without a matching mirror release")
			}
			for _, request := range *requests {
				if strings.Contains(request, peviewerMain) || strings.Contains(request, peviewerArchive) {
					t.Fatalf("unverified mirror must not be fetched: %v", *requests)
				}
			}
		})
	}
}
