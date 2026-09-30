package ui

import (
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/room"
	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/ui/theme"
)

// Drawing who else is in a shared workbook, and what they changed: each
// other participant's pointer is their cell in their color,
// double-underlined, with their initial in the row header; a cell they
// changed in the last markFresh has a ▘ in its corner in their color;
// the status line lists who's here, and the context line says who is on
// the active cell or last changed it. Colors and cues are theme.Peer.

// markFresh is how long a cell another changed stays marked.
const markFresh = 30 * time.Second

// shareFrame is what a frame draws of the others, worked out once as it
// starts: their pointers on the sheet shown, and the fresh marks.
type shareFrame struct {
	peers []room.Peer
	here  []room.Peer // on the sheet shown
	fresh []room.Mark // others' changes on the sheet shown, newest last
}

// shareTickMsg redraws once marks have gone stale.
type shareTickMsg struct{}

// frameShare works out the others for the frame about to be drawn.
func (m *Model) frameShare() {
	f := &m.share.frame
	f.peers, f.here, f.fresh = nil, f.here[:0], f.fresh[:0]
	m.frameAgents()
	seat := m.share.seat
	if seat == nil {
		return
	}
	f.peers = seat.Peers()
	for _, p := range f.peers {
		if p.Presence.Sheet == m.sheet {
			f.here = append(f.here, p)
		}
	}
	now := time.Now()
	for _, mk := range seat.Marks(0) {
		if mk.Author != seat.ID() && mk.Sheet == m.sheet && now.Sub(mk.At) < markFresh && mk.Kind != sheet.OpUndo {
			f.fresh = append(f.fresh, mk)
		}
	}
}

// peerAt is the other whose pointer is on a, if one is.
func (m *Model) peerAt(a sheet.Addr) (room.Peer, bool) {
	for _, p := range m.share.frame.here {
		if p.Presence.Cursor == a {
			return p, true
		}
	}
	return room.Peer{}, false
}

// peerRole is the role of a cell under another's pointer.
func (m *Model) peerRole(a sheet.Addr) (*lipgloss.Style, bool) {
	p, ok := m.peerAt(a)
	if !ok {
		return nil, false
	}
	st := m.th.PeerCell(p.Color)
	return &st, true
}

// freshMark is the newest of others' fresh changes covering a.
func (m *Model) freshMark(a sheet.Addr) (room.Mark, bool) {
	f := m.share.frame.fresh
	for i := len(f) - 1; i >= 0; i-- {
		if f[i].Rect.Contains(a) {
			return f[i], true
		}
	}
	return room.Mark{}, false
}

// changedMark draws the mark of a cell another changed lately over the
// first column of its text.
func (m *Model) changedMark(text string, a sheet.Addr, base lipgloss.Style, colored bool) string {
	mk, ok := m.freshMark(a)
	if !ok || m.sheet.ColWidth(a.Col) < 2 {
		return text
	}
	mark := m.th.PeerMark[mk.Color%theme.Peers]
	if colored {
		mark = base
	}
	return mark.Render("▘") + ansi.Cut(text, 1, m.sheet.ColWidth(a.Col))
}

// rowTags are the initials of those whose pointer is on row, as chips
// in their colors, and how many columns they take.
func (m *Model) rowTags(row int) (string, int) {
	var b strings.Builder
	n := 0
	for _, p := range m.share.frame.here {
		if p.Presence.Cursor.Row != row || n >= 2 {
			continue
		}
		b.WriteString(m.th.Peer[p.Color%theme.Peers].Render(peerInitial(p.Name, p.Agent)))
		n++
	}
	return b.String(), n
}

// initial is a name's first letter.
func initial(name string) string {
	for _, r := range name {
		return string(r)
	}
	return "?"
}

// peersStatus is the status line's part about the others: their names
// in their colors, and in a one-writer room who writes.
func (m *Model) peersStatus() string {
	peers := m.share.frame.peers
	if len(peers) == 0 {
		return m.agentsStatus()
	}
	var b strings.Builder
	b.WriteString("  ")
	for i, p := range peers {
		if i == 3 {
			b.WriteString(m.th.Muted.Render(" +" + strconv.Itoa(len(peers)-3)))
			break
		}
		if i > 0 {
			b.WriteString(" ")
		}
		b.WriteString(m.th.Peer[p.Color%theme.Peers].Render(" " + peerLabel(p.Name, p.Agent) + " "))
	}
	if seat := m.share.seat; seat.Mode() == room.View {
		if seat.Writing() {
			b.WriteString(m.th.Muted.Render("  you write"))
		} else {
			b.WriteString(m.th.Muted.Render("  " + seat.Writer() + " writes"))
		}
	}
	return b.String() + m.agentsStatus()
}

// shareLine is the context line's word on the active cell: who else is
// on it, or who changed it lately.
func (m *Model) shareLine() string {
	if m.share.seat == nil {
		return ""
	}
	if line := m.agentLine(); line != "" {
		return line
	}
	if p, ok := m.peerAt(m.cur); ok {
		what := " is here"
		if p.Presence.Editing {
			what = " is typing here"
		}
		return m.th.Peer[p.Color%theme.Peers].Render(" "+p.Name+" ") + m.th.Muted.Render(what)
	}
	if mk, ok := m.freshMark(m.cur); ok {
		ago := time.Since(mk.At).Round(time.Second)
		return m.th.Muted.Render("Changed by ") + m.th.Peer[mk.Color%theme.Peers].Render(" "+mk.Name+" ") +
			m.th.Muted.Render(" "+ago.String()+" ago: "+mk.Label)
	}
	return ""
}

// shareTick redraws once the freshest mark has gone stale, while there
// are marks.
func (m *Model) shareTick() tea.Cmd {
	if len(m.share.frame.fresh) == 0 || m.share.ticking || m.share.still {
		return nil
	}
	m.share.ticking = true
	return tea.Tick(markFresh, func(time.Time) tea.Msg { return shareTickMsg{} })
}
