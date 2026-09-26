package ui

import (
	"cmp"
	"runtime/debug"
	"slices"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"012/internal/sheet"
)

func init() {
	register(
		&command{id: "help", title: "Keyboard shortcuts", desc: "Show every key, grouped by what it does", run: func(m *Model) tea.Cmd {
			m.openOverlay(&shortcuts{})
			return nil
		}},
		&command{id: "help.functions", title: "Function list", desc: "Search the functions formulas can use", run: func(m *Model) tea.Cmd {
			p := newPicker(m, "Functions", "Type a function name, e.g. sum or if", 100, functionItems())
			p.action = "insert"
			m.openOverlay(p)
			return nil
		}},
		&command{id: "help.about", title: "About 012", desc: "Show the version", run: func(m *Model) tea.Cmd {
			m.openOverlay(&choiceBar{
				msg: "012 " + version() + ": Lotus 1-2-3 looks, Google Sheets keys.",
				choices: []choice{
					{key: "esc", label: "Close", run: func(*Model) tea.Cmd { return nil }},
				},
			})
			return nil
		}},
	)
	keymap["f1"] = "help"
	keymap["ctrl+/"] = "help"
}

// version is the release 012 was built from, or "devel" for a source
// build (including Go's pseudo-versions such as v0.0.0-2026...+dirty).
func version() string {
	if bi, ok := debug.ReadBuildInfo(); ok && strings.HasPrefix(bi.Main.Version, "v") && !strings.ContainsAny(bi.Main.Version, "-+") {
		return bi.Main.Version
	}
	return "devel"
}

// functionItems lists the spreadsheet functions for the function list;
// picking one starts a formula with it.
func functionItems() []pickItem {
	fns := sheet.Funcs()
	items := make([]pickItem, len(fns))
	for i, f := range fns {
		items[i] = pickItem{
			title: f.Name + "(" + f.Args + ")", name: len(f.Name), detail: f.Desc, desc: f.Desc,
			pick: func(m *Model) tea.Cmd {
				m.closeOverlay()
				m.startEntry(modeEnter, "="+f.Name+"(")
				return nil
			},
		}
	}
	return items
}

// helpRow is a line of the shortcuts view: a group heading, or keys and
// what they do.
type helpRow struct {
	heading string
	keys    []string
	action  string
}

// helpRows lists the shortcuts, generated from the keymap and the command
// registry so help can't drift from behavior. Movement and typing, which
// aren't commands, come first; then commands grouped by the menu they're
// in.
func helpRows() []helpRow {
	listed := map[string]bool{}
	var rows []helpRow
	group := func(title string, static []helpRow, ids ...string) {
		var cmds []helpRow
		for _, id := range ids {
			keys := keysFor(id)
			if listed[id] || len(keys) == 0 || commands[id] == nil {
				continue
			}
			listed[id] = true
			for i, k := range keys {
				keys[i] = keyLabel(k)
			}
			cmds = append(cmds, helpRow{keys: keys, action: commands[id].title})
		}
		if len(static)+len(cmds) > 0 {
			rows = append(rows, helpRow{heading: title})
			rows = append(rows, static...)
			rows = append(rows, cmds...)
		}
	}
	byTitle := func(pred func(id string) bool) []string {
		var ids []string
		for id := range commands {
			if pred(id) {
				ids = append(ids, id)
			}
		}
		slices.SortFunc(ids, func(a, b string) int { return cmp.Compare(commands[a].title, commands[b].title) })
		return ids
	}

	group("Moving around", []helpRow{
		{keys: []string{"Arrows"}, action: "Move"},
		{keys: []string{"Ctrl+arrows"}, action: "Jump to the edge of the data"},
		{keys: []string{"Tab", "Shift+Tab"}, action: "Right, left"},
		{keys: []string{"PgUp", "PgDn"}, action: "Screen up, down"},
		{keys: []string{"Alt+PgUp", "Alt+PgDn"}, action: "Screen left, right"},
		{keys: []string{"Home", "Ctrl+Home"}, action: "Column A, cell A1"},
		{keys: []string{"Ctrl+End"}, action: "Last used cell"},
	}, "goto")
	group("Selecting", []helpRow{
		{keys: []string{"Shift+arrows"}, action: "Extend the selection"},
		{keys: []string{"Ctrl+Shift+arrows"}, action: "Extend to the edge of the data"},
		{keys: []string{"Mouse"}, action: "Select by dragging or Shift+click"},
	}, byTitle(func(id string) bool { return strings.HasPrefix(id, "select.") })...)
	group("Entering data", []helpRow{
		{keys: []string{"Type"}, action: "Replace the cell; = starts a formula"},
		{keys: []string{"Enter", "Tab"}, action: "Accept and move down, right"},
		{keys: []string{"Arrows"}, action: "Pick cells for a formula"},
		{keys: []string{"Esc"}, action: "Cancel the entry"},
	}, "edit")
	group("Menus and search", []helpRow{
		{keys: []string{"Alt+letter"}, action: "Open a menu by its underlined letter"},
	}, "palette", "menu", "menu.context", "help")
	for _, d := range menuBar {
		var ids []string
		var walk func(items []menuItem)
		walk = func(items []menuItem) {
			for _, it := range items {
				if it.items != nil {
					walk(it.items)
				} else if !it.sep {
					ids = append(ids, it.cmd)
				}
			}
		}
		walk(visibleItems(d.items))
		group(d.title, nil, ids...)
	}
	group("Other", nil, byTitle(func(string) bool { return true })...)
	return rows
}

