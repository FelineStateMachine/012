package fileio

import (
	"bytes"
	"context"
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// wk1 encodes a worksheet for tests: 1-2-3 files don't ship with 012.
type wk1 struct{ bytes.Buffer }

func newWK1(version uint16) *wk1 {
	w := &wk1{}
	w.record(wkBOF, le16(version))
	return w
}

func le16(v uint16) []byte { return binary.LittleEndian.AppendUint16(nil, v) }

func le64(f float64) []byte { return binary.LittleEndian.AppendUint64(nil, math.Float64bits(f)) }

func (w *wk1) record(op uint16, payload ...[]byte) {
	body := bytes.Join(payload, nil)
	w.Write(le16(op))
	w.Write(le16(uint16(len(body))))
	w.Write(body)
}

func cellHead(format byte, a string) []byte {
	ad, _ := sheet.ParseAddr(a)
	return append([]byte{format}, append(le16(uint16(ad.Col)), le16(uint16(ad.Row))...)...)
}

func (w *wk1) label(a, text string) { w.record(wkLabel, cellHead(0xFF, a), []byte(text+"\x00")) }
func (w *wk1) number(a string, format byte, v float64) {
	w.record(wkNumber, cellHead(format, a), le64(v))
}
func (w *wk1) integer(a string, v int16) { w.record(wkInteger, cellHead(0xFF, a), le16(uint16(v))) }
func (w *wk1) formula(a string, format byte, cached float64, code []byte) {
	w.record(wkFormula, cellHead(format, a), le64(cached), le16(uint16(len(code))), code)
}
func (w *wk1) colWidth(col uint16, width byte) { w.record(wkColW, le16(col), []byte{width}) }
func (w *wk1) end() []byte                     { w.record(wkEOF); return w.Bytes() }

// Formula bytecode.
func fnum(v float64) []byte { return append([]byte{0x00}, le64(v)...) }
func fint(v int16) []byte   { return append([]byte{0x05}, le16(uint16(v))...) }
func fstr(s string) []byte  { return append(append([]byte{0x06}, s...), 0) }

// fref encodes a reference to target from the formula cell at; relative
// coordinates are stored as offsets with bit 15 set.
func fref(at, target string, absCol, absRow bool) []byte {
	a, _ := sheet.ParseAddr(at)
	t, _ := sheet.ParseAddr(target)
	col := uint16(t.Col)
	if !absCol {
		col = 0x8000 | uint16(uint8(int8(t.Col-a.Col)))
	}
	row := uint16(t.Row)
	if !absRow {
		row = 0x8000 | uint16(t.Row-a.Row)&0x1FFF
	}
	return append(le16(col), le16(row)...)
}
func fcell(at, target string) []byte { return append([]byte{0x01}, fref(at, target, false, false)...) }
func frange(at, from, to string) []byte {
	return append(append([]byte{0x02}, fref(at, from, false, false)...), fref(at, to, false, false)...)
}
func code(parts ...[]byte) []byte { return append(bytes.Join(parts, nil), 0x03) }

func TestWK1Formulas(t *testing.T) {
	op := func(b byte) []byte { return []byte{b} }
	va := func(b byte, n byte) []byte { return []byte{b, n} }
	for _, tc := range []struct {
		name string
		code []byte
		want string
		ok   bool
	}{
		{"reference", code(fcell("C3", "A1")), "A1", true},
		{"absolute", code(append([]byte{0x01}, fref("C3", "B2", true, true)...)), "$B$2", true},
		{"mixed", code(append([]byte{0x01}, fref("C3", "B2", true, false)...)), "$B2", true},
		{"below", code(fcell("A1", "D9")), "D9", true},
		{"precedence", code(fcell("C3", "A1"), fnum(2), op(0x09), fint(3), op(0x0B)), "(A1+2)*3", true},
		{"no extra parentheses", code(fcell("C3", "A1"), fnum(2), fint(3), op(0x0B), op(0x09)), "A1+2*3", true},
		{"left associative", code(fint(1), fint(2), fint(3), op(0x0A), op(0x0A)), "1-(2-3)", true},
		{"written parentheses", code(fint(1), fint(2), op(0x09), op(0x04)), "(1+2)", true},
		{"power and negation", code(fint(2), op(0x08), fint(2), op(0x0D)), "-2^2", true},
		{"negated power", code(fint(2), fint(2), op(0x0D), op(0x08)), "-(2^2)", true},
		{"comparison and logic", code(fcell("C3", "A1"), fint(1), op(0x13), fcell("C3", "B1"), fint(2), op(0x12), op(0x14)), "A1>1#AND#B1<2", true},
		{"not", code(fcell("C3", "A1"), op(0x16)), "#NOT#A1", true},
		{"strings", code(fstr(`say "hi"`), fcell("C3", "A1"), op(0x18)), `"say ""hi"""&A1`, true},
		{"sum", code(frange("C3", "A1", "A2"), fcell("C3", "B1"), va(0x50, 2)), "SUM(A1:A2,B1)", true},
		{"avg count min max", code(frange("C3", "A1", "B2"), va(0x51, 1), frange("C3", "A1", "B2"), va(0x52, 1), op(0x09)), "AVERAGE(A1:B2)+COUNTA(A1:B2)", true},
		{"if", code(fcell("C3", "A1"), fint(0), op(0x13), fstr("yes"), fstr("no"), op(0x3B)), `IF(A1>0,"yes","no")`, true},
		{"round", code(fnum(2.345), fint(2), op(0x3F)), "ROUND(2.345,2)", true},
		{"int truncates", code(fnum(-2.5), op(0x22)), "TRUNC(-2.5)", true},
		{"pi", code(op(0x26), fint(2), op(0x0B)), "PI()*2", true},
		{"pmt reorders", code(fint(1000), fnum(0.01), fint(12), op(0x38)), "PMT(0.01,12,-1000)", true},
		{"choose counts from 0", code(fcell("C3", "A1"), fint(10), fint(20), va(0x30, 3)), "CHOOSE((A1)+1,10,20)", true},
		{"vlookup offset", code(fcell("C3", "A1"), frange("C3", "D1", "E9"), fint(1), op(0x55)), "VLOOKUP(A1,D1:E9,2)", true},
		{"date years since 1900", code(fint(126), fint(9), fint(26), op(0x36)), "DATE(1900+126,9,26)", true},
		{"year", code(fcell("C3", "A1"), op(0x3E)), "(YEAR(A1)-1900)", true},
		{"no equivalent", code(fcell("C3", "A1"), fint(2), op(0x48)), "@STRING(A1,2)", false},
		{"database function", code(frange("C3", "A1", "B9"), fint(1), frange("C3", "D1", "D2"), va(0x5B, 3)), "@DSUM(A1:B9,1,D1:D2)", false},
		{"unknown opcode", code([]byte{0xEE}), "", false},
		{"stack underflow", code(op(0x09)), "", false},
		{"cut off", []byte{0x00, 1, 2}, "", false},
		{"off the sheet", code(fcell("A1", "A1")[:1], []byte{0xFB, 0x80, 0x00, 0x80}), "#REF!", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := lotusFormula(tc.code, addr(t, "C3"), wk1File)
			if tc.name == "below" {
				got, ok = lotusFormula(tc.code, addr(t, "A1"), wk1File)
			}
			if got != tc.want || ok != tc.ok {
				t.Errorf("= %q, %v; want %q, %v", got, ok, tc.want, tc.ok)
			}
			if ok {
				if _, err := sheet.Parse("=" + got); err != nil {
					t.Errorf("%q doesn't parse: %v", got, err)
				}
			}
		})
	}
}

