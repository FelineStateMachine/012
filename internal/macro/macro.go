// Package macro runs macros: Starlark scripts that act on a spreadsheet
// through a small API, as Google Sheets macros are Apps Script. A
// recorded macro is a list of Actions written out as a script (Source),
// so recorded and hand-written macros run the same way.
//
// Scripts can't reach the file system, the network, the clock or random
// numbers: the only way out is the Host. Every run has a step limit and
// can be cancelled from another goroutine. The package knows nothing of
// the terminal or the engine; internal/ui implements Host.
package macro

import (
	"fmt"
	"strconv"
	"strings"

	"go.starlark.net/syntax"
)

// Host is the spreadsheet a script acts on. References are A1 strings,
// optionally with a sheet ("Sheet2!B3:C9"); a range is a cell or a
// rectangle. Its methods are only called inside Env.Do.
type Host interface {
	// Values returns the computed values of ref, row by row: nil for a
	// blank, float64, string, bool, or an error value's code as a string
	// ("#DIV/0!").
	Values(ref string) ([][]any, error)
	// Formulas returns what was typed in each cell of ref, row by row,
	// when it's a formula, and "" otherwise.
	Formulas(ref string) ([][]string, error)
	// SetInput enters input, as if typed, in every cell of ref.
	SetInput(ref, input string) error
	// SetInputs enters a block of inputs, as if typed, from ref's first
	// cell.
	SetInputs(ref string, rows [][]string) error
	// SetFormula enters formula in ref's first cell and fills it over the
	// rest, adjusting relative references as a copy does.
	SetFormula(ref, formula string) error
	// Clear erases the contents of ref, or of the selection when ref is "".
	Clear(ref string) error
	// NumberFormat is the number format of ref's first cell: its kind
	// ("auto", "number", "currency"...) and decimals.
	NumberFormat(ref string) (kind string, decimals int, err error)
	// SetNumberFormat formats ref; decimals < 0 keeps the kind's default,
	// and pattern is for the "custom" kind.
	SetNumberFormat(ref, kind string, decimals int, pattern string) error

	// Selection is the selected range and the active cell in it.
	Selection() (sel, active string)
	// Select selects ref, switching sheets if it names one, with active
	// as the active cell ("" for ref's first cell).
	Select(ref, active string) error
	// Move moves the active cell by cols and rows and drops the
	// selection.
	Move(cols, rows int) error
	// Extend selects from the active cell to cols and rows away from it;
	// whole is "", "columns" or "rows".
	Extend(cols, rows int, whole string) error
	// Jump moves as Ctrl+arrows, Home, Ctrl+Home or Ctrl+End do (see
	// JumpTargets), extending the selection when extend is set.
	Jump(to string, extend bool) error
	// Enter stores text in the active cell as if typed and accepted, or
	// in every selected cell, references adjusted, when fill is set. With
	// an origin, a formula is taken as typed there and moved to the
	// active cell, relative references following, as in a copy.
	Enter(text string, fill bool, origin string) error
	// PasteText pastes tab-separated text at the active cell, as pasting
	// from another program does.
	PasteText(text string) error

	// Sheets lists the sheet names in tab order.
	Sheets() []string
	// ActiveSheet names the sheet shown.
	ActiveSheet() string
	// ActivateSheet shows the named sheet.
	ActivateSheet(name string) error
	// AddSheet adds a sheet after the one shown and shows it, returning
	// its name; name "" picks the next free one.
	AddSheet(name string) (string, error)
	// MoveSheet moves the sheet shown to position pos, counting from 1.
	MoveSheet(pos int) error

	// Run runs a registered command by id. A command that asks a
	// question (a width, a name, a confirmation) gets answer, which is
	// required then.
	Run(id string, answer *string) error
	// SetWidth sets the width of the columns cols, e.g. "B" or "B:D".
	SetWidth(cols string, width int) error
	// SetHeight sets the height of the rows rows, e.g. "3" or "3:5", in
	// lines; 0 fits them to their contents.
	SetHeight(rows string, height int) error
	// Fill fills from the selection as dragging the fill handle does: to
	// the range to, or by rows (down, or up when negative) or cols.
	Fill(to string, rows, cols int) error
}

// JumpTargets are the places Jump goes: the edge of the data in a
// direction (Ctrl+arrows), the row's first column (Home), A1 (Ctrl+Home)
// and the last used cell (Ctrl+End).
var JumpTargets = []string{"up", "down", "left", "right", "home", "start", "end"}

// Action is one step of a recording: a call of one of the script's
// functions, or a comment when Func is "". It is plain data, so a log
// can be kept, compared or saved before it becomes a script.
type Action struct {
	Func    string `json:"func,omitempty"`
	Args    []any  `json:"args,omitempty"` // string, int or bool
	Named   []Arg  `json:"named,omitempty"`
	Comment string `json:"comment,omitempty"`
}

// Arg is a named argument, e.g. fill=True.
type Arg struct {
	Name  string `json:"name"`
	Value any    `json:"value"`
}

// Call is a shorthand for an Action calling fn with args.
func Call(fn string, args ...any) Action { return Action{Func: fn, Args: args} }

// Note is a comment in a recording.
func Note(text string) Action { return Action{Comment: text} }

// With adds a named argument.
func (a Action) With(name string, v any) Action {
	a.Named = append(a.Named, Arg{name, v})
	return a
}

// String is the action as a line of Starlark.
func (a Action) String() string {
	if a.Func == "" {
		return "# " + strings.ReplaceAll(a.Comment, "\n", " ")
	}
	parts := make([]string, 0, len(a.Args)+len(a.Named))
	for _, v := range a.Args {
		parts = append(parts, literal(v))
	}
	for _, n := range a.Named {
		parts = append(parts, n.Name+"="+literal(n.Value))
	}
	return a.Func + "(" + strings.Join(parts, ", ") + ")"
}

// literal writes a Go value as a Starlark literal.
func literal(v any) string {
	switch v := v.(type) {
	case string:
		return syntax.Quote(v, false)
	case int:
		return strconv.Itoa(v)
	case bool:
		if v {
			return "True"
		}
		return "False"
	case nil:
		return "None"
	}
	panic(fmt.Sprintf("macro: no literal for %T", v))
}

// Source writes a recording as a script, one action per line, under a
// header comment.
func Source(header string, actions []Action) string {
	var b strings.Builder
	for _, l := range strings.Split(header, "\n") {
		b.WriteString(strings.TrimRight("# "+l, " ") + "\n")
	}
	for _, a := range actions {
		b.WriteString(a.String() + "\n")
	}
	return b.String()
}
