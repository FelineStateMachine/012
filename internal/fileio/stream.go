package fileio

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"slices"

	"github.com/FelineStateMachine/012/internal/telemetry"
)

// Streams: 012 - reads a table from standard input, and 012 --pipe
// writes one to standard output. A stream has no name to tell its
// format by, so ImportReader looks at the text: NUON (or JSON, which is
// NUON too) starts with [ or {, and anything else is read as delimited
// text, CSV or TSV by the delimiter found.

// ImportReader reads a table from r into a new sheet named name (as a
// file's sheet is named after the file), telling its format from the
// text; Result.Kind is NUON, JSON, CSV or TSV. It checks ctx between
// rows and updates opt.Progress.
func ImportReader(ctx context.Context, name string, r io.Reader, opt Options) (*Result, error) {
	span := telemetry.ParentFrom(ctx).Start("import", slog.String("format", "stream"))
	res, err := importReader(telemetry.WithParent(ctx, span.Parent()), r, opt)
	if err != nil {
		span.Fail(err)
		return nil, err
	}
	if book := res.Sheet.Book(); book.RenameSheet(res.Sheet, sheetName(name)) == nil {
		book.ClearHistory()
	}
	res.Sheet.LoadFitWidths() // a stream is CSV, TSV, JSON or NUON: no widths of its own
	if p := opt.Progress; p != nil {
		p.permille.Store(1000)
	}
	span.End(slog.String("kind", res.Kind.String()), slog.Int("rows", res.Rows))
	return res, nil
}

func importReader(ctx context.Context, r io.Reader, opt Options) (*Result, error) {
	defer context.AfterFunc(ctx, func() {
		if c, ok := r.(io.Closer); ok {
			c.Close() // unblocks a read from a pipe
		}
	})()
	br := bufio.NewReaderSize(r, sniffSize)
	progress := func(rows int) { opt.Progress.setRows(rows) }
	switch first, err := firstByte(br); {
	case err != nil:
		return nil, err
	case first == '[' || first == '{':
		res, json, err := readTable(ctx, br, opt.MaxCells, progress)
		if err != nil {
			return nil, err
		}
		res.Kind = NUON
		if json {
			res.Kind = JSON
		}
		return res, nil
	}
	s, rows, notes, comma, err := readDelimitedAs(ctx, br, CSV, opt.MaxCells, opt.Locale, progress)
	if err != nil {
		return nil, err
	}
	res := &Result{Sheet: s, Kind: CSV, Rows: rows, Notes: notes}
	if comma == '\t' {
		res.Kind = TSV
		res.Notes = slices.DeleteFunc(notes, func(n string) bool { return n == "separated by tabs" }) // it's TSV
	}
	return res, nil
}

// firstByte is the first byte of the text past a byte order mark and
// spaces, without taking it; 0 for text of nothing else.
func firstByte(br *bufio.Reader) (byte, error) {
	for n := 1; n <= sniffSize; n++ {
		b, err := br.Peek(n)
		if len(b) < n {
			if err == io.EOF {
				err = nil
			}
			return 0, err
		}
		switch c := b[n-1]; {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
		case n <= 3 && string(b[:n]) == "\xEF\xBB\xBF"[:n]:
		default:
			return c, nil
		}
	}
	return 0, nil
}

// Encode writes a snapshot to w in the text format k (see Kind.IsText):
// what 012 --pipe writes to standard output.
func Encode(w io.Writer, k Kind, snap *Snapshot) (*ExportResult, error) {
	encode := k.format().encode
	if encode == nil {
		return nil, fmt.Errorf("can't write %s to a stream", k)
	}
	rows, err := encode(w, snap)
	if err != nil {
		return nil, err
	}
	return &ExportResult{Rows: rows, Notes: formulaNotes(snap)}, nil
}
