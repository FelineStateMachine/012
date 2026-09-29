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
// importing them would make, formatted as cells are. A long output shows
// a window of its first rows and says how many more there are, until o
// expands it; its lines are drawn only when they're on screen, so an
// expanded output of ten thousand rows costs the rows showing.

// window is how many rows (or lines) an output shows collapsed.
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

// height is how many lines the output takes, collapsed or expanded, at
// width.
func (sh *shown) height(expanded bool, width int) int {
	switch sh.kind {
	case outNone:
		return 0
	case outTable:
		n := sh.total
		if !expanded && n > window {
			n = window
		}
		return 1 + n + sh.footerLines(expanded, width)
	case outText, outError:
		n := len(sh.wrapped(width))
		if !expanded && n > window {
			return window + 1
		}
		return n
	case outRecord, outList:
		if !expanded && sh.total > window {
			return window + 1
		}
		return sh.total
	}
	return 1
}

// footerLines is 1 when a table has more to say under its rows: rows or
// columns it doesn't show.
func (sh *shown) footerLines(expanded bool, width int) int {
	if !expanded && sh.total > window || sh.hiddenCols(width) > 0 {
		return 1
	}
	return 0
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

// line draws line i of the output at width.
func (sh *shown) line(th *theme.Theme, loc *locale.Locale, i int, expanded bool, width int) string {
	switch sh.kind {
	case outTable:
		return sh.tableLine(th, loc, i, expanded, width)
	case outText, outError:
		return sh.textLine(th, i, expanded, width)
	case outRecord, outList:
		if !expanded && sh.total > window && i == window {
			return th.Muted.Render("… " + more(sh.total-window, "more line") + "  (o shows all)")
		}
		return sh.fieldLine(th, loc, i, width)
	case outValue:
		return ansi.Truncate(cellText(sh.rows[0][0], loc), width, "…")
	case outUnsaved:
		return th.Muted.Render("not saved; run to see")
	case outBad:
		return th.Warning.Render(ansi.Truncate(sh.text[0], width, "…"))
	}
	return ""
}

// more counts what's left out: "9,991 more rows".
func more(n int, what string) string {
	s := strconv.Itoa(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	if n != 1 {
		what += "s"
	}
	return s + " " + what
}

// tableLine is a table's header (i 0), a row, or its footer.
func (sh *shown) tableLine(th *theme.Theme, loc *locale.Locale, i int, expanded bool, width int) string {
	ncols := sh.shownCols(width)
	rows := sh.total
	if !expanded && rows > window {
		rows = window
	}
	switch {
	case i == 0:
		var b strings.Builder
		for c := range ncols {
			if c > 0 {
				b.WriteString("  ")
			}
			b.WriteString(th.OutputHead.Render(pad(ansi.Truncate(sh.cols[c], sh.fit[c], "…"), sh.fit[c], sheet.AlignLeft)))
		}
		return b.String()
	case i <= rows:
		row := sh.rows[i-1]
		var b strings.Builder
		for c := range ncols {
			if c > 0 {
				b.WriteString("  ")
			}
			var lc sheet.LiveCell
			if c < len(row) {
				lc = row[c]
			}
			b.WriteString(pad(ansi.Truncate(cellText(lc, loc), sh.fit[c], "…"), sh.fit[c], align(lc)))
		}
		return b.String()
	}
	var parts []string
	if n := sh.total - rows; n > 0 {
		parts = append(parts, more(n, "more row"))
	}
	if n := sh.hiddenCols(width); n > 0 {
		parts = append(parts, more(n, "more column"))
	}
	hint := "Enter opens it"
	if !expanded && sh.total > window {
		hint = "o shows all, " + hint
	}
	return th.Muted.Render("… " + strings.Join(parts, ", ") + "  (" + hint + ")")
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
		key = th.OutputHead.Render(pad(ansi.Truncate(sh.cols[i], kw, "…"), kw, sheet.AlignLeft)) + "  "
	} else {
		n := len(strconv.Itoa(sh.total - 1))
		key = th.Muted.Render(pad(strconv.Itoa(i), n, sheet.AlignRight)) + "  "
	}
	room := max(width-ansi.StringWidth(key), 1)
	return key + ansi.Truncate(cellText(sh.rows[i][0], loc), room, "…")
}

// textLine is line i of text or an error, wrapped at width.
func (sh *shown) textLine(th *theme.Theme, i int, expanded bool, width int) string {
	lines := sh.wrapped(width)
	if !expanded && len(lines) > window && i == window {
		return th.Muted.Render("… " + more(len(lines)-window, "more line") + "  (o shows all)")
	}
	if i >= len(lines) {
		return ""
	}
	if sh.kind == outError {
		if i == 0 {
			return th.Warning.Render(lines[i])
		}
		return th.Muted.Render(lines[i])
	}
	return lines[i]
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
