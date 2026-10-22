package updater

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

// Mirror discovery is lazy: fetching every plugin page during catalog refreshes
// would hammer the site. Only exhausted totalcmd.net downloads reach this path.
const totalcmdPluginPageURLFormat = "https://totalcmd.net/plugins/%s"

var totalcmdPluginIDRe = regexp.MustCompile(`^[A-Za-z0-9_-][A-Za-z0-9_.-]*$`)

// totalcmdPluginID recognizes only the legacy counter and current main download
// routes, not arbitrary paths ending in download.php or other download variants.
func totalcmdPluginID(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || !isMirrorHTTPSURL(u) || !strings.EqualFold(u.Hostname(), "totalcmd.net") || u.RawPath != "" || u.Fragment != "" {
		return ""
	}
	var id string
	if u.Path == "/download.php" {
		query, err := url.ParseQuery(u.RawQuery)
		if err != nil || len(query["id"]) != 1 {
			return ""
		}
		id = query.Get("id")
	} else if parts := strings.Split(u.Path, "/"); len(parts) == 4 && parts[0] == "" && parts[1] == "download" && parts[3] == "main" && u.RawQuery == "" && !u.ForceQuery {
		id = parts[2]
	}
	if !totalcmdPluginIDRe.MatchString(id) {
		return ""
	}
	return id
}

func isMirrorHTTPSURL(u *url.URL) bool {
	return u != nil && u.Scheme == "https" && u.Hostname() != "" && u.User == nil && u.Opaque == "" && (u.Port() == "" || u.Port() == "443")
}

// isTrustedMirrorHost also requires HTTPS; plaintext remains forbidden even when
// the caller opted into an insecure primary download.
func isTrustedMirrorHost(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil || !isMirrorHTTPSURL(u) {
		return false
	}
	host := strings.ToLower(u.Hostname())
	return host == "wincmd.ru" || strings.HasSuffix(host, ".wincmd.ru")
}

func isTotalcmdPluginPage(u *url.URL, host, id string) bool {
	return isMirrorHTTPSURL(u) && strings.EqualFold(u.Hostname(), host) && u.Path == "/plugins/"+id && u.RawPath == "" && u.RawQuery == "" && !u.ForceQuery && u.Fragment == ""
}

// mirrorClient checks every redirect before it reaches the network, without
// changing the updater's client or its policy for normal catalog downloads.
func (u *Updater) mirrorClient(allowed func(*url.URL) bool) *http.Client {
	client := *u.client
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 || !allowed(req.URL) {
			return fmt.Errorf("untrusted or excessive mirror redirect: %s", req.URL)
		}
		if u.client.CheckRedirect != nil {
			return u.client.CheckRedirect(req, via)
		}
		return nil
	}
	return &client
}

// HEAD and GET can redirect differently, so both must enforce the mirror allowlist.
func (u *Updater) tryTotalCmdMirrors(ctx context.Context, mirrors []string, archiveFile, keepDir string) (securedPackage, error) {
	for _, link := range mirrors {
		if !isTrustedMirrorHost(link) {
			return securedPackage{}, fmt.Errorf("untrusted mirror URL: %s", link)
		}
	}
	fallback := *u
	fallback.client = u.mirrorClient(func(link *url.URL) bool {
		return isTrustedMirrorHost(link.String())
	})
	return fallback.tryDownloadCandidates(ctx, mirrors, archiveFile, keepDir, false)
}

func (u *Updater) mirrorResponse(ctx context.Context, method, rawURL string, allowed func(*url.URL) bool) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, rawURL, nil)
	if err != nil {
		return nil, err
	}
	if !allowed(req.URL) {
		return nil, fmt.Errorf("untrusted mirror URL: %s", rawURL)
	}
	req.Header.Set("User-Agent", "TotalPlug/1.0 (Total Commander Plugin Manager)")
	resp, err := u.mirrorClient(allowed).Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK || resp.Request == nil || !allowed(resp.Request.URL) {
		resp.Body.Close()
		return nil, fmt.Errorf("invalid mirror response: %s", rawURL)
	}
	return resp, nil
}

