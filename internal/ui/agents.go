package ui

import (
	"fmt"
	"os/user"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/cowork"
	"github.com/FelineStateMachine/012/internal/room"
	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/ui/picker"
	"github.com/FelineStateMachine/012/internal/ui/review"
)

// Live mode (docs/agents/live.md): the agent works in this session as a
// participant of its room. A local session invited to (012 --listen,
// File > Invite the agent) puts its workbook in a room of its own, with
// its person the first seat, and listens on a socket for 012 mcp
// --attach (package cowork); 012 serve's rooms are joined the same way.
// The agent's changes wait on the room's board as suggestions, marked
// in the grid (agentdraw.go) and settled in the review panel (package
// review); its questions and requests show on the context line
// (agentask.go).

// agentState is the session's part in live mode.
type agentState struct {
	listener *cowork.Listener // the session's socket, nil when not listening
	scope    cowork.Scope     // what the person chose, for the board once the room is joined
	asking   *cowork.Ask      // the agent's question on the context line
	frame    agentFrame       // what the frame draws of suggestions: agentdraw.go
}

func init() {
	register(
		&command{id: "agent.invite", macro: macroNever, title: "Invite an agent",
			desc:   "Let an agent on this computer work in this workbook with you (012 mcp --attach): pick what it may change",
			hidden: func(m *Model) bool { return m.share.reg != nil && !m.share.local }, run: (*Model).openInvite},
		&command{id: "agent.review", macro: macroNever, title: "Review suggestions",
			desc:   "List the agents' suggestions to accept or reject, whole or cell by cell",
			hidden: noRoom, enabled: func(m *Model) bool { return len(m.board().Pending()) > 0 }, run: (*Model).openReview},
		&command{id: "agent.direct", macro: macroNever, title: "Let agents edit directly",
			desc:   "Make agents' changes at once, as their own steps, rather than as suggestions to accept",
			hidden: noRoom, checked: func(m *Model) bool { return m.board().Direct }, run: (*Model).toggleDirect},
		&command{id: "agent.stop", macro: macroNever, title: "Stop inviting agents",
			desc:   "Close this session's socket: agents attached leave, their suggestions stay",
			hidden: func(m *Model) bool { return m.agents.listener == nil }, run: func(m *Model) tea.Cmd { m.StopListening(); return nil }},
	)
	keymap["ctrl+alt+a"] = "agent.review"
}

// noRoom hides what needs a room: live mode's commands before the agent
// is invited.
func noRoom(m *Model) bool { return m.share.seat == nil }

// board is the room's board of agents; nil outside a room. Call it in a
// turn.
func (m *Model) board() *cowork.Board {
	if m.share.seat == nil {
		return &cowork.Board{}
	}
	return cowork.BoardOf(m.share.seat)
}

// openInvite asks what the agent may change.
func (m *Model) openInvite() tea.Cmd {
	whole := picker.Item{Title: "The whole workbook", Desc: "It may change anything, suggested for you to accept unless you let agents edit directly",
		Pick: m.inviting(cowork.Scope{Kind: cowork.Workbook})}
	items := []picker.Item{whole}
	if !m.sheet.IsNotebook() {
		r := m.selection()
		items = append(items,
			picker.Item{Title: "This sheet", Detail: m.sheet.Name(), Desc: "It may change " + m.sheet.Name() + " only",
				Pick: m.inviting(cowork.Scope{Kind: cowork.OneSheet, Sheet: m.sheet})},
			picker.Item{Title: "The selection", Detail: r.String(), Desc: "It may change the cells of " + r.String() + " only",
				Pick: m.inviting(cowork.Scope{Kind: cowork.OneRange, Sheet: m.sheet, Range: r})})
	}
	items = append(items, picker.Item{Title: "Read only", Desc: "It reads the workbook, points and asks you, and changes nothing",
		Pick: m.inviting(cowork.Scope{Kind: cowork.ReadOnly})})
	for i := range items {
		items[i].Name = len(items[i].Title)
	}
	pk := m.newPicker("Invite an agent", "what it may change", 56, items)
	pk.Action = "invite"
	m.openOverlay(pk)
	return nil
}

