package fileio

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"runtime"
	"strings"
	"sync"

	"github.com/FelineStateMachine/012/internal/locale"
	"github.com/FelineStateMachine/012/internal/nuon"
	"github.com/FelineStateMachine/012/internal/sheet"
)

// Tails: a text table read as it arrives in pieces, as a file followed
// grows. The text is parsed by the importers' own readers (encoding/csv
// and nuon.Reader) on a goroutine of the tail's, reading from a pipe
// that waits for the next piece: a record or value cut off at the end of
// one piece waits there for the rest, so a partial last line is never
// read as a row. Rows come out typed as the importer types them: CSV and
// TSV fields as if typed in the locale, NUON and JSON values with their
// types (nuonCell).

// Grows reports whether a file of kind k can be followed as it grows,
// its new rows read without reading it again: text tables, which
// Tail reads.
func (k Kind) Grows() bool { return k == CSV || k == TSV || k == JSON || k == NUON }

// TailRows are the rows a piece of text completed.
type TailRows struct {
	// Header is the table's first row when it's new or changed: a CSV
	// file's first record, or NUON's column names as they're seen.
	Header sheet.LiveRow
	Rows   []sheet.LiveRow
}

// A Tail reads a table of kind k from text that arrives in pieces.
type Tail struct {
	p   *tailPipe
	out *tailOut
}

// tailPipe hands pieces of text to the tail's goroutine and tells when
// it has read them all: it has then parsed every row they complete.
type tailPipe struct {
	in      chan []byte
	starved chan struct{} // the goroutine wants more
	done    chan struct{} // the goroutine ended
	buf     []byte
	eof     bool
	once    sync.Once
}

// tailOut is what the goroutine parsed since the last piece; the Tail
// reads it only while the goroutine waits.
type tailOut struct {
	header  sheet.LiveRow
	changed bool // header changed
	rows    []sheet.LiveRow
	err     error
}

// Read gives the goroutine's reader the rest of the piece, or waits for
// the next one.
func (p *tailPipe) Read(b []byte) (int, error) {
	for len(p.buf) == 0 {
		if p.eof {
			return 0, io.EOF
		}
		p.starved <- struct{}{}
		chunk, ok := <-p.in
		if !ok {
			p.eof = true
			return 0, io.EOF
		}
		p.buf = chunk
	}
	n := copy(b, p.buf)
	p.buf = p.buf[n:]
	return n, nil
}

func (p *tailPipe) close() { p.once.Do(func() { close(p.in) }) }

// NewTail starts reading a table of kind k (see Kind.Grows). head is the
// start of the text, what it's sniffed by (its encoding, and a CSV
// file's delimiter and decimal separator), in the locale loc; the text
// itself comes through Feed, head included.
func NewTail(k Kind, head []byte, loc *locale.Locale) (*Tail, error) {
	if !k.Grows() {
		return nil, fmt.Errorf("can't follow %s files as they grow", k)
	}
	p := &tailPipe{in: make(chan []byte), starved: make(chan struct{}), done: make(chan struct{})}
	out := &tailOut{}
	var parse func()
	switch k {
	case CSV, TSV:
		parse = delimitedTail(p, out, k, head, loc)
	default:
		parse = tableTail(p, out)
	}
	go func() {
		defer close(p.done)
		parse()
	}()
	t := &Tail{p: p, out: out}
	// A tail dropped without Close ends its goroutine too.
	runtime.AddCleanup(t, func(p *tailPipe) { p.close() }, p)
	select {
	case <-p.starved:
	case <-p.done:
	}
	return t, nil
}

// errTailClosed is Feed's error once the tail is closed or failed.
var errTailClosed = errors.New("the tail is closed")

// Feed reads the next piece of text and returns the rows it completed.
// After an error the tail reads nothing more.
func (t *Tail) Feed(piece []byte) (TailRows, error) {
	select {
	case <-t.p.done:
		return TailRows{}, t.failure()
	default:
	}
	if len(piece) > 0 {
		t.p.in <- piece
		select {
		case <-t.p.starved:
		case <-t.p.done:
		}
	}
	out := t.out
	res := TailRows{Rows: out.rows}
	if out.changed {
		res.Header = out.header
	}
	out.rows, out.changed = nil, false
	if out.err != nil {
		return res, out.err
	}
	return res, nil
}

