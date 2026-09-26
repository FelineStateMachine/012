package sheet

import (
	"strconv"
	"strings"
)

// Worksheet bounds, matching Lotus 1-2-3 Release 2 (A..IV, 1..8192).
const (
	MaxCols = 256
	MaxRows = 8192
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

// ColName converts a zero-based column index to letters: 0 -> A, 26 -> AA.
func ColName(c int) string {
	if c < 26 {
		return string(rune('A' + c))
	}
	return string(rune('A'+c/26-1)) + string(rune('A'+c%26))
}

// ParseCol converts column letters (case-insensitive) to a zero-based index.
func ParseCol(s string) (int, bool) {
	if len(s) == 0 || len(s) > 2 {
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
	s = strings.ReplaceAll(s, "$", "")
	i := 0
	for i < len(s) && isLetter(s[i]) {
		i++
	}
	col, ok := ParseCol(s[:i])
	if !ok || i == len(s) {
		return Addr{}, false
	}
	row, err := strconv.Atoi(s[i:])
	if err != nil || s[i] == '+' || s[i] == '-' {
		return Addr{}, false
	}
	a := Addr{Col: col, Row: row - 1}
	return a, a.Valid()
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

// String returns the range in 1-2-3 notation, e.g. A1..B3.
func (r Rect) String() string {
	return r.From.String() + ".." + r.To.String()
}

// ParseRange parses "A1", "A1..B3" or "A1:B3".
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
	a, ok1 := ParseAddr(s[:sep])
	b, ok2 := ParseAddr(s[sep+width:])
	return NewRect(a, b), ok1 && ok2
}

func isLetter(c byte) bool {
	return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')
}

func isDigit(c byte) bool {
	return c >= '0' && c <= '9'
}
