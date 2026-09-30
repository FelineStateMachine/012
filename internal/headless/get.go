package headless

import (
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/FelineStateMachine/012/internal/fileio"
	"github.com/FelineStateMachine/012/internal/nuon"
	"github.com/FelineStateMachine/012/internal/sheet"
)

// Formats are what Get writes: text as the sheet shows it, or a table
// as 012 --pipe and exports write it.
var Formats = []string{"text", "csv", "tsv", "json", "nuon"}

// GetOptions say how Get writes.
type GetOptions struct {
	Format string // one of Formats; "" is text
	// Input writes what was typed (a formula, or an entry as stored)
	// rather than the value.
	Input bool
	// NoHeader names a JSON or NUON table's columns by their letters and
	// makes every row a record, rather than taking the names from the
	// first row.
	NoHeader bool
}

// Get writes the target's values. One cell is written alone: as it
// shows, or as a JSON or NUON value typed by its format (a number, a
// date, a file size). A range or sheet is written as a table: aligned
// columns as text, or as CSV, TSV, JSON or NUON, whose columns take
// their names from the first row (see GetOptions.NoHeader).
func Get(out io.Writer, t Target, o GetOptions) error {
	if o.Format == "" {
		o.Format = "text"
	}
	if t.Cell() {
		return getCell(out, t.Sheet, t.Range.From, o)
	}
	r := t.Range
	if t.Whole {
		r = sheet.Rect{}
	}
	snap := fileio.Snap(t.Sheet, r, t.Sheet.Name())
	snap.HiddenRows = nil // a script reads the whole range, whatever a filter shows
	if o.Input {
		inputs(snap)
	}
	if o.Format == "text" {
		return writeText(out, t.Sheet, snap, o.Input)
	}
	k, ok := fileio.KindNamed(o.Format)
	if !ok || !k.IsText() {
		return fmt.Errorf("--format %s: 012 get writes %s", o.Format, strings.Join(Formats, ", "))
	}
	if o.NoHeader && (k == fileio.JSON || k == fileio.NUON) {
		snap = lettered(snap)
	}
	if k == fileio.JSON {
		return writeJSONTable(out, snap)
	}
	_, err := fileio.Encode(out, k, snap)
	return err
}

// writeJSONTable writes snap as a JSON list of records named by its
// first row, one to a line, each value a Value.
func writeJSONTable(out io.Writer, snap *fileio.Snapshot) error {
	r := snap.Range
	cols := fileio.SnapColumns(snap)
	b := []byte("[")
	for row := r.From.Row + 1; row <= r.To.Row; row++ {
		if row > r.From.Row+1 {
			b = append(b, ',')
		}
		b = append(b, "\n{"...)
		for i, name := range cols {
			if i > 0 {
				b = append(b, ',')
			}
			b = append(nuon.AppendJSON(b, nuon.StringValue(name)), ':')
			c, ok := snap.Cells[sheet.Addr{Col: r.From.Col + i, Row: row}]
			if !ok {
				b = append(b, "null"...)
				continue
			}
			b = append(b, valueOf(c.Value, c.Format)...)
		}
		b = append(b, '}')
	}
	if r.To.Row > r.From.Row {
		b = append(b, '\n')
	}
	_, err := out.Write(append(b, "]\n"...))
	return err
}

// getCell writes one cell's value or input.
func getCell(out io.Writer, s *sheet.Sheet, a sheet.Addr, o GetOptions) error {
	c := fileio.SnapCell{Value: s.Value(a), Format: s.DisplayFormat(a)}
	text := s.LocalText(a)
	if o.Input {
		input := ""
		if cell := s.Cell(a); cell != nil {
			input = cell.Input
		}
		c, text = fileio.SnapCell{Value: sheet.Value{Kind: sheet.Text, Str: input}}, input
		if input == "" {
			c.Value = sheet.Value{}
		}
	}
	var line []byte
	switch o.Format {
	case "text":
		line = []byte(text)
	case "csv", "tsv":
		return getTable(out, s, a, o)
	case "json":
		line = valueOf(c.Value, c.Format)
	case "nuon":
		line = nuon.Append(nil, fileio.CellValue(c))
	default:
		return fmt.Errorf("--format %s: 012 get writes %s", o.Format, strings.Join(Formats, ", "))
	}
	_, err := out.Write(append(line, '\n'))
	return err
}

// getTable writes one cell as a table of one row: a CSV or TSV line.
func getTable(out io.Writer, s *sheet.Sheet, a sheet.Addr, o GetOptions) error {
	snap := fileio.Snap(s, sheet.Rect{From: a, To: a}, s.Name())
	snap.Range.To = a
	if o.Input {
		inputs(snap)
	}
	k, _ := fileio.KindNamed(o.Format)
	_, err := fileio.Encode(out, k, snap)
	return err
}

// inputs makes each cell of snap hold its input as text in place of its
// value; a cell an array spilled into has none.
func inputs(snap *fileio.Snapshot) {
	for a, c := range snap.Cells {
		v := sheet.Value{Kind: sheet.Text, Str: c.Input}
		if c.Input == "" {
			v = sheet.Value{}
		}
		snap.Cells[a] = fileio.SnapCell{Input: c.Input, Value: v}
	}
}

// lettered is snap with a first row naming each column by its letter,
// so the range's own first row becomes a record.
func lettered(snap *fileio.Snapshot) *fileio.Snapshot {
	out := *snap
	r := snap.Range
	out.Range.To.Row++
	out.Cells = make(map[sheet.Addr]fileio.SnapCell, len(snap.Cells)+r.To.Col-r.From.Col+1)
	for a, c := range snap.Cells {
		out.Cells[sheet.Addr{Col: a.Col, Row: a.Row + 1}] = c
	}
	for col := r.From.Col; col <= r.To.Col; col++ {
		name := sheet.ColName(col)
		out.Cells[sheet.Addr{Col: col, Row: r.From.Row}] = fileio.SnapCell{Input: name, Value: sheet.Value{Kind: sheet.Text, Str: name}}
	}
	return &out
}

// writeText writes snap's range as aligned columns of what each cell
// shows (or its input), numbers to the right, two spaces between
// columns and no spaces at the ends of lines.
func writeText(out io.Writer, s *sheet.Sheet, snap *fileio.Snapshot, input bool) error {
	r := snap.Range
	cols := r.To.Col - r.From.Col + 1
	text := func(a sheet.Addr) (string, bool) {
		c, ok := snap.Cells[a]
		if !ok {
			return "", false
		}
		if input {
			return c.Input, false
		}
		return s.LocalText(a), c.Value.Kind == sheet.Number
	}
	widths := make([]int, cols)
	for a := range snap.Cells {
		t, _ := text(a)
		widths[a.Col-r.From.Col] = max(widths[a.Col-r.From.Col], utf8.RuneCountInString(t))
	}
	var b strings.Builder
	for row := r.From.Row; row <= r.To.Row; row++ {
		var line strings.Builder
		for col := r.From.Col; col <= r.To.Col; col++ {
			t, right := text(sheet.Addr{Col: col, Row: row})
			if col > r.From.Col {
				line.WriteString("  ")
			}
			pad := strings.Repeat(" ", widths[col-r.From.Col]-utf8.RuneCountInString(t))
			if right {
				t = pad + t
			} else {
				t += pad
			}
			line.WriteString(t)
		}
		b.WriteString(strings.TrimRight(line.String(), " "))
		b.WriteByte('\n')
	}
	_, err := io.WriteString(out, b.String())
	return err
}
