package sheet

// Loaders (the native file reader, importers of other formats) build a
// sheet cell by cell and recalculate once at the end.

// Load stores an entry at a with its number format and style, without
// recalculating or recording undo. With an Automatic format the entry
// implies one as if typed ("$5" is currency, "9/26/2026" a date). A
// formula that fails to parse is rejected with a *ParseError and nothing
// is stored. Call RecalcAll when every cell is in.
func (s *Sheet) Load(a Addr, input string, f Format, st Style) error {
	if !a.Valid() {
		return nil
	}
	c, err := newCell(input, f, st, f.IsZero())
	if err != nil {
		return err
	}
	if c == nil {
		return nil
	}
	s.place(a, c)
	return nil
}

// LoadColWidth sets column c's width, as a loader does: without
// recording undo or recalculating.
func (s *Sheet) LoadColWidth(c, w int) {
	if c >= 0 && c < MaxCols {
		s.setWidth(c, w)
	}
}

// Unload removes a cell a loader stored, as an importer does with a row
// that doesn't fit whole.
func (s *Sheet) Unload(a Addr) { s.place(a, nil) }

// LoadFrozen freezes the first rows rows and cols columns, as a loader
// does: without recording undo. SetFrozen is the undoable way.
func (s *Sheet) LoadFrozen(rows, cols int) {
	s.view.frozenRows, s.view.frozenCols = clampInt(rows, 0, MaxFrozen), clampInt(cols, 0, MaxFrozen)
}

// LoadFilter puts filter f on the sheet (nil removes it), as a loader
// does: without recording undo. Its criteria apply once values are
// computed, as they do whenever values change.
func (s *Sheet) LoadFilter(f *Filter) {
	s.view.filter = f.clone()
	s.hidden.valid = false
}
