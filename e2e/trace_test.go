package e2e

import (
	"strings"
	"testing"
)

// Tracing stays on as the pointer moves, lists what's off screen, goes
// to one from a list, and Evaluate formula steps into a reference and
// back out.
func TestTraceAndEvaluate(t *testing.T) {
	t.Parallel()
	s := start(t, "")
	s.keys("<ctrl+g>", "C1", "<enter>")
	s.keys("2", "<enter>", "=C1*3", "<enter>", "=C2+C1", "<enter>")
	s.keys("<ctrl+g>", "C40", "<enter>", "=C3", "<enter>")
	s.keys("<ctrl+g>", "C2", "<enter>")
	s.waitForBar("C2", "=C1*3")

	s.keys("<alt+;>")
	s.waitFor("C2 reads C1↑; read by C3")
	s.keys("<down>")
	s.waitFor("C3 reads C2, C1↑; read by C40↓")

	// Go to an off-screen dependent from the list.
	s.keys("<alt+'>")
	s.waitFor("Precedents and dependents of C3")
	s.keys("C40", "<enter>")
	s.waitForName("C40")
	s.waitFor("C40 reads C3")

	// Evaluate C40: step into C3, compute it, step out.
	s.keys("<alt+=>")
	s.waitFor("Evaluate C40")
	s.waitFor("Next  C3 = 8")
	s.keys("<right>")
	s.waitFor("Evaluate C40 › C3")
	s.keys("<enter>", "<enter>", "<enter>")
	s.waitFor("Value  8")
	s.keys("<left>")
	s.waitFor("Value  8")
	s.eventually("back at C40", func() bool { return !strings.Contains(s.screen(), "›") })
	s.keys("<esc>")
	s.eventually("closed", func() bool { return strings.Contains(s.line(0), "READY") })

	s.keys("<alt+;>")
	s.eventually("tracing off", func() bool { return !strings.Contains(s.line(contextLine), "reads") })
}
