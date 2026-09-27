package sheet

import "slices"

// The before-images a step keeps of what it changes besides cells
// (record, in history.go), each taken the first time the step changes it.

// recordWidth saves column c's width before its first change in the open
// step.
func (s *Sheet) recordWidth(c int) {
	st := s.wb.hist.open
	if st == nil {
		return
	}
	k := colKey{s, c}
	if _, seen := st.widths[k]; !seen {
		st.widths[k] = s.widths[c]
	}
}

// recordName saves the named range with key k before its first change in
// the open step.
func (w *Workbook) recordName(k string) {
	st := w.hist.open
	if st == nil {
		return
	}
	if _, seen := st.names[k]; !seen {
		st.names[k] = w.namePtr(k)
	}
}

// recordCharts saves the sheet's charts before their first change in the
// open step. Charts are few, so the step keeps them all.
func (s *Sheet) recordCharts() {
	if st := s.wb.hist.open; st != nil {
		if _, seen := st.charts[s]; !seen {
			st.charts[s] = slices.Clone(s.charts)
		}
	}
}

// recordSheets saves the sheet list before its first change in the open
// step, and has the step recalculate everything when it ends.
func (w *Workbook) recordSheets() {
	w.structural = true
	if st := w.hist.open; st != nil && st.sheets == nil {
		st.sheets = w.sheetList()
	}
}

// recordSettings saves the workbook's settings before their first change
// in the open step.
func (w *Workbook) recordSettings() {
	if st := w.hist.open; st != nil && st.settings == nil {
		cur := w.settings
		st.settings = &cur
	}
}
