package updater

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/bodgit/sevenzip"
	"github.com/nwaples/rardecode"

	"totalplug/internal/config"
	"totalplug/internal/domain"
)

// sweepOnce keeps the leftover-sweep of %TEMP%\totalplug to the first Updater a
// process builds.
var sweepOnce sync.Once

var (
	ErrEmptyDownloadURL = errors.New("download URL is empty")
	ErrPluginNotFound   = errors.New("plugin file not found on disk")
	// ErrFileLocked marks a target file that could not be written because it is
	// held by another process (typically a running Total Commander).
	ErrFileLocked = errors.New("target file is locked by another process")
	// ErrInsecureScheme rejects downloads from plaintext or non-network schemes
	// that could deliver tampered archives.
	ErrInsecureScheme = errors.New("refusing insecure download scheme, https required")
	// ErrDownloadTooLarge aborts downloads above the configured size cap.
	ErrDownloadTooLarge = errors.New("download exceeds size limit")
	// ErrUnsupportedArchive marks a downloaded package in a format that cannot
	// be extracted (gzip, truncated HTML error pages, ...).
	ErrUnsupportedArchive = errors.New("unsupported archive format, only ZIP, RAR and 7z can be applied")
	// ErrExtractTooLarge aborts extraction whose cumulative decompressed output
	// exceeds the configured cap, so a small crafted archive cannot fill the disk.
	ErrExtractTooLarge = errors.New("archive expands beyond the decompressed size limit")
)

// DefaultMaxDownloadBytes caps archive size (256 MiB) to bound disk usage.
const DefaultMaxDownloadBytes int64 = 256 << 20

// DefaultMaxExtractBytes caps the total decompressed output of one archive
// (1 GiB). It is far above any legitimate Total Commander plugin yet bounds a
// highly-compressed "zip bomb", which the on-the-wire download cap alone cannot
// catch because such an archive is small until it is unpacked.
const DefaultMaxExtractBytes int64 = 1 << 30

// DefaultBackupsKept is how many newest backup directories remain per plugin
// after a successful update.
const DefaultBackupsKept = 3

// backupSuffix separates a plugin directory name from its backup timestamp.
const backupSuffix = ".bak_"

// updateSpace is the folder inside the run's scratch that one download unpacks
// into. It is emptied before each plugin rather than recreated, so a run reuses
// one path for every update it makes.
const updateSpace = "update"

// staleScratchAfter is how long a scratch folder may sit untouched before the
// next run treats it as the leftover of a process that died, and removes it.
// Long enough for the largest download in the catalog to finish on a slow link.
const staleScratchAfter = time.Hour

// PruneOldBackups deletes all but the newest `keep` backup directories of
// pluginDir (named "<pluginDir>.bak_<unix>"). Entries with an unparseable
// timestamp are left alone because they were not created by this tool.
// It returns the paths that were removed.
func PruneOldBackups(pluginDir string, keep int) ([]string, error) {
	if keep < 0 {
		return nil, fmt.Errorf("invalid backup keep count %d", keep)
	}

	type backup struct {
		path string
		ts   int64
	}

	entries, err := os.ReadDir(filepath.Dir(pluginDir))
	if err != nil {
		return nil, fmt.Errorf("reading backup parent dir: %w", err)
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
		backups = append(backups, backup{path: filepath.Join(filepath.Dir(pluginDir), e.Name()), ts: ts})
	}

	if len(backups) <= keep {
		return nil, nil
	}

	sort.Slice(backups, func(i, j int) bool { return backups[i].ts > backups[j].ts })
	var removed []string
	for _, b := range backups[keep:] {
		if err := os.RemoveAll(b.path); err != nil {
			return removed, fmt.Errorf("removing old backup %s: %w", b.path, err)
		}
		removed = append(removed, b.path)
	}
	return removed, nil
}

