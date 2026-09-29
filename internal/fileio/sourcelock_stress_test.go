//go:build stress && unix

package fileio

import (
	"errors"
	"io/fs"
	"os"
	"syscall"
	"testing"
	"time"
)

// benchLockPath is the lock timed runs on this machine share, as the
// speed gate takes it (internal/ui/speedlock_unix_test.go): a file
// locked with flock(2), or a directory made and removed after where
// there is no flock command (macOS).
const benchLockPath = "/tmp/012-bench.lock"

// benchLock waits up to ten minutes for the lock and returns its
// release; past that, or when it can't be taken, it times without it.
func benchLock(tb testing.TB) func() {
	deadline := time.Now().Add(10 * time.Minute)
	for {
		err := os.Mkdir(benchLockPath, 0o755)
		if err == nil {
			return func() { os.Remove(benchLockPath) }
		}
		info, serr := os.Stat(benchLockPath)
		switch {
		case !errors.Is(err, fs.ErrExist) || serr != nil:
			tb.Logf("timing without the lock: %v", err)
			return func() {}
		case info.Mode().IsRegular():
			return flockBench(tb)
		case time.Now().After(deadline):
			tb.Logf("timing without the lock: %s held too long", benchLockPath)
			return func() {}
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// flockBench takes the lock as a file, with flock(2).
func flockBench(tb testing.TB) func() {
	f, err := os.OpenFile(benchLockPath, os.O_RDWR, 0)
	if err != nil {
		tb.Logf("timing without the lock: %v", err)
		return func() {}
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		tb.Fatalf("locking %s: %v", benchLockPath, err)
	}
	return func() {
		syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		f.Close()
	}
}
