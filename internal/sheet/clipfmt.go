package sheet

import (
	"maps"
	"slices"
)

// Formats travel with copy, cut and move, as in Sheets. A copy takes the
// formatting its range shows, whatever it comes from (the cell, its row,
// its column, the sheet), and a paste gives each destination cell what
// its source showed, storing only what the destination's own lines don't
// already show. Copying whole columns or rows and pasting them as whole
// lines (at the top of a column, the start of a row, or A1 for the whole
// sheet) carries the line formats themselves, and cutting them moves the
// line formats, leaving the source lines plain. Cutting a block leaves
// its cells plain, over whatever lines cross it.

// clipFormats is the formatting of a copied range, taken at copy time.
type clipFormats struct {
	cols, rows     bool             // whole columns, whole rows were copied (both: the sheet)
	cells          map[Addr]lineFmt // what each stored cell showed, by offset
	colFmt, rowFmt map[int]lineFmt  // formatted columns and rows crossing the range, by offset
	sheet          lineFmt
}

// copyFormats takes the formatting of r.
func (s *Sheet) copyFormats(r Rect) clipFormats {
	f := clipFormats{cols: r.AllRows(), rows: r.AllCols(), cells: map[Addr]lineFmt{}, sheet: s.lines.sheet}
	for _, a := range s.cellsIn(r) {
		f.cells[Addr{Col: a.Col - r.From.Col, Row: a.Row - r.From.Row}] = s.effective(a)
	}
	f.colFmt = linesIn(s.lines.cols, r.From.Col, r.To.Col)
	f.rowFmt = linesIn(s.lines.rows, r.From.Row, r.To.Row)
	return f
}

// linesIn is the lines of m from lo to hi, by offset from lo.
func linesIn(m map[int]lineFmt, lo, hi int) map[int]lineFmt {
	out := map[int]lineFmt{}
	for n, l := range m {
		if n >= lo && n <= hi {
			out[n-lo] = l
		}
	}
	return out
}

// mode is how the formats land: on cells, or on whole lines.
func (f *clipFormats) mode() lineMode {
	switch {
	case f.cols && f.rows:
		return lineAll
	case f.cols:
		return lineCols
	case f.rows:
		return lineRows
	}
	return lineBlock
}

// lined reports whether any line format crosses the copied range.
func (f *clipFormats) lined() bool {
	return len(f.colFmt) > 0 || len(f.rowFmt) > 0 || !f.sheet.IsZero()
}

// want is what the cell at offset off of the copy showed.
func (f *clipFormats) want(off Addr) lineFmt {
	if l, ok := f.cells[off]; ok {
		return l
	}
	return fallback(f.rowFmt[off.Row], f.colFmt[off.Col], f.sheet)
}

// carry is the line format a pasted column or row gets for l, the
// source's, filling what it leaves to the source sheet's format where
// the destination sheet's differs.
func (f *clipFormats) carry(l, dstSheet lineFmt) lineFmt {
	if l.Format.IsZero() && f.sheet.Format != dstSheet.Format {
		l.Format = f.sheet.Format
	}
	if l.Style.IsZero() && f.sheet.Style != dstSheet.Style {
		l.Style = f.sheet.Style
	}
	return l
}

// lineMode is whether a paste or move lands on cells or on whole lines.
type lineMode int

const (
	lineBlock lineMode = iota
	lineCols
	lineRows
	lineAll
)

// pasteLayout is where a paste lands: dst, tiled by the clip's size,
// across by down times.
type pasteLayout struct {
	mode         lineMode
	dst          Rect
	tw, th       int // a tile's size; a whole line's length along it
	across, down int
}

// off is the offset within the copy that the cell at a receives.
func (p pasteLayout) off(a Addr) Addr {
	return Addr{Col: (a.Col - p.dst.From.Col) % p.tw, Row: (a.Row - p.dst.From.Row) % p.th}
}

