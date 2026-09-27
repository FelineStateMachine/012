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

// Unload removes a cell a loader stored, as an importer does with a row
// that doesn't fit whole.
func (s *Sheet) Unload(a Addr) { s.place(a, nil) }