// validateDownloadURL requires https; plain http is only accepted for loopback
// hosts so automated tests can serve archives from a local server.
func validateDownloadURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid download URL: %w", err)
	}
	switch strings.ToLower(u.Scheme) {
	case "https":
		return nil
	case "http":
		host := strings.ToLower(u.Hostname())
		if host == "localhost" || host == "127.0.0.1" || host == "::1" {
			return nil
		}
		return fmt.Errorf("%w: %s", ErrInsecureScheme, u.Host)
	default:
		return fmt.Errorf("%w: %q", ErrInsecureScheme, u.Scheme)
	}
}

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
	client           *http.Client
	maxDownloadBytes int64
	// maxExtractBytes bounds the cumulative decompressed output of one archive;
	// zero means no limit (only tests lower it).
	maxExtractBytes int64
	// progress, when set, receives UpdateProgress events during UpdatePlugin.
	// Only one update is expected at a time, so no locking is needed.
	progress func(domain.UpdateProgress)
}

// SetProgressListener installs a callback for update progress events, or
// removes the current one with nil.
func (u *Updater) SetProgressListener(fn func(domain.UpdateProgress)) {
	u.progress = fn
}

func (u *Updater) emit(p domain.UpdateProgress) {
	if u.progress != nil {
		u.progress(p)
	}
}

// countingReader tracks downloaded bytes and invokes report each time the
// integer percentage advances. total <= 0 means the size is unknown, so no
// incremental reports are emitted.
type countingReader struct {
	src    io.Reader
	total  int64
	read   int64
	last   int
	report func(percent float64)
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.src.Read(p)
	c.read += int64(n)
	if c.total > 0 && c.report != nil {
		if pct := int(float64(c.read) / float64(c.total) * 100); pct != c.last {
			c.last = pct
			c.report(float64(pct))
		}
	}
	return n, err
}

// extractLimiter enforces a ceiling on the total number of bytes unpacked from
// one archive. A single limiter is shared by every entry, so the cap applies to
// the cumulative decompressed size rather than to any one file; a small archive
// crafted to expand into a disk-filling payload is stopped the moment the total
// would go over.
type extractLimiter struct {
	written int64
	limit   int64
}

// writer returns an io.Writer that counts every byte against the limiter before
// writing it to dst, aborting with ErrExtractTooLarge when the cumulative total
// would exceed the cap.
func (l *extractLimiter) writer(dst io.Writer) io.Writer {
	return &limitedWriter{dst: dst, lim: l}
}

type limitedWriter struct {
	dst io.Writer
	lim *extractLimiter
}

func (w *limitedWriter) Write(p []byte) (int, error) {
	if w.lim.limit > 0 && w.lim.written+int64(len(p)) > w.lim.limit {
		return 0, fmt.Errorf("%w: decompressed size exceeds %d bytes", ErrExtractTooLarge, w.lim.limit)
	}
	w.lim.written += int64(len(p))
	return w.dst.Write(p)
}

// NewUpdater initializes an Updater with reasonable HTTP timeouts. The first
// construction of a process also takes the folders away that runs killed
// mid-download left in %TEMP%\totalplug.
func NewUpdater() *Updater {
	sweepOnce.Do(func() { config.SweepScratch(staleScratchAfter) })
	return &Updater{
		client: &http.Client{
			Timeout: 60 * time.Second,
			// The scheme rule has to be re-checked on every hop: without this
			// the client follows an https -> http redirect on its own and hands
			// back an archive fetched in plaintext, which is then unpacked over
			// the plugin directory.
			CheckRedirect: func(req *http.Request, _ []*http.Request) error {
				return validateDownloadURL(req.URL.String())
			},
		},
		maxDownloadBytes: DefaultMaxDownloadBytes,
		maxExtractBytes:  DefaultMaxExtractBytes,
	}
}

