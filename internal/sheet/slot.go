package sheet

import (
	"math"
	"strconv"
	"strings"

	"github.com/FelineStateMachine/012/internal/formula"
)

// A slot is how cellStore keeps one cell: 16 bytes in its column's
// block. Most cells are plain, an entry typed or imported that is a
// number, a boolean or text, with at most a format and a style; their
// slot holds the value itself, the text in the store's string table, the
// format and style in its table of looks, and the input only when it
// isn't the value's own text. Everything else (formulas, notes, what
// pivots and spills write, text typed with a leading ') is rich: the
// slot points at a whole Cell in the store's side table.
type slot struct {
	num float64 // slotNum's value; slotBool's, 1 or 0
	// ref is slotText's text in strs; slotNum's or slotBool's input in
	// strs when it isn't the value's text (0 when it is); slotRich's cell
	// in rich.
	ref  uint32
	look uint16 // the format and style in looks; 0 for neither
	kind uint8
	// dec is how a slotNum without ref prints its input: the value with
	// dec-1 decimals, or its shortest text when 0.
	dec uint8
}

// Kinds of slot.
const (
	slotBlank uint8 = iota // formatting only
	slotNum
	slotBool
	slotText
	slotRich
)

// maxDec bounds the decimals a number's input may have to be kept as a
// count rather than as text.
const maxDec = 40

// look is a cell's format and style, kept once per store however many
// cells share it.
type look struct {
	f  Format
	st Style
}

// strTable holds the text of plain cells, each distinct string once,
// counted by the cells using it; entry 0 is unused.
type strTable struct {
	strs  []string
	refs  []uint32
	index map[string]uint32
	free  []uint32
}

// add counts one more use of s and returns its entry.
func (t *strTable) add(s string) uint32 {
	if i, ok := t.index[s]; ok {
		t.refs[i]++
		return i
	}
	if t.index == nil {
		t.index = map[string]uint32{}
		t.strs, t.refs = []string{""}, []uint32{0}
	}
	var i uint32
	if n := len(t.free); n > 0 {
		i, t.free = t.free[n-1], t.free[:n-1]
		t.strs[i], t.refs[i] = s, 1
	} else {
		i = uint32(len(t.strs))
		t.strs, t.refs = append(t.strs, s), append(t.refs, 1)
	}
	t.index[s] = i
	return i
}

// release counts one fewer use of entry i, if it is one.
func (t *strTable) release(i uint32) {
	if i == 0 {
		return
	}
	if t.refs[i]--; t.refs[i] == 0 {
		delete(t.index, t.strs[i])
		t.strs[i] = ""
		t.free = append(t.free, i)
	}
}

// lookID is the entry of l in the table of looks, added if new, and
// false when the table is full.
func (st *cellStore) lookID(f Format, sty Style) (uint16, bool) {
	l := look{f, sty}
	if l == (look{}) {
		return 0, true
	}
	if i, ok := st.lookIdx[l]; ok {
		return i, true
	}
	if len(st.looks) > math.MaxUint16 {
		return 0, false
	}
	if st.lookIdx == nil {
		st.lookIdx = map[look]uint16{}
		st.looks = []look{{}}
	}
	i := uint16(len(st.looks))
	st.looks = append(st.looks, l)
	st.lookIdx[l] = i
	return i, true
}

// lookOf is sl's format and style.
func (st *cellStore) lookOf(sl slot) look {
	if sl.look == 0 {
		return look{}
	}
	return st.looks[sl.look]
}

// plainSlot is c as a plain slot, or false if c is rich. It adds the
// strings it uses to the table.
func (st *cellStore) plainSlot(c *Cell) (slot, bool) {
	if c.Note != "" || c.derived || c.spilled || !c.auto.IsZero() || strings.HasPrefix(c.Input, "=") {
		return slot{}, false
	}
	lk, ok := st.lookID(c.Format, c.Style)
	if !ok {
		return slot{}, false
	}
	sl := slot{look: lk}
	switch e := c.expr.(type) {
	case nil:
		switch {
		case c.Input == "":
			sl.kind = slotBlank
		case c.Format.Kind != FmtText && strings.HasPrefix(c.Input, "'"):
			return slot{}, false // its value isn't its input
		default:
			sl.kind, sl.ref = slotText, st.strs.add(c.Input)
		}
	case formula.Num:
		if math.IsNaN(e.V) || math.IsInf(e.V, 0) {
			return slot{}, false // its value is #NUM!
		}
		sl.kind, sl.num = slotNum, e.V
		if d, ok := numDec(e.V, c.Input); ok {
			sl.dec = d
		} else {
			sl.ref = st.strs.add(c.Input)
		}
	case formula.Bool:
		sl.kind = slotBool
		if e.V {
			sl.num = 1
		}
		if c.Input != boolText(e.V) {
			sl.ref = st.strs.add(c.Input)
		}
	default:
		return slot{}, false
	}
	return sl, true
}

