package checker

import (
	"fmt"
	"net/http"
	"time"
)

// newCheckClient builds the HTTP client the catalog fetch and the GitHub
// release lookup share. The redirect rule re-checks the scheme on every hop:
// the default policy follows an https -> http redirect on its own and hands
// back a response fetched in plaintext — the same downgrade the updater
// refuses for plugin archives, only for the catalog and release metadata that
// decide what gets downloaded. A plain-http first hop stays allowed, which is
// what the test servers run on; a hop to any other scheme is refused.
func newCheckClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
				return fmt.Errorf("refusing redirect to unsupported URL scheme %q (%s)", req.URL.Scheme, req.URL)
			}
			if len(via) > 0 && via[0].URL.Scheme == "https" && req.URL.Scheme != "https" {
				return fmt.Errorf("refusing insecure redirect from %s to %s", via[0].URL, req.URL)
			}
			return nil
		},
	}
}
