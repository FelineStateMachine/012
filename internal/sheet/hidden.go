package sheet

import (
	"errors"
	"slices"
)

// Hidden sheets, as Sheets' Hide sheet: a hidden sheet keeps its cells
// and stays in formulas, named ranges and pivot sources; it is left out
// of the tabs and of moving between sheets, and View > Hidden sheets
// shows it again. At least one sheet is always visible. Hiding and
// showing are undo steps that snapshot the sheet list (historysheets.go),
// and files keep the flag in an optional field older builds ignore.

// Hidden reports whether the sheet is hidden.
func (s *Sheet) Hidden() bool { return s.tabHidden }

// Visible returns the sheets that aren't hidden, in tab order.
func (w *Workbook) Visible() []*Sheet {
	return slices.DeleteFunc(slices.Clone(w.sheets), (*Sheet).Hidden)
}

// HiddenSheets returns the hidden sheets, in tab order.
func (w *Workbook) HiddenSheets() []*Sheet {
	return slices.DeleteFunc(slices.Clone(w.sheets), func(s *Sheet) bool { return !s.tabHidden })
}

// visibleCount is how many sheets aren't hidden.
func (w *Workbook) visibleCount() int {
	n := 0
	for _, s := range w.sheets {
		if !s.tabHidden {
			n++
		}
	}
	return n
}

// errLastVisible is why the last visible sheet can't be hidden or deleted.
var errLastVisible = errors.New("A spreadsheet needs at least one visible sheet")

// HideSheet hides s, as one undo step. The last visible sheet can't be
// hidden.
func (w *Workbook) HideSheet(s *Sheet) error {
	switch {
	case !s.live:
		return errors.New("That sheet was deleted")
	case s.tabHidden:
		return nil
	case w.visibleCount() == 1:
		return errLastVisible
	}
	w.change(s, "hide sheet "+s.name, Rect{}, func() {
		w.recordSheets()
		s.tabHidden = true
	})
	return nil
}

// UnhideSheet shows s again, as one undo step.
func (w *Workbook) UnhideSheet(s *Sheet) error {
	switch {
	case !s.live:
		return errors.New("That sheet was deleted")
	case !s.tabHidden:
		return nil
	}
	w.change(s, "show sheet "+s.name, Rect{}, func() {
		w.recordSheets()
		s.tabHidden = false
	})
	return nil
}

// settleHidden keeps a loaded workbook showable: if every sheet is
// hidden the first is shown, and a hidden sheet isn't the active one.
func (w *Workbook) settleHidden() {
	if w.visibleCount() == 0 {
		w.sheets[0].tabHidden = false
	}
	if w.sheets[w.Active()].tabHidden {
		w.active = w.Index(w.Visible()[0])
	}
}