// inviting is a picker's choice of scope.
func (m *Model) inviting(s cowork.Scope) func() tea.Cmd {
	return func() tea.Cmd {
		m.closeOverlay()
		m.invite(s)
		return nil
	}
}

// invite lets agents in with scope s: at once when the session already
// listens, else once it has joined its room (enterLocal).
func (m *Model) invite(s cowork.Scope) {
	m.agents.scope = s
	if m.share.seat != nil {
		m.board().Scope = s
		m.share.seat.Touch()
		if m.agents.listener == nil && m.share.local {
			m.startListening(m.share.seat.Key())
		}
		m.note = "Agents may change " + s.String()
		return
	}
	m.share.reg = room.NewRegistry(room.Edit)
	m.share.link = &peerLink{name: userName(), kick: make(chan struct{}, 1)}
	m.share.local = true
	key := "@local"
	if m.filename != "" {
		if k, ok := m.roomKey(m.filename); ok {
			key = k
		}
	}
	m.share.want = &joinReq{key: key, name: m.filename, book: m.book(), local: true}
}

// Listen has the session listen for agents from the start, letting
// them suggest changes to the whole workbook: 012 --listen.
func (m *Model) Listen() { m.invite(cowork.Scope{Kind: cowork.Workbook}) }

// StopListening closes the session's socket; agents attached leave.
func (m *Model) StopListening() {
	if m.agents.listener == nil {
		return
	}
	m.agents.listener.Close()
	m.agents.listener = nil
	m.note = "Stopped inviting agents"
}

// userName is who the person is to agents.
func userName() string {
	if u, err := user.Current(); err == nil && u.Username != "" {
		return filepath.Base(u.Username) // DOMAIN\name on Windows
	}
	return "you"
}

// enterLocal takes the local session's own workbook into its room, as
// it is: nothing reopens, unsaved changes stay unsaved, and the steps
// made so far stay the person's to undo.
func (m *Model) enterLocal(seat *room.Seat, req *joinReq) tea.Cmd {
	m.share.seat, m.share.seen = seat, seat.Last()
	seat.Book().Adopt(seat.ID())
	runs := m.nb.runs
	m.nb.runs = seat.Value("nb", func() any { return runs }).(*nbRuns)
	src := m.src.run
	if src == nil {
		src = &sourceHost{}
	}
	m.src.run = seat.Value("sources", func() any { return src }).(*sourceHost)
	saved := room.Saved{State: m.saved, Outputs: m.nb.saved, Name: m.filename, Stamp: m.disk}
	if m.filename != "" {
		saved.Key = req.key
	}
	seat.SetSaved(saved)
	b := cowork.BoardOf(seat)
	b.Host, b.Scope = seat.ID(), m.agents.scope
	m.startListening(req.key)
	return m.waitRoom()
}

// startListening opens the session's socket for agents joining the room
// with key.
func (m *Model) startListening(key string) {
	name := strings.TrimSuffix(filepath.Base(m.filename), filepath.Ext(m.filename))
	if m.filename == "" {
		name = "Untitled"
	}
	l, err := cowork.Listen(cowork.Options{Registry: m.share.reg, Room: func(string) (string, error) { return key, nil },
		Kind: "session", Workbook: name, Version: m.prefs.Version})
	if err != nil {
		m.warn = "Can't listen for agents: " + err.Error()
		return
	}
	m.agents.listener = l
	m.note = fmt.Sprintf("Agents may change %s: attach with 012 mcp --attach %s", m.agents.scope, name)
}

// waitRoom waits for the room to change, in a local session, where no
// server sends the program its roomMsg.
func (m *Model) waitRoom() tea.Cmd {
	if !m.share.local || m.share.still || m.share.waiting {
		return nil
	}
	m.share.waiting = true
	kick := m.share.link.kick
	return func() tea.Msg {
		<-kick
		return roomMsg{}
	}
}