func TestWK1Import(t *testing.T) {
	w := newWK1(0x0406)
	w.record(wkRange, le16(0), le16(0), le16(3), le16(6))
	w.colWidth(0, 14)
	w.label("A1", "'Budget 1986")
	w.label("B1", `"Right`)
	w.label("C1", "^Mid")
	w.label("D1", `\-`)
	w.label("A2", "'123")
	w.label("A3", "caf\xe9")
	w.integer("B2", 42)
	w.number("B3", 0x22, 1450.5)                                                     // currency, 2 decimals
	w.number("B4", 0x31, 0.125)                                                      // percent, 1 decimal
	w.number("B5", 0x72, 31679)                                                      // DD-MMM-YY
	w.number("B6", 0x02, 3.14159)                                                    // fixed, 2 decimals
	w.number("C2", 0x42, 1234567)                                                    // comma, 2 decimals
	w.formula("C3", 0x22, 1492.5, code(fcell("C3", "B2"), fcell("C3", "B3"), op9())) // =B2+B3
	w.formula("C4", 0xFF, 7, code(fcell("C4", "A1"), fint(2), []byte{0x48}))         // @STRING: kept
	w.record(wkString, cellHead(0xFF, "C4"), []byte("1,986.00\x00"))
	w.formula("C5", 0xFF, 5, code(fint(5), []byte{0x7A})) // unknown opcode: kept
	w.record(wkName, make([]byte, 24))
	name := filepath.Join(t.TempDir(), "budget.wk1")
	os.WriteFile(name, w.end(), 0o644)

	res, err := Import(context.Background(), name, Options{Progress: NewProgress()})
	if err != nil {
		t.Fatal(err)
	}
	s := res.Sheet
	for cell, want := range map[string]string{
		"A1": "Budget 1986", "B1": "Right", "C1": "Mid", "D1": "---------", "A2": "123", "A3": "café",
		"B2": "42", "B3": "$1,450.50", "B4": "12.5%", "B5": "24-Sep-86", "B6": "3.14", "C2": "1,234,567.00",
		"C3": "$1,492.50", "C4": "1,986.00", "C5": "5",
	} {
		if g := shown(s, addr(t, cell)); g != want {
			t.Errorf("%s shows %q, want %q", cell, g, want)
		}
	}
	if g := input(s, addr(t, "C3")); g != "=B2+B3" {
		t.Errorf("C3 input %q", g)
	}
	if g := input(s, addr(t, "A2")); g != "'123" {
		t.Errorf("A2 input %q", g)
	}
	if al := s.Cell(addr(t, "B1")).Style.Align; al != sheet.AlignRight {
		t.Errorf("B1 align %v", al)
	}
	if al := s.Cell(addr(t, "C1")).Style.Align; al != sheet.AlignCenter {
		t.Errorf("C1 align %v", al)
	}
	if w := s.ColWidth(0); w != 15 {
		t.Errorf("A width %d", w)
	}
	notes := strings.Join(res.Notes, "; ")
	for _, want := range []string{"1 named range not imported", "2 formulas kept as values, e.g. C4 @STRING(A1,2)"} {
		if !strings.Contains(notes, want) {
			t.Errorf("notes %q, want %q", notes, want)
		}
	}
}

