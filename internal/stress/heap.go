package stress

import (
	"runtime"
	"runtime/metrics"
	"time"
)

// SamplePeak samples the heap's live objects every 100 us until the
// function it returns is called, which returns the most seen.
func SamplePeak() func() uint64 {
	sample := []metrics.Sample{{Name: "/memory/classes/heap/objects:bytes"}}
	read := func() uint64 {
		metrics.Read(sample)
		return sample[0].Value.Uint64()
	}
	done := make(chan struct{})
	peak := make(chan uint64)
	go func() {
		hi := read()
		t := time.NewTicker(100 * time.Microsecond)
		defer t.Stop()
		for {
			select {
			case <-done:
				peak <- max(hi, read())
				return
			case <-t.C:
				hi = max(hi, read())
			}
		}
	}()
	return func() uint64 {
		close(done)
		return <-peak
	}
}

// LiveHeap is the heap in use after a collection.
func LiveHeap() uint64 {
	runtime.GC()
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return ms.HeapAlloc
}
