package config

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

// DataDirEnv pins the data directory. Tests use it to keep their files in their
// own temp dir instead of inheriting whatever install happens to be running.
const DataDirEnv = "TOTALPLUG_DATA_DIR"

// appFolder is the data folder's name, both next to the executable and inside
// the per-user config dir (%APPDATA%\totalplug on Windows). It is lowercase
// because that is the executable's name and the window title's name; the
// display name TotalPlug is what prose uses.
const appFolder = "totalplug"

// webviewFolder holds the WebView2 profile inside the data folder.
const webviewFolder = "webview2"

// cacheFolder holds the downloaded snapshots inside the data folder: they are
// re-fetchable, so a user who wants to clean up can delete the folder whole
// without touching the database or the overrides next to it.
const cacheFolder = "cache"

// backupFolder holds the copies of plugin directories taken before an update.
// They live here rather than beside the plugin because Total Commander's tree is
// the user's, and a folder of dead copies left in it is pollution he has to
// notice and clean up by hand.
const backupFolder = "backups"

// ScratchEnv pins the throwaway folder. Tests set it to keep a downloaded
// package out of the machine's real %TEMP%.
const ScratchEnv = "TOTALPLUG_SCRATCH_DIR"

// SettingsFileName is the small file that has to be readable before a window
// exists. Its name is exported because both the GUI, which writes it, and the
// CLI, which reads the backup policy out of it, have to look at the same path.
const SettingsFileName = "settings.json"

// runPrefix names a run's folder inside the scratch root.
const runPrefix = "run-"

var (
	discoverOnce sync.Once
	discovered   string
)

// DataDir returns the directory this run stores its files in. Resolution order:
//
//  1. TOTALPLUG_DATA_DIR, when set,
//  2. a totalplug folder next to the executable, if that folder accepts writes —
//     this is the portable install: copying the app's folder moves the whole
//     setup with it,
//  3. %APPDATA%\totalplug otherwise, for read-only locations (Program Files, a
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

// CacheDir returns the folder the downloaded snapshots live in. It is created on
// demand so every caller that names a cache file gets a directory it can
// actually write into.
func CacheDir() (string, error) {
	dir := filepath.Join(DataDir(), cacheFolder)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("create cache dir %s: %w", dir, err)
	}
	return dir, nil
}

// BackupSpace returns the folder the pre-update copies of plugin directories
// live in. Like the cache folder it is created on demand, and a user who wants
// to reclaim the space can delete the whole folder: an update simply makes new
// copies in it.
func BackupSpace() (string, error) {
	dir := BackupSpacePath()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("create backup space %s: %w", dir, err)
	}
	return dir, nil
}

// BackupSpacePath names the backup folder without creating it, for the settings
// panel that has to show where copies go without having made one yet.
func BackupSpacePath() string {
	return filepath.Join(DataDir(), backupFolder)
}

// SettingsPath names the settings file without creating anything, so a reader
// can say what is missing instead of making an empty data folder on the way.
func SettingsPath() string {
	return filepath.Join(DataDir(), SettingsFileName)
}

// ScratchRoot returns the one %TEMP% folder this app writes outside its data
// folder: %TEMP%\totalplug. Naming it after the app is what keeps a run that was
// killed mid-download from leaving a scattered `totalplug_update_*` pattern
// behind — there is a single folder to look at, and one sweep clears it.
func ScratchRoot() (string, error) {
	dir := scratchRootPath()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("create scratch root %s: %w", dir, err)
	}
	return dir, nil
}

// scratchRootPath names the root without creating it, for the paths that only
// have to remove something from there.
func scratchRootPath() string {
	if dir := strings.TrimSpace(os.Getenv(ScratchEnv)); dir != "" {
		return dir
	}
	return filepath.Join(os.TempDir(), appFolder)
}

// RunScratch returns the folder this process works in, named after its pid so
// every update of a run shares one folder and two runs never write into each
// other's. An --update-all applies plugin after plugin, and each of them clears
// the folder rather than creating a new one.
func RunScratch() (string, error) {
	root, err := ScratchRoot()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(root, runDirName())
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("create run scratch %s: %w", dir, err)
	}
	// MkdirAll leaves an existing folder's timestamp where it was, and SweepScratch
	// measures a run's age from exactly that timestamp. Writing it here — on every
	// call, which means at the start of every update — is what keeps a live run's
	// package out of reach of a sweep from another process.
	now := time.Now()
	if err := os.Chtimes(dir, now, now); err != nil {
		return "", fmt.Errorf("refresh run scratch %s: %w", dir, err)
	}
	return dir, nil
}

// ReleaseRunScratch takes the run's folder away on the way out, and the scratch
// root with it once no other run has left a folder behind: a run that finished
// normally should leave nothing at all in %TEMP%.
func ReleaseRunScratch() {
	root := scratchRootPath()
	_ = os.RemoveAll(filepath.Join(root, runDirName()))
	if rest, err := os.ReadDir(root); err == nil && len(rest) == 0 {
		_ = os.Remove(root)
	}
}

// SweepScratch removes run folders left behind by runs that died, keeping
// anything written to within maxAge. What refreshes that timestamp is
// RunScratch, which every update calls at its start, so the age is measured from
// the last item the run began rather than from when the process was made. A live
// run therefore loses its scratch only if some other run goes an hour without
// _starting_ an item — one very long download on a batch of one.
// It reads the root without creating it: a sweep has nothing to make. Returns how
// many folders went.
func SweepScratch(maxAge time.Duration) int {
	root := scratchRootPath()
	entries, err := os.ReadDir(root)
	if err != nil {
		return 0
	}
	removed := 0
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), runPrefix) {
			continue
		}
		info, err := entry.Info()
		if err != nil || time.Since(info.ModTime()) < maxAge {
			continue
		}
		if os.RemoveAll(filepath.Join(root, entry.Name())) == nil {
			removed++
		}
	}
	return removed
}

// runDirName is this process's folder name inside the scratch root.
func runDirName() string {
	return fmt.Sprintf("%s%d", runPrefix, os.Getpid())
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

// perUserDir is the %APPDATA%\totalplug path. It creates nothing: the portable
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

// importableFiles are the files a first portable run copies out of the per-user
// folder — exactly the ones this version reads back, and nothing else. A cache,
// a quarantined `.corrupt` file or something an abandoned feature left behind is
// not carried into a folder the user will open and try to understand.
//
// The names live here rather than with their owners because config cannot import
// the packages that own them; a renamed file therefore has to be renamed in this
// list too, or the portable run starts without it.
var importableFiles = []string{
	"installed_plugins.json",
	"user_overrides.json",
	"update_history.json",
	"settings.json",
}

// importPerUserData fills a brand-new portable folder from the per-user one, so
// the first portable run does not look like a fresh install. It copies the files
// it knows and leaves the originals alone: a run from another location still
// reads them. An empty folder is what makes this safe to repeat — once the app
// has written anything here, the folder owns its data.
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
		if entry.IsDir() || !slices.Contains(importableFiles, entry.Name()) {
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