// numDec is how input prints v, as slot.dec, or false if it isn't v
// printed plainly.
func numDec(v float64, input string) (uint8, bool) {
	if d, ok := plainDec(v, input); ok {
		return d, true
	}
	var buf [64]byte
	if string(strconv.AppendFloat(buf[:0], v, 'f', -1, 64)) == input {
		return 0, true
	}
	i := strings.IndexByte(input, '.')
	if i < 0 || len(input)-i-1 >= maxDec {
		return 0, false
	}
	d := len(input) - i - 1
	if string(strconv.AppendFloat(buf[:0], v, 'f', d, 64)) == input {
		return uint8(d + 1), true
	}
	return 0, false
}

// plainDec is numDec for the common input, a plain decimal (see
// plainForm) that is v, found without printing v. It reports false for
// any other input, which numDec then prints v to compare.
func plainDec(v float64, input string) (uint8, bool) {
	d, ok := plainForm(input)
	if !ok {
		return 0, false
	}
	if p, err := strconv.ParseFloat(input, 64); err != nil || math.Float64bits(p) != math.Float64bits(v) {
		return 0, false
	}
	return d, true
}

// plainForm reports whether input is a plain decimal, of at most 15
// significant digits without a sign but "-", leading zeros or an
// exponent, and returns how it prints its value as slot.dec. Such a
// decimal is its value's shortest text when it doesn't end in a zero
// after the point, and its value printed with as many decimals as it
// has otherwise: 15 digits always come back from a float64.
func plainForm(input string) (uint8, bool) {
	s := input
	if s != "" && s[0] == '-' {
		s = s[1:]
	}
	if s == "" || s[0] == '0' && len(s) > 1 && s[1] != '.' {
		return 0, false
	}
	point, digits, lead := -1, 0, true
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case c == '.' && point < 0 && i > 0 && i < len(s)-1:
			point = i
		case c >= '0' && c <= '9':
			if lead = lead && c == '0'; !lead {
				digits++
			}
		default:
			return 0, false
		}
	}
	d := 0
	if point >= 0 {
		d = len(s) - point - 1
	}
	if digits > 15 || d >= maxDec {
		return 0, false
	}
	if d == 0 || s[len(s)-1] != '0' {
		return 0, true
	}
	return uint8(d + 1), true
}

func boolText(b bool) string {
	if b {
		return "TRUE"
	}
	return "FALSE"
}

// input is the entry of a plain slot.
func (st *cellStore) input(sl slot) string {
	switch {
	case sl.kind == slotBlank:
		return ""
	case sl.ref != 0:
		return st.strs.strs[sl.ref]
	case sl.kind == slotBool:
		return boolText(sl.num != 0)
	}
	return strconv.FormatFloat(sl.num, 'f', int(sl.dec)-1, 64)
}

// slotValue is the value of a plain slot.
func (st *cellStore) slotValue(sl slot) Value {
	switch sl.kind {
	case slotNum:
		return Value{Kind: Number, Num: sl.num}
	case slotBool:
		return Value{Kind: Bool, Num: sl.num}
	case slotText:
		return Value{Kind: Text, Str: st.strs.strs[sl.ref]}
	}
	return Value{}
}

// view is a plain slot as a Cell of its own: changing it changes nothing
// stored.
func (st *cellStore) view(sl slot) *Cell {
	lk := st.lookOf(sl)
	c := &Cell{Input: st.input(sl), Value: st.slotValue(sl), Format: lk.f, Style: lk.st}
	switch sl.kind {
	case slotNum:
		c.expr = formula.Num{V: sl.num}
	case slotBool:
		c.expr = formula.Bool{V: sl.num != 0}
	}
	return c
}

// releaseSlot counts one fewer use of what sl refers to.
func (st *cellStore) releaseSlot(sl slot) {
	switch sl.kind {
	case slotRich:
		st.rich[sl.ref] = richCell{}
		st.richFree = append(st.richFree, sl.ref)
	case slotNum, slotBool, slotText:
		st.strs.release(sl.ref)
	}
}

// Estimated heap of the store's parts, for the undo history's budget
// (historysize.go): a slot with its share of the blocks and indexes, as
// BenchmarkMemory measures it, and a string's entry in the table.
const (
	slotBytes = 20
	strBytes  = 64
)

// size estimates the heap the cells hold: a slot each, and the rich
// cells and strings on top.
func (st *cellStore) size() int64 {
	n := int64(st.len()) * slotBytes
	for _, rc := range st.rich {
		n += cellSize(rc.c)
	}
	for _, s := range st.strs.strs {
		n += strBytes + int64(len(s))
	}
	return n
}

// richCell is a rich cell and where it is.
type richCell struct {
	c *Cell
	a Addr
}

// addRich stores c, at a, in the side table and returns its entry.
func (st *cellStore) addRich(a Addr, c *Cell) uint32 {
	if n := len(st.richFree); n > 0 {
		i := st.richFree[n-1]
		st.richFree = st.richFree[:n-1]
		st.rich[i] = richCell{c, a}
		return i
	}
	st.rich = append(st.rich, richCell{c, a})
	return uint32(len(st.rich) - 1)
}
