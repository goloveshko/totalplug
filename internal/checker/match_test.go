package checker

import (
	"testing"

	"github.com/goloveshko/totalplug/internal/domain"
)

// matchedEntry is the catalog entry the matcher chose for the installed
// plugin, or nil when it chose none.
func matchedEntry(local domain.Plugin, catalog []CatalogEntry) *CatalogEntry {
	return matchCatalogEntry(local, catalog).Entry
}

// peViewerCatalog is the shape the live catalog holds for PE Viewer after the
// builder folded the upload from 2012 into the one from 2025: one entry, the
// current version, the page of the current upload, and the file name of the
// older page — which is the name the installed copy answers to.
func peViewerCatalog() []CatalogEntry {
	return []CatalogEntry{
		{
			ID:   "totalcmd_PEViewer3",
			Name: "PE Viewer",
			Arch: "x32+x64",
			Match: MatchRule{
				Aliases:   []string{"pe viewer", "PEViewer3", "peviewer"},
				Filenames: []string{"peviewer3.wlx", "peviewer.wlx"},
			},
			Resolved: &ResolvedSource{
				Version:     "3.0.18",
				DownloadURL: "https://totalcmd.net/download.php?id=PEViewer3",
				WebURL:      "https://totalcmd.net/plugins/PEViewer3",
			},
		},
	}
}

// The plugin the site calls PE Viewer installs as "PEViewer.wlx", and the
// entry that carries the current release is the one the site serves under
// "PEViewer3". Before the catalog folded the older upload into it, the
// installed copy was answered with the 2012 release and a page of 2012.
func TestMatchCatalogEntryFindsTheCurrentUpload(t *testing.T) {
	catalog := peViewerCatalog()

	tests := []struct {
		name string
		path string
	}{
		{name: "the 32-bit build", path: `C:\Program Files\Total Commander\Plug-Ins\wlx\PEViewer\PEViewer.wlx`},
		{name: "the 64-bit build", path: `C:\Program Files\Total Commander\Plug-Ins\wlx\PEViewer\PEViewer.wlx64`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			local := domain.Plugin{
				Name:             "PEViewer",
				Path:             tt.path,
				Type:             domain.TypeLister,
				InstalledVersion: "3.0.18",
			}

			entry := matchedEntry(local, catalog)
			if entry == nil {
				t.Fatal("no catalog entry matched")
			}
			if entry.Resolved.Version != "3.0.18" {
				t.Errorf("version = %q, want 3.0.18", entry.Resolved.Version)
			}
			if want := "https://totalcmd.net/plugins/PEViewer3"; entry.Resolved.WebURL != want {
				t.Errorf("site = %q, want %q", entry.Resolved.WebURL, want)
			}
		})
	}
}

// uninstallerCatalog is the shape the live catalog holds for a plugin that
// ships in two builds under names that differ only in the bit suffix: the
// 32-bit entry comes first and carries the newer version, so a matcher that
// takes the first hit in catalog order hands the 64-bit plugin the 32-bit
// entry — and the update then unpacks the other build's archive.
func uninstallerCatalog() []CatalogEntry {
	return []CatalogEntry{
		{
			ID:   "totalcmd_uninstaller",
			Name: "Uninstaller",
			Arch: "x32",
			Match: MatchRule{
				Aliases:   []string{"uninstaller"},
				Filenames: []string{"uninstaller.wfx"},
			},
			Resolved: &ResolvedSource{Version: "1.8.1", DownloadURL: "https://example.invalid/uninstaller.zip"},
		},
		{
			ID:   "totalcmd_uninstaller64",
			Name: "Uninstaller64",
			Arch: "x32+x64",
			Match: MatchRule{
				Aliases:   []string{"uninstaller64"},
				Filenames: []string{"uninstaller64.wfx"},
			},
			Resolved: &ResolvedSource{Version: "1.0.1", DownloadURL: "https://example.invalid/uninstaller64.zip"},
		},
	}
}

