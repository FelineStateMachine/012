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
// isn't the value's own text. What pivots and spills write (derived
// cells) is a slot too, marked by its kind: the value alone, its entry
// being the value's text (derivedInput), and a spill's inferred format
// in its look. Everything else (formulas, notes, text typed with a
// leading ') is rich: the slot points at a whole Cell in the store's
// side table.
type slot struct {
	num float64 // slotNum's value; slotBool's, 1 or 0
	// ref is slotText's text in strs; slotNum's or slotBool's input in
	// strs when it isn't the value's text (0 when it is); slotRich's cell
	// in rich.
	ref  uint32
	look uint16 // the format, style and inferred format in looks; 0 for none
	// kind is one of the kinds below, with slotPivot or slotSpill added
	// for a derived cell.
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
	slotErr // a derived cell's error value, its code in strs
)

// Marks of a derived cell's slot, added to its kind: a pivot's result
// (Cell.derived) or part of a spilled array (Cell.spilled).
const (
	slotPivot   uint8 = 1 << 6
	slotSpill   uint8 = 1 << 7
	slotDerived       = slotPivot | slotSpill
)

// base is sl's kind without the marks of a derived cell.
func (sl slot) base() uint8 { return sl.kind &^ slotDerived }

// maxDec bounds the decimals a number's input may have to be kept as a
// count rather than as text.
const maxDec = 40

// look is a cell's format and style, and a spilled cell's inferred
// format, kept once per store however many cells share it.
type look struct {
	f    Format
	st   Style
	auto Format
}

// strTable holds the text of plain cells, each distinct string once,
// counted by the cells using it; entry 0 is unused.
type strTable struct {
	strs  []string
	refs  []uint32
	index map[string]uint32
	free  []uint32
	bytes int64 // the estimated heap the strings hold, strBytes and their text each
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
	t.bytes += strBytes + int64(len(s))
	return i
}

// release counts one fewer use of entry i, if it is one.
func (t *strTable) release(i uint32) {
	if i == 0 {
		return
	}
	if t.refs[i]--; t.refs[i] == 0 {
		t.bytes -= strBytes + int64(len(t.strs[i]))
		delete(t.index, t.strs[i])
		t.strs[i] = ""
		t.free = append(t.free, i)
	}
}

// lookID is the entry of l in the table of looks, added if new, and
// false when the table is full.
func (st *cellStore) lookID(l look) (uint16, bool) {
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

// lookOf is sl's look, which the caller mustn't change.
func (st *cellStore) lookOf(sl slot) *look {
	if sl.look == 0 {
		return &noLook
	}
	return &st.looks[sl.look]
}

// noLook is the look of a slot without one, never written.
var noLook look

// plainSlot is c as a plain slot, or false if c is rich. It adds the
// strings it uses to the table.
func (st *cellStore) plainSlot(c *Cell) (slot, bool) {
	switch {
	case c.Note != "" || c.typedText:
		return slot{}, false
	case c.derived || c.spilled:
		return st.derivedSlot(c)
	case !c.auto.IsZero() || strings.HasPrefix(c.Input, "="):
		return slot{}, false
	}
	lk, ok := st.lookID(look{f: c.Format, st: c.Style})
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

// derivedSlot is plainSlot for a derived cell without a note: its value,
// marked with what derived it, its entry being the value's text.
func (st *cellStore) derivedSlot(c *Cell) (slot, bool) {
	if c.expr != nil {
		return slot{}, false
	}
	lk, ok := st.lookID(look{c.Format, c.Style, c.auto})
	if !ok {
		return slot{}, false
	}
	return st.derivedValue(c.Value, lk, c.derived), true
}

// derivedValue is the slot of a derived cell showing v with look lk: a
// pivot's result if pivot, else a spilled cell. It adds the string v
// uses to the table.
func (st *cellStore) derivedValue(v Value, lk uint16, pivot bool) slot {
	sl := slot{look: lk, kind: slotSpill}
	if pivot {
		sl.kind = slotPivot
	}
	switch v.Kind {
	case Number:
		sl.kind |= slotNum
		sl.num = v.Num
	case Bool:
		sl.kind |= slotBool
		sl.num = v.Num
	case Text:
		sl.kind |= slotText
		sl.ref = st.strs.add(v.Str)
	case Error:
		sl.kind |= slotErr
		sl.ref = st.strs.add(v.Str)
	}
	return sl
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

// input is the entry of a plain slot. A derived slot's is its value's
// text, as derivedInput writes it: a number's shortest text, TRUE or
// FALSE, the text or the error code.
func (st *cellStore) input(sl slot) string {
	switch {
	case sl.base() == slotBlank:
		return ""
	case sl.ref != 0:
		return st.strs.strs[sl.ref]
	case sl.base() == slotBool:
		return boolText(sl.num != 0)
	}
	return strconv.FormatFloat(sl.num, 'f', int(sl.dec)-1, 64)
}

// slotValue is the value of a plain slot.
func (st *cellStore) slotValue(sl slot) Value {
	switch sl.base() {
	case slotNum:
		return Value{Kind: Number, Num: sl.num}
	case slotBool:
		return Value{Kind: Bool, Num: sl.num}
	case slotText:
		return Value{Kind: Text, Str: st.strs.strs[sl.ref]}
	case slotErr:
		return Value{Kind: Error, Str: st.strs.strs[sl.ref]}
	}
	return Value{}
}

// view is a plain slot as a Cell of its own: changing it changes nothing
// stored.
func (st *cellStore) view(sl slot) *Cell {
	lk := st.lookOf(sl)
	c := &Cell{Input: st.input(sl), Value: st.slotValue(sl), Format: lk.f, Style: lk.st}
	if sl.kind&slotDerived != 0 { // a derived cell has no expression
		c.auto, c.derived, c.spilled = lk.auto, sl.kind&slotPivot != 0, sl.kind&slotSpill != 0
		return c
	}
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
	switch sl.base() {
	case slotRich:
		st.rich[sl.ref] = richCell{}
		st.richFree = append(st.richFree, sl.ref)
	case slotNum, slotBool, slotText, slotErr:
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
	return n + st.strs.bytes
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
