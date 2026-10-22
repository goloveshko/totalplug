package updater

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/bodgit/sevenzip"
	"github.com/nwaples/rardecode"

	"github.com/goloveshko/totalplug/internal/config"
	"github.com/goloveshko/totalplug/internal/domain"
)

// sweepOnce keeps the leftover-sweep of %TEMP%\totalplug to the first Updater a
// process builds.
var sweepOnce sync.Once

var (
	ErrEmptyDownloadURL = errors.New("download URL is empty")
	ErrPluginNotFound   = errors.New("plugin file not found on disk")
	// ErrPluginNeedsOwnFolder refuses an update whose plugin file sits directly in
	// a folder it does not own, because the updater backs up and unpacks over that
	// whole folder.
	ErrPluginNeedsOwnFolder = errors.New("plugin does not live in a folder of its own")
	// ErrFileLocked marks a target file that could not be written because it is
	// held by another process (typically a running Total Commander).
	ErrFileLocked = errors.New("target file is locked by another process")
	// ErrInsecureScheme rejects downloads from plaintext or non-network schemes
	// that could deliver tampered archives.
	ErrInsecureScheme = errors.New("refusing insecure download scheme, https required")
	// ErrPlainHTTPSource marks the one insecure case the user can allow: a plain
	// HTTP source the opt-in download would accept. A ftp:// or a file:// link is
	// refused even with that opt-in, so the GUI must not offer a retry for it.
	// Errors for this case wrap both sentinels.
	ErrPlainHTTPSource = errors.New("source is plain HTTP")
	// ErrDownloadTooLarge aborts downloads above the configured size cap.
	ErrDownloadTooLarge = errors.New("download exceeds size limit")
	// ErrUnsupportedArchive marks a downloaded package in a format that cannot
	// be extracted (gzip, truncated HTML error pages, ...).
	ErrUnsupportedArchive = errors.New("unsupported archive format, only ZIP, RAR and 7z can be applied")
	// ErrExtractTooLarge aborts extraction whose cumulative decompressed output
	// exceeds the configured cap, so a small crafted archive cannot fill the disk.
	ErrExtractTooLarge = errors.New("archive expands beyond the decompressed size limit")
	// ErrBackupGone is what a rollback reports when the copy its history line
	// names is no longer on disk — retention caught up with it, or the data
	// folder was cleaned by hand.
	ErrBackupGone = errors.New("the kept copy of the plugin folder is gone")
	// ErrPluginOutsideTCRoot refuses a rollback whose target folder is not inside
	// a Total Commander install. The folder comes from a JSON file the user can
	// edit, and a rollback writes the copy's files into it and takes files away
	// from it, so "it is a path" is not enough to trust it.
	ErrPluginOutsideTCRoot = errors.New("plugin folder is not inside a Total Commander install")
)

// Outcome is what a successful update leaves behind: the copy of the version it
// replaced, and the files it wrote into the plugin folder. The list travels into
// the history line, because nothing else there says later which files in that
// folder this program itself put there.
type Outcome struct {
	BackupPath   string
	AppliedFiles []string
}

// DefaultMaxDownloadBytes caps archive size (256 MiB) to bound disk usage.
const DefaultMaxDownloadBytes int64 = 256 << 20

// DefaultMaxExtractBytes caps the total decompressed output of one archive
// (1 GiB). It is far above any legitimate Total Commander plugin yet bounds a
// highly-compressed "zip bomb", which the on-the-wire download cap alone cannot
// catch because such an archive is small until it is unpacked.
const DefaultMaxExtractBytes int64 = 1 << 30

// updateSpace is the folder inside the run's scratch that one download unpacks
// into. It is emptied before each plugin rather than recreated, so a run reuses
// one path for every update it makes.
const updateSpace = "update"

// keptSpace holds the packages this run already downloaded and unpacked, named
// after the URL that produced them, so that a retry of a plugin the update could
// not write (Total Commander still holding it) starts from the bytes already on
// disk instead of fetching them again. Only the run's own folder, so it goes with
// the run: on a normal exit, or with the sweep once a process dies.
const keptSpace = "downloaded"