// DownloadFile downloads a remote file to a local destination with context cancellation.
func (u *Updater) DownloadFile(ctx context.Context, url, destPath string) error {
	if err := validateDownloadURL(url); err != nil {
		return err
	}
	// Announce the source before the request so mirror attempts are visible in
	// the UI even when the server fails fast (404, redirect to dead host).
	u.emit(domain.UpdateProgress{Phase: "download", Percent: -1, URL: url})

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

	if u.maxDownloadBytes > 0 && resp.ContentLength > u.maxDownloadBytes {
		return fmt.Errorf("%w: server advertises %d bytes, limit is %d", ErrDownloadTooLarge, resp.ContentLength, u.maxDownloadBytes)
	}

	out, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("creating destination file: %w", err)
	}
	defer out.Close()

	var src io.Reader = resp.Body
	if u.maxDownloadBytes > 0 {
		src = io.LimitReader(resp.Body, u.maxDownloadBytes+1)
	}
	percent := -1.0
	if resp.ContentLength > 0 {
		percent = 0
	}
	u.emit(domain.UpdateProgress{Phase: "download", Percent: percent, URL: url})
	src = &countingReader{
		src:    src,
		total:  resp.ContentLength,
		report: func(p float64) { u.emit(domain.UpdateProgress{Phase: "download", Percent: p, URL: url}) },
	}
	written, err := io.Copy(out, src)
	if err != nil {
		return fmt.Errorf("writing downloaded payload: %w", err)
	}
	if u.maxDownloadBytes > 0 && written > u.maxDownloadBytes {
		return fmt.Errorf("%w: stream exceeded the limit of %d bytes", ErrDownloadTooLarge, u.maxDownloadBytes)
	}

	return nil
}

// detectArchiveFormat classifies a downloaded package by its magic bytes so
// unsupported formats fail fast with an actionable message instead of a
// confusing "zip: not a valid zip file" from the extractor.
func detectArchiveFormat(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("opening downloaded file: %w", err)
	}
	defer f.Close()

	head := make([]byte, 8)
	n, err := io.ReadFull(f, head)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return "", fmt.Errorf("reading downloaded file header: %w", err)
	}
	head = head[:n]

	switch {
	case len(head) >= 4 && bytes.Equal(head[:4], []byte("PK\x03\x04")):
		return "zip", nil
	case len(head) >= 6 && bytes.Equal(head[:6], []byte("Rar!\x1a\x07")):
		return "rar", nil
	case len(head) >= 6 && bytes.Equal(head[:6], []byte{0x37, 0x7A, 0xBC, 0xAF, 0x27, 0x1C}):
		return "7z", nil
	case len(head) >= 2 && head[0] == 0x1f && head[1] == 0x8b:
		return "gzip", nil
	default:
		return "", fmt.Errorf("%w: unrecognized file signature", ErrUnsupportedArchive)
	}
}

// sanitizeEntryPath maps one archive entry name to a path under targetDir,
// rejecting traversal and absolute/drive-prefixed names (Zip Slip) regardless
// of the archive format.
func sanitizeEntryPath(targetDir, name string) (string, error) {
	cleanName := filepath.Clean(name)
	if strings.HasPrefix(cleanName, "..") || filepath.IsAbs(cleanName) || filepath.VolumeName(cleanName) != "" {
		return "", fmt.Errorf("illegal file path in archive: %s", name)
	}
	return filepath.Join(targetDir, cleanName), nil
}

// ExtractZip extracts an archive safely to targetDir (mitigating Zip Slip vulnerability).
func (u *Updater) ExtractZip(zipPath, targetDir string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("opening zip file %q: %w", zipPath, err)
	}
	defer r.Close()

	lim := &extractLimiter{limit: u.maxExtractBytes}
	for _, f := range r.File {
		destPath, err := sanitizeEntryPath(targetDir, f.Name)
		if err != nil {
			return err
		}

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

		_, err = io.Copy(lim.writer(outFile), rc)
		rc.Close()
		outFile.Close()
		if err != nil {
			return fmt.Errorf("extracting entry %q: %w", f.Name, err)
		}
	}

	return nil
}

