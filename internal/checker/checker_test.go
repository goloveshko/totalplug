// File: internal/checker/checker_test.go
package checker

import (
	"testing"
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
