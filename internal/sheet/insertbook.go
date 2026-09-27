package sheet

import (
	"errors"
	"fmt"
	"slices"
)

// Importing into a workbook, as Sheets' Import location: "Insert new
// sheet(s)" moves every sheet of an imported workbook into this one, and
// "Replace current sheet" puts the imported sheet in the place of one.
// The imported workbook is built on its own (fileio) and given up to
// these; either is one undo step.

// ImportResult says what an import into a workbook did.
type ImportResult struct {
	Sheets  []*Sheet          // the sheets added, in tab order
	Renamed map[string]string // imported names changed to fit, old to new
	Names   int               // named ranges left out, their names taken
}

// InsertBook moves the sheets of src, an imported workbook, into w at
// index at (clamped to the ends; the UI puts them after the sheet shown),
// with src's named ranges, as one undo step labelled label. A sheet whose name w already has gets a number ("Sales 2"), and
// src's formulas follow the new name; a named range whose name w already
// has is left out. src is used up.
func (w *Workbook) InsertBook(src *Workbook, at int, label string) (ImportResult, error) {
	var res ImportResult
	if src == w || len(src.sheets) == 0 {
		return res, errors.New("Nothing to import")
	}
	sheets := slices.Clone(src.sheets)
	for _, s := range sheets {
		if w.Lookup(s.name) == nil {
			continue
		}
		name := freeIn(s.name, w, src)
		if res.Renamed == nil {
			res.Renamed = map[string]string{}
		}
		res.Renamed[s.name] = name
		if err := src.RenameSheet(s, name); err != nil {
			return res, err
		}
	}
	names := src.Names()
	for _, s := range sheets {
		src.detach(s)
		s.wb = w
	}
	src.sheets = nil
	// Undo shows the sheet the new ones follow, where the import started.
	at = clampInt(at, 0, len(w.sheets))
	shown := sheets[0]
	if at > 0 {
		shown = w.sheets[at-1]
	}
	w.change(shown, label, Rect{}, func() {
		w.recordSheets()
		for i, s := range sheets {
			w.insert(s, at+i)
		}
		for _, n := range names {
			if _, taken := w.LookupName(n.Name); taken {
				res.Names++
				continue
			}
			w.putName(nameKey(n.Name), &n)
		}
	})
	res.Sheets = sheets
	return res, nil
}

// ReplaceSheet puts s, the sheet of an imported workbook, in the place of
// dst, as one undo step labelled label: it takes dst's name and position,
// so formulas reading dst read it, and named ranges on dst move to it.
// dst's charts that fit s's data stay, drawing it, and the rest go (see
// keepCharts); it returns what became of each. dst's cells and the rest
// go; undo brings them back, charts too.
func (w *Workbook) ReplaceSheet(dst, s *Sheet, label string) ([]ChartFate, error) {
	switch {
	case !dst.live || dst.wb != w:
		return nil, errors.New("That sheet was deleted")
	case s.wb == w:
		return nil, errors.New("That sheet is already in the spreadsheet")
	}
	fates := s.keepCharts(dst)
	s.wb.detach(s)
	s.wb.sheets = slices.DeleteFunc(s.wb.sheets, func(t *Sheet) bool { return t == s })
	s.wb, s.tabHidden = w, dst.tabHidden
	w.change(dst, label, Rect{}, func() {
		w.recordSheets()
		i := w.Index(dst)
		w.remove(dst)
		s.name = dst.name
		w.insert(s, i)
		for _, n := range w.Names() {
			if n.Sheet == dst {
				n.Sheet = s
				w.putName(nameKey(n.Name), &n)
			}
		}
	})
	return fates, nil
}

// ChartFate says what replacing a sheet did to one of its charts.
type ChartFate struct {
	Name     string // the chart's title, or Chart 1, Chart 2 by position
	Was, Now Rect   // the range it drew, and draws
	// Removed is set when the range no longer fits the new data: it
	// holds nothing now, or it was a whole table and the new sheet's
	// table there has other series.
	Removed bool
}

// keepCharts gives s, replacing old, old's charts that fit its data. A
// chart that drew a whole block of old's data is re-pointed to the block
// s has at the same corner when it has as many series (columns, or rows
// for a chart by row), so a chart of last month's table draws this
// month's, however many rows it has; when s's block there has other
// series the chart goes. A chart of whole columns or rows, or of part of
// a block, keeps its range. A chart whose range holds nothing now goes
// too. ChartFate says which.
func (s *Sheet) keepCharts(old *Sheet) []ChartFate {
	fates := make([]ChartFate, 0, len(old.charts))
	for i, c := range old.charts {
		name := c.Title
		if name == "" {
			name = fmt.Sprintf("Chart %d", i+1)
		}
		was := c.Data
		now, fits := s.repoint(old, was, c.ByRow)
		_, filled := s.FilledBounds(now)
		f := ChartFate{Name: name, Was: was, Now: now, Removed: !fits || !filled}
		fates = append(fates, f)
		if !f.Removed {
			c.Data = now
			s.charts = append(s.charts, c)
		}
	}
	return fates
}

// repoint is the range on s a chart drawing d on old draws: s's block of
// data at d's corner when d was old's whole block there and the two have
// as many series, else d. Whole columns or rows, and part of a block,
// read any data as is. It reports false when d was a whole block and s
// has none like it there.
func (s *Sheet) repoint(old *Sheet, d Rect, byRow bool) (Rect, bool) {
	if d.AllRows() || d.AllCols() || old.Region(d.From) != d {
		return d, true
	}
	now := s.Region(d.From)
	if now.From != d.From || s.cells.get(now.From).Blank() && now.From == now.To {
		return d, false
	}
	if byRow && now.To.Row-now.From.Row != d.To.Row-d.From.Row ||
		!byRow && now.To.Col-now.From.Col != d.To.Col-d.From.Col {
		return d, false
	}
	return now, true
}

// freeIn returns base, or base with a number added ("Sales 2"), so that
// none of books has a sheet of that name, within the length limit.
func freeIn(base string, books ...*Workbook) string {
	trim := func(s string, n int) string {
		if r := []rune(s); len(r) > n {
			return string(r[:n])
		}
		return s
	}
	taken := func(name string) bool {
		return slices.ContainsFunc(books, func(w *Workbook) bool { return w.Lookup(name) != nil })
	}
	name := trim(base, maxSheetName)
	for n := 2; taken(name); n++ {
		suffix := fmt.Sprintf(" %d", n)
		name = trim(base, maxSheetName-len(suffix)) + suffix
	}
	return name
}