// ExtractRar unpacks a RAR4/RAR5 archive to targetDir. Entries are streamed
// sequentially by rardecode. Windows-authored plugin archives carry no usable
// POSIX permissions, so plain 0644/0755 are applied uniformly; a password on
// the archive is treated as an error by rardecode.
func (u *Updater) ExtractRar(rarPath, targetDir string) error {
	f, err := os.Open(rarPath)
	if err != nil {
		return fmt.Errorf("opening rar file %q: %w", rarPath, err)
	}
	defer f.Close()

	r, err := rardecode.NewReader(f, "")
	if err != nil {
		return fmt.Errorf("opening rar archive %q: %w", rarPath, err)
	}

	lim := &extractLimiter{limit: u.maxExtractBytes}
	for {
		hdr, err := r.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("reading rar archive %q: %w", rarPath, err)
		}

		destPath, err := sanitizeEntryPath(targetDir, hdr.Name)
		if err != nil {
			return err
		}

		if hdr.IsDir {
			if err := os.MkdirAll(destPath, 0755); err != nil {
				return fmt.Errorf("creating directory %q: %w", destPath, err)
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return fmt.Errorf("creating parent dir for %q: %w", destPath, err)
		}

		outFile, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
		if err != nil {
			return fmt.Errorf("opening file for write %q: %w", destPath, err)
		}

		if _, err := io.Copy(lim.writer(outFile), r); err != nil {
			outFile.Close()
			return fmt.Errorf("extracting rar entry %q: %w", hdr.Name, err)
		}
		if err := outFile.Close(); err != nil {
			return fmt.Errorf("closing extracted rar entry %q: %w", hdr.Name, err)
		}
	}
}

