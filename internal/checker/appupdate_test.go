package checker

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCheckAppRelease(t *testing.T) {
	tests := []struct {
		name       string
		current    string
		tag        string
		wantUpdate bool
		wantCompar bool
	}{
		{name: "older build offers the release", current: "1.0.0", tag: "v1.1.0", wantUpdate: true, wantCompar: true},
		{name: "same build is up to date", current: "1.1.0", tag: "v1.1.0", wantUpdate: false, wantCompar: true},
		{name: "newer build stays quiet", current: "1.2.0", tag: "v1.1.0", wantUpdate: false, wantCompar: true},
		{name: "dev build never compares", current: "dev", tag: "v1.1.0", wantUpdate: false, wantCompar: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"tag_name":"` + tt.tag + `","html_url":"https://github.com/goloveshko/totalplug/releases/tag/` + tt.tag + `"}`))
			}))
			defer server.Close()

			oldBase := GitHubAPIBase
			GitHubAPIBase = server.URL
			t.Cleanup(func() { GitHubAPIBase = oldBase })

			got, err := CheckAppRelease(context.Background(), tt.current)
			if err != nil {
				t.Fatal(err)
			}
			if got.Available != tt.wantUpdate {
				t.Errorf("Available = %v, want %v", got.Available, tt.wantUpdate)
			}
			if got.Comparable != tt.wantCompar {
				t.Errorf("Comparable = %v, want %v", got.Comparable, tt.wantCompar)
			}
			// The API reports "v1.1.0"; the comparison and the UI both use the bare form.
			if want := tt.tag[1:]; got.Latest != want {
				t.Errorf("Latest = %q, want %q", got.Latest, want)
			}
		})
	}
}
