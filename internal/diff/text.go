package diff

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"

	"github.com/FelineStateMachine/012/internal/nuon"
	"github.com/FelineStateMachine/012/internal/sheet"
)

// SGR codes of the text output's colors, from the 16 ANSI colors so the
// terminal's palette applies; the words carry the meaning without them.
const (
	sgrWhere = "\x1b[1m"  // bold: the place
	sgrField = "\x1b[36m" // cyan: what about it
	sgrOld   = "\x1b[31m" // red: what it was
	sgrNew   = "\x1b[32m" // green: what it is
	sgrReset = "\x1b[0m"
)

// Formats are what WriteChanges writes.
var Formats = []string{"text", "json", "nuon"}

// WriteChanges writes changes in format: text, a line each (colored with
// color), or a table of records with the columns kind, sheet, item,
// field, old and new as JSON or NUON.
func WriteChanges(w io.Writer, changes []Change, format string, color bool) error {
	switch format {
	case "", "text":
		return writeText(w, changes, color)
	case "json":
		return writeJSON(w, changes)
	case "nuon":
		return writeNUON(w, changes)
	}
	return fmt.Errorf("--format %s: 012 diff writes %s", format, strings.Join(Formats, ", "))
}

// writeText writes a line a change: its place, what about it changed,
// and "old → new", "+ new" for something added or "- old" for
// something removed.
func writeText(w io.Writer, changes []Change, color bool) error {
	bw := bufio.NewWriter(w)
	paint := func(sgr, s string) string {
		if !color || s == "" {
			return s
		}
		return sgr + s + sgrReset
	}
	for _, c := range changes {
		line := paint(sgrWhere, c.Where())
		if c.Field != "" {
			line += "  " + paint(sgrField, c.Field)
		}
		switch {
		case c.Kind == KindSheet:
			line += sheetText(c)
		case c.Old.Kind == nuon.Null:
			line += "  " + paint(sgrNew, "+ "+quoted(c.newText))
		case c.New.Kind == nuon.Null:
			line += "  " + paint(sgrOld, "- "+quoted(c.oldText))
		default:
			line += "  " + paint(sgrOld, quoted(c.oldText)) + " → " + paint(sgrNew, quoted(c.newText))
		}
		bw.WriteString(line + "\n")
	}
	return bw.Flush()
}

// sheetText is the end of a sheet's line: its name or place in the
// first workbook, for a sheet renamed or put elsewhere.
func sheetText(c Change) string {
	switch c.Field {
	case "renamed":
		return " from " + c.oldText
	case "moved":
		return " from " + c.oldText + " to " + c.newText
	}
	return ""
}

var recordCols = []string{"kind", "sheet", "item", "field", "old", "new"}

func (c Change) values() []nuon.Value {
	str := func(s string) nuon.Value { return nuon.StringValue(s) }
	return []nuon.Value{str(c.Kind), str(c.Sheet), str(c.Item), str(c.Field), c.Old, c.New}
}

func writeNUON(w io.Writer, changes []Change) error {
	tw := nuon.NewTableWriter(w, recordCols)
	for _, c := range changes {
		if err := tw.Write(c.values()); err != nil {
			return err
		}
	}
	return tw.Close()
}

func writeJSON(w io.Writer, changes []Change) error {
	bw := bufio.NewWriter(w)
	bw.WriteString("[")
	fields := make([]nuon.Field, len(recordCols))
	for i, c := range changes {
		for j, v := range c.values() {
			fields[j] = nuon.Field{Key: recordCols[j], Value: v}
		}
		if i > 0 {
			bw.WriteString(",")
		}
		bw.WriteString("\n")
		bw.Write(nuon.AppendJSONRecord(nil, fields))
	}
	if len(changes) > 0 {
		bw.WriteString("\n")
	}
	bw.WriteString("]\n")
	return bw.Flush()
}

// Dump writes a workbook as lines of text, one for each cell and each
// field, each naming its sheet: what git diffs when 012 diff --textconv
// is a .012 file's textconv. A formula's line ends with its value,
// unless it changes on every recalculation (NOW, RAND and what reads
// them), which would show as a change each time.
func Dump(w io.Writer, b *Book) error {
	bw := bufio.NewWriter(w)
	for _, s := range b.raw.sheets {
		sd := b.side(s)
		q := sheet.QuoteSheet(s.name)
		fmt.Fprintf(bw, "sheet %s\n", s.name)
		for _, k := range slices.Sorted(maps.Keys(s.fields)) {
			if k == "regions" {
				for _, name := range slices.Sorted(maps.Keys(named(s.fields[k]))) {
					fmt.Fprintf(bw, "%s region %s  %s\n", s.name, name, named(s.fields[k])[name])
				}
				continue
			}
			fmt.Fprintf(bw, "%s %s  %s\n", s.name, k, s.fields[k])
		}
		for _, at := range sortedAddrs(s.cells) {
			bw.WriteString(q + "!" + cellLine(sd, at, s.cells[at.String()]) + "\n")
		}
	}
	names, _ := object(b.raw.top["names"])
	for _, k := range slices.Sorted(maps.Keys(names)) {
		fmt.Fprintf(bw, "name %s  %s\n", k, rawText(names[k]))
	}
	macros := named(b.raw.top["macros"])
	for _, k := range slices.Sorted(maps.Keys(macros)) {
		fmt.Fprintf(bw, "macro %s  %s\n", k, macros[k])
	}
	for _, k := range slices.Sorted(maps.Keys(b.raw.top)) {
		if !quietKeys[k] && k != "names" && k != "macros" {
			fmt.Fprintf(bw, "workbook %s  %s\n", k, b.raw.top[k])
		}
	}
	return bw.Flush()
}

// cellLine is a cell as Dump writes it after its sheet: B7  =SUM(A1:A6)
// = 21  [currency, decimals 2]  note: …
func cellLine(sd side, at sheet.Addr, raw json.RawMessage) string {
	p := splitCell(raw)
	line := at.String() + "  " + quoted(p.input)
	if sheet.IsFormulaEntry(p.input) && sd.s != nil && !sd.vol[at] {
		_, shown := cellValue(sd.s, at)
		line += "  = " + quoted(shown)
	}
	if p.format != nil {
		line += "  [" + formatText(p.format) + "]"
	}
	if p.note != "" {
		line += "  note: " + quoted(p.note)
	}
	return line
}

// sortedAddrs are the cells' addresses in row order.
func sortedAddrs(cells map[string]json.RawMessage) []sheet.Addr {
	var out []sheet.Addr
	for k := range cells {
		if at, ok := sheet.ParseAddr(k); ok {
			out = append(out, at)
		}
	}
	slices.SortFunc(out, byRow)
	return out
}