func (u *Updater) mirrorPage(ctx context.Context, rawURL, host, id string) (*html.Node, error) {
	resp, err := u.mirrorResponse(ctx, http.MethodGet, rawURL, func(link *url.URL) bool {
		return isTotalcmdPluginPage(link, host, id)
	})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	// Reject oversized pages rather than parsing a truncated release label.
	const limit = 2 << 20
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if len(body) > limit {
		return nil, fmt.Errorf("mirror page too large")
	}
	return html.Parse(strings.NewReader(string(body)))
}

func mirrorAttr(n *html.Node, name string) string {
	for _, attr := range n.Attr {
		if attr.Key == name {
			return attr.Val
		}
	}
	return ""
}

func mirrorHasClass(n *html.Node, class string) bool {
	for _, value := range strings.Fields(mirrorAttr(n, "class")) {
		if value == class {
			return true
		}
	}
	return false
}

func findMirrorNode(n *html.Node, match func(*html.Node) bool) *html.Node {
	if n == nil {
		return nil
	}
	if n.Type == html.ElementNode && match(n) {
		return n
	}
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		if found := findMirrorNode(child, match); found != nil {
			return found
		}
	}
	return nil
}

func mirrorText(n *html.Node) string {
	if n.Type == html.TextNode {
		return n.Data
	}
	var text strings.Builder
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		text.WriteString(mirrorText(child))
	}
	return text.String()
}

// A mirror must advertise expectedVersion, or the installed version would be recorded incorrectly.
func (u *Updater) resolveTotalCmdMirrors(ctx context.Context, primaryURL string, expectedVersion string) []string {
	id := totalcmdPluginID(primaryURL)
	expectedVersion = strings.TrimSpace(expectedVersion)
	if id == "" || expectedVersion == "" {
		return nil
	}
	pageURL := fmt.Sprintf(totalcmdPluginPageURLFormat, id)
	page, err := u.mirrorPage(ctx, pageURL, "totalcmd.net", id)
	if err != nil {
		return nil
	}
	counterpart := findMirrorNode(page, func(n *html.Node) bool {
		link, err := url.Parse(mirrorAttr(n, "href"))
		return n.Data == "a" && mirrorAttr(n, "hreflang") == "ru" && err == nil && isTotalcmdPluginPage(link, "wincmd.ru", id)
	})
	if counterpart == nil {
		return nil
	}
	ruPageURL := mirrorAttr(counterpart, "href")
	page, err = u.mirrorPage(ctx, ruPageURL, "wincmd.ru", id)
	if err != nil {
		return nil
	}
	article := findMirrorNode(page, func(n *html.Node) bool { return n.Data == "article" && mirrorHasClass(n, "plugin-page") })
	header := findMirrorNode(article, func(n *html.Node) bool { return n.Data == "header" && mirrorHasClass(n, "plugin-head") })
	heading := findMirrorNode(header, func(n *html.Node) bool { return n.Data == "h1" })
	version := findMirrorNode(heading, func(n *html.Node) bool { return n.Data == "span" && mirrorHasClass(n, "ver") })
	if version == nil || strings.TrimSpace(mirrorText(version)) != expectedVersion {
		return nil
	}
	side := findMirrorNode(article, func(n *html.Node) bool { return n.Data == "aside" && mirrorHasClass(n, "plugin-side") })
	base, _ := url.Parse(ruPageURL)
	mainDownload := findMirrorNode(side, func(n *html.Node) bool {
		link, err := url.Parse(mirrorAttr(n, "href"))
		if n.Data != "a" || !mirrorHasClass(n, "btn-primary") || err != nil {
			return false
		}
		link = base.ResolveReference(link)
		return isMirrorHTTPSURL(link) && strings.EqualFold(link.Hostname(), "wincmd.ru") && link.Path == "/download/"+id+"/main" && link.RawPath == "" && link.RawQuery == "" && !link.ForceQuery && link.Fragment == ""
	})
	if mainDownload == nil {
		return nil
	}
	link, _ := url.Parse(mirrorAttr(mainDownload, "href"))
	// HEAD rejects off-site counters without downloading the archive twice.
	resp, err := u.mirrorResponse(ctx, http.MethodHead, base.ResolveReference(link).String(), func(link *url.URL) bool {
		return isTrustedMirrorHost(link.String())
	})
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	archive := resp.Request.URL
	switch strings.ToLower(path.Ext(archive.Path)) {
	case ".zip", ".rar", ".7z":
		return []string{archive.String()}
	default:
		return nil
	}
}
