//go:build stress && !unix

package fileio

import "testing"

// benchLock times without the lock where there is no flock(2).
func benchLock(testing.TB) func() { return func() {} }
