package ui

import (
	"log/slog"
	"slices"
	"strconv"

	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/telemetry"
)

// Find and replace is a bar on the context line rather than a dialog, in
// the spirit of less and vim: matches highlight in the grid as you type
// and the active cell follows the current one. Options are toggles shown
// as chips and switched with Alt keys, as in VS Code's find widget. The
// scope chip says where to search, as Sheets' "Search" choice: this
// sheet, all sheets, or the range selected when the bar opened; Alt+S
// goes through them.

type findBar struct {
	replace bool // Ctrl+H: a replacement field follows the query
	field   int  // 0 is the query, 1 the replacement
	fields  [2]string
	opts    sheet.FindOptions
	scope   *sheet.Rect  // the selection when the bar opened, if any
	home    *sheet.Sheet // the sheet scope is on
	where   findScope

	matches []cellOn
	index   map[cellOn]int
	cur     int    // index of the current match, -1 when none
	err     string // e.g. an invalid regular expression
	from    cellOn // where the search goes on from
}

// findScope is where the find bar searches.
type findScope int

const (
	inSheet findScope = iota // the sheet shown
	inAll                    // every sheet, in tab order
	inRange                  // the range selected when the bar opened
)

// cellOn is a cell on a particular sheet.
type cellOn struct {
	s *sheet.Sheet
	a sheet.Addr
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
		f = m.find
		if f == nil {
			f = &findBar{cur: -1}
		}
		f.scope, f.home = nil, m.sheet
		if f.where == inRange || f.where == inAll && m.book().Len() == 1 {
			f.where = inSheet
		}
		if m.hasRange() {
			r := m.selection()
			f.scope, f.where = &r, inRange
		}
		f.from = cellOn{m.sheet, m.cur}
		m.clearSelection()
		m.openOverlay(f)
		// Load the kept query so focus doesn't overwrite it.
		m.line.buf = []rune(f.fields[f.field])
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
	f.fields[f.field] = m.line.text()
	f.field = i
	m.line.set(f.fields[i])
}

// changed re-runs the search as the query is typed.
func (f *findBar) changed(m *Model) {
	f.fields[f.field] = m.line.text()
	if f.field == 0 {
		f.search(m)
	}
}

// options returns the search options, with the scope applied.
func (f *findBar) options() sheet.FindOptions {
	o := f.opts
	o.Within = nil
	if f.where == inRange {
		o.Within = f.scope
	}
	return o
}

// sheets are the sheets searched, in order.
func (f *findBar) sheets(m *Model) []*sheet.Sheet {
	switch f.where {
	case inAll:
		return m.book().Sheets()
	case inRange:
		if f.home.Live() {
			return []*sheet.Sheet{f.home}
		}
	}
	return []*sheet.Sheet{m.sheet}
}

// search finds all matches and makes the first one at or after where the
// search started the current one, so the view doesn't jump backwards
// while typing.
func (f *findBar) search(m *Model) {
	f.err, f.matches, f.index, f.cur = "", nil, nil, -1
	span := telemetry.Start("find")
	sheets := f.sheets(m)
	for _, s := range sheets {
		found, err := s.Find(f.fields[0], f.options())
		if err != nil {
			span.End(slog.Int("matches", 0))
			f.err = "Invalid regular expression"
			return
		}
		for _, a := range found {
			f.matches = append(f.matches, cellOn{s, a})
		}
	}
	span.End(slog.Int("matches", len(f.matches)), slog.Int("sheets", len(sheets)))
	f.index = make(map[cellOn]int, len(f.matches))
	for i, c := range f.matches {
		f.index[c] = i
	}
	if len(f.matches) == 0 {
		return
	}
	order := func(s *sheet.Sheet) int { return slices.Index(sheets, s) }
	f.cur = 0
	for i, c := range f.matches {
		if d := order(c.s) - order(f.from.s); d > 0 || d == 0 && (c.a.Row > f.from.a.Row || c.a.Row == f.from.a.Row && c.a.Col >= f.from.a.Col) {
			f.cur = i
			break
		}
	}
	f.show(m)
}

// show makes the current match the active cell, on its sheet.
func (f *findBar) show(m *Model) {
	c := f.matches[f.cur]
	m.showSheet(c.s)
	m.cur = c.a
}

// step moves to the next (+1) or previous (-1) match, wrapping around.
func (f *findBar) step(m *Model, d int) {
	if len(f.matches) == 0 {
		return
	}
	f.cur = (f.cur + d + len(f.matches)) % len(f.matches)
	f.show(m)
	f.from = f.matches[f.cur]
}

// nextScope is where Alt+S goes from where: the range (if there is one),
// the sheet, then all sheets (if there are several).
func (f *findBar) nextScope(m *Model) findScope {
	order := []findScope{inSheet}
	if m.book().Len() > 1 {
		order = append(order, inAll)
	}
	if f.scope != nil {
		order = append([]findScope{inRange}, order...)
	}
	return order[(slices.Index(order, f.where)+1)%len(order)]
}

func (f *findBar) key(m *Model, k tea.KeyPressMsg) tea.Cmd {
	switch k.String() {
	case "esc":
		m.find = f
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
		if next := f.nextScope(m); next != f.where {
			f.where = next
			f.from = cellOn{m.sheet, m.cur}
			f.search(m)
		}
	default:
		before := m.line.text()
		m.line.key(k)
		if m.line.text() != before {
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
	c := f.matches[f.cur]
	a := c.a
	changed, err := c.s.Replace(a, f.fields[0], f.fields[1], f.options())
	switch {
	case err != nil:
		m.note = "Can't replace in " + a.String() + ": " + err.Error()
		return
	case !changed:
		m.note = "Formulas in " + a.String() + " change only when searching within formulas"
	default:
		m.changed = true
	}
	f.from = c
	f.search(m)
	if changed && f.cur >= 0 && f.matches[f.cur] == c {
		f.step(m, 1) // the cell still matches; move past it
	}
}

func (f *findBar) replaceAll(m *Model) {
	// Across sheets, still one undo step.
	span := telemetry.Start("replace")
	n := 0
	err := m.book().Batch(sheet.Change{Label: "replace all", Sheet: m.sheet}, func() error {
		for _, s := range f.sheets(m) {
			k, err := s.ReplaceAll(f.fields[0], f.fields[1], f.options())
			n += k
			if err != nil {
				return err
			}
		}
		return nil
	})
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

// found reports whether a is a match of the open find bar.
func (m *Model) found(a sheet.Addr) bool {
	f, ok := m.overlay.(*findBar)
	if !ok {
		return false
	}
	_, hit := f.index[cellOn{m.sheet, a}]
	return hit
}
