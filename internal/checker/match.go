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

// matchCatalogEntry finds the catalog entry that describes the installed
// plugin, or nil when the catalog does not know it.
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
func matchCatalogEntry(local domain.Plugin, catalog []CatalogEntry) *CatalogEntry {
	// Tier 1: the file on disk. It is the plugin's identity, and it keeps
	// the marker that tells the two builds apart.
	localFileKey := pluginKey(filepath.Base(local.Path))
	if localFileKey != "" {
		if entry := bestMatch(catalog, local.Path, func(e *CatalogEntry) bool {
			for _, fn := range e.Match.Filenames {
				if pluginKey(fn) == localFileKey {
					return true
				}
			}
			return false
		}); entry != nil {
			return entry
		}
	}

	// Tier 2: the name the ini knows the plugin by, or one of the entry's
	// own aliases.
	localNameKey := pluginKey(local.Name)
	if localNameKey != "" {
		if entry := bestMatch(catalog, local.Path, func(e *CatalogEntry) bool {
			if pluginKey(e.Name) == localNameKey {
				return true
			}
			for _, alias := range e.Match.Aliases {
				if pluginKey(alias) == localNameKey {
					return true
				}
			}
			return false
		}); entry != nil {
			return entry
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
		return nil
	}
	return bestMatch(catalog, local.Path, func(e *CatalogEntry) bool {
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

// bestMatch returns the entry among those the test accepts that carries the
// newest release. The catalog holds several entries for the same plugin — the
// same file under names that differ in case or in a version tag — and taking
// the first of them in catalog order hid the newest release behind the
// oldest.
func bestMatch(catalog []CatalogEntry, localPath string, accept func(*CatalogEntry) bool) *CatalogEntry {
	var best *CatalogEntry
	for i := range catalog {
		entry := &catalog[i]
		if !accept(entry) {
			continue
		}
		if !servesThisBuild(entry, localPath) {
			continue
		}
		if best == nil || newerRelease(entry, best) {
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
	version := ""
	if entry.Resolved != nil {
		version = entry.Resolved.Version
	}
	otherVersion := ""
	if other.Resolved != nil {
		otherVersion = other.Resolved.Version
	}
	switch {
	case version == "":
		return false
	case otherVersion == "":
		return true
	}
	return CompareVersions(version, otherVersion) > 0
}