// layout is where pasting c into dst lands. Like Sheets, a destination
// that is an exact multiple of the clip's size is tiled; otherwise the
// clip is pasted once at dst's top-left corner. Whole lines pasted at
// the start of a line land on whole lines.
func (c *Clip) layout(dst Rect) (pasteLayout, error) {
	cols, rows := c.Size()
	p := pasteLayout{mode: c.formats.mode(), tw: cols, th: rows, across: 1, down: 1}
	switch {
	case p.mode == lineAll && dst.From == Addr{}:
		p.tw, p.th = MaxCols, MaxRows
	case p.mode == lineCols && dst.From.Row == 0:
		p.th = MaxRows
	case p.mode == lineRows && dst.From.Col == 0:
		p.tw = MaxCols
	default:
		p.mode = lineBlock
	}
	w, h := dst.To.Col-dst.From.Col+1, dst.To.Row-dst.From.Row+1
	switch p.mode {
	case lineBlock:
		if w%cols == 0 && h%rows == 0 {
			p.across, p.down = w/cols, h/rows
		}
	case lineCols:
		if w%cols == 0 {
			p.across = w / cols
		}
	case lineRows:
		if h%rows == 0 {
			p.down = h / rows
		}
	}
	p.dst = Rect{From: dst.From, To: Addr{Col: dst.From.Col + p.across*p.tw - 1, Row: dst.From.Row + p.down*p.th - 1}}
	if !p.dst.To.Valid() {
		return pasteLayout{}, ErrPasteEdge
	}
	return p, nil
}

// MoveRange is the range cutting c moves when pasted at to: the whole
// lines copied when to starts a line, else the copied cells.
func (c *Clip) MoveRange(to Addr) Rect {
	if p, err := c.layout(Rect{From: to, To: to}); err == nil && p.mode != lineBlock {
		return c.Range
	}
	return c.Src
}

// pasteFormats has the cells p covers show what the copy f showed: whole
// lines take its line formats, and cells whose lines don't show what
// their source did get formatting of their own.
func (s *Sheet) pasteFormats(f *clipFormats, p pasteLayout) {
	s.pasteLines(f, p)
	for _, a := range s.formatTargets(f, p) {
		s.placeFormat(a, f.want(p.off(a)))
	}
}

// pasteLines gives the whole lines p covers the copy's line formats.
func (s *Sheet) pasteLines(f *clipFormats, p pasteLayout) {
	switch p.mode {
	case lineAll:
		s.setLine(false, wholeSheet, f.sheet)
		for _, n := range unionKeys(s.lines.cols, f.colFmt) {
			s.setLine(false, n, f.colFmt[n])
		}
		for _, n := range unionKeys(s.lines.rows, f.rowFmt) {
			s.setLine(true, n, f.rowFmt[n])
		}
	case lineCols:
		for i := range p.across * p.tw {
			s.setLine(false, p.dst.From.Col+i, f.carry(f.colFmt[i%p.tw], s.lines.sheet))
		}
	case lineRows:
		for i := range p.down * p.th {
			s.setLine(true, p.dst.From.Row+i, f.carry(f.rowFmt[i%p.th], s.lines.sheet))
		}
	}
}

// formatTargets are the cells of p.dst that may need formatting of their
// own: those stored, and where a formatted line crosses the pasted lines
// or, for a block, every cell when lines are formatted on either side,
// up to materializeLimit cells made.
func (s *Sheet) formatTargets(f *clipFormats, p pasteLayout) []Addr {
	targets := s.cellsIn(p.dst)
	for _, r := range s.formatAreas(f, p) {
		for row := r.From.Row; row <= r.To.Row; row++ {
			for c := r.From.Col; c <= r.To.Col; c++ {
				if len(targets) > materializeLimit {
					return targets
				}
				if a := (Addr{Col: c, Row: row}); !s.cells.has(a) {
					targets = append(targets, a)
				}
			}
		}
	}
	return targets
}

