package fileio

import (
	"encoding/binary"
	"math"
	"strconv"
	"strings"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// Lotus formulas are stored as reverse Polish bytecode. lotusFormula
// rebuilds the expression in 012's syntax, adding parentheses where 012's
// precedence needs them. It reports false, with the formula written as
// far as possible in 1-2-3's own spelling, when something has no
// equivalent; the importer then keeps the cell's value.

// expr is a subexpression and how tightly it binds, using 012's binding
// powers (see sheet/parse.go), so parentheses go exactly where needed.
type expr struct {
	text  string
	power int
}

const (
	atomPower    = 100
	unaryPower   = 75
	notPower     = 20
	comparePower = 30
)

var lotusBinary = map[byte]struct {
	op    string
	power int
}{
	0x09: {"+", 40}, 0x0A: {"-", 40}, 0x0B: {"*", 50}, 0x0C: {"/", 50}, 0x0D: {"^", 70},
	0x0E: {"=", comparePower}, 0x0F: {"<>", comparePower}, 0x10: {"<=", comparePower},
	0x11: {">=", comparePower}, 0x12: {"<", comparePower}, 0x13: {">", comparePower},
	0x14: {"#AND#", 10}, 0x15: {"#OR#", 10}, 0x18: {"&", 35},
}

// lotusFunc is a 1-2-3 function: its name, its argument count (-1 when a
// count byte follows the opcode), and how to write it in 012's syntax.
// A nil write means 012 has no equivalent.
type lotusFunc struct {
	name  string
	args  int
	write func(args []string) string
}

// call writes a 012 function call.
func call(name string) func([]string) string {
	return func(args []string) string { return name + "(" + strings.Join(args, ",") + ")" }
}

// plusOne adds one to a 0-based 1-2-3 argument, as 012 counts from 1.
func plusOne(s string) string {
	if v, err := strconv.ParseFloat(s, 64); err == nil {
		return numInput(v + 1)
	}
	return "(" + s + ")+1"
}

var lotusFuncs = map[byte]lotusFunc{
	0x1F: {"NA", 0, call("NA")},
	0x20: {"ERR", 0, nil},
	0x21: {"ABS", 1, call("ABS")},
	0x22: {"INT", 1, call("TRUNC")},
	0x23: {"SQRT", 1, call("SQRT")},
	0x24: {"LOG", 1, call("LOG10")},
	0x25: {"LN", 1, call("LN")},
	0x26: {"PI", 0, call("PI")},
	0x27: {"SIN", 1, nil}, 0x28: {"COS", 1, nil}, 0x29: {"TAN", 1, nil},
	0x2A: {"ATAN2", 2, nil}, 0x2B: {"ATAN", 1, nil}, 0x2C: {"ASIN", 1, nil}, 0x2D: {"ACOS", 1, nil},
	0x2E: {"EXP", 1, call("EXP")},
	0x2F: {"MOD", 2, nil}, // 1-2-3 keeps the dividend's sign, Sheets the divisor's
	0x30: {"CHOOSE", -1, func(a []string) string {
		return call("CHOOSE")(append([]string{plusOne(a[0])}, a[1:]...))
	}},
	0x31: {"ISNA", 1, call("ISNA")},
	0x32: {"ISERR", 1, call("ISERROR")},
	0x33: {"FALSE", 0, call("FALSE")},
	0x34: {"TRUE", 0, call("TRUE")},
	0x35: {"RAND", 0, call("RAND")},
	0x36: {"DATE", 3, func(a []string) string { // years since 1900
		return call("DATE")([]string{"1900+" + paren(a[0]), a[1], a[2]})
	}},
	0x37: {"TODAY", 0, call("TODAY")},
	// PMT, PV and FV take (amount, rate, term); Sheets' take the rate
	// and term first, and a present value that is paid out is negative.
	0x38: {"PMT", 3, func(a []string) string { return call("PMT")([]string{a[1], a[2], "-" + paren(a[0])}) }},
	0x39: {"PV", 3, func(a []string) string { return call("PV")([]string{a[1], a[2], "-" + paren(a[0])}) }},
	0x3A: {"FV", 3, func(a []string) string { return call("FV")([]string{a[1], a[2], "-" + paren(a[0])}) }},
	0x3B: {"IF", 3, call("IF")},
	0x3C: {"DAY", 1, call("DAY")},
	0x3D: {"MONTH", 1, call("MONTH")},
	0x3E: {"YEAR", 1, func(a []string) string { return "(" + call("YEAR")(a) + "-1900)" }},
	0x3F: {"ROUND", 2, call("ROUND")},
	0x40: {"TIME", 3, call("TIME")},
	0x41: {"HOUR", 1, call("HOUR")},
	0x42: {"MINUTE", 1, call("MINUTE")},
	0x43: {"SECOND", 1, call("SECOND")},
	0x44: {"ISNUMBER", 1, call("ISNUMBER")},
	0x45: {"ISSTRING", 1, call("ISTEXT")},
	0x46: {"LENGTH", 1, call("LEN")},
	0x47: {"VALUE", 1, call("VALUE")},
	0x48: {"STRING", 2, nil},
	0x49: {"MID", 3, func(a []string) string { return call("MID")([]string{a[0], plusOne(a[1]), a[2]}) }},
	0x4A: {"CHAR", 1, nil},
	0x4B: {"CODE", 1, nil},
	0x4C: {"FIND", 3, func(a []string) string {
		return "(" + call("FIND")([]string{a[0], a[1], plusOne(a[2])}) + "-1)"
	}},
	0x4D: {"DATEVALUE", 1, call("DATEVALUE")},
	0x4E: {"TIMEVALUE", 1, call("TIMEVALUE")},
	0x4F: {"CELLPOINTER", 1, nil},
	0x50: {"SUM", -1, call("SUM")},
	0x51: {"AVG", -1, call("AVERAGE")},
	0x52: {"COUNT", -1, call("COUNTA")},
	0x53: {"MIN", -1, call("MIN")},
	0x54: {"MAX", -1, call("MAX")},
	0x55: {"VLOOKUP", 3, func(a []string) string { return call("VLOOKUP")([]string{a[0], a[1], plusOne(a[2])}) }},
	0x56: {"NPV", 2, call("NPV")},
	0x57: {"VAR", -1, call("VARP")},
	0x58: {"STD", -1, call("STDEVP")},
	0x59: {"IRR", 2, func(a []string) string { return call("IRR")([]string{a[1], a[0]}) }},
	0x5A: {"HLOOKUP", 3, func(a []string) string { return call("HLOOKUP")([]string{a[0], a[1], plusOne(a[2])}) }},
	0x5B: {"DSUM", -1, nil}, 0x5C: {"DAVG", -1, nil}, 0x5D: {"DCNT", -1, nil}, 0x5E: {"DMIN", -1, nil},
	0x5F: {"DMAX", -1, nil}, 0x60: {"DVAR", -1, nil}, 0x61: {"DSTD", -1, nil},
	0x62: {"INDEX", -1, func(a []string) string {
		if len(a) != 3 {
			return ""
		}
		return call("INDEX")([]string{a[0], plusOne(a[2]), plusOne(a[1])})
	}},
	0x63: {"COLS", 1, call("COLUMNS")},
	0x64: {"ROWS", 1, call("ROWS")},
	0x65: {"REPEAT", 2, call("REPT")},
	0x66: {"UPPER", 1, call("UPPER")},
	0x67: {"LOWER", 1, call("LOWER")},
	0x68: {"LEFT", 2, call("LEFT")},
	0x69: {"RIGHT", 2, call("RIGHT")},
	0x6A: {"REPLACE", 4, func(a []string) string {
		return call("REPLACE")([]string{a[0], plusOne(a[1]), a[2], a[3]})
	}},
	0x6B: {"PROPER", 1, call("PROPER")},
	0x6C: {"CELL", 2, nil},
	0x6D: {"TRIM", 1, call("TRIM")},
	0x6E: {"CLEAN", 1, nil},
	0x6F: {"S", 1, nil},
	0x70: {"N", 1, nil},
	0x71: {"EXACT", 2, call("EXACT")},
	0x72: {"APP", 1, nil},
	0x73: {"@", 1, nil},
	0x74: {"RATE", 3, nil},
	0x75: {"TERM", 3, nil},
	0x76: {"CTERM", 3, nil},
	0x77: {"SLN", 3, nil},
	0x78: {"SYD", 4, nil},
	0x79: {"DDB", 4, nil},
}

func paren(s string) string {
	if _, err := strconv.ParseFloat(s, 64); err == nil {
		return s
	}
	if isRef(s) {
		return s
	}
	return "(" + s + ")"
}

func isRef(s string) bool {
	_, ok := sheet.ParseAddr(strings.ReplaceAll(s, "$", ""))
	return ok
}

// lotusRef decodes a cell reference in a formula at cell at. Bit 15 of
// each coordinate marks it relative, stored as a two's complement offset
// from the formula's cell: 8 bits for columns, 11 (.wks) or 13 (.wk1)
// for rows.
func lotusRef(col, row uint16, at sheet.Addr, v wk1Version) (string, bool) {
	a := sheet.Addr{}
	ref := ""
	if col&0x8000 != 0 {
		a.Col = at.Col + int(int8(col&0xFF))
	} else {
		a.Col = int(col & 0xFF)
		ref += "$"
	}
	ref += sheet.ColName(max(a.Col, 0))
	rowBits := uint(13)
	if v == wksFile {
		rowBits = 11
	}
	mask := uint16(1)<<rowBits - 1
	if row&0x8000 != 0 {
		off := int(row & mask)
		if off&(1<<(rowBits-1)) != 0 {
			off -= 1 << rowBits
		}
		a.Row = at.Row + off
	} else {
		a.Row = int(row & mask)
		ref += "$"
	}
	if !a.Valid() {
		return "#REF!", false
	}
	return ref + strconv.Itoa(a.Row+1), true
}

// lotusFormula decodes the bytecode of the formula in cell at.
func lotusFormula(code []byte, at sheet.Addr, v wk1Version) (string, bool) {
	d := &lotusDecoder{code: code, at: at, version: v, ok: true}
	for d.i < len(code) {
		if !d.step() {
			return "", false
		}
	}
	if len(d.stack) != 1 {
		return "", false
	}
	return d.stack[0].text, d.ok
}

// lotusDecoder runs a formula's bytecode on a stack of subexpressions.
// ok turns false when a part has no equivalent in 012; decoding goes on
// so the formula can still be written in 1-2-3's spelling.
type lotusDecoder struct {
	code    []byte
	i       int // the next opcode
	at      sheet.Addr
	version wk1Version
	stack   []expr
	ok      bool
}

// lotusOperands decode the opcodes that are neither binary operators nor
// functions. Each reports false when the bytecode is malformed.
var lotusOperands = map[byte]func(*lotusDecoder) bool{
	0x00: (*lotusDecoder).number,
	0x01: (*lotusDecoder).cell,
	0x02: (*lotusDecoder).cellRange,
	0x03: (*lotusDecoder).end,
	0x04: (*lotusDecoder).parens,
	0x05: (*lotusDecoder).integer,
	0x06: (*lotusDecoder).str,
	0x08: unary("-", unaryPower),
	0x16: unary("#NOT#", notPower),
	0x17: unary("+", unaryPower),
}

// step decodes the opcode at d.i, reporting false when the bytecode is
// malformed.
func (d *lotusDecoder) step() bool {
	op := d.code[d.i]
	if decode, ok := lotusOperands[op]; ok {
		return decode(d)
	}
	if bin, ok := lotusBinary[op]; ok {
		return d.binary(bin.op, bin.power)
	}
	if fn, ok := lotusFuncs[op]; ok {
		return d.function(fn)
	}
	return false
}

func (d *lotusDecoder) push(text string, power int) {
	d.stack = append(d.stack, expr{text, power})
}

func (d *lotusDecoder) pop() (expr, bool) {
	if len(d.stack) == 0 {
		return expr{}, false
	}
	e := d.stack[len(d.stack)-1]
	d.stack = d.stack[:len(d.stack)-1]
	return e, true
}

// has reports whether n bytes of operands follow the opcode.
func (d *lotusDecoder) has(n int) bool { return d.i+1+n <= len(d.code) }

func (d *lotusDecoder) u16(at int) uint16 { return binary.LittleEndian.Uint16(d.code[at:]) }

// literal pushes a number; a negative one binds as a unary minus does.
func (d *lotusDecoder) literal(text string, negative bool) {
	if negative {
		d.push(text, unaryPower)
	} else {
		d.push(text, atomPower)
	}
}

func (d *lotusDecoder) number() bool {
	if !d.has(8) {
		return false
	}
	f := math.Float64frombits(binary.LittleEndian.Uint64(d.code[d.i+1:]))
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return false
	}
	d.literal(numInput(f), f < 0)
	d.i += 9
	return true
}

