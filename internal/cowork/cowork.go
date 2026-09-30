// Package cowork is live mode (docs/agents/live.md): the agent working in
// a running 012 session, as one of the participants of the session's
// room (package room). A session listens on a socket only its user can
// reach (Listen); 012 mcp --attach connects to it (Attach) and carries
// the agent's MCP messages to a server running in the session, whose
// tools stand on the room's workbook (Agent, an mcp.Live). What the
// agent reads is the workbook on the screen; what it changes is a
// proposal (sheet.Propose) on the room's Board, marked on the person's
// screen until they accept or reject it, or, when they let agents edit
// directly, a step of the agent's own. The Board also carries the
// agent's questions to the person (Ask) and what the person allows it.
//
// Everything on the Board is the room's, so it is read and changed in
// a turn (room.Seat.Do), as the workbook is.
package cowork

import (
	"errors"
	"fmt"
	"slices"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// ScopeKind is how much of the workbook the agent may change.
type ScopeKind int

const (
	// Workbook lets the agent change anything.
	Workbook ScopeKind = iota
	// OneSheet lets it change one sheet's cells, lines and charts.
	OneSheet
	// OneRange lets it change the cells of one range.
	OneRange
	// ReadOnly lets it change nothing.
	ReadOnly
)

// Scope is what the person invited the agent to change. The agent reads
// the whole workbook whatever the scope, as formulas in it read cells
// outside it; the scope bounds its changes and where it points.
type Scope struct {
	Kind  ScopeKind
	Sheet *sheet.Sheet // OneSheet, OneRange
	Range sheet.Rect   // OneRange
}

// String is the scope in words: "the whole workbook", "sheet Q3",
// "Q3!A1:C9", "read only".
func (s Scope) String() string {
	switch s.Kind {
	case OneSheet:
		return "sheet " + s.Sheet.Name()
	case OneRange:
		return sheet.Qualified(s.Sheet.Name(), s.Range)
	case ReadOnly:
		return "read only"
	}
	return "the whole workbook"
}

// ErrReadOnly refuses any change of the agent invited read only.
var ErrReadOnly = errors.New("you were invited read only: you can read the workbook, point at cells and ask the person, but not change anything; ask them to invite you again with a scope that lets you")

// Allows reports whether the agent may point at r of sheet s.
func (s Scope) Allows(sh *sheet.Sheet, r sheet.Rect) bool {
	switch s.Kind {
	case OneSheet:
		return sh == s.Sheet
	case OneRange:
		return sh == s.Sheet && s.Range.Contains(r.From) && s.Range.Contains(r.To)
	}
	return true
}

// Check refuses a proposal that changes anything outside the scope,
// saying what.
func (s Scope) Check(p *sheet.Proposal) error {
	switch s.Kind {
	case ReadOnly:
		return ErrReadOnly
	case Workbook:
		return nil
	}
	name := s.Sheet.Name()
	if p.Book || s.Kind == OneRange && len(p.Other) > 0 || slices.ContainsFunc(p.Sheets, func(n string) bool { return n != name }) {
		return s.outside(p.Other[0])
	}
	for _, c := range p.Cells {
		if c.Sheet != name || s.Kind == OneRange && !s.Range.Contains(c.At) {
			return s.outside("changes " + sheet.Qualified(c.Sheet, sheet.Rect{From: c.At, To: c.At}))
		}
	}
	return nil
}

func (s Scope) outside(what string) error {
	return fmt.Errorf("the change %s, outside what the person let you change (%s): keep to it, or ask them to invite you with a wider scope", what, s)
}

// Grants are what the person lets agents do beyond changing cells, for
// the session: run notebook cells with nu, and enter formulas that ask
// a hosted model over the network (JEV) directly, as untrusted macros
// ask before either. Without a grant, the agent's request asks the
// person first.
type Grants struct {
	Notebooks bool
	JEV       bool
}