// roomKicked catches up with the room, and waits for it again.
func (m *Model) roomKicked() tea.Cmd {
	m.share.waiting = false
	return tea.Batch(m.roomChanged(), m.waitRoom())
}

// leftLocal stops listening when the session leaves its room: it opened
// another file, and agents work on the one they were invited to.
func (m *Model) leftLocal() {
	if m.agents.listener != nil {
		m.StopListening()
		m.note = "Stopped inviting agents: they work on the workbook they were invited to"
	}
}

// toggleDirect turns direct edits by agents on or off.
func (m *Model) toggleDirect() tea.Cmd {
	if m.share.seat == nil {
		return nil
	}
	b := m.board()
	b.Direct = !b.Direct
	m.share.seat.Touch()
	if b.Direct {
		m.note = "Agents' changes are made at once, as their own steps: their undo takes them back"
	} else {
		m.note = "Agents' changes are suggestions for you to accept"
	}
	return nil
}

// openReview opens the suggestions panel.
func (m *Model) openReview() tea.Cmd {
	m.openOverlay(review.New(m.host()))
	return nil
}

// Suggestions are the board's, for the panel.
func (h host) Suggestions() []review.Item {
	var out []review.Item
	for _, s := range h.m.board().Pending() {
		it := review.Item{ID: s.ID, Agent: s.Agent, Color: s.Color, Label: s.Label, Message: s.Message, Whole: s.Whole(),
			Other: s.Other, Total: len(s.Cells), Failed: s.Failed}
		for i, c := range s.Cells {
			if s.Whole() || s.States[i] == cowork.CellPending {
				it.Cells = append(it.Cells, review.Cell{Index: i, Sheet: c.Sheet, At: c.At, Was: c.Was, Now: c.Now, Look: c.Look})
			}
		}
		out = append(out, it)
	}
	return out
}

func (h host) Accept(id int, cells []int) { h.m.settle(id, cells, true) }
func (h host) Reject(id int, cells []int) { h.m.settle(id, cells, false) }

// Reveal shows the cell a suggestion sets.
func (h host) Reveal(name string, a sheet.Addr) {
	if s := h.m.book().Lookup(name); s != nil && !s.Hidden() {
		h.m.showSheet(s)
		h.m.cur = a
		h.m.clearSelection()
	}
}

// settle accepts or rejects cells of suggestion id (nil: all it has
// waiting). What's accepted is one step, the agent's.
func (m *Model) settle(id int, cells []int, accept bool) {
	b, seat := m.board(), m.share.seat
	s := b.Get(id)
	if s == nil || seat == nil {
		return
	}
	what := "all of"
	if cells != nil && !s.Whole() {
		what = fmt.Sprintf("%d of %d cells of", len(cells), len(s.Cells))
	}
	if !accept {
		b.Reject(id, cells)
		seat.Touch()
		m.note = "Rejected " + what + " " + s.Agent + "'s suggestion: " + s.Label
		return
	}
	if !m.mayEdit() {
		return
	}
	if err := b.Accept(m.book(), id, cells); err != nil {
		m.warn = err.Error()
		return
	}
	seat.Touch()
	m.note = "Accepted " + what + " " + s.Agent + "'s suggestion: " + s.Label + " (" + s.Agent + "'s to undo)"
}

// agentRunCell runs the notebook cell the agent asked for, once allowed
// (cowork.Agent.RunCell), as Run does.
func (m *Model) agentRunCell(rc cowork.RunCell) tea.Cmd {
	s := m.book().Lookup(rc.Notebook)
	if s == nil || !s.IsNotebook() || rc.Cell >= len(s.NotebookCells()) {
		return nil
	}
	m.note = fmt.Sprintf("%s runs cell %d of %s", rc.Agent, rc.Cell+1, rc.Notebook)
	return m.runCells(s, rc.Cell, s.NotebookCells())
}
