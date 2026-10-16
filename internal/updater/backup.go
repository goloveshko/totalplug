package updater

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/goloveshko/totalplug/internal/config"
)

// backupSuffix separates a plugin directory name from its backup timestamp.
const backupSuffix = ".bak_"

// namelessSpace stands in for a grouping folder that has no name of its own —
// a plugin directory sitting straight at a drive root, which is what a relative
// path in a wincmd.ini can produce.
const namelessSpace = "root"

// illegalNameChars are the characters a Windows folder name cannot hold. The one
// this code meets in practice is the colon of a drive root ("C:"), which is the
// namespace a plugin folder directly on a drive would get.
const illegalNameChars = `<>:"/\|?*`

// BackupPlugin copies the plugin directory into the app's backup space and
// returns the copy's path. The original is left in place so the update can be
// applied on top of it and the copy stays available for manual recovery.
//
// Before copying, it carries over any "<name>.bak_<unix>" folders this tool used
// to leave beside the plugin, so updating a plugin is also what cleans the Total
// Commander tree it lives in.
func (u *Updater) BackupPlugin(pluginDir string) (string, error) {
	space, err := prepareBackupSpace(pluginDir)
	if err != nil {
		return "", err
	}
	relocateLegacyBackups(pluginDir, space)

	backupDir := filepath.Join(space, filepath.Base(pluginDir)+backupSuffix+strconv.FormatInt(time.Now().Unix(), 10))
	if err := copyTree(pluginDir, backupDir); err != nil {
		return "", fmt.Errorf("creating backup of %s: %w", pluginDir, err)
	}
	return backupDir, nil
}

// backupNamespace names the folder of the shared backup space that holds one
// plugin's copies: its own directory name inside the folder Total Commander
// groups it under. The grouping level is what keeps wfx\CudaLister and
// wlx\CudaLister from sharing a folder, because the plugin id that would say so
// ("WFX:c:/…") contains a colon and cannot be a Windows name.
func backupNamespace(pluginDir string) (string, error) {
	space, err := config.BackupSpace()
	if err != nil {
		return "", err
	}
	grouping := filepath.Base(filepath.Dir(pluginDir))
	if grouping == "" || grouping == "." || grouping == string(filepath.Separator) {
		grouping = namelessSpace
	}
	return filepath.Join(space, sanitizeName(grouping)), nil
}

// prepareBackupSpace returns the namespace folder, created so the caller can
// write into it right away.
func prepareBackupSpace(pluginDir string) (string, error) {
	dir, err := backupNamespace(pluginDir)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("create backup space %s: %w", dir, err)
	}
	return dir, nil
}

func sanitizeName(name string) string {
	return strings.Map(func(r rune) rune {
		if strings.ContainsRune(illegalNameChars, r) || r < 32 {
			return '_'
		}
		return r
	}, name)
}

// legacyBackups lists the "<pluginDir>.bak_<unix>" folders left beside a plugin
// by the version that wrote them there. Names with an unparseable timestamp are
// skipped: they were not made by this tool, so nothing here claims to know what
// they hold.
func legacyBackups(pluginDir string) []string {
	parent := filepath.Dir(pluginDir)
	entries, err := os.ReadDir(parent)
	if err != nil {
		return nil
	}
	prefix := filepath.Base(pluginDir) + backupSuffix
	var found []string
	for _, e := range entries {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), prefix) {
			continue
		}
		if _, err := strconv.ParseInt(strings.TrimPrefix(e.Name(), prefix), 10, 64); err != nil {
			continue
		}
		found = append(found, filepath.Join(parent, e.Name()))
	}
	return found
}

// relocateLegacyBackups moves the folders left in the Total Commander tree into
// the plugin's namespace, so retention has one place to clean. A folder that
// cannot be moved stays where it is and says so: the update itself is already
// running by then, and junk in a data folder is not a reason to fail it.
func relocateLegacyBackups(pluginDir, space string) {
	for _, old := range legacyBackups(pluginDir) {
		target := filepath.Join(space, filepath.Base(old))
		if err := os.Rename(old, target); err == nil {
			continue
		}
		// Another volume, or a data folder on a share: copy and only then take
		// the original away, so a failed copy cannot lose the only good copy.
		if err := copyTree(old, target); err != nil {
			fmt.Printf("[!] Could not move the old backup %s into %s: %v\n", old, space, err)
			continue
		}
		if err := os.RemoveAll(old); err != nil {
			fmt.Printf("[!] Left the old backup %s in place after copying it: %v\n", old, err)
		}
	}
}

// pruneBackupSpace applies the retention policy to one plugin's namespace
// folder. A copy is dropped only when it is both past the newest KeepNewest and
// older than MaxAgeDays, so the floor protects the recent history of a plugin
// nobody updates often and the age limit decides what happens to the rest. With
// no age limit nothing is dropped at all: the count alone is never a reason to
// delete a copy that is still young.
// Entries this tool did not write (a name whose timestamp does not parse) are
// left alone. It returns the paths that were removed.
func pruneBackupSpace(pluginDir string, policy config.BackupPolicy) ([]string, error) {
	maxAgeDays, limited := policy.AgeLimit()
	if !limited {
		return nil, nil
	}
	space, err := backupNamespace(pluginDir)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(space)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading backup space %s: %w", space, err)
	}

	type backup struct {
		path string
		ts   int64
	}

	prefix := filepath.Base(pluginDir) + backupSuffix
	var backups []backup
	for _, e := range entries {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), prefix) {
			continue
		}
		ts, err := strconv.ParseInt(strings.TrimPrefix(e.Name(), prefix), 10, 64)
		if err != nil {
			continue
		}
		backups = append(backups, backup{path: filepath.Join(space, e.Name()), ts: ts})
	}

	sort.Slice(backups, func(i, j int) bool { return backups[i].ts > backups[j].ts })
	cutoff := time.Now().AddDate(0, 0, -maxAgeDays).Unix()

	var removed []string
	for i, b := range backups {
		if i < policy.KeepNewest || b.ts > cutoff {
			continue
		}
		if err := os.RemoveAll(b.path); err != nil {
			return removed, fmt.Errorf("removing old backup %s: %w", b.path, err)
		}
		removed = append(removed, b.path)
	}
	return removed, nil
}
