package ui

import (
	"context"
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

// Agents read without color too: their ◆ on the row header and before
// their name, and a ◇ on each cell a suggestion of theirs would set.
func TestMonochromeAgents(t *testing.T) {
	ls := startLive(t)
	ls.suggest("", "A1", "48")
	ls.agent.Focus(context.Background(), "B2")
	ls.sync()
	ls.sh.turn(func() {
		m := ls.m
		m.frameShare()
		if row := cellText(monoLine(m, gridTop)); !strings.Contains(row, "◇") {
			t.Errorf("row 1 %q", row)
		}
		if row := cellText(monoLine(m, gridTop+1)); !strings.HasPrefix(row, "◆") {
			t.Errorf("row 2's header %q", row)
		}
		if status := cellText(monoLine(m, m.height-1)); !strings.Contains(status, "◆ claude") || !strings.Contains(status, "◇ 1 suggestion") {
			t.Errorf("status line %q", status)
		}
	})
}
