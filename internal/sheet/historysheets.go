package sheet

import (
	"maps"
	"slices"

	"github.com/FelineStateMachine/012/internal/formula"
)

// Undo steps that add, delete, rename or reorder sheets snapshot the
// workbook's list of sheets.

// sheetList is the order and names of the sheets, and which are hidden.
type sheetList struct {
	order  []*Sheet
	names  map[*Sheet]string
	hidden map[*Sheet]bool
}

func (w *Workbook) sheetList() *sheetList {
	l := &sheetList{order: slices.Clone(w.sheets), names: map[*Sheet]string{}, hidden: map[*Sheet]bool{}}
	for _, s := range w.sheets {
		l.names[s] = s.name
		if s.tabHidden {
			l.hidden[s] = true
		}
	}
	return l
}

// setSheets restores a sheet list: sheets not in it are detached, the
// rest renamed and attached in its order.
func (w *Workbook) setSheets(l *sheetList) {
	for _, s := range w.sheets {
		if !slices.Contains(l.order, s) {
			w.detach(s)
		}
	}
	w.byKey = map[string]*Sheet{}
	w.sheets = slices.Clone(l.order)
	for _, s := range w.sheets {
		s.name = l.names[s]
		s.tabHidden = l.hidden[s]
		if s.live {
			w.byKey[formula.SheetKey(s.name)] = s
		} else {
			w.attach(s)
		}
	}
	w.structural = true
}

func (l *sheetList) equal(m *sheetList) bool {
	return slices.Equal(l.order, m.order) && maps.Equal(l.names, m.names) && maps.Equal(l.hidden, m.hidden)
}