func (d *lotusDecoder) integer() bool {
	if !d.has(2) {
		return false
	}
	n := int16(d.u16(d.i + 1))
	d.literal(strconv.Itoa(int(n)), n < 0)
	d.i += 3
	return true
}

// ref decodes the cell reference whose coordinates start at code[at].
func (d *lotusDecoder) ref(at int) string {
	ref, good := lotusRef(d.u16(at), d.u16(at+2), d.at, d.version)
	d.ok = d.ok && good
	return ref
}

func (d *lotusDecoder) cell() bool {
	if !d.has(4) {
		return false
	}
	d.push(d.ref(d.i+1), atomPower)
	d.i += 5
	return true
}

func (d *lotusDecoder) cellRange() bool {
	if !d.has(8) {
		return false
	}
	from, to := d.ref(d.i+1), d.ref(d.i+5)
	d.push(from+":"+to, atomPower)
	d.i += 9
	return true
}

func (d *lotusDecoder) end() bool {
	d.i = len(d.code)
	return true
}

// parens are parentheses as the formula was written.
func (d *lotusDecoder) parens() bool {
	e, good := d.pop()
	if !good {
		return false
	}
	d.push("("+e.text+")", atomPower)
	d.i++
	return true
}

func (d *lotusDecoder) str() bool {
	end := indexByte(d.code[d.i+1:], 0)
	if end < 0 {
		return false
	}
	s := lics(string(d.code[d.i+1 : d.i+1+end]))
	d.push(`"`+strings.ReplaceAll(s, `"`, `""`)+`"`, atomPower)
	d.i += end + 2
	return true
}

