package checker

import (
	"path/filepath"
	"strings"

	"github.com/goloveshko/totalplug/internal/domain"
)

// The file extensions Total Commander gives its plugins, the 64-bit build
// carrying the 64 on the extension ("asnview.wlx64").
var pluginExtensions = []string{
	".wcx64", ".wlx64", ".wfx64", ".wdx64",
	".wcx", ".wlx", ".wfx", ".wdx",
}

// catalogMatch is what the catalog says about an installed plugin.
type catalogMatch struct {
	// Entry is the catalog entry that describes the installed plugin, or nil
	// when the catalog does not know it.
	Entry *CatalogEntry
	// Ambiguous is true when the catalog holds several plugins for the
	// installed file and the installed version cannot tell them apart. The
	// client says so instead of choosing one: choosing would offer the user
	// somebody else's plugin.
	Ambiguous bool
}

// matchCatalogEntry finds the catalog entry that describes the installed
// plugin, or reports that the catalog does not know it.
//
// The catalog holds both builds of many plugins under names that differ only
// in their bit marker ("Uninstaller" and "Uninstaller64"), and the loose
// match — the one that drops the marker, the "plugin" word and every
// non-Latin letter — cannot tell them apart. Walking the catalog entry by
// entry and taking the first hit let a 64-bit plugin answer to the 32-bit
// entry and be updated with the other build's archive, which is how a 64-bit
// plugin folder ends up holding a 32-bit plugin's files.
//
// So the catalog is walked by tier instead of by entry, and every tier keeps
// the bit marker: a name that says outright which plugin this is wins over a
// name that only looks like it.
func matchCatalogEntry(local domain.Plugin, catalog []CatalogEntry) catalogMatch {
	// Tier 1: the file on disk. It is the plugin's identity, and it keeps
	// the marker that tells the two builds apart.
	localFileKey := pluginKey(filepath.Base(local.Path))
	if localFileKey != "" {
		match := bestMatch(catalog, local, func(e *CatalogEntry) bool {
			for _, fn := range e.Match.Filenames {
				if pluginKey(fn) == localFileKey {
					return true
				}
			}
			return false
		})
		if match.Entry != nil || match.Ambiguous {
			return match
		}
	}

	// Tier 2: the name the ini knows the plugin by, or one of the entry's
	// own aliases.
	localNameKey := pluginKey(local.Name)
	if localNameKey != "" {
		match := bestMatch(catalog, local, func(e *CatalogEntry) bool {
			if pluginKey(e.Name) == localNameKey {
				return true
			}
			for _, alias := range e.Match.Aliases {
				if pluginKey(alias) == localNameKey {
					return true
				}
			}
			return false
		})
		if match.Entry != nil || match.Ambiguous {
			return match
		}
	}

	// Tier 3: the loose name match, with the bit marker dropped. It is the
	// last resort because it cannot tell the two builds of a plugin apart.
	normLocal := normalizeName(local.Name)
	if normLocal == "" {
		// A name made of nothing but non-Latin letters normalizes to the
		// empty string, and the empty string matches everything: the catalog
		// holds entries whose names are in Cyrillic, and a Russian-named
		// plugin must not answer to the first of them.
		return catalogMatch{}
	}
	return bestMatch(catalog, local, func(e *CatalogEntry) bool {
		return normalizeName(e.Name) == normLocal || normalizeName(e.ID) == normLocal
	})
}

// pluginKey is the identity of a plugin: the name without the file extension,
// lowercased. The bit marker stays in it — "archview64" and "archview" are the
// two builds of one plugin, and only the first says which is which.
func pluginKey(name string) string {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return ""
	}
	key := strings.ToLower(filepath.Base(trimmed))
	for _, ext := range pluginExtensions {
		if strings.HasSuffix(key, ext) {
			return strings.TrimSuffix(key, ext)
		}
	}
	return key
}