// packageKey names the slot one URL owns. A hash rather than the last path
// segment because hosts hand the same filename out from different addresses, and
// a query string is part of what is being fetched.
func packageKey(rawURL string) string {
	sum := sha256.Sum256([]byte(rawURL))
	return fmt.Sprintf("%x", sum)
}

// staleScratchAfter is how long a scratch folder may sit untouched before the
// next run treats it as the leftover of a process that died, and removes it.
// Long enough for the largest download in the catalog to finish on a slow link.
const staleScratchAfter = time.Hour

// validateDownloadURL requires https; plain http is accepted for loopback hosts
// so automated tests can serve archives from a local server, and for any host
// when allowInsecure is set — that flag comes only from the GUI's explicit
// "download anyway" confirmation for a plugin the catalog offers over HTTP.
func validateDownloadURL(rawURL string, allowInsecure bool) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid download URL: %w", err)
	}
	switch strings.ToLower(u.Scheme) {
	case "https":
		return nil
	case "http":
		if allowInsecure {
			return nil
		}
		host := strings.ToLower(u.Hostname())
		if host == "localhost" || host == "127.0.0.1" || host == "::1" {
			return nil
		}
		return fmt.Errorf("%w: %s (%w)", ErrInsecureScheme, u.Host, ErrPlainHTTPSource)
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

// asLocked turns a failed filesystem call into the answer the UI has advice for:
// a file held by a running Total Commander is not a disk error.
func asLocked(err error) error {
	if isLockedErr(err) {
		return fmt.Errorf("%w: %v", ErrFileLocked, err)
	}
	return err
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
				return validateDownloadURL(req.URL.String(), false)
			},
		},
		maxDownloadBytes: DefaultMaxDownloadBytes,
		maxExtractBytes:  DefaultMaxExtractBytes,
	}
}

// DownloadFile downloads a remote file to a local destination with context cancellation.
// Plain HTTP is refused; UpdatePluginInsecure reaches the opt-in variant below.
func (u *Updater) DownloadFile(ctx context.Context, url, destPath string) error {
	return u.downloadFile(ctx, url, destPath, false)
}

// downloadFile is DownloadFile with a plain-HTTP allowance chosen by the caller.
// Redirects get the same policy as the first request, or an HTTPS source could
// quietly bounce the archive to plaintext.
func (u *Updater) downloadFile(ctx context.Context, url, destPath string, allowInsecure bool) error {
	if err := validateDownloadURL(url, allowInsecure); err != nil {
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

	client := u.client
	if allowInsecure {
		permissive := *u.client
		permissive.CheckRedirect = func(req *http.Request, _ []*http.Request) error {
			return validateDownloadURL(req.URL.String(), true)
		}
		client = &permissive
	}
	resp, err := client.Do(req)
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

// copyTree recursively copies srcDir into dstDir, merging with any existing
// files. When it stops, it also names the path relative to srcDir it stopped at,
// which tells a caller how far the folder was changed before the failure.
func copyTree(srcDir, dstDir string) (string, error) {
	var stopped string
	err := filepath.Walk(srcDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			stopped = relWithin(srcDir, path)
			return walkErr
		}

		rel := relWithin(srcDir, path)
		target := filepath.Join(dstDir, rel)
		if info.IsDir() {
			if err := os.MkdirAll(target, info.Mode()); err != nil {
				stopped = rel
				return err
			}
			return nil
		}
		if err := copyFile(path, target, info.Mode().Perm()); err != nil {
			stopped = rel
			return err
		}
		return nil
	})
	return stopped, err
}

func relWithin(srcDir, path string) string {
	rel, err := filepath.Rel(srcDir, path)
	if err != nil {
		return path
	}
	return rel
}

