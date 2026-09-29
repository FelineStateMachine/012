package diff

import (
	"encoding/json"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/FelineStateMachine/012/internal/fileio"
	"github.com/FelineStateMachine/012/internal/formula"
	"github.com/FelineStateMachine/012/internal/functions"
	"github.com/FelineStateMachine/012/internal/nuon"
	"github.com/FelineStateMachine/012/internal/sheet"
)

// cellParts are the parts of a cell that change separately: what was
// typed, its note, and its formatting (every other field of the cell
// in the file), which merging keeps apart too.
type cellParts struct {
	input, note string
	format      json.RawMessage // a compact object, or nil for none
}

// splitCell reads a cell as the file stores it: its input alone, or an
// object with the input, the note and the formatting.
func splitCell(raw json.RawMessage) cellParts {
	var p cellParts
	if raw == nil || json.Unmarshal(raw, &p.input) == nil {
		return p
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return p
	}
	json.Unmarshal(fields["input"], &p.input)
	json.Unmarshal(fields["note"], &p.note)
	delete(fields, "input")
	delete(fields, "note")
	if len(fields) > 0 {
		p.format, _ = json.Marshal(fields) // keys sorted
	}
	return p
}

// joinCell is the inverse of splitCell; nil for a cell with nothing.
func joinCell(p cellParts) json.RawMessage {
	if p.format == nil && p.note == "" {
		if p.input == "" {
			return nil
		}
		raw, _ := json.Marshal(p.input)
		return raw
	}
	fields := map[string]json.RawMessage{}
	json.Unmarshal(p.format, &fields)
	if p.input != "" {
		fields["input"], _ = json.Marshal(p.input)
	}
	if p.note != "" {
		fields["note"], _ = json.Marshal(p.note)
	}
	raw, _ := json.Marshal(fields)
	return raw
}

// formatText describes a cell's formatting in words from its fields:
// currency, decimals 2, bold.
func formatText(raw json.RawMessage) string {
	if raw == nil {
		return ""
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return string(raw)
	}
	var parts []string
	for _, k := range slices.Sorted(maps.Keys(fields)) {
		v := string(fields[k])
		var s string
		switch {
		case v == "true":
			parts = append(parts, k)
		case k == "format" && json.Unmarshal(fields[k], &s) == nil:
			parts = append([]string{s}, parts...)
		case json.Unmarshal(fields[k], &s) == nil:
			parts = append(parts, k+" "+s)
		default:
			parts = append(parts, k+" "+v)
		}
	}
	return strings.Join(parts, ", ")
}

// sheetDiff compares a sheet's cells, regions and other fields; name is
// what the changes call the sheet.
func sheetDiff(a, b side, name string) []Change {
	out := cellChanges(a, b, name)
	out = append(out, notebookChanges(name, field(a.raw, "notebookCells"), field(b.raw, "notebookCells"))...)
	out = append(out, listChanges(KindRegion, name, field(a.raw, "regions"), field(b.raw, "regions"))...)
	keys := map[string]bool{}
	for _, s := range []*rawSheet{a.raw, b.raw} {
		for k := range s.fields {
			keys[k] = true
		}
	}
	for _, k := range slices.Sorted(maps.Keys(keys)) {
		if k != "regions" && k != "notebookCells" {
			out = append(out, fieldChanges(KindLayout, name, k, a.raw.fields[k], b.raw.fields[k])...)
		}
	}
	return out
}

func field(s *rawSheet, k string) json.RawMessage { return s.fields[k] }

// cellChanges compares each cell either side has contents, formatting
// or a value in, row by row.
func cellChanges(a, b side, name string) []Change {
	var out []Change
	for _, at := range cellAddrs(a, b) {
		out = append(out, cellChange(a, b, name, at)...)
	}
	return out
}

// cellAddrs are the cells either side has something in, in row order.
func cellAddrs(a, b side) []sheet.Addr {
	addrs := map[sheet.Addr]bool{}
	for _, sd := range []side{a, b} {
		for k := range sd.raw.cells {
			if at, ok := sheet.ParseAddr(k); ok {
				addrs[at] = true
			}
		}
		if sd.s != nil {
			for _, at := range sd.s.Addrs() {
				addrs[at] = true
			}
		}
	}
	return slices.SortedFunc(maps.Keys(addrs), byRow)
}

func byRow(x, y sheet.Addr) int {
	if x.Row != y.Row {
		return x.Row - y.Row
	}
	return x.Col - y.Col
}

