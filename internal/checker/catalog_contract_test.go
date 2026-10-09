package checker

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestCatalogSampleContract guards the two-repo boundary: the builder in
// totalplug-catalog and this consumer hand-duplicate the CatalogEntry JSON
// tags independently, so a renamed or dropped key on the producer side would
// decode to zero values silently and show "Not in catalog" for real users.
// testdata/catalog.sample.json is captured from actual catalog.min.json, so
// decoding it here proves the consumer tags still line up with the wire shape
// (and that the client survives min.json not carrying the "source" block).
func TestCatalogSampleContract(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "catalog.sample.json"))
	if err != nil {
		t.Fatal(err)
	}

	var snap CatalogSnapshot
	if err := json.Unmarshal(raw, &snap); err != nil {
		t.Fatalf("decode sample: %v", err)
	}

	if snap.Version != 1 {
		t.Errorf("Version = %d, want 1", snap.Version)
	}
	if snap.GeneratedAt.IsZero() {
		t.Error("generated_at did not decode")
	}
	if len(snap.Plugins) != 4 {
		t.Fatalf("loaded %d plugins, want 4", len(snap.Plugins))
	}

	byID := make(map[string]CatalogEntry, len(snap.Plugins))
	for _, p := range snap.Plugins {
		byID[p.ID] = p
	}

	// Every entry must expose the fields the checker maps into domain.Plugin.
	for id, p := range byID {
		if p.Name == "" {
			t.Errorf("%s: name not decoded", id)
		}
		if len(p.Match.Filenames) == 0 {
			t.Errorf("%s: match.filenames not decoded", id)
		}
		if p.Resolved == nil {
			t.Fatalf("%s: resolved not decoded", id)
		}
		if p.Resolved.Version == "" || p.Resolved.DownloadURL == "" || p.Resolved.ResolvedFrom == "" {
			t.Errorf("%s: resolved version/download_url/resolved_from incomplete: %+v", id, p.Resolved)
		}
	}

	// catalog.min.json no longer ships the "source" declaration; the decoder
	// must tolerate its absence rather than fail or leave required fields empty.
	var generic struct {
		Plugins []map[string]any `json:"plugins"`
	}
	if err := json.Unmarshal(raw, &generic); err != nil {
		t.Fatal(err)
	}
	for _, p := range generic.Plugins {
		if _, ok := p["source"]; ok {
			t.Errorf("sample entry %v unexpectedly carries 'source'", p["id"])
		}
	}

	// Nested alternative sources decode, and downloadCandidates reads them.
	cu := byID["cudalister"]
	total, ok := cu.AvailableSources["totalcmd"]
	if !ok {
		t.Fatalf("cudalister: available_sources.totalcmd not decoded: %v", cu.AvailableSources)
	}
	if total.Version != "1.8.6.1" || total.DownloadURL == "" {
		t.Errorf("cudalister totalcmd alternative incomplete: %+v", total)
	}
	if cu.Resolved.PublishedAt.IsZero() {
		t.Error("cudalister resolved.published_at did not decode")
	}
	if !cu.Resolved.HasSource {
		t.Error("cudalister resolved.has_source did not decode")
	}
	// downloadCandidates reads entry.Resolved tags; the older totalcmd
	// alternative is deliberately excluded (different version), so only the
	// primary surfaces — asserting that proves the resolved tags feed through.
	cuCandidates := downloadCandidates(cu)
	if len(cuCandidates) != 1 || cuCandidates[0] != cu.Resolved.DownloadURL {
		t.Errorf("downloadCandidates(cudalister) = %v, want just the primary github url", cuCandidates)
	}

	// A plain base-feed entry has no alternatives; the direct path decodes too.
	if len(byID["totalcmd_13954"].AvailableSources) != 0 {
		t.Errorf("totalcmd_13954 should have no available_sources, got %v", byID["totalcmd_13954"].AvailableSources)
	}
	if got := downloadCandidates(byID["totalcmd_13954"]); len(got) != 1 {
		t.Errorf("downloadCandidates(totalcmd_13954) = %d, want 1", len(got))
	}
	if byID["directsample"].Resolved.ResolvedFrom != "direct" {
		t.Errorf("directsample resolved_from = %q, want %q", byID["directsample"].Resolved.ResolvedFrom, "direct")
	}
}