func TestMatchCatalogEntryTellsTheTwoBuildsApart(t *testing.T) {
	catalog := uninstallerCatalog()

	tests := []struct {
		name string
		local domain.Plugin
		want string
	}{
		{
			name: "the 64-bit plugin gets the 64-bit entry",
			local: domain.Plugin{
				Name: "Uninstaller64",
				Path: `C:\TC\Plug-Ins\wfx\Uninstaller64\uninstaller64.wfx64`,
			},
			want: "totalcmd_uninstaller64",
		},
		{
			name: "the 32-bit plugin gets the 32-bit entry",
			local: domain.Plugin{
				Name: "Uninstaller",
				Path: `C:\TC\Plug-Ins\wfx\Uninstaller\uninstaller.wfx`,
			},
			want: "totalcmd_uninstaller",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := matchedEntry(tt.local, catalog)
			if entry == nil {
				t.Fatal("no catalog entry matched")
			}
			if entry.ID != tt.want {
				t.Errorf("matched %q, want %q", entry.ID, tt.want)
			}
		})
	}
}

// A 64-bit plugin whose name is the plain one must not be handed the 32-bit
// archive either: the file says which build it is.
func TestMatchCatalogEntryKeepsTheArchitectureApart(t *testing.T) {
	catalog := uninstallerCatalog()

	local := domain.Plugin{
		Name: "Uninstaller",
		Path: `C:\TC\Plug-Ins\wfx\Uninstaller64\uninstaller64.wfx64`,
	}

	entry := matchedEntry(local, catalog)
	if entry == nil {
		t.Fatal("no catalog entry matched")
	}
	if entry.ID != "totalcmd_uninstaller64" {
		t.Errorf("matched %q, want %q", entry.ID, "totalcmd_uninstaller64")
	}
}

// A name made of nothing but non-Latin letters normalizes to the empty
// string, and the empty string matches every entry in the catalog.
func TestMatchCatalogEntryRefusesAnEmptyNormalizedName(t *testing.T) {
	catalog := []CatalogEntry{
		{ID: "totalcmd_archive", Name: "Архив", Match: MatchRule{Aliases: []string{"архив"}}, Resolved: &ResolvedSource{Version: "1.0"}},
		{ID: "totalcmd_other", Name: "Другой", Match: MatchRule{Aliases: []string{"другой"}}, Resolved: &ResolvedSource{Version: "1.0"}},
	}

	local := domain.Plugin{Name: "Архиватор", Path: `C:\TC\Plug-Ins\wfx\Архиватор\arch.wfx`}

	if entry := matchedEntry(local, catalog); entry != nil {
		t.Errorf("matched %q, want no match", entry.ID)
	}
}

// The catalog holds several entries for the same plugin; the newest release
// among them is the one the user wants to hear about.
func TestMatchCatalogEntryTakesTheNewestRelease(t *testing.T) {
	catalog := []CatalogEntry{
		{
			ID:      "totalcmd_old",
			Name:    "ArchView",
			Arch:    "x32+x64",
			Match:   MatchRule{Filenames: []string{"archview.wlx"}},
			Resolved: &ResolvedSource{Version: "0.9.1.2"},
		},
		{
			ID:      "totalcmd_new",
			Name:    "ArchView",
			Arch:    "x32+x64",
			Match:   MatchRule{Filenames: []string{"archview.wlx"}},
			Resolved: &ResolvedSource{Version: "0.9.3.0"},
		},
	}

	local := domain.Plugin{Name: "ArchView", Path: `C:\TC\Plug-Ins\wlx\ArchView\archview.wlx`}

	entry := matchedEntry(local, catalog)
	if entry == nil {
		t.Fatal("no catalog entry matched")
	}
	if entry.ID != "totalcmd_new" {
		t.Errorf("matched %q, want %q", entry.ID, "totalcmd_new")
	}
}

// mdViewCatalog is the shape the live catalog holds for the two MDView
// plugins for Total Commander: both install "mdview.wlx", and their version
// numbers belong to different release lines - the WebView2 plugin counts from
// zero, the MSHTML one from three.
func mdViewCatalog() []CatalogEntry {
	return []CatalogEntry{
		{
			ID:       "mdview",
			Name:     "MDView (WebView2)",
			Homepage: "https://github.com/rweijnen/MDView",
			Arch:     "x32+x64",
			Match: MatchRule{
				Aliases:   []string{"mdview", "wlx_mdview", "markdown viewer"},
				Filenames: []string{"mdview.wlx", "mdview.wlx64"},
			},
			Resolved: &ResolvedSource{
				Version:     "0.2.4",
				DownloadURL: "https://github.com/rweijnen/MDView/releases/download/v0.2.4/MDView-v0.2.4.zip",
			},
		},
		{
			ID:       "mdview_mshtml",
			Name:     "MDView (MSHTML)",
			Homepage: "https://github.com/jumpifequal/mdview-for-Total-Commander",
			Arch:     "x32+x64",
			Match: MatchRule{
				Aliases:   []string{"mdview", "mdview_lister_extension"},
				Filenames: []string{"mdview.wlx", "mdview.wlx64"},
			},
			Resolved: &ResolvedSource{
				Version:     "3.8.2",
				DownloadURL: "https://totalcmd.net/download.php?id=mdview_lister_extension",
			},
		},
	}
}

