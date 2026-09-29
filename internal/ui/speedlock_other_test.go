//go:build !unix

package ui

import "testing"

// benchLock is a no-op where there is no flock: the speed gate times
// without waiting for other runs.
func benchLock(*testing.T) func() { return func() {} }
