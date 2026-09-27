package formula

import (
	"strconv"
	"strings"
)

// Worksheet bounds, matching Excel (A..XFD, 1..1048576). A reference
// past them isn't a reference: XFE1 reads as a name.
const (
	MaxCols = 16384
	MaxRows = 1048576
)

// Addr identifies a cell by zero-based column and row.
type Addr struct {
	Col, Row int
}

// String returns the A1-style name of the cell.
func (a Addr) String() string {
	return ColName(a.Col) + strconv.Itoa(a.Row+1)
}

// Valid reports whether a lies inside the worksheet.
func (a Addr) Valid() bool {
	return a.Col >= 0 && a.Col < MaxCols && a.Row >= 0 && a.Row < MaxRows
}

// ColName converts a zero-based column index to letters: 0 -> A, 26 -> AA,
// 702 -> AAA.
func ColName(c int) string {
	if c < 26 {
		return string(rune('A' + c))
	}
	var b [8]byte
	i := len(b)
	for c++; c > 0; c = (c - 1) / 26 {
		i--
		b[i] = byte('A' + (c-1)%26)
	}
	return string(b[i:])
}

// ParseCol converts column letters (case-insensitive) to a zero-based index.
func ParseCol(s string) (int, bool) {
	if len(s) == 0 || len(s) > 3 {
		return 0, false
	}
	c := 0
	for _, r := range strings.ToUpper(s) {
		if r < 'A' || r > 'Z' {
			return 0, false
		}
		c = c*26 + int(r-'A'+1)
	}
	c--
	return c, c < MaxCols
}

// ParseAddr parses an A1-style reference. Absolute markers ($A$1) are
// accepted and ignored.
func ParseAddr(s string) (Addr, bool) {
	a, _, ok := ParseRef(s)
	return a, ok
}

// Abs records which parts of a reference are absolute ($A$1). Copying a
// formula shifts only the relative parts.
type Abs uint8

const (
	AbsCol Abs = 1 << iota
	AbsRow
)

// ParseRef parses a reference written as A1, $A1, A$1 or $A$1.
func ParseRef(s string) (Addr, Abs, bool) {
	var abs Abs
	if rest, ok := strings.CutPrefix(s, "$"); ok {
		s, abs = rest, AbsCol
	}
	i := 0
	for i < len(s) && isLetter(s[i]) {
		i++
	}
	col, ok := ParseCol(s[:i])
	if !ok {
		return Addr{}, 0, false
	}
	digits := s[i:]
	if rest, ok := strings.CutPrefix(digits, "$"); ok {
		digits, abs = rest, abs|AbsRow
	}
	if digits == "" || strings.ContainsFunc(digits, func(r rune) bool { return r < '0' || r > '9' }) {
		return Addr{}, 0, false
	}
	row, err := strconv.Atoi(digits)
	a := Addr{Col: col, Row: row - 1}
	return a, abs, err == nil && a.Valid()
}

// RefString writes a with its absolute markers, e.g. $A1.
func RefString(a Addr, abs Abs) string {
	var b strings.Builder
	if abs&AbsCol != 0 {
		b.WriteByte('$')
	}
	b.WriteString(ColName(a.Col))
	if abs&AbsRow != 0 {
		b.WriteByte('$')
	}
	b.WriteString(strconv.Itoa(a.Row + 1))
	return b.String()
}

// Rect is an inclusive rectangular range of cells.
type Rect struct {
	From, To Addr
}

// NewRect returns the normalized rectangle spanning a and b.
func NewRect(a, b Addr) Rect {
	return Rect{
		From: Addr{Col: min(a.Col, b.Col), Row: min(a.Row, b.Row)},
		To:   Addr{Col: max(a.Col, b.Col), Row: max(a.Row, b.Row)},
	}
}

// Contains reports whether a lies inside r.
func (r Rect) Contains(a Addr) bool {
	return a.Col >= r.From.Col && a.Col <= r.To.Col &&
		a.Row >= r.From.Row && a.Row <= r.To.Row
}

// AllRows reports whether r spans every row: whole columns, A:A.
func (r Rect) AllRows() bool { return r.From.Row == 0 && r.To.Row == MaxRows-1 }

// AllCols reports whether r spans every column: whole rows, 1:1.
func (r Rect) AllCols() bool { return r.From.Col == 0 && r.To.Col == MaxCols-1 }

