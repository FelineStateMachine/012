package sheet

import "github.com/FelineStateMachine/012/internal/functions"

// Decimal arithmetic is a per-workbook calculation setting; what it
// computes in decimal is the function library's (functions/decimal.go).

// arith returns the formula to evaluate under the workbook's arithmetic
// setting.
func (w *Workbook) arith(n Node) Node {
	if w.decimal {
		return functions.Decimalize(n)
	}
	return n
}

// Decimal reports whether the workbook computes in decimal.
func (w *Workbook) Decimal() bool { return w.decimal }

// SetDecimal turns decimal arithmetic on or off for every sheet, as one
// undo step, and recalculates every formula.
func (w *Workbook) SetDecimal(on bool) {
	if on == w.decimal {
		return
	}
	label := "turn off decimal arithmetic"
	if on {
		label = "turn on decimal arithmetic"
	}
	w.change(w.sheets[w.Active()], label, Rect{}, func() {
		w.recordDecimal()
		w.decimal = on
		w.structural = true // every formula computes differently
	})
}

// Decimal and SetDecimal on a sheet are its workbook's.
func (s *Sheet) Decimal() bool      { return s.wb.Decimal() }
func (s *Sheet) SetDecimal(on bool) { s.wb.SetDecimal(on) }
