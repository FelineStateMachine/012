package fileio

import (
	"bufio"
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"unicode/utf16"
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// sniffSize is how much of a file the dialect sniffer looks at.
const sniffSize = 64 << 10

// delimiters are the separators the sniffer considers, in order of
// preference when they score the same.
var delimiters = []rune{',', '\t', ';', '|'}

// Dialect is how a delimited file is written.
type Dialect struct {
	Comma    rune
	Encoding string // "UTF-8", "UTF-16" or "Windows-1252"
}

// sniff picks the delimiter that splits the sample's records into the
// same number of fields most consistently, preferring more fields.
// Quoted fields may contain delimiters and line breaks.
func sniff(sample []byte, truncated bool) rune {
	best, bestScore := ',', 0.0
	for _, d := range delimiters {
		counts := fieldCounts(sample, byte(d), truncated)
		if len(counts) == 0 {
			continue
		}
		// The most common count of delimiters per record, and how many
		// records have it.
		freq := map[int]int{}
		mode := 0
		for _, n := range counts {
			freq[n]++
			if freq[n] > freq[mode] || freq[n] == freq[mode] && n > mode {
				mode = n
			}
		}
		if mode == 0 {
			continue
		}
		score := float64(freq[mode]) / float64(len(counts)) * (1 + float64(min(mode, 20))/100)
		if score > bestScore {
			best, bestScore = d, score
		}
	}
	return best
}

// fieldCounts counts delimiters outside quotes in each of the first
// records of sample, skipping blank lines and a record cut off at the
// end of a truncated sample.
func fieldCounts(sample []byte, d byte, truncated bool) []int {
	var counts []int
	n, inQuote, blank := 0, false, true
	for _, c := range sample {
		switch {
		case c == '"':
			inQuote = !inQuote
			blank = false
		case inQuote:
		case c == d:
			n++
			blank = false
		case c == '\n':
			if !blank {
				counts = append(counts, n)
			}
			n, blank = 0, true
			if len(counts) == 50 {
				return counts
			}
		case c != '\r':
			blank = false
		}
	}
	if !blank && !truncated {
		counts = append(counts, n)
	}
	return counts
}

// decode returns a reader of f's text as UTF-8 and the encoding found: a
// UTF-8 or UTF-16 byte order mark decides, and text that isn't valid
// UTF-8 is read as Windows-1252, what Excel writes on Windows.
func decode(r *bufio.Reader) (io.Reader, string, error) {
	head, err := r.Peek(sniffSize)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, bufio.ErrBufferFull) {
		return nil, "", err
	}
	switch {
	case bytes.HasPrefix(head, []byte{0xEF, 0xBB, 0xBF}):
		r.Discard(3)
		return r, "UTF-8", nil
	case bytes.HasPrefix(head, []byte{0xFF, 0xFE}), bytes.HasPrefix(head, []byte{0xFE, 0xFF}):
		big := head[0] == 0xFE
		r.Discard(2)
		all, err := io.ReadAll(r)
		if err != nil {
			return nil, "", err
		}
		u := make([]uint16, len(all)/2)
		for i := range u {
			if big {
				u[i] = uint16(all[2*i])<<8 | uint16(all[2*i+1])
			} else {
				u[i] = uint16(all[2*i+1])<<8 | uint16(all[2*i])
			}
		}
		return bytes.NewReader([]byte(string(utf16.Decode(u)))), "UTF-16", nil
	}
	// A sample cut mid-character is still UTF-8.
	check := head
	for i := 0; i < utf8.UTFMax && len(check) > 0 && !utf8.Valid(check); i++ {
		check = check[:len(check)-1]
	}
	if len(head) > 0 && !utf8.Valid(check) {
		return charmap.Windows1252.NewDecoder().Reader(r), "Windows-1252", nil
	}
	return r, "UTF-8", nil
}

// countingReader counts the bytes read through it.
type countingReader struct {
	r io.Reader
	n int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}

func importCSV(ctx context.Context, name string, opt Options) (*Result, error) {
	return importDelimited(ctx, name, CSV, opt.Progress)
}