// Two MDView plugins install the same file, and the newer version number
// belongs to the other one: the MSHTML plugin is three years ahead of the
// WebView2 one. The installed version is what tells them apart - and without
// it, the newer one would be offered to the user of the other plugin.
func TestMatchCatalogEntryTellsTheTwoMDViewsApart(t *testing.T) {
	catalog := mdViewCatalog()

	tests := []struct {
		name    string
		version string
		want    string
	}{
		{name: "the WebView2 plugin", version: "0.2.4", want: "mdview"},
		{name: "the WebView2 plugin, as the PE resource states it", version: "0.2.4.0", want: "mdview"},
		{name: "the MSHTML plugin", version: "3.8.2", want: "mdview_mshtml"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			local := domain.Plugin{
				Name:             "mdview",
				Path:             `C:\TC\Plug-Ins\wlx\mdview\mdview.wlx`,
				InstalledVersion: tt.version,
				Confidence:       domain.ConfidenceExact,
			}

			entry := matchedEntry(local, catalog)
			if entry == nil {
				t.Fatal("no catalog entry matched")
			}
			if entry.ID != tt.want {
				t.Errorf("matched %q, want %q", entry.ID, tt.want)
			}
		})
	}
}

// Without a version that can be read there is nothing to tell the two MDView
// plugins apart, and offering either of them would offer the user the other
// one. The match is refused and says so.
func TestMatchCatalogEntryRefusesAnUnreadableVersionForAMultiplePluginFile(t *testing.T) {
	catalog := mdViewCatalog()

	local := domain.Plugin{
		Name:       "mdview",
		Path:       `C:\TC\Plug-Ins\wlx\mdview\mdview.wlx`,
		Confidence: domain.ConfidenceFallback,
	}

	match := matchCatalogEntry(local, catalog)
	if match.Entry != nil {
		t.Errorf("matched %q, want no match", match.Entry.ID)
	}
	if !match.Ambiguous {
		t.Error("the match is not reported as ambiguous")
	}
}

// A version scraped out of a readme is prose about the plugin, not a number
// about the build that is installed, so it does not tell the two apart either.
func TestMatchCatalogEntryRefusesAReadmeVersionForAMultiplePluginFile(t *testing.T) {
	catalog := mdViewCatalog()

	local := domain.Plugin{
		Name:             "mdview",
		Path:             `C:\TC\Plug-Ins\wlx\mdview\mdview.wlx`,
		InstalledVersion: "3.8.2",
		Confidence:       domain.ConfidenceText,
	}

	match := matchCatalogEntry(local, catalog)
	if match.Entry != nil {
		t.Errorf("matched %q, want no match", match.Entry.ID)
	}
	if !match.Ambiguous {
		t.Error("the match is not reported as ambiguous")
	}
}

// One plugin in two rows is not two plugins: the newest release of it still
// wins, even when the installed copy is on another release line.
func TestMatchCatalogEntryTakesTheNewestReleaseOfOnePlugin(t *testing.T) {
	catalog := []CatalogEntry{
		{
			ID:       "mdview",
			Name:     "MDView",
			Homepage: "https://github.com/rweijnen/MDView",
			Match:    MatchRule{Filenames: []string{"mdview.wlx"}},
			Resolved: &ResolvedSource{Version: "1.2.0"},
		},
		{
			ID:       "mdview_0",
			Name:     "MDView",
			Homepage: "https://github.com/rweijnen/MDView",
			Match:    MatchRule{Filenames: []string{"mdview.wlx"}},
			Resolved: &ResolvedSource{Version: "0.9.1"},
		},
	}

	local := domain.Plugin{
		Name:             "mdview",
		Path:             `C:\TC\Plug-Ins\wlx\mdview\mdview.wlx`,
		InstalledVersion: "0.9.1",
		Confidence:       domain.ConfidenceExact,
	}

	entry := matchedEntry(local, catalog)
	if entry == nil {
		t.Fatal("no catalog entry matched")
	}
	if entry.ID != "mdview" {
		t.Errorf("matched %q, want %q", entry.ID, "mdview")
	}
}