// unary decodes a prefix operator that binds with the given power.
func unary(sign string, power int) func(*lotusDecoder) bool {
	return func(d *lotusDecoder) bool {
		x, good := d.pop()
		if !good {
			return false
		}
		if x.power < power {
			x.text = "(" + x.text + ")"
		}
		d.push(sign+x.text, power)
		d.i++
		return true
	}
}

func (d *lotusDecoder) binary(op string, power int) bool {
	r, good1 := d.pop()
	l, good2 := d.pop()
	if !good1 || !good2 {
		return false
	}
	if l.power < power {
		l.text = "(" + l.text + ")"
	}
	if r.power <= power {
		r.text = "(" + r.text + ")"
	}
	d.push(l.text+op+r.text, power)
	d.i++
	return true
}

// function decodes a call whose arguments are on the stack. A function
// with no equivalent is written as 1-2-3 spells it, clearing ok.
func (d *lotusDecoder) function(fn lotusFunc) bool {
	n, size := fn.args, 1
	if n < 0 {
		if !d.has(1) {
			return false
		}
		n, size = int(d.code[d.i+1]), 2
	}
	if n > len(d.stack) {
		return false
	}
	args := make([]string, n)
	for k := range args {
		args[k] = d.stack[len(d.stack)-n+k].text
	}
	d.stack = d.stack[:len(d.stack)-n]
	text := ""
	if fn.write != nil && (fn.args >= 0 || n > 0) {
		text = fn.write(args)
	}
	if text == "" {
		d.ok = false
		text = "@" + fn.name
		if n > 0 {
			text += "(" + strings.Join(args, ",") + ")"
		}
	}
	d.push(text, atomPower)
	d.i += size
	return true
}