func importTSV(ctx context.Context, name string, opt Options) (*Result, error) {
	return importDelimited(ctx, name, TSV, opt.Progress)
}

func exportCSV(_ context.Context, name string, snap *Snapshot, _ ExportOptions) (*ExportResult, error) {
	return exportDelimited(name, CSV, snap)
}

func exportTSV(_ context.Context, name string, snap *Snapshot, _ ExportOptions) (*ExportResult, error) {
	return exportDelimited(name, TSV, snap)
}

func importDelimited(ctx context.Context, name string, k Kind, prog *Progress) (*Result, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	defer context.AfterFunc(ctx, func() { f.Close() })() // unblocks a read from a pipe
	var size int64
	if st, err := f.Stat(); err == nil && st.Mode().IsRegular() {
		size = st.Size()
	}
	counter := &countingReader{r: f}
	s, rows, notes, err := readDelimited(ctx, counter, k, func(rows int) {
		prog.setRows(rows)
		prog.setFrac(counter.n, size)
	})
	if err != nil {
		return nil, err
	}
	return &Result{Sheet: s, Rows: rows, Notes: notes}, nil
}

// readDelimited reads CSV or TSV text, sniffing the delimiter of CSV.
// Each field is entered as if typed. progress is called every few
// hundred rows.
func readDelimited(ctx context.Context, in io.Reader, k Kind, progress func(rows int)) (*sheet.Sheet, int, []string, error) {
	br := bufio.NewReaderSize(in, sniffSize)
	text, enc, err := decode(br)
	if err != nil {
		return nil, 0, nil, err
	}
	tr := bufio.NewReaderSize(text, sniffSize)
	sample, _ := tr.Peek(sniffSize)
	comma := '\t'
	if k == CSV {
		comma = sniff(sample, len(sample) == sniffSize)
	}
	cr := csv.NewReader(tr)
	cr.Comma = comma
	cr.LazyQuotes = true
	cr.FieldsPerRecord = -1
	cr.ReuseRecord = true

	b := newBuilder()
	row := 0
	for ; ; row++ {
		// Report before reading on: a pipe may keep the next read waiting.
		if row%256 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, 0, nil, err
			}
			progress(row)
		}
		rec, err := cr.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			var pe *csv.ParseError
			if errors.As(err, &pe) {
				return nil, 0, nil, fmt.Errorf("line %d: %v", pe.Line, pe.Err)
			}
			return nil, 0, nil, err
		}
		for col, field := range rec {
			if field != "" {
				b.entry(sheet.Addr{Col: col, Row: row}, field)
			}
		}
	}
	progress(row)
	var notes []string
	if k == CSV && comma != ',' {
		notes = append(notes, "separated by "+delimiterName(comma))
	}
	if enc != "UTF-8" {
		notes = append(notes, "read as "+enc)
	}
	s, notes := b.finish(notes)
	return s, row, notes, nil
}

func delimiterName(r rune) string {
	switch r {
	case '\t':
		return "tabs"
	case ';':
		return "semicolons"
	case '|':
		return "bars"
	}
	return "commas"
}

// exportDelimited writes the snapshot's displayed values, as Sheets'
// Download as CSV does: formulas become their results, formats show.
func exportDelimited(name string, k Kind, snap *Snapshot) (*ExportResult, error) {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	if k == TSV {
		w.Comma = '\t'
	}
	rows := snap.rows()
	if err := w.WriteAll(rows); err != nil {
		return nil, err
	}
	formulas := 0
	for _, c := range snap.Cells {
		if c.Formula {
			formulas++
		}
	}
	res := &ExportResult{Rows: len(rows)}
	if formulas > 0 {
		res.Notes = append(res.Notes, count(formulas, "formula", "formulas")+" saved as values")
	}
	return res, writeFile(name, buf.Bytes())
}

// writeFile writes data atomically: to a temporary file first, then
// renamed over the target.
func writeFile(name string, data []byte) error {
	tmp := name + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, name); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}
