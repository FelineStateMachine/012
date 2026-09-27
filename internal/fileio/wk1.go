package fileio

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"strings"

	"012/internal/sheet"
)

// Lotus 1-2-3 worksheets (.wks from Release 1A, .wk1 from Release 2) are
// a stream of little-endian records: a 16-bit opcode, a 16-bit length
// and the payload. This reader follows the LibreOffice and Gnumeric
// import filters.

const (
	wkBOF     = 0x00
	wkEOF     = 0x01
	wkRange   = 0x06 // the active area
	wkColW    = 0x08 // column width
	wkName    = 0x0B // a named range
	wkBlank   = 0x0C
	wkInteger = 0x0D
	wkNumber  = 0x0E
	wkLabel   = 0x0F
	wkFormula = 0x10
	wkString  = 0x33 // the text result of the formula before it
)

// wk1Version tells .wks (2,048 rows) from .wk1 (8,192 rows), which
// encode relative row offsets in formulas with different widths.
type wk1Version int

const (
	wksFile wk1Version = iota
	wk1File
)

func importWK1(ctx context.Context, name string, prog *Progress) (*Result, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	defer context.AfterFunc(ctx, func() { f.Close() })() // unblocks a read from a pipe
	var size int64
	if st, err := f.Stat(); err == nil {
		size = st.Size()
	}
	cr := &countingReader{r: f}
	s, notes, rows, err := readWK1(ctx, bufio.NewReader(cr), func(cells int) {
		prog.setRows(cells)
		prog.setFrac(cr.n, size)
	})
	if err != nil {
		return nil, err
	}
	return &Result{Sheet: s, Rows: rows, Notes: notes}, nil
}

// wk1Reader holds the state of one import.
type wk1Reader struct {
	b       *builder
	version wk1Version
	// pending is a formula whose value was kept, waiting for a STRING
	// record with its text result.
	pending   sheet.Addr
	hasString bool
	style     map[sheet.Addr]sheet.Style
	names     int
	maxRow    int
}

// readWK1 reads a worksheet record by record. progress gets the number
// of cells read so far.
func readWK1(ctx context.Context, r io.Reader, progress func(cells int)) (*sheet.Sheet, []string, int, error) {
	w := &wk1Reader{b: newBuilder(), version: wk1File}
	var head [4]byte
	cells := 0
	for i := 0; ; i++ {
		if _, err := io.ReadFull(r, head[:]); err != nil {
			if i == 0 {
				return nil, nil, 0, errors.New("not a Lotus 1-2-3 worksheet: the file is empty")
			}
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				break // no EOF record; keep what was read, as 1-2-3 does
			}
			return nil, nil, 0, err
		}
		op := binary.LittleEndian.Uint16(head[0:])
		n := binary.LittleEndian.Uint16(head[2:])
		if i == 0 && (op != wkBOF || n != 2) {
			return nil, nil, 0, errors.New("not a Lotus 1-2-3 worksheet: it doesn't start with a BOF record")
		}
		data := make([]byte, n)
		if _, err := io.ReadFull(r, data); err != nil {
			return nil, nil, 0, fmt.Errorf("record %d (opcode 0x%02X) is cut off", i+1, op)
		}
		if op == wkEOF {
			break
		}
		if w.record(op, data) {
			cells++
		}
		if i%512 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, nil, 0, err
			}
			progress(cells)
		}
	}
	progress(cells)
	var notes []string
	if w.names > 0 {
		notes = append(notes, count(w.names, "named range", "named ranges")+" not imported")
	}
	s, notes := w.b.finish(notes)
	return s, notes, w.maxRow + 1, nil
}

