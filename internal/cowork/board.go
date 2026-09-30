package cowork

import (
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/FelineStateMachine/012/internal/room"
	"github.com/FelineStateMachine/012/internal/sheet"
)

// BoardKey is the room value (room.Seat.Value) holding its Board.
const BoardKey = "agents"

// BoardOf is the Board of seat's room, made the first time. Call it in
// a turn.
func BoardOf(seat *room.Seat) *Board {
	return seat.Value(BoardKey, func() any { return &Board{} }).(*Board)
}

// Board is what a room shares of its agents: their suggestions, their
// questions to the people, and what the people let them do.
type Board struct {
	// Scope is what agents may change, as the person who invited them
	// chose; the whole workbook in 012 serve.
	Scope Scope
	// Direct lets agents' changes be made at once, as their own steps,
	// rather than suggested ("Let agents edit directly").
	Direct bool
	Grants Grants
	// Host is the seat of the person who invited the agents, who is
	// asked their questions; 0 in 012 serve, where any person is.
	Host int

	list    []*Suggestion // oldest first
	next    int
	asks    []*Ask
	nextAsk int
}

// CellState is what became of one cell of a suggestion.
type CellState int

const (
	CellPending CellState = iota
	CellAccepted
	CellRejected
)

// Suggestion is one of the agents's change waiting for a person: the proposal,
// who made it and why, and what became of each of its cells.
type Suggestion struct {
	ID      int
	Author  int    // the agent's seat
	Agent   string // its name
	Color   int
	Message string
	At      time.Time
	*sheet.Proposal
	// States are its cells', in the order of Proposal.Cells; a change
	// made whole only has one, for all of it.
	States []CellState
	// Failed is why accepting it failed, when it did.
	Failed   string
	reported bool
}

// Pending reports whether any of it waits for the person.
func (s *Suggestion) Pending() bool { return slices.Contains(s.States, CellPending) }

// Count is how many of its cells are in state st.
func (s *Suggestion) Count(st CellState) int {
	n := 0
	for _, x := range s.States {
		if x == st {
			n++
		}
	}
	return n
}

// Outcome is what became of it in words: pending, accepted, accepted in
// part or rejected.
func (s *Suggestion) Outcome() string {
	switch acc, rej := s.Count(CellAccepted), s.Count(CellRejected); {
	case s.Pending():
		return "pending"
	case rej == 0:
		return "accepted"
	case acc == 0:
		return "rejected"
	}
	return "accepted in part"
}

// Add puts a suggestion on the board and numbers it.
func (b *Board) Add(s *Suggestion) int {
	b.next++
	s.ID = b.next
	if s.At.IsZero() {
		s.At = time.Now()
	}
	n := len(s.Cells)
	if s.Whole() || n == 0 {
		n = 1
	}
	s.States = make([]CellState, n)
	b.list = append(b.list, s)
	return s.ID
}

// Pending are the suggestions still waiting, oldest first.
func (b *Board) Pending() []*Suggestion {
	var out []*Suggestion
	for _, s := range b.list {
		if s.Pending() {
			out = append(out, s)
		}
	}
	return out
}

// Get is the suggestion numbered id.
func (b *Board) Get(id int) *Suggestion {
	for _, s := range b.list {
		if s.ID == id {
			return s
		}
	}
	return nil
}

// Of are the suggestions agent's seat made, oldest first.
func (b *Board) Of(author int) []*Suggestion {
	var out []*Suggestion
	for _, s := range b.list {
		if s.Author == author {
			out = append(out, s)
		}
	}
	return out
}

// At is the newest pending suggestion setting the cell a of the sheet
// named name, and the index of the cell in it.
func (b *Board) At(name string, a sheet.Addr) (*Suggestion, int, bool) {
	for i := len(b.list) - 1; i >= 0; i-- {
		s := b.list[i]
		for j, c := range s.Cells {
			if c.Sheet == name && c.At == a && s.state(j) == CellPending {
				return s, j, true
			}
		}
	}
	return nil, 0, false
}

// state is cell i's state; all of a whole change's cells are its one.
func (s *Suggestion) state(i int) CellState {
	if s.Whole() {
		return s.States[0]
	}
	return s.States[i]
}

// ErrGone is accepting or rejecting a suggestion no longer waiting.
var ErrGone = errors.New("that suggestion isn't waiting any more")

// Accept makes the cells of suggestion id that are picked (nil: all it
// has pending) on w, as one step in the agent's name, and marks them
// accepted. A change made whole only is accepted whole.
func (b *Board) Accept(w *sheet.Workbook, id int, cells []int) error {
	s := b.Get(id)
	if s == nil || !s.Pending() {
		return ErrGone
	}
	if s.Whole() && cells != nil {
		return sheet.ErrWhole
	}
	pick := b.picked(s, cells)
	author := w.Author()
	w.SetAuthor(s.Author)
	err := s.Apply(w, func(i int) bool { return pick[i] })
	w.SetAuthor(author)
	if err != nil {
		s.Failed = err.Error()
		return fmt.Errorf("accepting %s's suggestion: %w", s.Agent, err)
	}
	b.mark(s, pick, CellAccepted)
	return nil
}

// Reject marks the cells of suggestion id picked (nil: all it has
// pending) rejected, changing nothing.
func (b *Board) Reject(id int, cells []int) error {
	s := b.Get(id)
	if s == nil || !s.Pending() {
		return ErrGone
	}
	b.mark(s, b.picked(s, cells), CellRejected)
	return nil
}

// picked are the pending cells of s among cells, all of them for nil.
func (b *Board) picked(s *Suggestion, cells []int) map[int]bool {
	pick := map[int]bool{}
	for i := range s.Cells {
		if s.state(i) == CellPending && (cells == nil || slices.Contains(cells, i)) {
			pick[i] = true
		}
	}
	return pick
}

// mark sets the picked cells' state.
func (b *Board) mark(s *Suggestion, pick map[int]bool, st CellState) {
	if s.Whole() || len(s.Cells) == 0 {
		s.States[0] = st
		return
	}
	for i := range pick {
		s.States[i] = st
	}
}

// Resolved are the agent's suggestions settled since it last asked,
// which it hasn't been told of.
func (b *Board) Resolved(author int) []*Suggestion {
	var out []*Suggestion
	for _, s := range b.list {
		if s.Author == author && !s.Pending() && !s.reported {
			s.reported = true
			out = append(out, s)
		}
	}
	return out
}

// Notice is what the agent is told of a settled suggestion.
func (s *Suggestion) Notice() string {
	what := fmt.Sprintf("Suggestion %d (%s) was %s", s.ID, s.Label, s.Outcome())
	if n := len(s.Cells); n > 1 && s.Outcome() == "accepted in part" {
		what += fmt.Sprintf(": %d of its %d cells", s.Count(CellAccepted), n)
	}
	if s.Failed != "" {
		what += "; accepting it failed: " + s.Failed
	}
	return what
}
