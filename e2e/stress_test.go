package e2e

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// TestStressKeyLatency imports an 8192x26 CSV with telemetry on, then
// measures key press to screen through a real terminal: arrow keys, and
// entries that recalculate 200 SUMs over a whole column. It checks the
// latencies against a generous bound and that the event log recorded the
// import, the recalcs and the frames. Slow, so only with STRESS=1 (make
// stress-e2e); O12_E2E_LOG keeps the log, e.g. for the observability
// stack.
func TestStressKeyLatency(t *testing.T) {
	if os.Getenv("STRESS") == "" {
		t.Skip("set STRESS=1 to run")
	}
	dir := t.TempDir()
	var b strings.Builder
	for r := range 8192 {
		for c := range 26 {
			if c > 0 {
				b.WriteByte(',')
			}
			fmt.Fprintf(&b, "%d.%02d", (r*31+c*7)%1000, (r+c)%100)
		}
		b.WriteByte('\n')
	}
	writeText(t, filepath.Join(dir, "big.csv"), b.String())
	log := os.Getenv("O12_E2E_LOG")
	if log == "" {
		log = filepath.Join(dir, "events.jsonl")
	}
	s := startWith(t, options{dir: dir, cols: 200, rows: 60, env: []string{"O12_LOG=" + log}}, "big.csv")
	s.waitFor("Imported big.csv (8,192 rows)")

	// 200 formulas summing column A, in column AB (off screen).
	s.keys("<f5>", "AB1", "<enter>")
	s.waitForName("AB1")
	for range 200 {
		s.keys("=SUM(A1:A8192)", "<enter>")
	}
	s.keys("<f5>", "A1", "<enter>")
	s.waitForName("A1")

	arrows := s.latencies(60, func(i int) (string, string) {
		if i%2 == 0 {
			return "<down>", "A2"
		}
		return "<up>", "A1"
	})
	// Enter commits a number typed into A1, recalculating the 200 SUMs,
	// and moves down; the next round goes back up first.
	edits := s.latencies(20, func(i int) (string, string) {
		if i > 0 {
			s.keys("<up>")
			s.waitForName("A1")
		}
		s.keys(fmt.Sprint(i%10))
		s.waitForEntry(fmt.Sprint(i%10))
		return "<enter>", "A2"
	})
	report(t, "arrow", arrows)
	report(t, "edit", edits)
	if p := pct(arrows, 95); p > 250*time.Millisecond {
		t.Errorf("arrow key p95 %v", p)
	}

	s.keys("<ctrl+q>")
	s.waitFor("You have unsaved changes.")
	s.keys("d")
	s.waitExit()
	counts := events(t, log)
	for _, ev := range []string{"start", "import", "recalc", "frames"} {
		if counts[ev] == 0 {
			t.Errorf("no %s events in %s: %v", ev, log, counts)
		}
	}
	t.Logf("events: %v", counts)
}

// latencies presses n keys, each returned by next with the name box
// text that shows it has been handled, and times each from the write
// to the pty until the screen shows it.
func (s *session) latencies(n int, next func(i int) (key, want string)) []time.Duration {
	s.t.Helper()
	var out []time.Duration
	for i := range n {
		key, want := next(i)
		start := time.Now()
		s.keys(key)
		deadline := start.Add(waitTimeout)
		for s.nameBox() != want {
			if time.Now().After(deadline) {
				s.t.Fatalf("waiting for %s after %s; screen:\n%s", want, key, s.screen())
			}
			time.Sleep(200 * time.Microsecond)
		}
		out = append(out, time.Since(start))
	}
	return out
}

// nameBox is the name box's text.
func (s *session) nameBox() string {
	l := s.line(barLine)
	return strings.TrimSpace(l[:min(barX, len(l))])
}

func pct(ds []time.Duration, p int) time.Duration {
	ds = slices.Clone(ds)
	slices.Sort(ds)
	return ds[max((len(ds)*p+99)/100-1, 0)]
}

func report(t *testing.T, what string, ds []time.Duration) {
	t.Logf("%s: n=%d p50=%v p95=%v max=%v", what, len(ds), pct(ds, 50), pct(ds, 95), pct(ds, 100))
}

// events counts the event log's lines by event name.
func events(t *testing.T, path string) map[string]int {
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	counts := map[string]int{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var ev struct{ Msg string }
		if err := json.Unmarshal(sc.Bytes(), &ev); err != nil {
			t.Fatalf("log line %q: %v", sc.Text(), err)
		}
		counts[ev.Msg]++
	}
	return counts
}
