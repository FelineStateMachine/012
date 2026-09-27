package ui

import (
	"log/slog"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/telemetry"
)

// Find and replace is a bar on the context line rather than a dialog, in
// the spirit of less and vim: matches highlight in the grid as you type
// and the active cell follows the current one. Options are toggles shown
// as chips and switched with Alt keys, as in VS Code's find widget.

type findBar struct {
	replace bool // Ctrl+H: a replacement field follows the query
	field   int  // 0 is the query, 1 the replacement
	fields  [2]string
	opts    sheet.FindOptions
	scope   *sheet.Rect // the selection when the bar opened, if any
	scoped  bool        // search only within scope

	matches []sheet.Addr
	index   map[sheet.Addr]int
	cur     int    // index of the current match, -1 when none
	err     string // e.g. an invalid regular expression
	from    sheet.Addr
}

const findID = "find"

func init() {
	register(
		&command{id: "edit.find", title: "Find", desc: "Find text in the sheet as you type", run: func(m *Model) tea.Cmd {
			m.openFind(false)
			return nil
		}},
		&command{id: "edit.replace", title: "Find and replace", desc: "Find text and replace it", run: func(m *Model) tea.Cmd {
			m.openFind(true)
			return nil
		}},
	)
	keymap["ctrl+f"] = "edit.find"
	keymap["ctrl+h"] = "edit.replace"
}

// openFind opens the bar, keeping the last search. A multi-cell selection
// becomes the search scope, as Sheets' "specific range".
func (m *Model) openFind(replace bool) {
	f, ok := m.overlay.(*findBar)
	if !ok {
		f = m.lastFind
		if f == nil {
			f = &findBar{cur: -1}
		}
		f.scope, f.scoped = nil, false
		if m.hasRange() {
			r := m.selection()
			f.scope, f.scoped = &r, true
		}
		f.from = m.cur
		m.clearSelection()
		m.openOverlay(f)
		// Load the kept query so focus doesn't overwrite it.
		m.buf = []rune(f.fields[f.field])
	}
	f.replace = f.replace || replace
	if replace && f.fields[0] != "" {
		f.focus(m, 1)
	} else {
		f.focus(m, 0)
	}
	f.search(m)
}

func (f *findBar) indicator() string { return "FIND" }

func (f *findBar) layout(*Model) []box { return nil }

// focus moves editing to field i, keeping the other field's text.
func (f *findBar) focus(m *Model, i int) {
	f.fields[f.field] = string(m.buf)
	f.field = i
	m.buf = []rune(f.fields[i])
	m.bufPos = len(m.buf)
}

// changed re-runs the search as the query is typed.
func (f *findBar) changed(m *Model) {
	f.fields[f.field] = string(m.buf)
	if f.field == 0 {
		f.search(m)
	}
}

// options returns the search options, with the scope applied.
func (f *findBar) options() sheet.FindOptions {
	o := f.opts
	o.Within = nil
	if f.scoped {
		o.Within = f.scope
	}
	return o
}

// search finds all matches and makes the first one at or after where the
// search started the current one, so the view doesn't jump backwards
// while typing.
func (f *findBar) search(m *Model) {
	f.err, f.matches, f.index, f.cur = "", nil, nil, -1
	span := telemetry.Start("find")
	found, err := m.sheet.Find(f.fields[0], f.options())
	span.End(slog.Int("matches", len(found)))
	if err != nil {
		f.err = "Invalid regular expression"
		return
	}
	f.matches = found
	f.index = make(map[sheet.Addr]int, len(found))
	for i, a := range found {
		f.index[a] = i
	}
	if len(found) == 0 {
		return
	}
	f.cur = 0
	for i, a := range found {
		if a.Row > f.from.Row || a.Row == f.from.Row && a.Col >= f.from.Col {
			f.cur = i
			break
		}
	}
	m.cur = found[f.cur]
}

// step moves to the next (+1) or previous (-1) match, wrapping around.
func (f *findBar) step(m *Model, d int) {
	if len(f.matches) == 0 {
		return
	}
	f.cur = (f.cur + d + len(f.matches)) % len(f.matches)
	m.cur = f.matches[f.cur]
	f.from = m.cur
}