func op9() []byte { return []byte{0x09} }

func TestWK1Errors(t *testing.T) {
	for _, tc := range []struct {
		name string
		data []byte
		want string
	}{
		{"empty", nil, "the file is empty"},
		{"not lotus", []byte("PK\x03\x04 a zip file"), "doesn't start with a BOF record"},
		{"cut off", append(newWK1(0x0406).Bytes(), 0x0F, 0x00, 0x20, 0x00, 1, 2), "record 2 (opcode 0x0F) is cut off"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, _, err := readWK1(context.Background(), bytes.NewReader(tc.data), func(int) {})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %v, want %q", err, tc.want)
			}
		})
	}
	// A file without an EOF record keeps what was read.
	w := newWK1(0x0404)
	w.integer("A1", 7)
	s, _, _, err := readWK1(context.Background(), bytes.NewReader(w.Bytes()), func(int) {})
	if err != nil || shown(s, addr(t, "A1")) != "7" {
		t.Errorf("no EOF: %v", err)
	}
}

func FuzzReadWK1(f *testing.F) {
	w := newWK1(0x0406)
	w.label("A1", "'x")
	w.number("B1", 0x22, 2)
	w.formula("C1", 0xFF, 3, code(fcell("C1", "B1"), fint(1), op9()))
	w.formula("C2", 0xFF, 3, code(frange("C2", "A1", "B1"), []byte{0x50, 1}))
	f.Add(w.end())
	f.Add(newWK1(0x0404).end())
	f.Fuzz(func(t *testing.T, data []byte) {
		readWK1(context.Background(), bytes.NewReader(data), func(int) {})
		lotusFormula(data, sheet.Addr{Col: 3, Row: 3}, wk1File)
	})
}
