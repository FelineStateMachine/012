package headless

import (
	"github.com/FelineStateMachine/012/internal/sheet"
)

// Problem is a cell whose formula shows an error after recalculating.
type Problem struct {
	Sheet string
	Addr  sheet.Addr
	Value string // the error as the cell shows it: #DIV/0!
	Why   string // what the screen's context line says about it, or ""
	// JEV is set when the error is a JEV function's with nothing to
	// answer it: --jev asks the model.
	JEV bool
}

// At is the problem's cell with its sheet: Q3!B7.
func (p Problem) At() string {
	return sheet.Qualified(p.Sheet, sheet.Rect{From: p.Addr, To: p.Addr})
}

// Recalc recomputes every formula of w and returns the cells showing
// errors, sheet by sheet in tab order and row by row, and whether a
// circular reference was found.
func Recalc(w *sheet.Workbook) (problems []Problem, circular bool) {
	w.RecalcAll()
	return Errors(w), w.Circular
}

// Errors returns the cells of w showing errors, without recalculating.
// A cell an array spilled an error into is its formula's problem, so
// only formulas are listed.
func Errors(w *sheet.Workbook) []Problem {
	var out []Problem
	for _, s := range w.Sheets() {
		for _, a := range s.Addrs() {
			v := s.Value(a)
			if v.Kind != sheet.Error {
				continue
			}
			c := s.Cell(a)
			if c == nil || !c.IsFormula() {
				continue
			}
			out = append(out, Problem{Sheet: s.Name(), Addr: a, Value: v.Str, Why: s.ExplainError(a),
				JEV: len(s.RemoteCalls(a)) > 0 && (v == sheet.ErrNoRemote || sheet.IsPending(v))})
		}
	}
	return out
}
