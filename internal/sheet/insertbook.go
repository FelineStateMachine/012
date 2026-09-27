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

// InsertBook moves the sheets of src, an imported workbook, into w after
// its last sheet, with src's named ranges, as one undo step labelled
// label. A sheet whose name w already has gets a number ("Sales 2"), and
// src's formulas follow the new name; a named range whose name w already
// has is left out. src is used up.
func (w *Workbook) InsertBook(src *Workbook, label string) (ImportResult, error) {
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
	w.change(sheets[0], label, Rect{}, func() {
		w.recordSheets()
		for _, s := range sheets {
			w.insert(s, len(w.sheets))
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
// dst's cells, charts and the rest go with it; undo brings them back.
func (w *Workbook) ReplaceSheet(dst, s *Sheet, label string) error {
	switch {
	case !dst.live || dst.wb != w:
		return errors.New("That sheet was deleted")
	case s.wb == w:
		return errors.New("That sheet is already in the spreadsheet")
	}
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
	return nil
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