// record handles one record, reporting whether it was a cell.
func (w *wk1Reader) record(op uint16, d []byte) bool {
	switch op {
	case wkBOF:
		if len(d) >= 2 && binary.LittleEndian.Uint16(d) == 0x0404 {
			w.version = wksFile
		}
		return false
	case wkColW:
		if len(d) >= 3 {
			col := int(binary.LittleEndian.Uint16(d))
			if col < sheet.MaxCols && d[2] > 0 {
				w.b.s.SetColWidth(col, int(d[2])+1)
			}
		}
		return false
	case wkName:
		w.names++
		return false
	case wkBlank, wkInteger, wkNumber, wkLabel, wkFormula, wkString:
	default:
		return false
	}
	if len(d) < 5 {
		return false
	}
	format := d[0]
	a := sheet.Addr{Col: int(binary.LittleEndian.Uint16(d[1:])), Row: int(binary.LittleEndian.Uint16(d[3:]))}
	body := d[5:]
	f := lotusFormat(format)
	w.maxRow = max(w.maxRow, a.Row)
	switch op {
	case wkBlank:
		w.b.put(a, "", f, sheet.Style{})
	case wkInteger:
		if len(body) < 2 {
			return false
		}
		w.b.number(a, float64(int16(binary.LittleEndian.Uint16(body))), f, sheet.Style{})
	case wkNumber:
		if len(body) < 8 {
			return false
		}
		v := math.Float64frombits(binary.LittleEndian.Uint64(body))
		w.b.number(a, fromExcelSerial(v, f), f, sheet.Style{})
	case wkLabel:
		w.label(a, body)
	case wkFormula:
		if len(body) < 10 {
			return false
		}
		v := math.Float64frombits(binary.LittleEndian.Uint64(body))
		size := int(binary.LittleEndian.Uint16(body[8:]))
		code := body[10:]
		if size < len(code) {
			code = code[:size]
		}
		w.hasString = false
		text, ok := lotusFormula(code, a, w.version)
		keep := func() {
			w.b.number(a, fromExcelSerial(v, f), f, sheet.Style{})
			w.pending, w.hasString = a, true
		}
		if !ok {
			w.b.kept(a, text, keep)
			return true
		}
		w.b.formula(a, "="+text, f, sheet.Style{}, keep)
	case wkString:
		if w.hasString && w.pending == a {
			w.b.text(a, lics(cstring(body)), sheet.Format{}, sheet.Style{})
		}
		w.hasString = false
		return false
	}
	return true
}

// label stores a label, whose first character sets its alignment: '
// left, " right, ^ centered, \ repeated to fill the cell.
func (w *wk1Reader) label(a sheet.Addr, body []byte) {
	s := lics(cstring(body))
	if s == "" {
		return
	}
	var st sheet.Style
	switch s[0] {
	case '\'':
		s = s[1:]
	case '"':
		s, st.Align = s[1:], sheet.AlignRight
	case '^':
		s, st.Align = s[1:], sheet.AlignCenter
	case '\\':
		s = s[1:]
		if s != "" {
			width := w.b.s.ColWidth(a.Col) - 1
			s = strings.Repeat(s, max(width/len([]rune(s)), 1))
		}
	case '|':
		s = s[1:]
	}
	w.b.text(a, s, sheet.Format{}, st)
}

// cstring is the NUL-terminated string at the start of b.
func cstring(b []byte) string {
	if i := indexByte(b, 0); i >= 0 {
		b = b[:i]
	}
	return string(b)
}

func indexByte(b []byte, c byte) int {
	for i, x := range b {
		if x == c {
			return i
		}
	}
	return -1
}

// lics decodes the Lotus International Character Set as Latin-1, which
// it matches for the accented letters people used.
func lics(s string) string {
	ascii := true
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			ascii = false
			break
		}
	}
	if ascii {
		return s
	}
	r := make([]rune, len(s))
	for i := 0; i < len(s); i++ {
		r[i] = rune(s[i])
	}
	return string(r)
}

// lotusFormat maps a cell's format byte: bit 7 is protection, bits 4-6
// the type and bits 0-3 decimals, or for type 7 a special format.
func lotusFormat(b byte) sheet.Format {
	typ, n := (b>>4)&7, int(b&0x0F)
	dec := min(n, sheet.MaxDecimals)
	switch typ {
	case 0: // fixed
		pat := "0"
		if dec > 0 {
			pat += "." + strings.Repeat("0", dec)
		}
		return sheet.Format{Kind: sheet.FmtCustom, Pattern: pat}
	case 1:
		return sheet.Format{Kind: sheet.FmtScientific, Decimals: dec}
	case 2:
		return sheet.Format{Kind: sheet.FmtCurrency, Decimals: dec}
	case 3:
		return sheet.Format{Kind: sheet.FmtPercent, Decimals: dec}
	case 4:
		return sheet.Format{Kind: sheet.FmtNumber, Decimals: dec}
	case 7:
		switch n {
		case 2:
			return sheet.Format{Kind: sheet.FmtDate, Pattern: "d-mmm-yy"}
		case 3:
			return sheet.Format{Kind: sheet.FmtDate, Pattern: "d-mmm"}
		case 4:
			return sheet.Format{Kind: sheet.FmtDate, Pattern: "mmm-yy"}
		case 7:
			return sheet.Format{Kind: sheet.FmtTime, Pattern: "h:mm:ss AM/PM"}
		case 8:
			return sheet.Format{Kind: sheet.FmtTime, Pattern: "h:mm AM/PM"}
		case 9:
			return sheet.Format{Kind: sheet.FmtDate, Pattern: "mm/dd/yy"}
		case 10:
			return sheet.Format{Kind: sheet.FmtDate, Pattern: "mm/dd"}
		case 11:
			return sheet.Format{Kind: sheet.FmtTime, Pattern: "hh:mm:ss"}
		case 12:
			return sheet.Format{Kind: sheet.FmtTime, Pattern: "hh:mm"}
		}
	}
	return sheet.Format{} // general, +/-, text (shows formulas), hidden, default
}
