package updater

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

// Lazy totalcmd.net mirror fallback. The catalog does not carry mirrors
// (scraping ~1200 plugin pages from CI every 6h would hammer the site), so
// when every known source fails we fetch the plugin page from the user's own
// machine and retry the mirrors listed there.

var (
	// totalcmdPluginPageURLFormat is a var so tests can point it at a server.
	totalcmdPluginPageURLFormat = "https://totalcmd.net/plugring/%s.html"
	// trustedMirrorSuffixes limits which mirror hosts we will download from:
	// exact match or subdomain of a listed suffix.
	trustedMirrorSuffixes = []string{"wincmd.ru"}
)

// mirrorAnchorRe matches <a href="https://...">Mirror</a> on a plugring page.
// The page is CP1251, but URLs and the "Mirror" label are ASCII, so matching
// on raw bytes is safe.
var mirrorAnchorRe = regexp.MustCompile(`(?i)<a\b[^>]*href="(https://[^"]+)"[^>]*>\s*Mirror\s*</a>`)

// totalcmdPluginID returns the totalcmd.net plugin id if rawURL is a
// totalcmd.net download.php link, or "" for anything else.
func totalcmdPluginID(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	if !strings.EqualFold(u.Hostname(), "totalcmd.net") || !strings.HasSuffix(u.Path, "/download.php") {
		return ""
	}
	return u.Query().Get("id")
}

// isTrustedMirrorHost reports whether host equals or is a subdomain of one of
// the trusted mirror suffixes.
func isTrustedMirrorHost(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	for _, suffix := range trustedMirrorSuffixes {
		if host == suffix || strings.HasSuffix(host, "."+suffix) {
			return true
		}
	}
	return false
}

// resolveTotalCmdMirrors fetches the plugring page for the plugin behind
// primaryURL and returns mirror download URLs on trusted hosts. Any failure
// yields nil: this is a best-effort fallback.
func (u *Updater) resolveTotalCmdMirrors(ctx context.Context, primaryURL string) []string {
	id := totalcmdPluginID(primaryURL)
	if id == "" {
		return nil
	}

	pageURL := fmt.Sprintf(totalcmdPluginPageURLFormat, url.PathEscape(id))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("User-Agent", "TotalPlug/1.0 (Total Commander Plugin Manager)")

	resp, err := u.client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		if err == nil {
			resp.Body.Close()
		}
		return nil
	}
	defer resp.Body.Close()

	// Pages are tens of KB; the cap bounds damage from a misconfigured host.
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil
	}

	var mirrors []string
	seen := make(map[string]bool)
	for _, m := range mirrorAnchorRe.FindAllStringSubmatch(string(body), -1) {
		link := m[1]
		if seen[link] || !isTrustedMirrorHost(link) {
			continue
		}
		seen[link] = true
		mirrors = append(mirrors, link)
	}
	return mirrors
}