// bestMatch returns the entry among those the test accepts that describes the
// installed plugin.
//
// The catalog holds several entries for the same plugin — the same file under
// names that differ in case or in a version tag — and taking the first of
// them in catalog order hid the newest release behind the oldest.
//
// It also holds different plugins that install the same file: two MDView
// plugins for Total Commander install "mdview.wlx" each, one on WebView2 and
// one on the older MSHTML engine, and they install over each other. Their
// version numbers have nothing to do with each other, so "the newer release
// wins" hands the WebView2 user somebody else's plugin — the MSHTML one is
// three years ahead of the WebView2 one.
//
// The installed version is what tells them apart: 0.2.4 is the WebView2 MDView
// and 3.8.2 is the MSHTML one. Among the candidates, the one whose release
// line carries the installed version wins, and inside that line the newest
// release wins. When no candidate is on that line, the line nearest to the
// installed one is the closest thing to it. When the installed version cannot
// be read, nothing tells the candidates apart, and the match is refused rather
// than guessed.
func bestMatch(catalog []CatalogEntry, local domain.Plugin, accept func(*CatalogEntry) bool) catalogMatch {
	var candidates []*CatalogEntry
	for i := range catalog {
		entry := &catalog[i]
		if !accept(entry) {
			continue
		}
		if !servesThisBuild(entry, local.Path) {
			continue
		}
		candidates = append(candidates, entry)
	}

	if len(candidates) == 0 {
		return catalogMatch{}
	}
	if len(candidates) == 1 {
		return catalogMatch{Entry: candidates[0]}
	}

	// Several entries claim the same plugin. That is one plugin in two
	// releases when the entries are the same plugin, and two plugins that
	// install the same file when they are not.
	if !severalPlugins(candidates) {
		return catalogMatch{Entry: newestOf(candidates)}
	}

	// Two plugins, one file: the installed version says which one is on the
	// disk.
	if !trustedVersion(local) {
		return catalogMatch{Ambiguous: true}
	}
	line, ok := versionLine(local.InstalledVersion)
	if !ok {
		return catalogMatch{Ambiguous: true}
	}

	// The release line the installed version belongs to. The candidates on it
	// are the plugin the user has.
	var sameLine []*CatalogEntry
	for _, entry := range candidates {
		if entryLine, ok := versionLine(entryVersion(entry)); ok && entryLine == line {
			sameLine = append(sameLine, entry)
		}
	}
	if len(sameLine) > 0 {
		return catalogMatch{Entry: newestOf(sameLine)}
	}

	// No candidate is on that line. A candidate whose version the catalog
	// never resolved could be the installed plugin - it cannot be placed on
	// any line - so nothing is offered rather than the wrong plugin.
	for _, entry := range candidates {
		if _, ok := versionLine(entryVersion(entry)); !ok {
			return catalogMatch{Ambiguous: true}
		}
	}

	// Every candidate has a version and none of them is on the installed
	// line: the catalog may be ahead of the installed copy - a plugin that
	// moved from 0.x to 1.0 numbers its releases differently. The line
	// nearest to the installed one is the closest thing to it; two lines
	// equally near tell nothing apart.
	nearest, ok := nearestLine(candidates, line)
	if !ok {
		return catalogMatch{Ambiguous: true}
	}
	return catalogMatch{Entry: nearest}
}

// nearestLine returns the candidate whose release line lies nearest to the
// installed one. The bool is false when two different plugins lie equally
// near: then the catalog cannot say which one is installed.
func nearestLine(candidates []*CatalogEntry, line int) (*CatalogEntry, bool) {
	var best *CatalogEntry
	bestDistance := 0
	for _, entry := range candidates {
		entryLine, ok := versionLine(entryVersion(entry))
		if !ok {
			continue
		}
		distance := entryLine - line
		if distance < 0 {
			distance = -distance
		}
		if best == nil || distance < bestDistance {
			best, bestDistance = entry, distance
			continue
		}
		if distance == bestDistance && pluginHomepage(entry) != pluginHomepage(best) {
			return nil, false
		}
	}
	return best, best != nil
}