// formatAreas are the parts of p.dst whose blank cells may show other
// than their source: where formatted rows cross pasted columns, formatted
// columns cross pasted rows, or a whole block when lines are formatted
// on either side and it is small enough.
func (s *Sheet) formatAreas(f *clipFormats, p pasteLayout) []Rect {
	d := p.dst
	var out []Rect
	switch p.mode {
	case lineCols:
		for _, n := range unionKeys(s.lines.rows, f.rowFmt) {
			out = append(out, Rect{From: Addr{Col: d.From.Col, Row: n}, To: Addr{Col: d.To.Col, Row: n}})
		}
	case lineRows:
		for _, n := range unionKeys(s.lines.cols, f.colFmt) {
			out = append(out, Rect{From: Addr{Col: n, Row: d.From.Row}, To: Addr{Col: n, Row: d.To.Row}})
		}
	case lineBlock:
		area := (d.To.Col - d.From.Col + 1) * (d.To.Row - d.From.Row + 1)
		if area <= materializeLimit && (f.lined() || !s.lines.none()) {
			out = append(out, d)
		}
	}
	return out
}

// moveFormats gives dst on s, where the cells of src on sheet from land,
// the formatting f they showed at src; whole lines take their line
// formats along, leaving the source lines plain.
func (s *Sheet) moveFormats(from *Sheet, f *clipFormats, src, dst Rect) {
	p := pasteLayout{mode: f.mode(), dst: dst, across: 1, down: 1,
		tw: src.To.Col - src.From.Col + 1, th: src.To.Row - src.From.Row + 1}
	switch p.mode {
	case lineAll:
		from.setLine(false, wholeSheet, lineFmt{})
		for _, n := range slices.Sorted(maps.Keys(from.lines.cols)) {
			from.setLine(false, n, lineFmt{})
		}
		for _, n := range slices.Sorted(maps.Keys(from.lines.rows)) {
			from.setLine(true, n, lineFmt{})
		}
	case lineCols:
		for n := range f.colFmt {
			from.setLine(false, src.From.Col+n, lineFmt{})
		}
	case lineRows:
		for n := range f.rowFmt {
			from.setLine(true, src.From.Row+n, lineFmt{})
		}
	case lineBlock:
		from.plainSource(src, dst, from == s)
	}
	s.pasteFormats(f, p)
}

// plainSource leaves the cells a block was cut from showing plain, as
// Sheets clears them: where a formatted row, column or the sheet crosses
// src, its blank cells get plain formatting of their own, up to
// materializeLimit cells looked at, except those in dst when the block
// lands on the same sheet.
func (s *Sheet) plainSource(src, dst Rect, same bool) {
	var areas []Rect
	if !s.lines.sheet.IsZero() {
		areas = []Rect{src}
	} else {
		for n := range linesIn(s.lines.rows, src.From.Row, src.To.Row) {
			row := src.From.Row + n
			areas = append(areas, Rect{From: Addr{Col: src.From.Col, Row: row}, To: Addr{Col: src.To.Col, Row: row}})
		}
		for n := range linesIn(s.lines.cols, src.From.Col, src.To.Col) {
			col := src.From.Col + n
			areas = append(areas, Rect{From: Addr{Col: col, Row: src.From.Row}, To: Addr{Col: col, Row: src.To.Row}})
		}
	}
	seen := 0
	for _, r := range areas {
		for row := r.From.Row; row <= r.To.Row; row++ {
			for c := r.From.Col; c <= r.To.Col; c++ {
				if seen++; seen > materializeLimit {
					return
				}
				if a := (Addr{Col: c, Row: row}); !(same && dst.Contains(a)) && !s.cells.has(a) {
					s.placeFormat(a, lineFmt{})
				}
			}
		}
	}
}

// unionKeys are the keys of a and b, in order.
func unionKeys(a, b map[int]lineFmt) []int {
	keys := slices.Collect(maps.Keys(a))
	for n := range b {
		if _, ok := a[n]; !ok {
			keys = append(keys, n)
		}
	}
	slices.Sort(keys)
	return keys
}
