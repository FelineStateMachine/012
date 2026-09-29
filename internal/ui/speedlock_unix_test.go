//go:build unix

package ui

import (
	"errors"
	"io/fs"
	"os"
	"syscall"
	"testing"
	"time"
)

// benchLockPath is the lock timed runs on this machine share, so their
// timings don't overlap. Two conventions meet there: a file locked with
// flock(2), as `flock /tmp/012-bench.lock <command>` takes it where the
// flock command exists, and a directory made with mkdir and removed
// after, which needs no flock (macOS has none). The speed gate takes
// whichever the path holds, a directory when it holds nothing.
const benchLockPath = "/tmp/012-bench.lock"

// benchLockWait is how long the gate waits for a directory lock before
// timing without it (the lock may be stale, left by a run that was
// killed).
const benchLockWait = 10 * time.Minute

// benchLock waits for the lock and returns its release.
func benchLock(t *testing.T) func() {
	t.Helper()
	deadline := time.Now().Add(benchLockWait)
	logged := false
	for {
		err := os.Mkdir(benchLockPath, 0o755)
		if err == nil {
			return func() { os.Remove(benchLockPath) }
		}
		info, serr := os.Stat(benchLockPath)
		switch {
		case !errors.Is(err, fs.ErrExist) || serr != nil:
			t.Logf("timing without the lock: %v", err)
			return func() {}
		case info.Mode().IsRegular():
			return flockFile(t)
		case time.Now().After(deadline):
			t.Logf("timing without the lock: %s held for %v", benchLockPath, benchLockWait)
			return func() {}
		}
		if !logged {
			t.Logf("waiting for %s", benchLockPath)
			logged = true
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// flockFile takes the lock as a file, with flock(2).
func flockFile(t *testing.T) func() {
	f, err := os.OpenFile(benchLockPath, os.O_RDWR, 0)
	if err != nil {
		t.Logf("timing without the lock: %v", err)
		return func() {}
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		t.Fatalf("locking %s: %v", benchLockPath, err)
	}
	return func() {
		syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		f.Close()
	}
}
