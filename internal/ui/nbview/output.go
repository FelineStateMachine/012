package nbview

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/fileio"
	"github.com/FelineStateMachine/012/internal/locale"
	"github.com/FelineStateMachine/012/internal/notebook"
	"github.com/FelineStateMachine/012/internal/nuon"
	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/ui/theme"
)

// Outputs as they show under a code cell. What a cell printed, NUON, is
// parsed once into a shown output: a table (a list of records) or a
// record, which the UI draws as its own grid (grid.go), a list, text,
// or a single value; values are the cells importing them would make,
// formatted as cells are. A long output scrolls in a window of its own
// (outwin.go) until O shows it whole; its lines are drawn only when
// they're on screen, so an output of ten thousand rows costs the rows
// showing.

// window is how many rows (or lines) an output's window shows.
const window = 10

// outKind is what an output holds.
type outKind int

const (
	outNone    outKind = iota // nothing printed
	outTable                  // a list of records: a grid
	outRecord                 // a record: a grid of its fields
	outList                   // a list of values that aren't records
	outText                   // a string, its lines
	outValue                  // a single value
	outError                  // the run failed
	outUnsaved                // too large for the file to keep
	outBad                    // what nu printed isn't NUON
)

// shown is an output parsed for showing.
type shown struct {
	kind  outKind
	cols  int                // a table's columns
	rows  [][]sheet.LiveCell // a list's items, or the one value
	text  []string           // text's lines, or an error's
	total int                // rows or lines in all
	wrap  []string           // text wrapped at wrapW
	wrapW int
	// data is a table's or record's grid as NUON (gridData), which the
	// UI makes the grid from.
	data []byte
	grid Grid
}

// parse reads an output for showing.
func parse(o *notebook.Output) *shown {
	switch {
	case o == nil:
		return &shown{}
	case o.Err != "":
		text := []string{o.Err}
		if o.Detail != "" {
			text = append(text, o.Detail)
		}
		return &shown{kind: outError, text: text, total: len(text)}
	case o.Unsaved:
		return &shown{kind: outUnsaved, total: 1}
	case len(o.NUON) == 0:
		return &shown{}
	}
	v, err := nuon.Parse(o.NUON)
	if err != nil {
		return &shown{kind: outBad, text: []string{"nu's output isn't NUON: " + err.Error()}, total: 1}
	}
	sh := shownOf(v)
	if sh.kind == outTable || sh.kind == outRecord {
		sh.data = gridData(v, o.NUON)
	}
	return sh
}

// shownOf is how value v shows.
func shownOf(v nuon.Value) *shown {
	switch v.Kind {
	case nuon.Null:
		return &shown{}
	case nuon.String:
		lines := strings.Split(strings.TrimRight(v.Str, "\n"), "\n")
		return &shown{kind: outText, text: lines, total: len(lines)}
	case nuon.Record:
		if len(v.Fields) == 0 {
			return &shown{}
		}
		return &shown{kind: outRecord, total: len(v.Fields), cols: 2}
	case nuon.List:
		if len(v.List) == 0 {
			return &shown{}
		}
		if isTable(v.List) {
			return &shown{kind: outTable, total: len(v.List), cols: countCols(v.List)}
		}
		sh := &shown{kind: outList, total: len(v.List)}
		for _, it := range v.List {
			sh.rows = append(sh.rows, []sheet.LiveCell{fileio.NUONCell(it)})
		}
		return sh
	}
	return &shown{kind: outValue, rows: [][]sheet.LiveCell{{fileio.NUONCell(v)}}, total: 1}
}

// isTable reports whether a list is records, as nushell draws a table.
func isTable(list []nuon.Value) bool {
	for _, it := range list {
		if it.Kind != nuon.Record {
			return false
		}
	}
	return true
}

// countCols is how many columns a list of records names.
func countCols(list []nuon.Value) int {
	seen := map[string]bool{}
	for _, rec := range list {
		for _, f := range rec.Fields {
			seen[f.Key] = true
		}
	}
	return len(seen)
}

// isGrid reports whether the output is drawn as the UI's grid.
func (sh *shown) isGrid() bool { return sh.grid != nil }

// cellText is a value as a cell shows it, whole.
func cellText(lc sheet.LiveCell, loc *locale.Locale) string {
	if lc.V.Kind == sheet.Empty {
		return ""
	}
	return strings.ReplaceAll(sheet.FormatTextIn(lc.V, lc.F, loc), "\n", " ")
}

// more counts what's left out: "9,991 more rows".
func more(n int, what string) string {
	if n != 1 {
		what += "s"
	}
	return grouped(n) + " " + what
}

// grouped is n with its thousands apart: 9,991.
func grouped(n int) string {
	s := strconv.Itoa(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

// fieldLine is a list's item, its index first as nushell shows one.
func (sh *shown) fieldLine(th *theme.Theme, loc *locale.Locale, i, width int) string {
	if i >= len(sh.rows) {
		return ""
	}
	n := len(strconv.Itoa(sh.total - 1))
	num := strconv.Itoa(i)
	key := th.Muted.Render(strings.Repeat(" ", max(n-len(num), 0))+num) + "  "
	room := max(width-ansi.StringWidth(key), 1)
	return key + ansi.Truncate(cellText(sh.rows[i][0], loc), room, "…")
}

// wrapped is the text's lines wrapped at width, kept for the width last
// asked.
func (sh *shown) wrapped(width int) []string {
	if sh.wrapW != width || sh.wrap == nil {
		sh.wrap = sh.wrap[:0]
		for i, l := range sh.text {
			if sh.kind == outError && i == 0 {
				l = "× " + l
			}
			sh.wrap = append(sh.wrap, wrapText(l, width)...)
		}
		sh.wrapW = width
	}
	return sh.wrap
}

// wrapText breaks a line of text into lines at most width wide, at
// spaces where it can.
func wrapText(s string, width int) []string {
	if width < 1 || ansi.StringWidth(s) <= width {
		return []string{s}
	}
	return strings.Split(ansi.Wrap(s, width, " "), "\n")
}
