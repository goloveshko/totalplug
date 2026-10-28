package scanner

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// pluginstInfName is the file Total Commander reads when it installs a plugin:
// it names the plugin, the file it lives in and the version the plugin says it
// is. It is the one place a plugin states its own version in a structured
// field, so it is worth reading on its own instead of scraping it for a word
// that looks like a version.
const pluginstInfName = "pluginst.inf"

// versionFromPluginstInf returns the version the plugin declares in the
// pluginst.inf next to it, or "" when that file is missing, unreadable, or says
// nothing about a version.
//
// The file is an INI whose "[plugininstall]" section carries "key=value" lines,
// the version under "version". Only that key is read: the same file also holds
// a free-form description, and a number taken out of a sentence is a guess.
func versionFromPluginstInf(pluginPath string) string {
	if pluginPath == "" {
		return ""
	}

	f, err := os.Open(filepath.Join(filepath.Dir(pluginPath), pluginstInfName))
	if err != nil {
		return ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok || !strings.EqualFold(strings.TrimSpace(key), "version") {
			continue
		}
		if version := normalizeDeclaredVersion(value); version != "" {
			return version
		}
	}

	return ""
}

// normalizeDeclaredVersion trims a declared version down to the part a
// comparison can use: quotes and trailing text off, and the leading numeric
// run kept. "1.2.3.4" stays whole, "1.2b" becomes "1.2", and "v1.2" becomes
// "1.2" — a declared version that cannot be read at all is refused rather than
// passed on as something the comparison would have to guess about.
func normalizeDeclaredVersion(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, `"'`)
	value = strings.TrimSpace(value)

	// A trailing comment is not part of the version.
	if idx := strings.IndexAny(value, " ;#"); idx >= 0 {
		value = strings.TrimSpace(value[:idx])
	}
	value = strings.TrimPrefix(value, "v")
	value = strings.TrimPrefix(value, "V")
	value = strings.TrimSpace(value)

	// Keep the leading numeric run: everything from the first character that is
	// neither a digit nor a dot ends the version.
	end := 0
	for end < len(value) {
		c := value[end]
		if (c >= '0' && c <= '9') || c == '.' {
			end++
			continue
		}
		break
	}

	version := strings.Trim(value[:end], ".")
	if version == "" || !strings.Contains(version, ".") {
		// A bare number is not a version the comparison can weigh against a
		// dotted one, and a declared "1" next to a catalog "1.0.8" would read as
		// outdated forever.
		return ""
	}
	return version
}
