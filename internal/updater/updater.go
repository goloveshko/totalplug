package updater

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"totalplug/internal/domain"
)

var (
	ErrEmptyDownloadURL = errors.New("download URL is empty")
	ErrPluginNotFound   = errors.New("plugin file not found on disk")
	// ErrFileLocked marks a target file that could not be written because it is
	// held by another process (typically a running Total Commander).
	ErrFileLocked = errors.New("target file is locked by another process")
)

// isLockedErr reports whether err indicates a file that is in use or denies
// write access, matching the Windows error codes for access denied, sharing
// violation and lock violation.
func isLockedErr(err error) bool {
	if errors.Is(err, os.ErrPermission) {
		return true
	}
	var errno syscall.Errno
	if errors.As(err, &errno) {
		switch errno {
		case 5, 32, 33: // ERROR_ACCESS_DENIED, ERROR_SHARING_VIOLATION, ERROR_LOCK_VIOLATION
			return true
		}
	}
	return false
}

// Updater handles downloading and applying plugin updates safely.
type Updater struct {
	client *http.Client
}

// NewUpdater initializes an Updater with reasonable HTTP timeouts.
func NewUpdater() *Updater {
	return &Updater{
		client: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// DownloadFile downloads a remote file to a local destination with context cancellation.
func (u *Updater) DownloadFile(ctx context.Context, url, destPath string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("creating HTTP request: %w", err)
	}
	req.Header.Set("User-Agent", "TotalPlug/1.0 (Total Commander Plugin Manager)")

	resp, err := u.client.Do(req)
	if err != nil {
		return fmt.Errorf("performing download request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected HTTP status: %s", resp.Status)
	}

	out, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("creating destination file: %w", err)
	}
	defer out.Close()

	if _, err = io.Copy(out, resp.Body); err != nil {
		return fmt.Errorf("writing downloaded payload: %w", err)
	}

	return nil
}

// ExtractZip extracts an archive safely to targetDir (mitigating Zip Slip vulnerability).
func (u *Updater) ExtractZip(zipPath, targetDir string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("opening zip file %q: %w", zipPath, err)
	}
	defer r.Close()

	for _, f := range r.File {
		cleanName := filepath.Clean(f.Name)
		if strings.HasPrefix(cleanName, "..") || filepath.IsAbs(cleanName) {
			return fmt.Errorf("illegal file path in zip: %s", f.Name)
		}

		destPath := filepath.Join(targetDir, cleanName)

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(destPath, f.Mode()); err != nil {
				return fmt.Errorf("creating directory %q: %w", destPath, err)
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return fmt.Errorf("creating parent dir for %q: %w", destPath, err)
		}

		outFile, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return fmt.Errorf("opening file for write %q: %w", destPath, err)
		}

		rc, err := f.Open()
		if err != nil {
			outFile.Close()
			return fmt.Errorf("reading entry %q: %w", f.Name, err)
		}

		_, err = io.Copy(outFile, rc)
		rc.Close()
		outFile.Close()
		if err != nil {
			return fmt.Errorf("extracting entry %q: %w", f.Name, err)
		}
	}

	return nil
}

// BackupPlugin creates a timestamped copy of the plugin directory. The original
// directory is left in place so the update can be applied on top of it and the
// backup stays available for manual recovery.
func (u *Updater) BackupPlugin(pluginDir string) (string, error) {
	backupDir := fmt.Sprintf("%s.bak_%d", pluginDir, time.Now().Unix())
	if err := copyTree(pluginDir, backupDir); err != nil {
		return "", fmt.Errorf("creating backup of %s: %w", pluginDir, err)
	}
	return backupDir, nil
}

// copyTree recursively copies srcDir into dstDir, merging with any existing files.
func copyTree(srcDir, dstDir string) error {
	return filepath.Walk(srcDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}

		target := filepath.Join(dstDir, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		return copyFile(path, target, info.Mode().Perm())
	})
}

func copyFile(src, dst string, mode os.FileMode) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("opening source %q: %w", src, err)
	}
	defer srcFile.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return fmt.Errorf("creating parent dir for %q: %w", dst, err)
	}

	dstFile, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return fmt.Errorf("opening destination %q: %w", dst, err)
	}
	defer dstFile.Close()

	if _, err = io.Copy(dstFile, srcFile); err != nil {
		return fmt.Errorf("copying %q to %q: %w", src, dst, err)
	}
	return nil
}

// applyTree copies extracted update files over the plugin directory and
// returns the relative paths of files that were successfully applied.
func applyTree(srcDir, dstDir string) ([]string, error) {
	var applied []string
	err := filepath.Walk(srcDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}

		target := filepath.Join(dstDir, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}

		if err := copyFile(path, target, info.Mode().Perm()); err != nil {
			return err
		}
		applied = append(applied, rel)
		return nil
	})
	return applied, err
}

// rollback restores the listed files from the backup directory. Files that did
// not exist before the update (no backup counterpart) are removed again.
func rollback(dstDir, backupDir string, applied []string) error {
	for _, rel := range applied {
		target := filepath.Join(dstDir, rel)
		original := filepath.Join(backupDir, rel)

		info, err := os.Stat(original)
		if os.IsNotExist(err) {
			if rmErr := os.Remove(target); rmErr != nil {
				return fmt.Errorf("removing newly added %q: %w", target, rmErr)
			}
			continue
		}
		if err != nil {
			return fmt.Errorf("stating backup %q: %w", original, err)
		}

		if err := copyFile(original, target, info.Mode().Perm()); err != nil {
			return fmt.Errorf("restoring %q: %w", target, err)
		}
	}
	return nil
}

// UpdatePlugin downloads an update, backs up the current plugin directory, and
// applies the new files. If applying fails, the files touched so far are rolled
// back from the backup. On success the backup directory is kept and returned.
func (u *Updater) UpdatePlugin(ctx context.Context, plugin domain.Plugin) (string, error) {
	if plugin.DownloadURL == "" {
		return "", ErrEmptyDownloadURL
	}

	pluginDir := filepath.Dir(plugin.Path)
	if _, err := os.Stat(pluginDir); os.IsNotExist(err) {
		return "", ErrPluginNotFound
	}

	tmpDir, err := os.MkdirTemp("", "totalplug_update_*")
	if err != nil {
		return "", fmt.Errorf("creating temp directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	archiveFile := filepath.Join(tmpDir, "package.zip")
	if err := u.DownloadFile(ctx, plugin.DownloadURL, archiveFile); err != nil {
		return "", fmt.Errorf("download failed: %w", err)
	}

	extractedDir := filepath.Join(tmpDir, "extracted")
	if err := u.ExtractZip(archiveFile, extractedDir); err != nil {
		return "", fmt.Errorf("extraction failed: %w", err)
	}

	backupDir, err := u.BackupPlugin(pluginDir)
	if err != nil {
		return "", fmt.Errorf("backup failed: %w", err)
	}

	applied, err := applyTree(extractedDir, pluginDir)
	if err != nil {
		locked := isLockedErr(err)
		if restoreErr := rollback(pluginDir, backupDir, applied); restoreErr != nil {
			return "", fmt.Errorf("apply failed: %v; automatic rollback failed: %v, restore from %s manually", err, restoreErr, backupDir)
		}
		os.RemoveAll(backupDir)
		if locked {
			return "", fmt.Errorf("apply failed (changes rolled back from backup): %w: %v", ErrFileLocked, err)
		}
		return "", fmt.Errorf("apply failed (changes rolled back from backup): %w", err)
	}

	return backupDir, nil
}
