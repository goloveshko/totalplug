package config

import (
	"encoding/json"
	"fmt"
	"os"
)

// A backup policy belongs to config rather than to the GUI because the CLI has
// to honour it too: `totalplug --update-all` prunes the same folders the window
// writes into. settings.json is therefore read by two programs, and one struct
// here is what keeps their idea of the file from drifting apart.
//
// The file itself is written by the GUI, which also keeps the window geometry
// beside these keys.

// Retention defaults. KeepNewest matches the count the updater has always left
// behind; MaxAgeDays is new, and is the answer to "how long does a dead copy
// stay in my data folder".
const (
	DefaultKeepNewest = 3
	DefaultMaxAgeDays = 30
)

// BackupPolicy is how long a pre-update copy of a plugin lives. A copy is
// dropped only when it is both past the newest KeepNewest and older than
// MaxAgeDays, so the two numbers are a floor and a deadline, not two caps.
type BackupPolicy struct {
	// KeepNewest is how many newest backups of a plugin are never deleted,
	// however old they get. Zero removes that protection, so the age limit alone
	// decides.
	KeepNewest int `json:"keep_newest"`
	// MaxAgeDays is the age past which a copy that KeepNewest does not protect is
	// deleted. Zero means no age limit, which means nothing is ever deleted: the
	// count on its own is not treated as a reason to throw away a young copy.
	MaxAgeDays int `json:"max_age_days"`
}

// DefaultBackupPolicy is what a run uses until the settings file says otherwise.
func DefaultBackupPolicy() BackupPolicy {
	return BackupPolicy{KeepNewest: DefaultKeepNewest, MaxAgeDays: DefaultMaxAgeDays}
}

// AgeLimit gives the day count and says whether an age limit applies at all.
// Days rather than hours is deliberate: a backup is not worth a finer clock than
// the user can set in the panel.
func (p BackupPolicy) AgeLimit() (ageDays int, limited bool) {
	return p.MaxAgeDays, p.MaxAgeDays > 0
}

// loadBackupPolicy is the file's "backup" object, or nil when the object is
// absent — the distinction a plain zero value cannot make.
type backupPolicyFile struct {
	Backup *BackupPolicy `json:"backup"`
}

// LoadBackupPolicy reads the retention policy from the settings file. A missing
// or unreadable file, a corrupt one, or one written before these keys existed
// all yield the defaults: a backup policy is not a reason to refuse an update.
func LoadBackupPolicy() BackupPolicy {
	path := SettingsPath()
	data, err := os.ReadFile(path)
	if err != nil {
		return DefaultBackupPolicy()
	}
	var file backupPolicyFile
	if err := json.Unmarshal(data, &file); err != nil {
		fmt.Printf("[!] Ignoring corrupt settings in %s: %v\n", path, err)
		return DefaultBackupPolicy()
	}
	if file.Backup == nil {
		return DefaultBackupPolicy()
	}
	return NormalizeBackupPolicy(*file.Backup)
}

// NormalizeBackupPolicy takes the negative answers a hand-edited file or a
// mistyped field can hold and leaves the meaning of zero alone: zero keep means
// no floor, zero days means no age limit, and a negative number is neither of
// those, it is a typo.
func NormalizeBackupPolicy(p BackupPolicy) BackupPolicy {
	if p.KeepNewest < 0 {
		p.KeepNewest = 0
	}
	if p.MaxAgeDays < 0 {
		p.MaxAgeDays = 0
	}
	return p
}