// Extract7z unpacks a 7z archive to targetDir using the pure-Go sevenzip
// reader. Like ExtractRar, archive-stored permissions are ignored in favor of
// 0644/0755 defaults.
func (u *Updater) Extract7z(sevenzPath, targetDir string) error {
	f, err := os.Open(sevenzPath)
	if err != nil {
		return fmt.Errorf("opening 7z file %q: %w", sevenzPath, err)
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return fmt.Errorf("stating 7z file %q: %w", sevenzPath, err)
	}

	r, err := sevenzip.NewReader(f, stat.Size())
	if err != nil {
		return fmt.Errorf("opening 7z archive %q: %w", sevenzPath, err)
	}

	lim := &extractLimiter{limit: u.maxExtractBytes}
	for _, file := range r.File {
		destPath, err := sanitizeEntryPath(targetDir, file.Name)
		if err != nil {
			return err
		}

		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(destPath, 0755); err != nil {
				return fmt.Errorf("creating directory %q: %w", destPath, err)
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return fmt.Errorf("creating parent dir for %q: %w", destPath, err)
		}

		rc, err := file.Open()
		if err != nil {
			return fmt.Errorf("reading 7z entry %q: %w", file.Name, err)
		}

		outFile, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
		if err != nil {
			rc.Close()
			return fmt.Errorf("opening file for write %q: %w", destPath, err)
		}

		if _, err := io.Copy(lim.writer(outFile), rc); err != nil {
			rc.Close()
			outFile.Close()
			return fmt.Errorf("extracting 7z entry %q: %w", file.Name, err)
		}
		rc.Close()
		if err := outFile.Close(); err != nil {
			return fmt.Errorf("closing extracted 7z entry %q: %w", file.Name, err)
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

// downloadCandidates returns the ordered list of URLs to try for a plugin:
// the checker-provided candidates when present, otherwise the single primary
// URL. The primary stays first so mirrors are only used as fallback.
func downloadCandidates(plugin domain.Plugin) []string {
	if len(plugin.DownloadURLs) > 0 {
		return plugin.DownloadURLs
	}
	if plugin.DownloadURL != "" {
		return []string{plugin.DownloadURL}
	}
	return nil
}

// tryDownloadCandidates downloads and validates archives from an ordered URL
// list, returning the first usable archive format. An empty format means all
// candidates failed; lastErr is the most recent failure.
func (u *Updater) tryDownloadCandidates(ctx context.Context, urls []string, archiveFile string) (format string, lastErr error) {
	for _, candidate := range urls {
		if err := u.DownloadFile(ctx, candidate, archiveFile); err != nil {
			lastErr = fmt.Errorf("download failed: %w", err)
			continue
		}
		f, err := detectArchiveFormat(archiveFile)
		if err != nil {
			lastErr = err
			continue
		}
		if f != "zip" && f != "rar" && f != "7z" {
			lastErr = fmt.Errorf("%w: downloaded file is %s", ErrUnsupportedArchive, f)
			continue
		}
		return f, nil
	}
	return "", lastErr
}

// UpdatePlugin downloads an update, backs up the current plugin directory, and
// applies the new files. If applying fails, the files touched so far are rolled
// back from the backup. On success the backup directory is kept and returned,
// and older backups beyond DefaultBackupsKept are pruned.
//
// Every candidate URL (primary + catalog mirrors) is tried until one yields a
// usable archive; backup and apply happen only after a good package is secured.
func (u *Updater) UpdatePlugin(ctx context.Context, plugin domain.Plugin) (string, error) {
	candidates := downloadCandidates(plugin)
	if len(candidates) == 0 {
		return "", ErrEmptyDownloadURL
	}

	pluginDir := filepath.Dir(plugin.Path)
	if _, err := os.Stat(pluginDir); os.IsNotExist(err) {
		return "", ErrPluginNotFound
	}

	tmpDir, err := config.RunScratch()
	if err != nil {
		return "", fmt.Errorf("creating temp directory: %w", err)
	}
	// A run downloads every plugin of an --update-all into its own scratch folder,
	// so each one starts from an empty `update` inside it rather than unpacking
	// over what the previous package left.
	tmpDir = filepath.Join(tmpDir, updateSpace)
	if err := os.RemoveAll(tmpDir); err != nil {
		return "", fmt.Errorf("clearing temp directory: %w", err)
	}
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		return "", fmt.Errorf("creating temp directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	archiveFile := filepath.Join(tmpDir, "package.archive")
	format, lastErr := u.tryDownloadCandidates(ctx, candidates, archiveFile)

	if format == "" {
		// All catalog sources failed: for totalcmd.net links, lazily parse the
		// plugin page for mirrors and retry them (fallback maintained by the
		// community, e.g. wincmd.ru).
		if mirrors := u.resolveTotalCmdMirrors(ctx, plugin.DownloadURL); len(mirrors) > 0 {
			// DownloadFile announces each candidate URL, so mirror attempts are
			// already visible in the UI.
			var mirrorErr error
			format, mirrorErr = u.tryDownloadCandidates(ctx, mirrors, archiveFile)
			if mirrorErr != nil {
				lastErr = mirrorErr
			}
		}
	}
	if format == "" {
		// Broken catalog/upstream links are common (totalcmd.net download
		// counters redirect to dead hosts), so point at the plugin page.
		err := lastErr
		if plugin.WebURL != "" {
			err = fmt.Errorf("%w; plugin page: %s", lastErr, plugin.WebURL)
		}
		if len(candidates) > 1 {
			err = fmt.Errorf("all %d download sources failed, last: %w", len(candidates), err)
		}
		return "", err
	}

	extractedDir := filepath.Join(tmpDir, "extracted")
	u.emit(domain.UpdateProgress{Phase: "extract", Percent: -1})
	switch format {
	case "zip":
		err = u.ExtractZip(archiveFile, extractedDir)
	case "rar":
		err = u.ExtractRar(archiveFile, extractedDir)
	case "7z":
		err = u.Extract7z(archiveFile, extractedDir)
	}
	if err != nil {
		return "", fmt.Errorf("extraction failed: %w", err)
	}

	backupDir, err := u.BackupPlugin(pluginDir)
	if err != nil {
		return "", fmt.Errorf("backup failed: %w", err)
	}

	u.emit(domain.UpdateProgress{Phase: "install", Percent: -1})
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

	// Best-effort cleanup: the update already succeeded, so old backups must
	// not turn a good update into a reported failure.
	_, _ = PruneOldBackups(pluginDir, DefaultBackupsKept)

	return backupDir, nil
}