// failure is why the goroutine ended.
func (t *Tail) failure() error {
	if t.out.err != nil {
		return t.out.err
	}
	return errTailClosed
}

// Close ends the tail's goroutine.
func (t *Tail) Close() { t.p.close() }

// delimitedTail parses CSV or TSV: fields as if typed, the delimiter and
// the numbers' decimal separator sniffed from head as an import does.
func delimitedTail(p *tailPipe, out *tailOut, k Kind, head []byte, loc *locale.Locale) func() {
	enc, bom := encodingOf(head)
	sample := head[bom:]
	if enc != encUTF8 {
		b, _ := io.ReadAll(decodeAs(strings.NewReader(string(sample)), enc))
		sample = b
	}
	comma := '\t'
	if k == CSV {
		comma = sniff(sample, len(head) >= sniffSize, csvComma(loc))
	}
	loc, _ = numberLocale(sample, comma, loc)
	return func() {
		skip := &skipReader{r: p, n: bom}
		cr := csv.NewReader(decodeAs(skip, enc))
		cr.Comma, cr.LazyQuotes, cr.FieldsPerRecord = comma, true, -1
		first := true
		for {
			rec, err := cr.Read()
			if err != nil {
				if !errors.Is(err, io.EOF) {
					out.err = err
				}
				return
			}
			row := make(sheet.LiveRow, len(rec))
			for i, field := range rec {
				if field != "" {
					v, f := sheet.EntryValue(entryInput(field, loc))
					row[i] = sheet.LiveCell{V: v, F: f}
				}
			}
			if first {
				first, out.header, out.changed = false, row, true
				continue
			}
			out.rows = append(out.rows, row)
		}
	}
}

// skipReader leaves out the first n bytes of r: a byte order mark.
type skipReader struct {
	r io.Reader
	n int
}

func (s *skipReader) Read(b []byte) (int, error) {
	for s.n > 0 {
		var one [4]byte
		m, err := s.r.Read(one[:min(s.n, len(one))])
		s.n -= m
		if err != nil {
			return 0, err
		}
	}
	return s.r.Read(b)
}

// tableTail parses NUON or JSON: the columns named as rows name them,
// the header growing as new names appear.
func tableTail(p *tailPipe, out *tailOut) func() {
	return func() {
		r := nuon.NewReader(p)
		cols := map[string]int{}
		zone := zone()
		col := func(name string) int {
			i, ok := cols[name]
			if !ok {
				i = len(cols)
				cols[name] = i
				out.header = append(out.header, textCell(name))
				out.changed = true
			}
			return i
		}
		for {
			fields, err := r.Next()
			if err != nil {
				if !errors.Is(err, io.EOF) {
					out.err = err
				}
				return
			}
			for _, c := range r.Header() {
				col(c)
			}
			row := make(sheet.LiveRow, len(cols))
			for _, f := range fields {
				i := col(f.Key)
				if i >= len(row) {
					row = append(row, make(sheet.LiveRow, i+1-len(row))...)
				}
				row[i] = nuonCell(f.Value, zone)
			}
			out.rows = append(out.rows, row)
		}
	}
}

// Rows reads a sheet an import made as a linked region's rows: its first
// row as the header and the rest under it, from column A, values with
// the formats they show in. A file that is read again whole when it
// changes (XLSX, SQLite, Parquet) is read this way.
func Rows(s *sheet.Sheet) TailRows {
	used, ok := s.UsedRange()
	if !ok {
		return TailRows{}
	}
	var res TailRows
	for r := 0; r <= used.To.Row; r++ {
		row := make(sheet.LiveRow, used.To.Col+1)
		for c := range row {
			a := sheet.Addr{Col: c, Row: r}
			if s.Filled(a) {
				row[c] = sheet.LiveCell{V: s.Value(a), F: s.DisplayFormat(a)}
			}
		}
		if r == 0 {
			res.Header = row
			continue
		}
		res.Rows = append(res.Rows, row)
	}
	return res
}

// textCell is a cell showing s as text, as the builder stores it.
func textCell(s string) sheet.LiveCell {
	v, _ := sheet.EntryValue("'" + flatten(s))
	return sheet.LiveCell{V: v}
}
