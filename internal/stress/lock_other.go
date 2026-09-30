//go:build !unix

package stress

// Logger is what BenchLock says why it times without the lock through:
// a testing.TB.
type Logger interface {
	Logf(format string, args ...any)
	Fatalf(format string, args ...any)
}

// BenchLock times without the lock where there is no flock(2).
func BenchLock(Logger) func() { return func() {} }