// severalPlugins reports whether the candidates are different plugins rather
// than two releases of one.
//
// The homepage is the plugin's own address, so two entries that share it are
// one plugin in two rows. An entry without a homepage cannot be placed, and
// counts with every other entry that cannot be placed either: the catalog
// cannot tell those apart either, and guessing would be worse than taking
// the newest of them.
func severalPlugins(candidates []*CatalogEntry) bool {
	home := pluginHomepage(candidates[0])
	for _, entry := range candidates[1:] {
		if pluginHomepage(entry) != home {
			return true
		}
	}
	return false
}

// pluginHomepage is the address of the plugin an entry is about.
func pluginHomepage(entry *CatalogEntry) string {
	return strings.ToLower(strings.TrimSuffix(strings.TrimSpace(entry.Homepage), "/"))
}

// trustedVersion reports whether the installed version is one the client may
// rely on. A version the plugin states itself — in its PE resource, in its
// own pluginst.inf, or set by the user in the database — is a number about
// this build. A number scraped out of a readme is prose, and an unknown
// version is no number at all.
func trustedVersion(local domain.Plugin) bool {
	switch local.Confidence {
	case domain.ConfidenceExact, domain.ConfidenceINF, domain.ConfidenceManual:
		return isResolvableVersion(local.InstalledVersion)
	}
	return false
}

// versionLine is the release line a version belongs to: its first number.
// "0.2.4" and "0.3.0" are one line, "3.8.2" is another.
//
// Two plugins that install the same file number their releases on their own,
// so the line is what tells them apart: the WebView2 MDView counts from zero
// and the MSHTML one from three.
func versionLine(v string) (int, bool) {
	numbers := parseVersionParts(cleanVersionString(v))
	if len(numbers) == 0 {
		return 0, false
	}
	return numbers[0], true
}

// entryVersion is the version the catalog resolved for the entry.
func entryVersion(entry *CatalogEntry) string {
	if entry == nil || entry.Resolved == nil {
		return ""
	}
	return entry.Resolved.Version
}

// newestOf returns the candidate that carries the newest release.
func newestOf(candidates []*CatalogEntry) *CatalogEntry {
	best := candidates[0]
	for _, entry := range candidates[1:] {
		if newerRelease(entry, best) {
			best = entry
		}
	}
	return best
}

// servesThisBuild reports whether the entry can serve the installed build.
//
// An entry whose archive carries both builds can serve either one, so it is
// always offered. An entry with one build can only serve the build the
// installed file is: the two are told apart by the bit marker in the file
// name — on the extension ("asnview.wlx64") or on the stem ("archview64.wlx").
//
// The marker is read the same way on both sides, so an entry about a plugin
// whose own name ends in 64 ("D64") agrees with itself and is not thrown away.
func servesThisBuild(entry *CatalogEntry, localPath string) bool {
	if entry == nil {
		return false
	}
	arch := strings.ToLower(strings.TrimSpace(entry.Arch))
	servesBoth := arch == "" || arch == "any" ||
		(strings.Contains(arch, "32") && strings.Contains(arch, "64"))
	if servesBoth {
		return true
	}
	if len(entry.Match.Filenames) == 0 {
		return true
	}
	return bitMarker(entry.Match.Filenames[0]) == bitMarker(localPath)
}

// bitMarker tells the 64-bit build of a plugin from the 32-bit one by the name
// the file carries.
func bitMarker(name string) string {
	lower := strings.ToLower(filepath.Base(name))
	for _, ext := range pluginExtensions {
		if strings.HasSuffix(lower, ext) && strings.HasSuffix(ext, "64") {
			return "64"
		}
	}
	stem := pluginKey(lower)
	if len(stem) > 2 && strings.HasSuffix(stem, "64") {
		return "64"
	}
	return "32"
}

// newerRelease reports whether entry carries a newer release than the one
// already found. An entry without a resolved version never wins against one
// that has one.
func newerRelease(entry, other *CatalogEntry) bool {
	version := entryVersion(entry)
	otherVersion := entryVersion(other)
	switch {
	case version == "":
		return false
	case otherVersion == "":
		return true
	}
	return CompareVersions(version, otherVersion) > 0
}
