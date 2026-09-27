// Package value holds what a cell computes to and how it is shown: the
// Value of a cell, its number Format, and the parsing of typed text into
// numbers, dates and times. It sits below the function library
// (internal/functions) and the engine (internal/sheet), which both work
// in these terms; the engine's API names them too, so callers see one
// package.
package value

import (
	"cmp"
	"math"
	"strings"

	"github.com/FelineStateMachine/012/internal/numfmt"
)

// Kind is the type of a computed cell value.
type Kind int

const (
	Empty Kind = iota
	Number
	Text
	Bool
	Error
	// Array marks an array or LAMBDA while a formula is evaluated
	// (internal/functions keeps them; Num says which). No cell ever holds
	// one: a formula computing an array shows its first value and spills
	// the rest.
	Array
)

// Value is the computed contents of a cell.
type Value struct {
	Kind Kind
	Num  float64 // numbers, and booleans as 1 or 0
	Str  string  // text, or the error code such as #DIV/0!
}

// String renders a value the way a cell shows it in General format.
func (v Value) String() string {
	switch v.Kind {
	case Number:
		return numfmt.General(v.Num)
	case Bool:
		if v.Num != 0 {
			return "TRUE"
		}
		return "FALSE"
	}
	return v.Str
}

// Error values, using Google Sheets codes.
var (
	ErrDiv0  = Value{Kind: Error, Str: "#DIV/0!"}
	ErrValue = Value{Kind: Error, Str: "#VALUE!"}
	ErrName  = Value{Kind: Error, Str: "#NAME?"}
	ErrNA    = Value{Kind: Error, Str: "#N/A"}
	ErrNum   = Value{Kind: Error, Str: "#NUM!"}
	ErrRef   = Value{Kind: Error, Str: "#REF!"} // also circular references
)

// Num is the number v, or #NUM! for NaN and infinities.
func Num(v float64) Value {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return ErrNum
	}
	return Value{Kind: Number, Num: v}
}

// Boolean is TRUE or FALSE.
func Boolean(b bool) Value {
	if b {
		return Value{Kind: Bool, Num: 1}
	}
	return Value{Kind: Bool}
}

// Str is the text s.
func Str(s string) Value { return Value{Kind: Text, Str: s} }

// ErrOf returns a pointer to a copy of v, for the *Value error results of
// argument helpers. Taking &v of a parameter directly would move it to
// the heap on every call, erroneous or not: one allocation per cell read
// by SUM over a range.
func ErrOf(v Value) *Value { return &v }

// ToNum coerces v for arithmetic as Sheets does: blanks are 0, booleans
// 1 or 0, numeric text (including dates such as "2026-09-26") is its
// number, other text is #VALUE!.
func ToNum(v Value) (float64, *Value) {
	switch v.Kind {
	case Empty:
		return 0, nil
	case Number, Bool:
		return v.Num, nil
	case Text:
		if n, _, ok := ParseValue(v.Str); ok {
			return n, nil
		}
		return 0, &ErrValue
	}
	return 0, ErrOf(v)
}

// AsText converts a value for string concatenation.
func AsText(v Value) string {
	if v.Kind == Empty {
		return ""
	}
	return v.String()
}

// compareText orders text ignoring case, as upper case: byte by byte
// while both are ASCII, which most text is, so comparing a column of it
// allocates nothing.
func compareText(a, b string) int {
	for i := 0; i < len(a) && i < len(b); i++ {
		x, y := a[i], b[i]
		if x >= utf8RuneSelf || y >= utf8RuneSelf {
			return strings.Compare(strings.ToUpper(a[i:]), strings.ToUpper(b[i:]))
		}
		if x >= 'a' && x <= 'z' {
			x -= 'a' - 'A'
		}
		if y >= 'a' && y <= 'z' {
			y -= 'a' - 'A'
		}
		if x != y {
			return cmp.Compare(x, y)
		}
	}
	return cmp.Compare(len(a), len(b))
}

// utf8RuneSelf is where multi-byte UTF-8 starts.
const utf8RuneSelf = 0x80

// Compare orders values like Sheets: numbers < text < booleans, text is
// case-insensitive, and a blank equals 0 or "".
func Compare(l, r Value) int {
	rank := func(v Value) int {
		switch v.Kind {
		case Text:
			return 1
		case Bool:
			return 2
		}
		return 0
	}
	if l.Kind == Empty && r.Kind == Text {
		l = Value{Kind: Text}
	}
	if r.Kind == Empty && l.Kind == Text {
		r = Value{Kind: Text}
	}
	if d := rank(l) - rank(r); d != 0 {
		return d
	}
	if l.Kind == Text {
		return compareText(l.Str, r.Str)
	}
	switch {
	case l.Num < r.Num:
		return -1
	case l.Num > r.Num:
		return 1
	}
	return 0
}
