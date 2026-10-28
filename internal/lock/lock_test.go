package lock

import (
	"errors"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestAcquireKeepsOtherRunsOut(t *testing.T) {
	dir := t.TempDir()

	// The lock is held by a thread of its own. A Windows mutex is recursive
	// for the thread that owns it: the same thread could take it again and
	// would not be kept out, so the holder has to be another thread for the
	// test to say anything about a second run.
	held := make(chan struct{})
	release := make(chan struct{})
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		guard, err := Acquire(dir, time.Second)
		if err != nil {
			t.Errorf("holding Acquire: %v", err)
			close(held)
			return
		}
		close(held)
		<-release
		guard.Release()
	}()
	<-held

	if _, err := Acquire(dir, 150*time.Millisecond); !errors.Is(err, ErrBusy) {
		t.Errorf("second Acquire returned %v, want %v", err, ErrBusy)
	}

	close(release)

	// After it is released, the next run gets in.
	deadline := time.Now().Add(2 * time.Second)
	for {
		guard, err := Acquire(dir, 150*time.Millisecond)
		if err == nil {
			guard.Release()
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("Acquire after release returned %v", err)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestDifferentFoldersDoNotLockEachOtherOut(t *testing.T) {
	first, err := Acquire(filepath.Join(t.TempDir(), "one"), time.Second)
	if err != nil {
		t.Fatalf("first Acquire: %v", err)
	}
	defer first.Release()

	second, err := Acquire(filepath.Join(t.TempDir(), "two"), time.Second)
	if err != nil {
		t.Fatalf("second Acquire: %v", err)
	}
	second.Release()
}

func TestReleaseIsSafeToCallTwice(t *testing.T) {
	guard, err := Acquire(t.TempDir(), time.Second)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	guard.Release()
	guard.Release()
}
