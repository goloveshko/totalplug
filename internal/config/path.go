package config

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// DataDirEnv pins the data directory. Tests use it to keep their files in their
// own temp dir instead of inheriting whatever install happens to be running.
const DataDirEnv = "TOTALPLUG_DATA_DIR"

// appFolder is the data folder's name, both next to the executable and inside
// the per-user config dir (%APPDATA%\TotalPlug on Windows).
const appFolder = "TotalPlug"

// webviewFolder holds the WebView2 profile inside the data folder.
const webviewFolder = "webview2"

var (
	discoverOnce sync.Once
	discovered   string
)

// DataDir returns the directory this run stores its files in. Resolution order:
//
//  1. TOTALPLUG_DATA_DIR, when set,
//  2. a TotalPlug folder next to the executable, if that folder accepts writes —
//     this is the portable install: copying the app's folder moves the whole
//     setup with it,
//  3. %APPDATA%\TotalPlug otherwise, for read-only locations (Program Files, a
//     write-protected share, a mounted image).
//
// The answer is remembered for the life of the process, so a file read from one
// directory is never written back into another.
func DataDir() string {
	if dir := strings.TrimSpace(os.Getenv(DataDirEnv)); dir != "" {
		_ = os.MkdirAll(dir, 0755)
		return dir
	}
	discoverOnce.Do(func() { discovered = resolveDataDir() })
	return discovered
}

// WebviewDataDir returns the folder the WebView2 profile lives in: its
// localStorage holds the frontend's theme, language and font scale, so keeping
// it inside the data folder is what makes a portable install really portable.
// The folder is created here because Wails aborts with a message box when handed
// a path it cannot use.
func WebviewDataDir() (string, error) {
	dir := filepath.Join(DataDir(), webviewFolder)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("create webview data dir %s: %w", dir, err)
	}
	return dir, nil
}

func resolveDataDir() string {
	exePath, err := os.Executable()
	if err != nil {
		return dataDirIn("")
	}
	return dataDirIn(exeDirFor(exePath))
}

// exeDirFor maps a running binary to the folder that should keep its data.
// `go run` and `go test` compile into a throwaway go-build folder, and data
// written there is deleted with the build cache, so those runs point at the
// working tree's bin/ — the folder the same code is really installed in.
func exeDirFor(exePath string) string {
	dir := filepath.Dir(exePath)
	if !strings.Contains(dir, "go-build") {
		return dir
	}
	if cwd, err := os.Getwd(); err == nil {
		return filepath.Join(cwd, "bin")
	}
	return dir
}

// dataDirIn chooses between a portable folder under exeDir and the per-user
// folder. It takes the directory as an argument so a test can hand it one it
// owns instead of the real executable's location.
func dataDirIn(exeDir string) string {
	if exeDir != "" && isWritable(exeDir) {
		dir := filepath.Join(exeDir, appFolder)
		if err := os.MkdirAll(dir, 0755); err == nil {
			importPerUserData(dir)
			return dir
		}
	}
	if dir := userDir(); dir != "" {
		return dir
	}
	return exeDir
}

// isWritable probes the folder instead of trusting its ACL: what matters is
// whether this user can create a file in it right now.
func isWritable(dir string) bool {
	probe := filepath.Join(dir, ".write_test")
	if err := os.WriteFile(probe, []byte("ok"), 0600); err != nil {
		return false
	}
	_ = os.Remove(probe)
	return true
}

// perUserDir is the %APPDATA%\TotalPlug path. It creates nothing: the portable
// branch has to look at this folder without leaving an empty one behind.
func perUserDir() string {
	base, err := os.UserConfigDir()
	if err != nil || base == "" {
		return ""
	}
	return filepath.Join(base, appFolder)
}

// userDir is the per-user data folder, created on demand.
func userDir() string {
	dir := perUserDir()
	if dir == "" {
		return ""
	}
	_ = os.MkdirAll(dir, 0755)
	return dir
}

// importPerUserData fills a brand-new portable folder from the per-user one, so
// the first portable run does not look like a fresh install. It copies files it
// finds and leaves the originals alone: a run from another location still reads
// them. An empty folder is what makes this safe to repeat — once the app has
// written anything here, the folder owns its data.
func importPerUserData(dir string) {
	legacy := perUserDir()
	if legacy == "" || legacy == dir {
		return
	}
	if current, err := os.ReadDir(dir); err != nil || len(current) > 0 {
		return
	}
	saved, err := os.ReadDir(legacy)
	if err != nil {
		return
	}
	for _, entry := range saved {
		if entry.IsDir() {
			continue
		}
		_ = copyFile(filepath.Join(legacy, entry.Name()), filepath.Join(dir, entry.Name()))
	}
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}
