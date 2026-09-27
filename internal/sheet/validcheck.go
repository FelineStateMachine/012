package sheet

import "slices"

// Validation of what pastes and fills write, as Sheets does it: they
// don't ask cell by cell, so the UI checks the range they wrote after
// the fact, and takes the whole change back (Discard) when a rule that
// rejects fails.

// InvalidIn lists the cells in r whose contents fail their validation,
// in reading order: blanks never do. A cell's rule decides whether its
// entry is refused (Reject) or kept and marked. The cost is the cells
// stored where r meets a rule's ranges.
func (s *Sheet) InvalidIn(r Rect) []*InvalidEntry {
	var out []*InvalidEntry
	for _, v := range s.rules.validations {
		for _, vr := range v.Ranges {
			in := Rect{
				From: Addr{Col: max(vr.From.Col, r.From.Col), Row: max(vr.From.Row, r.From.Row)},
				To:   Addr{Col: min(vr.To.Col, r.To.Col), Row: min(vr.To.Row, r.To.Row)},
			}
			if in.From.Col > in.To.Col || in.From.Row > in.To.Row {
				continue
			}
			for _, a := range s.cellsIn(in) {
				if c := s.cells.get(a); !c.Blank() && s.Look(a).Invalid {
					out = append(out, &InvalidEntry{Addr: a, Help: v.HelpText(), Reject: v.Reject})
				}
			}
		}
	}
	slices.SortFunc(out, func(a, b *InvalidEntry) int {
		if a.Addr.Row != b.Addr.Row {
			return a.Addr.Row - b.Addr.Row
		}
		return a.Addr.Col - b.Addr.Col
	})
	return out
}

// Discard takes back the last change and forgets it, so redo can't bring
// it back: a paste or fill refused after the fact. It reports whether
// there was a change to take back.
func (w *Workbook) Discard() bool {
	if _, ok := w.swap(true); !ok {
		return false
	}
	w.hist.redo = w.hist.redo[:len(w.hist.redo)-1]
	return true
}

// Discard takes back the workbook's last change; see Workbook.Discard.
func (s *Sheet) Discard() bool { return s.wb.Discard() }
