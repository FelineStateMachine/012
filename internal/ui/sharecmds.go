package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/room"
	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/ui/picker"
)

// The commands of a shared workbook: who's here, and handing writing
// over in a one-writer room; what undo says when another's step is in
// the way; and whose change an entry replaced.

func init() {
	unshared := func(m *Model) bool { return m.share.reg == nil }
	register(
		&command{id: "share.who", macro: macroNever, title: "Who's here", desc: "List who shares this workbook and where each is; pick one to go to their cell",
			hidden: unshared, enabled: func(m *Model) bool { return m.shared() }, run: (*Model).openWho},
		&command{id: "share.hand", macro: macroNever, title: "Hand over writing", desc: "Let someone else edit the shared workbook, and follow them",
			hidden: unshared, enabled: func(m *Model) bool {
				seat := m.share.seat
				return seat != nil && seat.Mode() == room.View && seat.Writing() && seat.Others() > 0
			}, run: (*Model).openHandOver},
	)
}

// nbReading are the notebook's commands that change nothing a follower
// in a one-writer room may not: moving about and looking.
var nbReading = map[string]bool{"nb.open": true, "nb.goto": true, "nb.toc": true, "nb.toggle_output": true, "nb.toggle_whole": true,
	"nb.open_output": true, "nb.command_mode": true, "nb.copy": true}

// edits reports whether command c changes the workbook, which a
// follower in a one-writer room may not do.
func edits(c *command) bool {
	if strings.HasPrefix(c.id, "nb.") {
		return !nbReading[c.id]
	}
	return c.macro == macroRecord
}

// openWho lists who shares the workbook, to go to one of them.
func (m *Model) openWho() tea.Cmd {
	var items []picker.Item
	for _, p := range m.share.seat.Peers() {
		where := m.peerWhere(p)
		desc := "Go to " + p.Name + "'s cell"
		detail := where
		switch {
		case p.Writing && m.share.seat.Mode() == room.View:
			detail += ", writing"
		case p.Presence.Editing:
			detail += ", typing"
		}
		items = append(items, picker.Item{Title: p.Name, Name: len(p.Name), Detail: detail, Desc: desc,
			Pick: func() tea.Cmd { m.closeOverlay(); m.goToPeer(p); return nil }})
	}
	pk := m.newPicker("Who's here", "a name", 50, items)
	pk.Action = "go"
	m.openOverlay(pk)
	return nil
}

// peerWhere is where p is, Sheet2!B3, or the notebook they're on.
func (m *Model) peerWhere(p room.Peer) string {
	s := p.Presence.Sheet
	if s == nil || !s.Live() {
		return "arriving"
	}
	if s.IsNotebook() {
		return s.Name()
	}
	return sheet.Qualified(s.Name(), sheet.Rect{From: p.Presence.Cursor, To: p.Presence.Cursor})
}

// goToPeer shows p's sheet with the pointer on their cell.
func (m *Model) goToPeer(p room.Peer) {
	s := p.Presence.Sheet
	if s == nil || !s.Live() || s.Hidden() {
		return
	}
	m.showSheet(s)
	if !s.IsNotebook() {
		m.cur = p.Presence.Cursor
		m.clearSelection()
	}
	m.note = "At " + p.Name + "'s cell"
}

// openHandOver asks whom to hand writing to.
func (m *Model) openHandOver() tea.Cmd {
	var items []picker.Item
	for _, p := range m.share.seat.Peers() {
		items = append(items, picker.Item{Title: p.Name, Name: len(p.Name), Detail: m.peerWhere(p), Desc: "Let " + p.Name + " write; you follow",
			Pick: func() tea.Cmd {
				m.closeOverlay()
				if m.share.seat.HandTo(p.ID) {
					m.note = p.Name + " writes now: you follow"
				}
				return nil
			}})
	}
	pk := m.newPicker("Hand over writing", "a name", 50, items)
	pk.Action = "hand over"
	m.openOverlay(pk)
	return nil
}

// undoBlocked says why the user's undo (or redo) can't go ahead: a later
// change of someone else's changed the same cells.
func (m *Model) undoBlocked(redo bool) bool {
	if m.share.seat == nil {
		return false
	}
	w := m.book()
	check, verb := w.UndoBlocked, "undo "+w.UndoLabel()
	if redo {
		check, verb = w.RedoBlocked, "redo"
	}
	b, blocked := check()
	if !blocked {
		return false
	}
	m.warn = "Can't " + verb + ": " + m.peerName(b.Author) + " has changed it since, and undo takes back only your own changes"
	return true
}

// peerName is the name of the participant with id, or "someone" when
// they've left.
func (m *Model) peerName(id int) string {
	for _, p := range m.share.seat.Peers() {
		if p.ID == id {
			return p.Name
		}
	}
	return "someone who left"
}

// roomLast is the room's latest operation, 0 outside a room.
func (m *Model) roomLast() uint64 {
	if m.share.seat == nil {
		return 0
	}
	return m.share.seat.Last()
}

// overwrote says whose change the entry just made replaced: someone
// else changed the cell while it was being typed. The server's order
// decides, and the last entry stays.
func (m *Model) overwrote() {
	seat := m.share.seat
	if seat == nil {
		return
	}
	for _, mk := range seat.Marks(m.share.entryFrom) {
		if mk.Author != seat.ID() && mk.Sheet == m.entrySheet() && mk.Rect.Contains(m.cur) && mk.Kind != sheet.OpUndo {
			m.warn = "Your entry replaced " + mk.Name + "'s change to " + m.cur.String() + ", made while you typed"
		}
	}
}

// roomOwned has what cmd reports go to the room's keeper when the
// workbook is shared, rather than back to this model, which may have
// left by then.
func (m *Model) roomOwned(cmd tea.Cmd) tea.Cmd {
	seat := m.share.seat
	if seat == nil || cmd == nil {
		return cmd
	}
	return func() tea.Msg {
		if msg := cmd(); msg != nil {
			seat.Post(msg)
		}
		return nil
	}
}
