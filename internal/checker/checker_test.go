// File: internal/checker/checker_test.go
package checker

import (
	"testing"

	"totalplug/internal/domain"
)

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		v1       string
		v2       string
		expected int // -1 (v1 < v2), 0 (v1 == v2), 1 (v1 > v2)
	}{
		{"1.0.0", "1.0.0", 0},
		{"1.2.0", "1.2.5", -1},
		{"2.0.1", "1.9.9", 1},
		{"v1.8.6.1", "1.8.6.1", 0},
		{"1.8.6.0", "1.8.6.1", -1},
		{"1.2 (build 200)", "1.2", 0},
		{"2.0", "2.0.0.0", 0},
		{"0.9.9", "1.0.0", -1},
		{"10.0.1", "2.0.5", 1},
	}

	for _, tt := range tests {
		t.Run(tt.v1+"_vs_"+tt.v2, func(t *testing.T) {
			got := CompareVersions(tt.v1, tt.v2)
			if got != tt.expected {
				t.Errorf("CompareVersions(%q, %q) = %d; want %d", tt.v1, tt.v2, got, tt.expected)
			}
		})
	}
}

func TestNormalizeName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"CudaLister", "cudalister"},
		{"wlx_cudalister_64", "cudalister"},
		{"tc_total7zip plugin", "total7zip"},
		{"TC mdview ext", "mdview"},
		{"Plugins", "plugin"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := normalizeName(tt.input)
			if got != tt.expected {
				t.Errorf("normalizeName(%q) = %q; want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestIsResolvableVersion(t *testing.T) {
	resolvable := []string{"1.2.3", "v2.0", "1.2 (build 200)"}
	for _, v := range resolvable {
		if !isResolvableVersion(v) {
			t.Errorf("isResolvableVersion(%q) = false, want true", v)
		}
	}
	unresolvable := []string{"Unknown", "N/A", "", "n/a", "no version info"}
	for _, v := range unresolvable {
		if isResolvableVersion(v) {
			t.Errorf("isResolvableVersion(%q) = true, want false", v)
		}
	}
}

func TestClassifyUpdate(t *testing.T) {
	tests := []struct {
		name       string
		installed  string
		latest     string
		confidence domain.VersionConfidence
		want       domain.PluginStatus
	}{
		{"known older offers update", "1.8.6.1", "1.8.6.3", domain.ConfidenceExact, domain.StatusUpdateAvail},
		{"known current is up to date", "1.0.0", "1.0.0", domain.ConfidenceExact, domain.StatusUpToDate},
		{"known newer stays up to date", "2.0.0", "1.5.0", domain.ConfidenceExact, domain.StatusUpToDate},
		{"unknown installed never auto-updates", "Unknown", "1.8.6.3", domain.ConfidenceFallback, domain.StatusUncertain},
		{"NA installed never auto-updates", "N/A", "1.8.6.3", domain.ConfidenceExact, domain.StatusUncertain},
		{"fallback confidence blocks update", "1.0.0", "2.0.0", domain.ConfidenceFallback, domain.StatusUncertain},
		{"unparseable latest is uncertain", "1.0.0", "latest", domain.ConfidenceExact, domain.StatusUncertain},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyUpdate(tt.installed, tt.latest, tt.confidence)
			if got != tt.want {
				t.Errorf("classifyUpdate(%q, %q, %q) = %q; want %q",
					tt.installed, tt.latest, tt.confidence, got, tt.want)
			}
		})
	}
}
