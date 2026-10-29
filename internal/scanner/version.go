package scanner

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

type VS_FIXEDFILEINFO struct {
	Signature        uint32
	StrucVersion     uint32
	FileVersionMS    uint32
	FileVersionLS    uint32
	ProductVersionMS uint32
	ProductVersionLS uint32
	FileFlagsMask    uint32
	FileFlags        uint32
	FileOS           uint32
	FileType         uint32
	FileSubtype      uint32
	FileDateMS       uint32
	FileDateLS       uint32
}

type LANGANDCODEPAGE struct {
	Language uint16
	CodePage uint16
}

// maxTranslations caps how many entries the translation array is read through:
// the fixed-size slice type that wraps the API's pointer cannot address more,
// and a real plugin never carries more than a handful.
const maxTranslations = 1 << 16

var (
	modversion                  = windows.NewLazySystemDLL("version.dll")
	procGetFileVersionInfoSizeW = modversion.NewProc("GetFileVersionInfoSizeW")
	procGetFileVersionInfoW     = modversion.NewProc("GetFileVersionInfoW")
	procVerQueryValueW          = modversion.NewProc("VerQueryValueW")

	verRegexes = []*regexp.Regexp{
		// "Version 1.2", "version=1.2" and "Version: 1.2" all say the same
		// thing. The separator is optional and the keyword may sit right
		// against the number, which is how an INI file writes it — a rule that
		// demanded whitespace after the keyword missed every pluginst.inf.
		regexp.MustCompile(`(?i)\b(?:version|release)\b\s*[:=]?\s*v?([0-9]+(?:\.[0-9]+){1,3})`),
		regexp.MustCompile(`(?i)\bver\.?\b\s*[:=]?\s*v?([0-9]+(?:\.[0-9]+){1,3})`),
		// A bare "v1.2.3.4" in a sentence. The word boundary keeps it out of
		// words that merely end in a v ("TLSv1.2").
		regexp.MustCompile(`(?i)\bv([0-9]+(?:\.[0-9]+){1,3})`),
	}
)

func queryVersionString(buf []byte, langCodePage string, key string) string {
	subBlock, err := windows.UTF16PtrFromString(fmt.Sprintf(`\StringFileInfo\%s\%s`, langCodePage, key))
	if err != nil {
		return ""
	}

	var strPtr *uint16
	var strLen uint32
	ret, _, _ := procVerQueryValueW.Call(
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(unsafe.Pointer(subBlock)),
		uintptr(unsafe.Pointer(&strPtr)),
		uintptr(unsafe.Pointer(&strLen)),
	)

	if ret != 0 && strPtr != nil && strLen > 0 {
		return strings.TrimSpace(windows.UTF16PtrToString(strPtr))
	}
	return ""
}

func getVersionFromStrings(buf []byte) string {
	subBlock, _ := windows.UTF16PtrFromString(`\VarFileInfo\Translation`)
	var lpBuffer *LANGANDCODEPAGE
	var lenInfo uint32

	ret, _, _ := procVerQueryValueW.Call(
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(unsafe.Pointer(subBlock)),
		uintptr(unsafe.Pointer(&lpBuffer)),
		uintptr(unsafe.Pointer(&lenInfo)),
	)

	var langCodePages []string
	if ret != 0 && lpBuffer != nil && lenInfo >= 4 {
		// lenInfo is the byte size of the translation array; the typed slice
		// below can only address maxTranslations entries, so a resource that
		// claims more than that — a crafted or corrupt DLL — must be clamped
		// instead of slicing past the type and panicking the whole scan.
		count := lenInfo / 4
		if count > maxTranslations {
			count = maxTranslations
		}
		translations := (*[maxTranslations]LANGANDCODEPAGE)(unsafe.Pointer(lpBuffer))[:count:count]
		for _, t := range translations {
			langCodePages = append(langCodePages, fmt.Sprintf("%04x%04x", t.Language, t.CodePage))
		}
	}

	langCodePages = append(langCodePages, "040904b0", "040904e4", "000004b0", "04090000", "041904b0")

	for _, lcp := range langCodePages {
		if ver := queryVersionString(buf, lcp, "FileVersion"); ver != "" {
			return ver
		}
		if ver := queryVersionString(buf, lcp, "ProductVersion"); ver != "" {
			return ver
		}
	}

	return ""
}

