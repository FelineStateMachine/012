package sheet

import (
	"slices"
	"strconv"
	"strings"

	"github.com/FelineStateMachine/012/internal/formula"
	"github.com/FelineStateMachine/012/internal/functions"
)

// How Steps shows the formula: as written, with the parts computed so
// far written as their values and the part computed next marked. The
// formula printer writes it, from a copy of the formula in which each
// value stands as a name made of a marker, so the parentheses come out
// as they would; the marked part is found by printing the copy twice,
// once with a marker in its place.

// Segment is a piece of the formula Steps shows.
type Segment struct {
	Text string
	// Value marks a value a part computed, written in its place.
	Value bool
	// Next marks the part computed next.
	Next bool
}

// Markers in the printed formula: a value, \x01 and its index and \x02,
// and the part computed next, \x03.
const (
	valueOpen  = "\x01"
	valueClose = "\x02"
	nextMark   = "\x03"
)

// Text is the formula as it stands: "=" and its text, with the parts
// computed so far as their values and the next part marked, or once
// every part is computed, the formula's value.
func (st *Steps) Text() []Segment {
	if st.Done() {
		return []Segment{{Text: st.Result(), Value: true}}
	}
	shown, values := st.expr, []string(nil)
	var replaced [][]int
	for i := st.next - 1; i >= 0; i-- {
		p := st.parts[i]
		if inside(p.path, replaced) {
			continue
		}
		replaced = append(replaced, p.path)
		mark := formula.Name{Name: valueOpen + strconv.Itoa(len(values)) + valueClose}
		values = append(values, partText(p.Part))
		shown = functions.ReplaceAt(shown, p.path, func(Node) Node { return mark })
	}
	whole := formula.Expr(shown)
	marked := formula.Expr(functions.ReplaceAt(shown, st.parts[st.next].path, func(Node) Node { return formula.Name{Name: nextMark} }))
	at := strings.Index(marked, nextMark)
	after := marked[at+len(nextMark):]
	end := len(whole) - len(after)
	out := []Segment{{Text: "="}}
	out = appendValues(out, whole[:at], values, false)
	out = appendValues(out, whole[at:end], values, true)
	return appendValues(out, whole[end:], values, false)
}

// inside reports whether path is within one of paths.
func inside(path []int, paths [][]int) bool {
	for _, p := range paths {
		if len(p) <= len(path) && slices.Equal(p, path[:len(p)]) {
			return true
		}
	}
	return false
}

// appendValues splits text at its value markers into segments.
func appendValues(out []Segment, text string, values []string, next bool) []Segment {
	for text != "" {
		i := strings.Index(text, valueOpen)
		if i < 0 {
			return append(out, Segment{Text: text, Next: next})
		}
		if i > 0 {
			out = append(out, Segment{Text: text[:i], Next: next})
		}
		j := strings.Index(text, valueClose)
		k, _ := strconv.Atoi(text[i+len(valueOpen) : j])
		out = append(out, Segment{Text: values[k], Value: true, Next: next})
		text = text[j+len(valueClose):]
	}
	return out
}

// partText writes what a part computed as a formula would write it: a
// number plainly, text in quotes, an array in braces (its first rows and
// columns, then …). A blank reads as 0, as it does in arithmetic.
func partText(p functions.Part) string {
	switch {
	case p.Lambda:
		return "LAMBDA"
	case p.Array != nil:
		return arrayText(p.Array)
	}
	return valueText(p.Value)
}

func valueText(v Value) string {
	switch v.Kind {
	case Empty:
		return "0"
	case Text:
		return `"` + strings.ReplaceAll(v.Str, `"`, `""`) + `"`
	}
	return v.String()
}

func arrayText(a *functions.Array) string {
	var b strings.Builder
	b.WriteByte('{')
	for r := range a.DRows {
		if r > 0 {
			b.WriteByte(';')
		}
		for c := range a.DCols {
			if c > 0 {
				b.WriteByte(',')
			}
			b.WriteString(valueText(a.At(r, c)))
		}
		if a.Cols > a.DCols {
			b.WriteString(",…")
		}
	}
	if a.Rows > a.DRows {
		b.WriteString(";…")
	}
	b.WriteByte('}')
	return b.String()
}

// pathKey is a path as a map key.
func pathKey(path []int) string {
	var b strings.Builder
	for _, i := range path {
		b.WriteString(strconv.Itoa(i))
		b.WriteByte('.')
	}
	return b.String()
}
