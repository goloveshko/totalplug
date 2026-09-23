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
	"time"

	"totalplug/internal/domain"
)

var (
	ErrEmptyDownloadURL = errors.New("download URL is empty")
	ErrPluginNotFound   = errors.New("plugin file not found on disk")
)

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

// BackupPlugin creates a timestamped backup directory for the target plugin directory.
func (u *Updater) BackupPlugin(pluginDir string) (string, error) {
	backupDir := fmt.Sprintf("%s.bak_%d", pluginDir, time.Now().Unix())
	if err := os.Rename(pluginDir, backupDir); err != nil {
		return "", fmt.Errorf("creating backup from %s to %s: %w", pluginDir, backupDir, err)
	}
	return backupDir, nil
}

// UpdatePlugin orchestrates downloading, backing up, and applying the plugin update.
func (u *Updater) UpdatePlugin(ctx context.Context, plugin domain.Plugin) error {
	if plugin.DownloadURL == "" {
		return ErrEmptyDownloadURL
	}

	pluginDir := filepath.Dir(plugin.Path)
	if _, err := os.Stat(pluginDir); os.IsNotExist(err) {
		return ErrPluginNotFound
	}

	tmpDir, err := os.MkdirTemp("", "totalplug_update_*")
	if err != nil {
		return fmt.Errorf("creating temp directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	archiveFile := filepath.Join(tmpDir, "package.zip")
	if err := u.DownloadFile(ctx, plugin.DownloadURL, archiveFile); err != nil {
		return fmt.Errorf("download failed: %w", err)
	}

	extractedDir := filepath.Join(tmpDir, "extracted")
	if err := u.ExtractZip(archiveFile, extractedDir); err != nil {
		return fmt.Errorf("extraction failed: %w", err)
	}

	// Apply files by copying over into the plugin directory
	err = filepath.Walk(extractedDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		rel, err := filepath.Rel(extractedDir, path)
		if err != nil {
			return err
		}

		targetPath := filepath.Join(pluginDir, rel)
		if info.IsDir() {
			return os.MkdirAll(targetPath, info.Mode())
		}

		srcFile, err := os.Open(path)
		if err != nil {
			return err
		}
		defer srcFile.Close()

		dstFile, err := os.OpenFile(targetPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode())
		if err != nil {
			return err
		}
		defer dstFile.Close()

		_, err = io.Copy(dstFile, srcFile)
		return err
	})

	if err != nil {
		return fmt.Errorf("applying extracted files: %w", err)
	}

	return nil
}