// cellChange compares one cell's input, value, format and note.
func cellChange(a, b side, name string, at sheet.Addr) []Change {
	key := at.String()
	pa, pb := splitCell(a.raw.cells[key]), splitCell(b.raw.cells[key])
	item := func(field, old, new string) Change { return textChange(KindCell, name, key, field, old, new) }
	var out []Change
	if pa.input != pb.input {
		out = append(out, item("input", pa.input, pb.input))
	}
	if showsValue(pa, pb) && !a.vol[at] && !b.vol[at] {
		if c, ok := valueChange(a.s, b.s, at); ok {
			c.Sheet, c.Item = name, key
			out = append(out, c)
		}
	}
	if !equal(pa.format, pb.format) {
		c := item("format", formatText(pa.format), formatText(pb.format))
		c.Old, c.New = recordValue(pa.format), recordValue(pb.format)
		out = append(out, c)
	}
	if pa.note != pb.note {
		out = append(out, item("note", pa.note, pb.note))
	}
	return out
}

// showsValue reports whether a cell's value says more than its input:
// it's a formula on either side, or it has no input on either (a value
// an array spilled or a region's table).
func showsValue(a, b cellParts) bool {
	return sheet.IsFormulaEntry(a.input) || sheet.IsFormulaEntry(b.input) || a.input == "" && b.input == ""
}

// valueChange compares the values the two sheets compute at a.
func valueChange(a, b *sheet.Sheet, at sheet.Addr) (Change, bool) {
	oldV, oldT := cellValue(a, at)
	newV, newT := cellValue(b, at)
	if oldT == newT && nuon.Equal(oldV, newV) {
		return Change{}, false
	}
	return Change{Kind: KindCell, Field: "value", Old: oldV, New: newV, oldText: oldT, newText: newT}, true
}

// cellValue is a cell's value typed by its format, and as it shows.
func cellValue(s *sheet.Sheet, at sheet.Addr) (nuon.Value, string) {
	if s == nil || s.Value(at).Kind == sheet.Empty {
		return nuon.NullValue(), ""
	}
	return fileio.CellValue(fileio.SnapCell{Value: s.Value(at), Format: s.DisplayFormat(at)}), s.LocalText(at)
}

// recordValue is a JSON object as a NUON record, or null.
func recordValue(raw json.RawMessage) nuon.Value {
	if raw == nil {
		return nuon.NullValue()
	}
	v, err := nuon.Parse(raw)
	if err != nil {
		return nuon.StringValue(string(raw))
	}
	return v
}

// volatiles are the cells of w whose values change on every
// recalculation, which comparing would report as changed each time:
// formulas calling a volatile function (NOW, RAND), what reads them,
// directly or not, on any sheet, and the cells their arrays spill into.
func volatiles(w *sheet.Workbook) map[*sheet.Sheet]map[sheet.Addr]bool {
	out := map[*sheet.Sheet]map[sheet.Addr]bool{}
	if w == nil {
		return out
	}
	var queue []sheet.Target
	mark := func(s *sheet.Sheet, at sheet.Addr) {
		if out[s] == nil {
			out[s] = map[sheet.Addr]bool{}
		}
		if !out[s][at] {
			out[s][at] = true
			queue = append(queue, sheet.Target{Sheet: s, Range: sheet.Rect{From: at, To: at}})
		}
	}
	for _, s := range w.Sheets() {
		for _, at := range s.Addrs() {
			if c := s.Cell(at); c != nil && c.IsFormula() && callsVolatile(c.Input) {
				mark(s, at)
			}
		}
	}
	for len(queue) > 0 {
		t := queue[0]
		queue = queue[1:]
		if area, ok := t.Sheet.SpillArea(t.Range.From); ok {
			for row := area.From.Row; row <= area.To.Row; row++ {
				for col := area.From.Col; col <= area.To.Col; col++ {
					mark(t.Sheet, sheet.Addr{Col: col, Row: row})
				}
			}
		}
		for _, d := range t.Sheet.Dependents(t.Range.From) {
			mark(d.Sheet, d.Range.From)
		}
	}
	return out
}

// callsVolatile reports whether a formula calls a volatile function.
func callsVolatile(input string) bool {
	n, err := sheet.Parse(input)
	if err != nil {
		return false
	}
	found := false
	var walk func(formula.Node)
	walk = func(n formula.Node) {
		if call, ok := n.(formula.Call); ok {
			if f := functions.Of(call); f != nil && f.Volatile {
				found = true
			}
		}
		formula.EachChild(n, walk)
	}
	walk(n)
	return found
}

// quoted writes text as the text output shows a side: as it is, or
// quoted when spaces at its ends or line breaks would hide.
func quoted(s string) string {
	if s != strings.TrimSpace(s) || strings.ContainsAny(s, "\n\t") {
		return strconv.Quote(s)
	}
	return s
}