func (f *findBar) key(m *Model, k tea.KeyPressMsg) tea.Cmd {
	switch k.String() {
	case "esc":
		m.lastFind = f
		m.closeOverlay()
	case "enter":
		if f.replace && f.field == 1 {
			f.replaceOne(m)
		} else {
			f.step(m, 1)
		}
	case "shift+enter", "up":
		f.step(m, -1)
	case "down":
		f.step(m, 1)
	case "ctrl+enter", "alt+a":
		if f.replace {
			f.replaceAll(m)
		}
	case "tab", "shift+tab":
		if f.replace {
			f.focus(m, 1-f.field)
		}
	case "ctrl+f":
		f.focus(m, 0)
	case "ctrl+h":
		f.replace = true
		f.focus(m, 1)
	case "alt+c":
		f.opts.MatchCase = !f.opts.MatchCase
		f.search(m)
	case "alt+w":
		f.opts.WholeCell = !f.opts.WholeCell
		f.search(m)
	case "alt+r":
		f.opts.Regex = !f.opts.Regex
		f.search(m)
	case "alt+=":
		f.opts.InFormulas = !f.opts.InFormulas
		f.search(m)
	case "alt+s":
		if f.scope != nil {
			f.scoped = !f.scoped
			f.search(m)
		}
	default:
		before := string(m.buf)
		m.lineKey(k)
		if string(m.buf) != before {
			f.changed(m)
		}
	}
	return nil
}

// replaceOne replaces the current match and moves to the next one.
func (f *findBar) replaceOne(m *Model) {
	if f.cur < 0 {
		return
	}
	a := f.matches[f.cur]
	changed, err := m.sheet.Replace(a, f.fields[0], f.fields[1], f.options())
	switch {
	case err != nil:
		m.note = "Can't replace in " + a.String() + ": " + err.Error()
		return
	case !changed:
		m.note = "Formulas in " + a.String() + " change only when searching within formulas"
	default:
		m.changed = true
	}
	f.from = a
	f.search(m)
	if changed && f.cur >= 0 && f.matches[f.cur] == a {
		f.step(m, 1) // the cell still matches; move past it
	}
}

func (f *findBar) replaceAll(m *Model) {
	span := telemetry.Start("replace")
	n, err := m.sheet.ReplaceAll(f.fields[0], f.fields[1], f.options())
	span.End(slog.Int("replaced", n))
	if n > 0 {
		m.changed = true
	}
	switch {
	case err != nil:
		m.note = "Replaced " + cellCount(n) + ", then stopped: " + err.Error()
	case n == 0:
		m.note = "Nothing to replace"
	default:
		m.note = "Replaced " + cellCount(n)
	}
	f.search(m)
}

func cellCount(n int) string {
	if n == 1 {
		return "1 cell"
	}
	return strconv.Itoa(n) + " cells"
}

// Pieces of the bar, left to right: each field, then the option chips.
type findPart struct {
	text   string
	field  int    // -1 for chips
	toggle string // the Alt key a chip toggles
}

func (f *findBar) parts(m *Model) []findPart {
	label := func(name string, i int) string {
		style := m.th.muted
		if f.field == i {
			style = m.th.key
		}
		return style.Render(name) + m.th.muted.Render(searchPrompt)
	}
	parts := []findPart{{text: label("Find", 0) + f.fieldText(m, 0), field: 0}}
	if f.replace {
		parts = append(parts, findPart{text: label("Replace", 1) + f.fieldText(m, 1), field: 1})
	}
	chip := func(on bool, name, key string) findPart {
		style := m.th.muted
		if on {
			style = m.th.menuSelected
		}
		return findPart{text: style.Render(" " + name + " "), field: -1, toggle: key}
	}
	parts = append(parts,
		chip(f.opts.MatchCase, "Aa", "alt+c"),
		chip(f.opts.WholeCell, "Whole", "alt+w"),
		chip(f.opts.Regex, ".*", "alt+r"),
		chip(f.opts.InFormulas, "=", "alt+="))
	if f.scope != nil {
		parts = append(parts, chip(f.scoped, "in "+f.scope.String(), "alt+s"))
	}
	return parts
}

