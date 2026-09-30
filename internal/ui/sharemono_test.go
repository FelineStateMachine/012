package ui

import (
	"strings"
	"testing"

	"github.com/FelineStateMachine/012/internal/room"
)

// Others read without color: their pointer's cell double-underlined,
// their initial on its row's header, their name on the status line,
// and a cell they just changed marked with ▘.
func TestMonochromePeers(t *testing.T) {
	r := newRooms(t, room.Edit)
	ann := r.open("ann", "@mono")
	bob := r.open("bob", "@mono")
	bob.press("<down>", "x", "<enter>", "<right>")
	r.sync()
	ann.sh.turn(func() {
		m := ann.m
		m.frameShare()
		c := cellAt(m, addr("B3"))
		if !every(c, true, func(c monoCell) bool { return c.underline == "2" }) {
			t.Errorf("bob's cell: %+v", c)
		}
		if row := cellText(monoLine(m, gridTop+2)); !strings.HasPrefix(row, "b") {
			t.Errorf("row 3's header %q", row)
		}
		if row := cellText(monoLine(m, gridTop+1)); !strings.Contains(row, "▘x") {
			t.Errorf("row 2 %q", row)
		}
		if !strings.Contains(cellText(monoLine(m, m.height-1)), " bob ") {
			t.Error("bob isn't on the status line")
		}
	})
}