// shortcuts is the keyboard shortcuts view: a scrollable box over the
// grid, in two columns when the screen is wide enough.
type shortcuts struct {
	top int
}

const shortcutsID = "shortcuts"

func (s *shortcuts) indicator() string { return "HELP" }

// lines renders the rows, as one or two columns, and returns their width.
func (s *shortcuts) lines(m *Model) ([]string, int) {
	rows := helpRows()
	keyW, actW := 0, 0
	for _, r := range rows {
		keyW = max(keyW, ansi.StringWidth(m.chips(r.keys)))
		actW = max(actW, ansi.StringWidth(r.action))
	}
	// On narrow screens actions give way (truncated) so the keys fit.
	room := min(m.width-4, 160)
	if 1+keyW+2+actW > room {
		actW = max(room-3-keyW, 16)
	}
	colW := 1 + keyW + 2 + actW
	render := func(rows []helpRow) []string {
		var out []string
		for i, r := range rows {
			switch {
			case r.heading != "" && i > 0:
				out = append(out, "", m.th.title.Render(" "+r.heading))
			case r.heading != "":
				out = append(out, m.th.title.Render(" "+r.heading))
			default:
				out = append(out, " "+padRight(ansi.Truncate(r.action, actW, "…"), actW)+"  "+m.chips(r.keys))
			}
		}
		return out
	}
	if 2*colW+3 > room {
		return render(rows), min(colW, room)
	}
	// Two columns, split at the group boundary nearest the middle.
	split, best := 0, len(rows)
	for i, r := range rows {
		if d := abs(len(rows) - 2*i); r.heading != "" && d < best {
			split, best = i, d
		}
	}
	left, right := render(rows[:split]), render(rows[split:])
	out := make([]string, max(len(left), len(right)))
	for i := range out {
		var l, r string
		if i < len(left) {
			l = left[i]
		}
		if i < len(right) {
			r = right[i]
		}
		out[i] = padRight(l, colW) + "   " + r
	}
	return out, 2*colW + 3
}

func abs(x int) int { return max(x, -x) }

// chips renders keys as key chips, e.g. [Ctrl+/] [F1].
func (m *Model) chips(keys []string) string {
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = m.chip(k)
	}
	return strings.Join(parts, " ")
}

// visible is how many lines fit between the menu bar and the status line.
func (s *shortcuts) visible(m *Model, total int) int {
	return max(min(total, m.height-2-2-1), 1)
}

func (s *shortcuts) layout(m *Model) []box {
	lines, w := s.lines(m)
	inner := w + 1
	n := s.visible(m, len(lines)+1)
	s.top = clamp(s.top, 0, max(len(lines)+1-n, 0))
	lines = append([]string{""}, lines...) // breathing room under the title
	rows := make([]string, n)
	for i := range rows {
		if j := s.top + i; j < len(lines) {
			rows[i] = cells(m.th.menuBar, lines[j], inner)
		} else {
			rows[i] = strings.Repeat(" ", inner)
		}
	}
	footer := ""
	if n < len(lines) {
		footer = strconv.Itoa(s.top+1) + "-" + strconv.Itoa(s.top+n) + " of " + strconv.Itoa(len(lines))
	}
	b := m.frame(inner, "Keyboard shortcuts", footer, rows)
	w, h := ansi.StringWidth(b[0]), len(b)
	return []box{{id: shortcutsID, x: (m.width - w) / 2, y: max(menuLine+1, (m.height-1-h)/2), lines: b}}
}

func (s *shortcuts) key(m *Model, k tea.KeyPressMsg) tea.Cmd {
	lines, _ := s.lines(m)
	page := s.visible(m, len(lines)+1)
	switch k.String() {
	case "up", "k":
		s.top--
	case "down", "j":
		s.top++
	case "pgup":
		s.top -= page
	case "pgdown", "space":
		s.top += page
	case "home":
		s.top = 0
	case "end":
		s.top = len(lines)
	case "esc", "enter", "q", "f1", "ctrl+/":
		m.closeOverlay()
	}
	s.top = clamp(s.top, 0, max(len(lines)+1-page, 0))
	return nil
}

func (s *shortcuts) mouse(m *Model, e mouseEvent) tea.Cmd {
	switch {
	case e.kind == mouseWheel && e.button == tea.MouseWheelUp:
		s.top = max(s.top-3, 0)
	case e.kind == mouseWheel && e.button == tea.MouseWheelDown:
		s.top += 3 // layout clamps
	case e.kind == mousePress && e.box != shortcutsID:
		m.closeOverlay()
	}
	return nil
}

func (s *shortcuts) status(m *Model) (string, string) {
	return "", m.keyHints("Up/Down", "scroll", "Esc", "close")
}
