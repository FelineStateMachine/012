package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/ui/formula"
	"github.com/FelineStateMachine/012/internal/ui/suggest"
)

// Formula suggestions and signatures are package suggest; here is what
// they ask of the model.

// typeKey applies a line-editing key to the entry. Typing or deleting
// shows suggestions; moving the caret hides them, as in Sheets.
func (m *Model) typeKey(k tea.KeyPressMsg) {
	before := m.line.Text()
	m.line.Key(k)
	m.entry.assist = suggest.List{Active: m.line.Text() != before}
}

// inFunction reports whether the caret is inside a known function's
// parentheses.
func (m *Model) inFunction() bool {
	_, ok := sheet.LookupFunc(formula.ScanCaret(m.storedFormula(m.line.Buf), m.line.Pos).Fn)
	return ok
}

// inTable reports whether the caret is inside a structured reference's
// brackets, Sales[Am.
func (m *Model) inTable() bool {
	_, ok := suggest.InTable(m.storedFormula(m.line.Buf), m.line.Pos)
	return ok
}

// The suggestions' host.

// Typing reports whether an entry is being typed in the cell (ENTER or
// EDIT) with nothing open over it.
func (h host) Typing() bool {
	return (h.m.mode == modeEnter || h.m.mode == modeEdit) && h.m.overlay == nil
}

func (h host) EntrySheet() *sheet.Sheet { return h.m.entrySheet() }
func (h host) Stored(buf []rune) []rune { return h.m.storedFormula(buf) }
func (h host) TextX() int               { return formulaBarTextX() }
