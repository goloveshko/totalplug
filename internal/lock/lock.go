// Package lock keeps two TotalPlug runs from writing the same files at the
// same time.
//
// The app has two faces — a window and a command line — and both can be open
// at once: a user leaves the window open while a scheduled task runs an
// update. Two runs then write the same plugin folder, the same history file
// and the same caches, and the loser of the race either fails with a sharing
// violation or writes a half-written file over a good one.
//
// The lock is a named mutex, not a lock file: the operating system hands it
// back when the process dies, so a run that was killed leaves nothing behind
// for the next one to trip over.
package lock

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/windows"
)

// ErrBusy says another TotalPlug run is working on the same data folder.
var ErrBusy = errors.New("another TotalPlug run is working on this data folder")

// Guard is an exclusive hold on one data folder. Release gives it back.
type Guard struct {
	handle windows.Handle
	once   sync.Once
}

// mutexName derives the mutex name from the data folder, so two different
// folders — a portable copy and an installed one — do not lock each other out.
func mutexName(dataDir string) string {
	abs, err := filepath.Abs(dataDir)
	if err != nil {
		abs = dataDir
	}
	sum := sha256.Sum256([]byte(strings.ToLower(abs)))
	return "TotalPlug-" + hex.EncodeToString(sum[:8])
}

// createMutex opens the named mutex, preferring the machine-wide name so a
// scheduled task in another session of the same user is covered too. That name
// is refused for a plain user account, and the session-wide one is then as
// good as it gets.
//
// A mutex that is already there is not an error: opening the one another run
// holds is exactly what is wanted. CreateMutex answers ERROR_ALREADY_EXISTS
// for that, along with a usable handle.
func createMutex(name string) (windows.Handle, error) {
	handle, err := windows.CreateMutex(nil, false, windows.StringToUTF16Ptr(`Global\`+name))
	if err == nil || errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		return handle, nil
	}
	handle, err = windows.CreateMutex(nil, false, windows.StringToUTF16Ptr(`Local\`+name))
	if err != nil && !errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		return 0, fmt.Errorf("creating the run lock: %w", err)
	}
	return handle, nil
}

// Acquire takes the exclusive lock for the data folder, waiting up to timeout
// for another run to let it go. It answers ErrBusy when the wait runs out.
func Acquire(dataDir string, timeout time.Duration) (*Guard, error) {
	handle, err := createMutex(mutexName(dataDir))
	if err != nil {
		return nil, err
	}

	event, _ := windows.WaitForSingleObject(handle, uint32(timeout.Milliseconds()))
	switch event {
	case uint32(windows.WAIT_OBJECT_0):
		return &Guard{handle: handle}, nil
	case uint32(windows.WAIT_TIMEOUT):
		_ = windows.CloseHandle(handle)
		return nil, ErrBusy
	default:
		_ = windows.CloseHandle(handle)
		return nil, fmt.Errorf("waiting for the run lock failed")
	}}

// Release lets the next run in. Calling it twice is harmless.
func (g *Guard) Release() {
	if g == nil {
		return
	}
	g.once.Do(func() {
		_ = windows.ReleaseMutex(g.handle)
		_ = windows.CloseHandle(g.handle)
	})
}