// fieldText is a field's text: the live edit buffer when focused.
func (f *findBar) fieldText(m *Model, i int) string {
	if f.field == i {
		return string(m.buf)
	}
	return f.fields[i]
}

// gapAfter separates a part from the next: wider after a field, so the
// query and the chips read as separate groups.
func gapAfter(p findPart) string {
	if p.field >= 0 {
		return "   "
	}
	return " "
}

// spans returns each part with the x where it starts on the context line.
func (f *findBar) spans(m *Model) ([]findPart, []int) {
	parts := f.parts(m)
	xs := make([]int, len(parts))
	x := 0
	for i, p := range parts {
		xs[i] = x
		x += ansi.StringWidth(p.text) + len(gapAfter(p))
	}
	return parts, xs
}

// line renders the bar for the context line, with the match count on the
// right.
func (f *findBar) line(m *Model) (left, right string) {
	var b strings.Builder
	parts := f.parts(m)
	for i, p := range parts {
		b.WriteString(p.text)
		if i < len(parts)-1 {
			b.WriteString(gapAfter(p))
		}
	}
	switch {
	case f.err != "":
		right = m.th.warning.Render(f.err)
	case f.fields[0] == "":
	case len(f.matches) == 0:
		right = m.th.warning.Render("No matches")
	default:
		right = m.th.muted.Render(strconv.Itoa(f.cur+1) + " of " + strconv.Itoa(len(f.matches)))
	}
	return b.String(), right
}

// cursor puts the terminal cursor in the focused field, whose text ends
// its part.
func (f *findBar) cursor(m *Model) (x, y int) {
	parts, xs := f.spans(m)
	for i, p := range parts {
		if p.field == f.field {
			return xs[i] + ansi.StringWidth(p.text) - ansi.StringWidth(string(m.buf[m.bufPos:])), contextLine
		}
	}
	return 0, contextLine
}

// mouse focuses a field or flips a chip on the bar; a click anywhere
// else closes the bar and lands where it was clicked.
func (f *findBar) mouse(m *Model, e mouseEvent) tea.Cmd {
	if e.kind != mousePress {
		return nil
	}
	if e.y != contextLine {
		m.lastFind = f
		m.closeOverlay()
		return m.handlePress(tea.Mouse{X: e.x, Y: e.y, Button: e.button})
	}
	parts, xs := f.spans(m)
	for i, p := range parts {
		if e.x >= xs[i] && e.x < xs[i]+ansi.StringWidth(p.text) {
			if p.field >= 0 {
				f.focus(m, p.field)
				return nil
			}
			return f.key(m, keyFor(p.toggle))
		}
	}
	return nil
}

// keyFor makes a key press from a keystroke like "alt+c".
func keyFor(s string) tea.KeyPressMsg {
	k := tea.KeyPressMsg{}
	parts := strings.Split(s, "+")
	for _, p := range parts[:len(parts)-1] {
		if p == "alt" {
			k.Mod |= tea.ModAlt
		}
	}
	k.Code = rune(parts[len(parts)-1][0])
	return k
}

// status shows the keys for moving and replacing, and a reminder of the
// option keys when there's room. Narrow screens keep the most useful keys.
func (f *findBar) status(m *Model) (string, string) {
	pairs := []string{"Enter", "next", "Shift+Enter", "previous", "Esc", "close"}
	if f.replace {
		pairs = []string{"Enter", "replace", "Ctrl+Enter", "all", "Tab", "field", "Esc", "close"}
	}
	desc := "Alt+C/W/R/= options"
	if f.scope != nil {
		desc = "Alt+C/W/R/=/S options"
	}
	for {
		keys := m.keyHints(pairs...)
		switch {
		case ansi.StringWidth(desc)+3+ansi.StringWidth(keys) <= m.width:
			return m.th.muted.Render(desc), keys
		case desc != "":
			desc = ""
		case len(pairs) > 2:
			pairs = append(pairs[:len(pairs)-4], pairs[len(pairs)-2:]...) // keep Esc
		default:
			return "", keys
		}
	}
}

// found reports whether a is a match of the open find bar.
func (m *Model) found(a sheet.Addr) bool {
	f, ok := m.overlay.(*findBar)
	if !ok {
		return false
	}
	_, hit := f.index[a]
	return hit
}
