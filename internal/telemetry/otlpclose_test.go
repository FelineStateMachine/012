package telemetry

import (
	"net"
	"testing"
	"time"
)

func TestOTLPCloseIsBounded(t *testing.T) {
	c := newCollector(t)
	c.delay = 2 * time.Second
	otlpSetup(t, OTLPConfig{Endpoint: c.srv.URL, Timeout: 100 * time.Millisecond})
	begin := time.Now()
	for range 100 {
		Start("sort").End() // never waits on the network
	}
	if d := time.Since(begin); d > 100*time.Millisecond {
		t.Errorf("100 spans took %v", d)
	}
	Close()
	if d := time.Since(begin); d > 1500*time.Millisecond {
		t.Errorf("Close with a stuck collector took %v", d)
	}
}

// An endpoint that can't be reached (an inherited OTEL_* variable naming
// a collector that's gone) costs nothing at exit once a send has failed.
func TestOTLPCloseSkipsADeadEndpoint(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	dead := "http://" + l.Addr().String()
	l.Close() // nothing listens there now
	otlpSetup(t, OTLPConfig{Endpoint: dead, Timeout: 3 * time.Second})
	for range batchSize {
		Start("sort").End()
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		exp.mu.Lock()
		down := exp.down
		exp.mu.Unlock()
		if down {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the failed send was never noticed")
		}
		time.Sleep(10 * time.Millisecond)
	}
	Start("sort").End()
	begin := time.Now()
	Close()
	if d := time.Since(begin); d > 200*time.Millisecond {
		t.Errorf("Close after a failed send took %v", d)
	}
}