func copyFile(src, dst string, mode os.FileMode) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("opening source %s: %w", src, err)
	}
	defer srcFile.Close()

	dir := filepath.Dir(dst)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating parent dir for %s: %w", dst, err)
	}

	// Written to a sibling and renamed into place instead of being truncated
	// where it lies, so a copy that fails leaves the target byte-intact. An
	// in-place write that broke halfway left a damaged file that rollback did
	// not know about, and the backup holding the only good copy was then
	// deleted.
	//
	// The rename reaches a file another process merely holds open: the old one
	// goes into delete-pending and disappears when that process lets go, which is
	// why an update gets through with Total Commander running. A file the process
	// has *loaded* is the exception — the mapped image is held without share
	// delete, so the rename answers access denied until Total Commander unloads
	// the plugin. What a held file also blocks is removing the folder that
	// contains it, so nothing here tries that.
	tmpFile, err := os.CreateTemp(dir, ".totalplug-copy-*")
	if err != nil {
		return fmt.Errorf("creating temp for %s: %w", dst, err)
	}
	tmpName := tmpFile.Name()
	// A successful rename leaves nothing at tmpName, so this only ever deletes
	// the file of a failed copy; the chmod first is because Windows refuses to
	// delete a read-only file.
	defer func() {
		_ = os.Chmod(tmpName, 0600)
		_ = os.Remove(tmpName)
	}()

	if _, err := io.Copy(tmpFile, srcFile); err != nil {
		tmpFile.Close()
		return fmt.Errorf("copying %s to %s: %w", src, dst, err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("closing temp for %s: %w", dst, err)
	}
	if err := os.Chmod(tmpName, mode); err != nil {
		return fmt.Errorf("setting mode on %s: %w", dst, err)
	}
	if err := os.Rename(tmpName, dst); err != nil {
		return fmt.Errorf("replacing %s: %w", dst, err)
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

		// Recording only the files that were replaced is safe because copyFile
		// renames into place: a copy that failed left its target exactly as it
		// was, so there is nothing for rollback to undo.
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

// securedPackage is one usable package: where it lies on disk, which URL produced
// it (that URL names its slot in the kept folder), which reader unpacks it, and
// whether the bytes were already paid for by this run.
type securedPackage struct {
	path   string
	source string
	format string
	kept   bool
}

// isExtractable reports whether one of the archive readers can open a format
// detectArchiveFormat answered with.
func isExtractable(format string) bool {
	switch format {
	case "zip", "rar", "7z":
		return true
	}
	return false
}

// takeKeptPackage returns the package this run kept for one URL, when it is still
// there and still something the extractors can read. A slot that fails that check
// goes out with the check, so a corrupt leftover cannot block the download the
// retry needs.
func takeKeptPackage(rawURL, keepDir string) (securedPackage, bool) {
	slot := filepath.Join(keepDir, packageKey(rawURL))
	info, err := os.Stat(slot)
	if err != nil || info.Size() == 0 {
		return securedPackage{}, false
	}
	format, err := detectArchiveFormat(slot)
	if err != nil || !isExtractable(format) {
		_ = os.Remove(slot)
		return securedPackage{}, false
	}
	return securedPackage{path: slot, source: rawURL, format: format, kept: true}, true
}

// keepPackage moves a package that has proved it unpacks into the run's kept
// folder under the URL that produced it, and says where the bytes ended up. It is
// best effort: a run that cannot keep a package still holds it for this attempt.
func keepPackage(archiveFile, rawURL, keepDir string) string {
	if err := os.MkdirAll(keepDir, 0755); err != nil {
		return archiveFile
	}
	slot := filepath.Join(keepDir, packageKey(rawURL))
	if err := os.Rename(archiveFile, slot); err != nil {
		return archiveFile
	}
	return slot
}

// forgetPackage drops the kept copy of a package whose update went through: the
// bytes did their job, and the folder should hold only what a retry still needs.
func forgetPackage(rawURL, keepDir string) {
	_ = os.Remove(filepath.Join(keepDir, packageKey(rawURL)))
}

// All failures are retained so a later dead mirror cannot hide an HTTP refusal.
func (u *Updater) tryDownloadCandidates(ctx context.Context, urls []string, archiveFile, keepDir string, allowInsecure bool) (securedPackage, error) {
	var failures []error
	for _, candidate := range urls {
		if kept, ok := takeKeptPackage(candidate, keepDir); ok {
			return kept, nil
		}
		if err := u.downloadFile(ctx, candidate, archiveFile, allowInsecure); err != nil {
			failures = append(failures, fmt.Errorf("download failed: %w", err))
			continue
		}
		format, err := detectArchiveFormat(archiveFile)
		if err != nil {
			failures = append(failures, err)
			continue
		}
		if !isExtractable(format) {
			failures = append(failures, fmt.Errorf("%w: downloaded file is %s", ErrUnsupportedArchive, format))
			continue
		}
		return securedPackage{path: archiveFile, source: candidate, format: format}, nil
	}
	return securedPackage{}, errors.Join(failures...)
}

// tcExeNames are the executables that only ever sit in a Total Commander install
// root, which is how one is recognised from a plugin folder.
var tcExeNames = []string{"wincmd.exe", "totalcmd64.exe"}

func isTotalCommanderRoot(dir string) bool {
	for _, exe := range tcExeNames {
		if _, err := os.Stat(filepath.Join(dir, exe)); err == nil {
			return true
		}
	}
	return false
}

// pluginInsideTCRoot reports whether dir sits under a Total Commander install,
// which is what bounds the folders an update or a rollback may write to.
func pluginInsideTCRoot(dir string) bool {
	for parent := filepath.Dir(dir); ; parent = filepath.Dir(parent) {
		if isTotalCommanderRoot(parent) {
			return true
		}
		next := filepath.Dir(parent)
		if next == parent {
			return false
		}
	}
}

// validatePluginFolder is what both an update and a rollback check the target
// against before either copies a tree over it.
//
// A wincmd.ini entry can point straight at a file rather than into a folder of
// its own, and then the whole of the folder is what gets backed up and
// overwritten — so the two ways that goes wrong are refused: the entry naming the
// Total Commander install itself, and one naming a shared type folder like wfx
// that holds every plugin of that kind.
func validatePluginFolder(pluginDir string) error {
	if _, err := os.Stat(pluginDir); os.IsNotExist(err) {
		return ErrPluginNotFound
	}
	if isTotalCommanderRoot(pluginDir) {
		return fmt.Errorf("%w: %s is the Total Commander install directory", ErrPluginNeedsOwnFolder, pluginDir)
	}
	if config.IsGenericPluginDir(filepath.Base(pluginDir)) {
		return fmt.Errorf("%w: %s holds every plugin of its type", ErrPluginNeedsOwnFolder, pluginDir)
	}
	return nil
}

// UpdatePlugin downloads an update, backs up the current plugin directory, and
// applies the new files. If applying fails, the files touched so far are rolled
// back from the backup. On success the backup directory is kept and returned
// with the list of files that were written, and the copies the retention policy
// in the settings file no longer wants are pruned from the app's backup space.
//
// Every candidate URL (primary + catalog mirrors) is tried until one yields a
// usable archive; backup and apply happen only after a good package is secured.
// One that unpacked is held in the run's folder until the apply goes through or
// the run ends, so retrying a plugin the update could not write starts from those
// bytes rather than fetching them a second time.
func (u *Updater) UpdatePlugin(ctx context.Context, plugin domain.Plugin) (Outcome, error) {
	return u.updatePlugin(ctx, plugin, false)
}

// UpdatePluginInsecure is UpdatePlugin for an archive the user explicitly chose
// to fetch over plain HTTP after the GUI warning. The CLI never calls it.
func (u *Updater) UpdatePluginInsecure(ctx context.Context, plugin domain.Plugin) (Outcome, error) {
	return u.updatePlugin(ctx, plugin, true)
}

func (u *Updater) updatePlugin(ctx context.Context, plugin domain.Plugin, allowInsecure bool) (Outcome, error) {
	candidates := downloadCandidates(plugin)
	if len(candidates) == 0 {
		return Outcome{}, ErrEmptyDownloadURL
	}

	pluginDir := filepath.Dir(plugin.Path)
	if err := validatePluginFolder(pluginDir); err != nil {
		return Outcome{}, err
	}

	runDir, err := config.RunScratch()
	if err != nil {
		return Outcome{}, fmt.Errorf("creating temp directory: %w", err)
	}
	// A run downloads every plugin of an --update-all into its own scratch folder,
	// so each one starts from an empty `update` inside it rather than unpacking
	// over what the previous package left. The `downloaded` beside it is emptied by
	// nobody until the run ends: it is what a retry reads back.
	tmpDir := filepath.Join(runDir, updateSpace)
	keepDir := filepath.Join(runDir, keptSpace)
	if allowInsecure {
		// Permissive redirects must never populate another plugin's strict cache.
		keepDir = filepath.Join(keepDir, "http", packageKey(plugin.Path))
	}
	if err := os.RemoveAll(tmpDir); err != nil {
		return Outcome{}, fmt.Errorf("clearing temp directory: %w", err)
	}
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		return Outcome{}, fmt.Errorf("creating temp directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	archiveFile := filepath.Join(tmpDir, "package.archive")
	pkg, downloadErr := u.tryDownloadCandidates(ctx, candidates, archiveFile, keepDir, allowInsecure)

	if pkg.format == "" {
		// All catalog sources failed; discover mirrors lazily.
		if mirrors := u.resolveTotalCmdMirrors(ctx, plugin.DownloadURL); len(mirrors) > 0 {
			var mirrorErr error
			pkg, mirrorErr = u.tryDownloadCandidates(ctx, mirrors, archiveFile, keepDir, allowInsecure)
			downloadErr = errors.Join(downloadErr, mirrorErr)
		}
	}
	if pkg.format == "" {
		// Broken catalog/upstream links are common (totalcmd.net download
		// counters redirect to dead hosts), so point at the plugin page.
		err := downloadErr
		if plugin.WebURL != "" {
			err = fmt.Errorf("%w; plugin page: %s", downloadErr, plugin.WebURL)
		}
		if len(candidates) > 1 {
			err = fmt.Errorf("all %d download sources failed: %w", len(candidates), err)
		}
		return Outcome{}, err
	}

	extractedDir := filepath.Join(tmpDir, "extracted")
	u.emit(domain.UpdateProgress{Phase: "extract", Percent: -1})
	switch pkg.format {
	case "zip":
		err = u.ExtractZip(pkg.path, extractedDir)
	case "rar":
		err = u.ExtractRar(pkg.path, extractedDir)
	case "7z":
		err = u.Extract7z(pkg.path, extractedDir)
	}
	if err != nil {
		// Magic bytes alone did not make this package readable, so it is not worth
		// keeping: a retry has to be free to fetch another one.
		if pkg.kept {
			forgetPackage(pkg.source, keepDir)
		}
		return Outcome{}, fmt.Errorf("extraction failed: %w", err)
	}
	// Unpacking proved the bytes good, so from here on they are worth keeping for a
	// retry: an apply that Total Commander blocks should not ask the host for the
	// same package again once the user closes it and tries once more.
	if !pkg.kept {
		pkg.path = keepPackage(archiveFile, pkg.source, keepDir)
	}

	backupDir, err := u.BackupPlugin(pluginDir)
	if err != nil {
		return Outcome{}, fmt.Errorf("backup failed: %w", err)
	}

	u.emit(domain.UpdateProgress{Phase: "install", Percent: -1})
	applied, err := applyTree(extractedDir, pluginDir)
	if err != nil {
		locked := isLockedErr(err)
		if restoreErr := rollback(pluginDir, backupDir, applied); restoreErr != nil {
			return Outcome{}, fmt.Errorf("apply failed: %v; automatic rollback failed: %v, restore from %s manually", err, restoreErr, backupDir)
		}
		os.RemoveAll(backupDir)
		if locked {
			return Outcome{}, fmt.Errorf("apply failed (changes rolled back from backup): %w: %v", ErrFileLocked, err)
		}
		return Outcome{}, fmt.Errorf("apply failed (changes rolled back from backup): %w", err)
	}

	// Best-effort cleanup: the update already succeeded, so old backups must
	// not turn a good update into a reported failure.
	// The policy is read here rather than when the Updater was built, so a
	// settings change made while this process runs — by this window or by a CLI
	// run — is what the next update honours.
	_, _ = pruneBackupSpace(pluginDir, config.LoadBackupPolicy())

	// The kept package has been written into the plugin folder, so nothing a retry
	// still needs is waiting on it.
	forgetPackage(pkg.source, keepDir)

	return Outcome{BackupPath: backupDir, AppliedFiles: applied}, nil
}