func getFileVersion(filePath string) (string, error) {
	pathPtr, err := windows.UTF16PtrFromString(filePath)
	if err != nil {
		return "", err
	}

	var dummy uint32
	size, _, _ := procGetFileVersionInfoSizeW.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		uintptr(unsafe.Pointer(&dummy)),
	)
	if size == 0 {
		return "", fmt.Errorf("version resource missing")
	}

	buf := make([]byte, size)
	ret, _, _ := procGetFileVersionInfoW.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		0,
		size,
		uintptr(unsafe.Pointer(&buf[0])),
	)
	if ret == 0 {
		return "", fmt.Errorf("failed to read version resource")
	}

	var fixedInfo *VS_FIXEDFILEINFO
	var lenInfo uint32
	rootPtr, _ := windows.UTF16PtrFromString(`\`)

	ret, _, _ = procVerQueryValueW.Call(
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(unsafe.Pointer(rootPtr)),
		uintptr(unsafe.Pointer(&fixedInfo)),
		uintptr(unsafe.Pointer(&lenInfo)),
	)

	if ret != 0 && fixedInfo != nil {
		major := fixedInfo.FileVersionMS >> 16
		minor := fixedInfo.FileVersionMS & 0xFFFF
		patch := fixedInfo.FileVersionLS >> 16
		build := fixedInfo.FileVersionLS & 0xFFFF

		if major > 0 || minor > 0 || patch > 0 {
			if build > 0 {
				return fmt.Sprintf("%d.%d.%d.%d", major, minor, patch, build), nil
			}
			return fmt.Sprintf("%d.%d.%d", major, minor, patch), nil
		}
	}

	if strVer := getVersionFromStrings(buf); strVer != "" {
		return strVer, nil
	}

	return "", fmt.Errorf("no PE version found")
}

func getFileVersionFromCompanionDLLs(pluginPath string) string {
	dir := filepath.Dir(pluginPath)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		names = append(names, entry.Name())
	}

	chosen := pickCompanionDLL(filepath.Base(pluginPath), names)
	if chosen == "" {
		return ""
	}
	ver, err := getFileVersion(filepath.Join(dir, chosen))
	if err != nil {
		return ""
	}
	return ver
}

// pickCompanionDLL chooses the helper DLL that belongs to the plugin.
//
// Only a DLL that belongs to the plugin counts. A plugin folder can hold a
// shared runtime too, and a plugin that reported the version of the SQLite
// library beside it was reporting a number that has nothing to do with it.
// The names are read without their digits, so "MediaInfo64.dll" belongs to
// "TCMediaInfo.wdx" and to "MediaInfo.wdx", and "sqlite3.dll" belongs to
// nothing.
func pickCompanionDLL(pluginFile string, names []string) string {
	pluginStem := nameStem(pluginFile)
	if pluginStem == "" {
		return ""
	}

	best := ""
	bestStem := ""
	bestScore := -1

	for _, name := range names {
		if strings.ToLower(filepath.Ext(name)) != ".dll" {
			continue
		}
		stem := nameStem(name)
		if !belongsTo(stem, pluginStem) {
			continue
		}
		// The helper whose name belongs to the plugin most closely is the one
		// that belongs to it: a folder may hold both "MediaInfo64.dll" and
		// "MediaInfoTools.dll" next to a plugin called "MediaInfo".
		if bestScore >= 0 && !betterHelper(stem, bestStem, pluginStem) {
			continue
		}
		best, bestStem, bestScore = name, stem, commonPrefixLen(stem, pluginStem)
	}

	return best
}

// betterHelper reports whether the helper a belongs to the plugin more closely
// than the helper b: the one whose name shares more with the plugin's wins,
// and the one whose name is closest in length wins a tie — the exact match is
// the plugin's own library, a longer name is a library it merely starts with.
func betterHelper(a, b, pluginStem string) bool {
	prefixA := commonPrefixLen(a, pluginStem)
	prefixB := commonPrefixLen(b, pluginStem)
	if prefixA != prefixB {
		return prefixA > prefixB
	}
	return abs(len(a)-len(pluginStem)) < abs(len(b)-len(pluginStem))
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// nameStem is a file name without its extension and without the digits that
// mark a build: "MediaInfo64.dll", "multiarc2012.wcx" and "killcopywcx2.wcx"
// all become a stem the plugin's own name can be compared with.
func nameStem(name string) string {
	stem := strings.ToLower(name)
	if ext := filepath.Ext(stem); ext != "" {
		stem = strings.TrimSuffix(stem, ext)
	}
	for len(stem) > 0 && stem[len(stem)-1] >= '0' && stem[len(stem)-1] <= '9' {
		stem = stem[:len(stem)-1]
	}
	return stem
}

// belongsTo reports whether a helper's name belongs to the plugin's name: the
// shorter of the two must be the beginning or the end of the longer, so
// "mediainfo" belongs to "tcmediainfo", "isowriter" to "iso" and "killcopy"
// to "killcopywcx" — while "sqlite" belongs to nothing called "foo".
func belongsTo(helper, plugin string) bool {
	if helper == "" || plugin == "" {
		return false
	}
	if helper == plugin {
		return true
	}
	shorter, longer := helper, plugin
	if len(shorter) > len(longer) {
		shorter, longer = longer, shorter
	}
	if len(shorter) < 3 {
		return false
	}
	return strings.HasPrefix(longer, shorter) || strings.HasSuffix(longer, shorter)
}

// commonPrefixLen counts the characters two names share from the start.
func commonPrefixLen(a, b string) int {
	n := 0
	for n < len(a) && n < len(b) && a[n] == b[n] {
		n++
	}
	return n
}

func scanFileForVersion(filePath string) string {
	file, err := os.Open(filePath)
	if err != nil {
		return ""
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lineCount := 0
	for scanner.Scan() && lineCount < 40 {
		line := scanner.Text()
		for _, re := range verRegexes {
			matches := re.FindStringSubmatch(line)
			if len(matches) > 1 {
				return matches[1]
			}
		}
		lineCount++
	}
	return ""
}

func getVersionFromReadme(pluginPath string) string {
	baseDir := filepath.Dir(pluginPath)
	searchDirs := []string{
		baseDir,
		filepath.Join(baseDir, "readme"),
		filepath.Join(baseDir, "doc"),
		filepath.Join(baseDir, "docs"),
		filepath.Join(baseDir, "help"),
	}

	candidates := []string{
		"readme.txt", "readme.htm", "readme.html", "read_me.txt",
		"pluginst.inf", "history.txt", "changelog.txt", "version.txt",
	}

	for _, dir := range searchDirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			entryLower := strings.ToLower(entry.Name())

			for _, cand := range candidates {
				if entryLower == cand {
					filePath := filepath.Join(dir, entry.Name())
					if ver := scanFileForVersion(filePath); ver != "" {
						return ver
					}
				}
			}
		}
	}

	return ""
}