// String returns the range as A1:B3, A1 for a single cell, or A:C and
// 2:5 for whole columns and rows.
func (r Rect) String() string {
	switch {
	case r.From == r.To:
		return r.From.String()
	case r.AllRows(), r.AllCols():
		return RangeString(r, [2]Abs{})
	}
	return r.From.String() + ":" + r.To.String()
}

// ParseRange parses "A1", "A1:B3", 1-2-3 style "A1..B3", or whole
// columns "A:C" and rows "2:5".
func ParseRange(s string) (Rect, bool) {
	s = strings.TrimSpace(s)
	sep := strings.Index(s, "..")
	width := 2
	if sep < 0 {
		sep, width = strings.Index(s, ":"), 1
	}
	if sep < 0 {
		a, ok := ParseAddr(s)
		return Rect{From: a, To: a}, ok
	}
	if r, _, ok := ParseLines(s[:sep], s[sep+width:]); ok {
		return r, true
	}
	a, ok1 := ParseAddr(s[:sep])
	b, ok2 := ParseAddr(s[sep+width:])
	return NewRect(a, b), ok1 && ok2
}

// ParseLines parses the ends of whole columns ("A", "$C") or whole rows
// ("2", "$5") as the range they span, with their absolute markers.
func ParseLines(from, to string) (Rect, [2]Abs, bool) {
	c0, abs0, ok0 := parseLine(from, false)
	c1, abs1, ok1 := parseLine(to, false)
	if ok0 && ok1 {
		r := NewRect(Addr{Col: c0}, Addr{Col: c1, Row: MaxRows - 1})
		return r, orderAbs(c0 > c1, abs0, abs1), true
	}
	r0, abs0, ok0 := parseLine(from, true)
	r1, abs1, ok1 := parseLine(to, true)
	if ok0 && ok1 {
		r := NewRect(Addr{Row: r0}, Addr{Col: MaxCols - 1, Row: r1})
		return r, orderAbs(r0 > r1, abs0, abs1), true
	}
	return Rect{}, [2]Abs{}, false
}

func orderAbs(swap bool, a, b Abs) [2]Abs {
	if swap {
		return [2]Abs{b, a}
	}
	return [2]Abs{a, b}
}

// parseLine parses a column's letters or a row's number, with an
// optional $.
func parseLine(s string, row bool) (int, Abs, bool) {
	rest, abs := strings.CutPrefix(s, "$")
	if !row {
		c, ok := ParseCol(rest)
		if abs {
			return c, AbsCol, ok
		}
		return c, 0, ok
	}
	if rest == "" || strings.ContainsFunc(rest, func(r rune) bool { return r < '0' || r > '9' }) {
		return 0, 0, false
	}
	n, err := strconv.Atoi(rest)
	if err != nil || n < 1 || n > MaxRows {
		return 0, 0, false
	}
	if abs {
		return n - 1, AbsRow, true
	}
	return n - 1, 0, true
}

// RangeString writes a range as written in a formula, with its absolute
// markers: A1:B3, $A$1:B3, or whole columns (A:C) and rows (2:5).
func RangeString(r Rect, abs [2]Abs) string {
	switch {
	case r.AllRows():
		return lineString(ColName(r.From.Col), abs[0]&AbsCol) + ":" + lineString(ColName(r.To.Col), abs[1]&AbsCol)
	case r.AllCols():
		return lineString(strconv.Itoa(r.From.Row+1), abs[0]&AbsRow) + ":" + lineString(strconv.Itoa(r.To.Row+1), abs[1]&AbsRow)
	}
	return RefString(r.From, abs[0]) + ":" + RefString(r.To, abs[1])
}

func lineString(s string, abs Abs) string {
	if abs != 0 {
		return "$" + s
	}
	return s
}

// LooksLikeRef reports whether an upper-case name reads as a cell in A1 or
// R1C1 style, even beyond this sheet's edges, so names stay unambiguous in
// other spreadsheets too.
func LooksLikeRef(k string) bool {
	letters := strings.TrimLeft(k, "ABCDEFGHIJKLMNOPQRSTUVWXYZ")
	if letters != k && letters != "" && strings.Trim(letters, "0123456789") == "" {
		return true
	}
	rest, ok := strings.CutPrefix(k, "R")
	if !ok {
		rest, ok = k, strings.HasPrefix(k, "C")
	}
	if !ok {
		return false
	}
	rest = strings.TrimLeft(rest, "0123456789")
	rest, _ = strings.CutPrefix(rest, "C")
	return strings.Trim(rest, "0123456789") == ""
}

func isLetter(c byte) bool {
	return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')
}

func isDigit(c byte) bool {
	return c >= '0' && c <= '9'
}
