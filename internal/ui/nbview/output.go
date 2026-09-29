package nbview

import (
	"slices"
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
// parsed once into a shown output: a table (a list of records), a
// record, a list, text, or a single value; values are the cells
// importing them would make, formatted as cells are. A long output
// scrolls in a window of its own (outwin.go) until O shows it whole; its
// lines are drawn only when they're on screen, so an output of ten
// thousand rows costs the rows showing.

// window is how many rows (or lines) an output's window shows.
const window = 10

// maxColWidth caps how wide fitting makes a column, as a sheet's.
const maxColWidth = 30

// outKind is what an output holds.
type outKind int

const (
	outNone    outKind = iota // nothing printed
	outTable                  // a list of records
	outRecord                 // a record: its fields, one a line
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
	cols  []string           // a table's column names, a record's keys
	rows  [][]sheet.LiveCell // a table's rows, a list's items, a record's values, a value
	text  []string           // text's lines, or an error's
	fit   []int              // the columns' fitted widths
	total int                // rows or lines in all
	wrap  []string           // text wrapped at wrapW
	wrapW int
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
	sh.fitColumns()
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
		sh := &shown{kind: outRecord, total: len(v.Fields)}
		for _, f := range v.Fields {
			sh.cols = append(sh.cols, f.Key)
			sh.rows = append(sh.rows, []sheet.LiveCell{fileio.NUONCell(f.Value)})
		}
		return sh
	case nuon.List:
		if len(v.List) == 0 {
			return &shown{}
		}
		if isTable(v.List) {
			return tableOf(v.List)
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

// tableOf is a list of records as a table: the columns in the order the
// records name them.
func tableOf(list []nuon.Value) *shown {
	sh := &shown{kind: outTable, total: len(list)}
	index := map[string]int{}
	for _, rec := range list {
		for _, f := range rec.Fields {
			if _, ok := index[f.Key]; !ok {
				index[f.Key] = len(sh.cols)
				sh.cols = append(sh.cols, f.Key)
			}
		}
	}
	sh.rows = make([][]sheet.LiveCell, len(list))
	for i, rec := range list {
		row := make([]sheet.LiveCell, len(sh.cols))
		for _, f := range rec.Fields {
			row[index[f.Key]] = fileio.NUONCell(f.Value)
		}
		sh.rows[i] = row
	}
	return sh
}

// fitSample is how many rows at each end fitting the columns reads of a
// table too long to read whole.
const fitSample = 1000

// fitColumns sizes a table's columns to their widest text, the header
// included, at most maxColWidth: all its rows, or the first and last
// of a long table.
func (sh *shown) fitColumns() {
	if sh.kind != outTable {
		return
	}
	sh.fit = make([]int, len(sh.cols))
	for c, name := range sh.cols {
		sh.fit[c] = ansi.StringWidth(name)
	}
	rows := sh.rows
	if len(rows) > 20*fitSample {
		rows = append(slices.Clip(rows[:fitSample]), rows[len(rows)-fitSample:]...)
	}
	for _, row := range rows {
		for c, lc := range row {
			sh.fit[c] = max(sh.fit[c], ansi.StringWidth(cellText(lc, locale.Canonical)))
		}
	}
	for c := range sh.fit {
		sh.fit[c] = min(max(sh.fit[c], 1), maxColWidth)
	}
}

// cellText is a value as a cell shows it, whole.
func cellText(lc sheet.LiveCell, loc *locale.Locale) string {
	if lc.V.Kind == sheet.Empty {
		return ""
	}
	return strings.ReplaceAll(sheet.FormatTextIn(lc.V, lc.F, loc), "\n", " ")
}

// shownCols is how many of a table's columns fit in width.
func (sh *shown) shownCols(width int) int {
	x := 0
	for c, w := range sh.fit {
		if c > 0 {
			x += 2
		}
		if x+w > width && c > 0 {
			return c
		}
		x += w
	}
	return len(sh.fit)
}

func (sh *shown) hiddenCols(width int) int { return len(sh.fit) - sh.shownCols(width) }

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

// align is how a value lines up in its column, as in a cell.
func align(lc sheet.LiveCell) sheet.Align {
	switch lc.V.Kind {
	case sheet.Number:
		if lc.F.Kind == sheet.FmtText {
			return sheet.AlignLeft
		}
		return sheet.AlignRight
	case sheet.Bool, sheet.Error:
		return sheet.AlignCenter
	}
	return sheet.AlignLeft
}

func pad(s string, w int, a sheet.Align) string {
	gap := max(w-ansi.StringWidth(s), 0)
	switch a {
	case sheet.AlignRight:
		return strings.Repeat(" ", gap) + s
	case sheet.AlignCenter:
		return strings.Repeat(" ", gap/2) + s + strings.Repeat(" ", gap-gap/2)
	}
	return s + strings.Repeat(" ", gap)
}

// fieldLine is a record's field, key: value, or a list's item, its
// index first as nushell shows one.
func (sh *shown) fieldLine(th *theme.Theme, loc *locale.Locale, i, width int) string {
	if i >= len(sh.rows) {
		return ""
	}
	var key string
	if sh.kind == outRecord {
		kw := 0
		for _, k := range sh.cols {
			kw = max(kw, ansi.StringWidth(k))
		}
		kw = min(kw, 24)
		k := ansi.Truncate(sh.cols[i], kw, "…")
		key = th.OutputHead.Render(k) + strings.Repeat(" ", kw-ansi.StringWidth(k)+2)
	} else {
		n := len(strconv.Itoa(sh.total - 1))
		key = th.Muted.Render(pad(strconv.Itoa(i), n, sheet.AlignRight)) + "  "
	}
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