// The catalog may be ahead of the installed copy: a plugin that moved from
// 0.x to 1.0 numbers its releases on another line, and then the line nearest
// to the installed one is the closest thing to it - the other plugin's line
// is three numbers away.
func TestMatchCatalogEntryTakesTheNearestLineWhenTheInstalledOneIsGone(t *testing.T) {
	catalog := mdViewCatalog()
	catalog[0].Resolved.Version = "1.0.0"

	local := domain.Plugin{
		Name:             "mdview",
		Path:             `C:\TC\Plug-Ins\wlx\mdview\mdview.wlx`,
		InstalledVersion: "0.2.4",
		Confidence:       domain.ConfidenceExact,
	}

	entry := matchedEntry(local, catalog)
	if entry == nil {
		t.Fatal("no catalog entry matched")
	}
	if entry.ID != "mdview" {
		t.Errorf("matched %q, want %q", entry.ID, "mdview")
	}
}

// Two plugins equally near the installed version tell nothing apart, and the
// match is refused rather than guessed.
func TestMatchCatalogEntryRefusesTwoEquallyNearLines(t *testing.T) {
	catalog := mdViewCatalog()
	catalog[0].Resolved.Version = "2.0.0"
	catalog[1].Resolved.Version = "4.0.0"

	local := domain.Plugin{
		Name:             "mdview",
		Path:             `C:\TC\Plug-Ins\wlx\mdview\mdview.wlx`,
		InstalledVersion: "3.0.0",
		Confidence:       domain.ConfidenceExact,
	}

	match := matchCatalogEntry(local, catalog)
	if match.Entry != nil {
		t.Errorf("matched %q, want no match", match.Entry.ID)
	}
	if !match.Ambiguous {
		t.Error("the match is not reported as ambiguous")
	}
}

func TestPluginKeyKeepsTheBitMarker(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"Uninstaller64", "uninstaller64"},
		{`C:\TC\Plug-Ins\wfx\Uninstaller64\uninstaller64.wfx64`, "uninstaller64"},
		{`C:\TC\Plug-Ins\wlx\ASNView\asnview.wlx64`, "asnview"},
		{`C:\TC\Plug-Ins\wlx\ASNView\asnview.wlx`, "asnview"},
		{"ArchView", "archview"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := pluginKey(tt.name); got != tt.want {
				t.Errorf("pluginKey(%q) = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}

func TestBitMarker(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{`C:\TC\Plug-Ins\wfx\Uninstaller64\uninstaller64.wfx64`, "64"},
		{`C:\TC\Plug-Ins\wfx\Uninstaller\uninstaller.wfx`, "32"},
		{`C:\TC\Plug-Ins\wlx\ASNView\asnview.wlx64`, "64"},
		{`C:\TC\Plug-Ins\wlx\ArchView\archview64.wlx`, "64"},
		{`C:\TC\Plug-Ins\wcx\D64\d64.wcx`, "64"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := bitMarker(tt.path); got != tt.want {
				t.Errorf("bitMarker(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

// An entry whose archive carries both builds can serve either one.
func TestServesThisBuild(t *testing.T) {
	both := &CatalogEntry{Arch: "x32+x64", Match: MatchRule{Filenames: []string{"asnview.wlx"}}}
	if !servesThisBuild(both, `C:\TC\Plug-Ins\wlx\ASNView\asnview.wlx64`) {
		t.Error("an entry with both builds must serve the 64-bit build")
	}

	x32 := &CatalogEntry{Arch: "x32", Match: MatchRule{Filenames: []string{"uninstaller.wfx"}}}
	if servesThisBuild(x32, `C:\TC\Plug-Ins\wfx\Uninstaller64\uninstaller64.wfx64`) {
		t.Error("an x32 entry must not serve a 64-bit plugin")
	}

	// A plugin whose own name ends in 64 reads the same on both sides, so the
	// entry about it is not thrown away.
	d64 := &CatalogEntry{Arch: "x32", Match: MatchRule{Filenames: []string{"d64.wcx"}}}
	if !servesThisBuild(d64, `C:\TC\Plug-Ins\wcx\D64\d64.wcx`) {
		t.Error("the entry about D64 must serve D64")
	}
}
