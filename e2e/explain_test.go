package e2e

import (
	"strings"
	"testing"
)

// TestLongErrorOnF1 checks that at 60 columns an error's explanation
// too long for the context line ends in … with an F1 chip, that F1
// shows all of it in a box, and that a key closes the box.
func TestLongErrorOnF1(t *testing.T) {
	s := startWith(t, options{cols: 60, rows: 16})
	s.keys("='Quarterly results by region 26'!A1+1", "<enter>", "<up>")
	s.waitFor("#REF!")
	s.eventually("the explanation cut, with F1", func() bool {
		l := s.line(2)
		return strings.Contains(l, "Unresolved sheet") && strings.Contains(l, "…") && strings.Contains(l, "F1")
	})
	s.keys("<f1>")
	s.waitFor("#REF! in A1")
	s.waitFor("'Quarterly results by")
	s.keys("<esc>")
	s.eventually("the box closed", func() bool { return !strings.Contains(s.screen(), "#REF! in A1") })
	if !strings.Contains(s.line(0), "READY") {
		t.Errorf("mode after the box: %q", s.line(0))
	}
}
