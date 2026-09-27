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

var (
	modversion                  = windows.NewLazySystemDLL("version.dll")
	procGetFileVersionInfoSizeW = modversion.NewProc("GetFileVersionInfoSizeW")
	procGetFileVersionInfoW     = modversion.NewProc("GetFileVersionInfoW")
	procVerQueryValueW          = modversion.NewProc("VerQueryValueW")

	verRegexes = []*regexp.Regexp{
		regexp.MustCompile(`(?i)(?:version|ver\b|release)\s+:?\s*v?([0-9]+(?:\.[0-9]+)+)`),
		regexp.MustCompile(`(?i)v([0-9]+\.[0-9]+(?:\.[0-9]+)?)`),
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
		count := lenInfo / 4
		translations := (*[1 << 16]LANGANDCODEPAGE)(unsafe.Pointer(lpBuffer))[:count:count]
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

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext == ".dll" {
			dllPath := filepath.Join(dir, entry.Name())
			if ver, err := getFileVersion(dllPath); err == nil && ver != "" {
				return ver
			}
		}
	}
	return ""
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
