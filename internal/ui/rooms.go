package ui

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// Shared is a served session's guarded model as Bubble Tea runs it when
// workbooks are shared: every turn the model takes (Init, Update, View)
// holds its room's lock (room.Seat.Do), with its steps made in its own
// name, so sessions act on the workbook one at a time, in the order the
// server takes them. Between turns it joins and leaves rooms, never
// holding two locks. It also holds back input while someone else's step
// is open (a macro they run spans several turns) and hands it on once
// the step ends, and in a one-writer room it takes back what a
// follower changed by a way no check stopped.
type Shared struct {
	g *Guarded
}

// InRooms runs g's model in the rooms it was given (Model.ShareRooms).
func InRooms(g *Guarded) *Shared { return &Shared{g: g} }

// Attach sends the program a roomMsg whenever the model's room changes,
// until ctx ends.
func (s *Shared) Attach(ctx context.Context, p *tea.Program) {
	link := s.g.m.share.link
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-link.kick:
				p.Send(roomMsg{})
			}
		}
	}()
}

// Init implements tea.Model.
func (s *Shared) Init() tea.Cmd {
	var cmd tea.Cmd
	s.turn(func() { cmd = s.g.Init() })
	return tea.Batch(cmd, s.between())
}

// Update implements tea.Model.
func (s *Shared) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	s.turn(func() { cmd = s.update(msg) })
	return s, tea.Batch(cmd, s.between())
}

// View implements tea.Model.
func (s *Shared) View() tea.View {
	var v tea.View
	s.turn(func() { v = s.g.View() })
	return v
}

// update is a turn's Update: input held while another's step is open,
// and handed on once it ends.
func (s *Shared) update(msg tea.Msg) tea.Cmd {
	m := s.g.m
	seat := m.share.seat
	if seat != nil && isInput(msg) {
		if open, who := seat.Book().InStep(); open && who != seat.ID() {
			m.share.held = append(m.share.held, msg)
			return nil
		}
	}
	_, cmd := s.g.Update(msg)
	if _, ok := msg.(roomMsg); !ok || len(m.share.held) == 0 {
		return cmd
	}
	if open, _ := seat.Book().InStep(); open {
		return cmd
	}
	held := m.share.held
	m.share.held = nil
	cmds := []tea.Cmd{cmd}
	for _, in := range held {
		_, c := s.g.Update(in)
		cmds = append(cmds, c)
	}
	return tea.Batch(cmds...)
}

// isInput reports whether msg is the user's input, which waits while
// another's step is open.
func isInput(msg tea.Msg) bool {
	switch msg.(type) {
	case tea.KeyPressMsg, tea.PasteMsg, tea.MouseClickMsg, tea.MouseReleaseMsg, tea.MouseMotionMsg, tea.MouseWheelMsg:
		return true
	}
	return false
}

// turn runs fn holding the model's room, if it has one; after it, the
// others learn where the user is.
func (s *Shared) turn(fn func()) {
	m := s.g.m
	seat := m.share.seat
	if seat == nil {
		fn()
		return
	}
	seat.Do(func(w *sheet.Workbook) {
		w.SetTrace(m.spans)
		before := w.LastOp()
		fn()
		if s.g.crash != nil {
			if open, who := w.InStep(); open && who == seat.ID() {
				w.EndStep() // the crashed session's macro ends here
			}
			return
		}
		if m.share.seat == seat && m.book() == w {
			s.g.m.takeBack(w, before)
			seat.Move(m.presence())
		}
	})
}

// takeBack undoes the steps a follower made in a one-writer room, which
// only the writer edits.
func (m *Model) takeBack(w *sheet.Workbook, before uint64) {
	seat := m.share.seat
	if seat.Writing() {
		return
	}
	ops, _ := w.OpsSince(before)
	for _, o := range ops {
		if o.Author == seat.ID() && o.Kind == sheet.OpDo {
			w.Undo()
			m.mayEdit() // says why
		}
	}
}

// between joins and leaves rooms after a turn: leaving the room of a
// workbook the model no longer shows, and joining the one it asked for.
func (s *Shared) between() tea.Cmd {
	m := s.g.m
	if s.g.crash != nil {
		return nil
	}
	if seat := m.share.seat; seat != nil && (m.share.want != nil || m.book() != seat.Book()) {
		m.share.seat = nil
		m.leftRoom(seat.Leave())
	}
	req := m.share.want
	if req == nil {
		return nil
	}
	m.share.want = nil
	seat, created := m.share.reg.Join(req.key, m.share.link, func() *sheet.Workbook {
		if req.book == nil {
			return sheet.New().Book()
		}
		return req.book
	})
	var cmd tea.Cmd
	seat.Do(func(*sheet.Workbook) {
		cmd = s.g.do(func() tea.Cmd { return m.enterRoom(seat, created, req) })
		seat.Move(m.presence())
	})
	return cmd
}

// leftRoom lets go of what the model shared in a room it left: the last
// one out stops what the room ran; otherwise it goes on without this
// model.
func (m *Model) leftRoom(last bool) {
	if last {
		m.stopCells()
		return
	}
	m.nb.runs = &nbRuns{}
	for _, f := range m.follow.by {
		m.dropFollower(f)
	}
}

// Model is the model run.
func (s *Shared) Model() *Model { return s.g.m }
