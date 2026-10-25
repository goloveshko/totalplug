package checker

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// An https -> http redirect is the downgrade the updater refuses for archives;
// the catalog and GitHub lookups must refuse it just the same, because what
// they hand back decides what gets downloaded.
func TestCheckClientRefusesHTTPSDowngradeRedirect(t *testing.T) {
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"downgraded":true}`))
	}))
	defer plain.Close()

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, plain.URL, http.StatusFound)
	}))
	defer srv.Close()

	client := newCheckClient(time.Second)
	// Trust only the test server's own certificate; the rule under test is the
	// redirect policy, not TLS verification.
	client.Transport = &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}} //nolint:gosec

	_, err := client.Get(srv.URL)
	if err == nil || !strings.Contains(err.Error(), "refusing insecure redirect") {
		t.Fatalf("https -> http redirect must be refused, got: %v", err)
	}
}

// A plain-http first hop stays allowed — the test servers run on it — and an
// http -> http redirect along the way must keep working.
func TestCheckClientAllowsSameSchemeRedirect(t *testing.T) {
	dest := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer dest.Close()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, dest.URL, http.StatusFound)
	}))
	defer srv.Close()

	resp, err := newCheckClient(time.Second).Get(srv.URL)
	if err != nil {
		t.Fatalf("http -> http redirect must be followed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200 after redirect", resp.StatusCode)
	}
}
