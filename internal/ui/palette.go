package ui

import (
	"cmp"
	"maps"
	"slices"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sahilm/fuzzy"

	"github.com/FelineStateMachine/012/internal/ui/overlay"
	"github.com/FelineStateMachine/012/internal/ui/theme"
)

// The command palette ("Search the menus" in Sheets, Alt+/) finds any
// registered command by fuzzy matching its title and menu path, and shows
// its shortcut. The function list uses the same picker.

func init() {
	register(&command{id: "palette", macro: macroNever, title: "Search the menus", desc: "Find and run any command by name", run: func(m *Model) tea.Cmd {
		m.openOverlay(newPicker(m, "Search the menus", "Type a command, e.g. save or width", 76, paletteItems(m)))
		return nil
	}})
	for _, k := range []string{"alt+/", "ctrl+k", "ctrl+shift+p"} {
		keymap[k] = "palette"
	}
}

// paletteItems lists every command: those in menus first, in menu order
// with their menu path, then the rest by title.
func paletteItems(m *Model) []pickItem {
	var items []pickItem
	seen := map[string]bool{"palette": true}
	add := func(id, path string) {
		if seen[id] {
			return
		}
		seen[id] = true
		c := commands[id]
		items = append(items, pickItem{
			title: c.title, name: len(c.title), detail: path, key: m.shortcut(id), desc: c.desc,
			off: !c.available(m), pick: func(m *Model) tea.Cmd { return m.runFromOverlay(id) },
		})
	}
	var walk func(items []menuItem, path string)
	walk = func(items []menuItem, path string) {
		for _, it := range items {
			switch {
			case it.items != nil:
				walk(it.items, path+" › "+it.label())
			case !it.sep:
				add(it.cmd, path)
			}
		}
	}
	for _, d := range menuBar {
		walk(visibleItems(d.items), d.title)
	}
	items = append(items, macroRunItems(m)...)
	rest := make([]string, 0, len(commands))
	for id := range commands {
		rest = append(rest, id)
	}
	slices.SortFunc(rest, func(a, b string) int { return cmp.Compare(commands[a].title, commands[b].title) })
	for _, id := range rest {
		add(id, "")
	}
	return items
}

// pickItem is one row of a picker.
type pickItem struct {
	title  string // e.g. "Save as"
	name   int    // bytes of title that are searched, e.g. just "SUM" of "SUM(value, ...)"
	detail string // a dimmed second column, also searched, e.g. the menu path
	key    string // shortcut, shown as a key chip
	desc   string // what it does, for the status line
	off    bool   // unavailable right now
	pick   func(m *Model) tea.Cmd
}

// haystack is what a search matches against.
func (it pickItem) haystack() string { return it.title[:it.name] + "  " + it.detail }

// pickMatch is an item that matches the search, with the matched byte
// offsets in its title and detail.
type pickMatch struct {
	item            *pickItem
	inTitle, inDesc []int
}

// picker is a searchable list in a box, in the manner of fzf: a search
// field on top, results below with matched characters highlighted, and
// the highlighted result's description on the status line. The search is
// edited in Model.line with the usual line-editing keys.
type picker struct {
	m           *Model // the model it acts on
	title       string
	placeholder string
	maxW        int
	action      string // what Enter does, for the key hints
	items       []pickItem
	shown       []pickMatch
	overlay.List

	// enter, when set, gets the first look at Enter with the search text,
	// e.g. to take a typed path; it reports whether it handled it.
	enter func(m *Model, query string) (tea.Cmd, bool)

	// answers marks a picker that is the question of a command that
	// changes the workbook: a recording keeps the title picked as the
	// command's answer, and scripts answer it with a title.
	answers bool
}

const (
	pickerID     = "picker"
	searchPrompt = " › "
)

func newPicker(m *Model, title, placeholder string, maxW int, items []pickItem) *picker {
	p := &picker{m: m, title: title, placeholder: placeholder, maxW: maxW, items: items}
	m.line.Clear()
	p.Changed()
	return p
}

func (p *picker) Indicator() string { return "MENU" }

// Changed filters the items by the search. Best matches come first; ties
// keep menu order.
func (p *picker) Changed() {
	m := p.m
	p.Sel, p.Top = 0, 0
	p.shown = p.shown[:0]
	q := strings.TrimSpace(m.line.Text())
	if q == "" {
		for i := range p.items {
			p.shown = append(p.shown, pickMatch{item: &p.items[i]})
		}
		return
	}
	// Rank title and path matches together by fuzzy score, so a tight
	// match like "file" on the File menu beats letters scattered across a
	// title. Title matches get a small bonus and win ties. Titles with a
	// word starting with the query ("col" in Column width) come before
	// the rest, such as "Command line", which only spells it across words.
	names, hay := make([]string, len(p.items)), make([]string, len(p.items))
	for i, it := range p.items {
		names[i], hay[i] = it.title[:it.name], it.haystack()
	}
	const titleBonus = 10
	type ranked struct {
		pm     pickMatch
		score  int
		order  int
		prefix bool // a word of the title starts with the query
	}
	best := map[int]*ranked{}
	for _, mt := range fuzzy.FindNoSort(q, names) {
		best[mt.Index] = &ranked{pickMatch{item: &p.items[mt.Index], inTitle: mt.MatchedIndexes}, mt.Score + titleBonus, mt.Index, false}
	}
	for _, mt := range fuzzy.FindNoSort(q, hay) {
		if r, ok := best[mt.Index]; ok && r.score >= mt.Score {
			continue
		}
		it := &p.items[mt.Index]
		pm := pickMatch{item: it}
		for _, i := range mt.MatchedIndexes {
			switch {
			case i < it.name:
				pm.inTitle = append(pm.inTitle, i)
			case i >= it.name+2:
				pm.inDesc = append(pm.inDesc, i-it.name-2)
			}
		}
		best[mt.Index] = &ranked{pm, mt.Score, mt.Index, false}
	}
	for i, r := range best {
		r.prefix = wordPrefix(names[i], q)
	}
	all := slices.Collect(maps.Values(best))
	slices.SortFunc(all, func(a, b *ranked) int {
		if a.prefix != b.prefix {
			if a.prefix {
				return -1
			}
			return 1
		}
		if c := cmp.Compare(b.score, a.score); c != 0 {
			return c
		}
		return cmp.Compare(a.order, b.order)
	})
	for _, r := range all {
		p.shown = append(p.shown, r.pm)
	}
}

// wordPrefix reports whether a word of title starts with q, ignoring case.
func wordPrefix(title, q string) bool {
	q = strings.ToLower(q)
	for w := range strings.FieldsSeq(strings.ToLower(title)) {
		if strings.HasPrefix(w, q) {
			return true
		}
	}
	return false
}

func (p *picker) Key(k tea.KeyPressMsg) tea.Cmd {
	m := p.m
	rows := p.rows(m)
	switch k.String() {
	case "up", "ctrl+p", "shift+tab":
		p.Move(-1, len(p.shown))
	case "down", "ctrl+n", "tab":
		p.Move(1, len(p.shown))
	case "pgup":
		p.Sel = max(p.Sel-rows, 0)
	case "pgdown":
		p.Sel = max(min(p.Sel+rows, len(p.shown)-1), 0)
	case "enter":
		return p.pick(m)
	case "esc":
		p.close(m)
	default:
		before := m.line.Text()
		m.line.Key(k)
		if m.line.Text() != before {
			p.Changed()
		}
	}
	return nil
}

func (p *picker) pick(m *Model) tea.Cmd {
	if p.enter != nil {
		if cmd, ok := p.enter(m, strings.TrimSpace(m.line.Text())); ok {
			return cmd
		}
	}
	if p.Sel >= len(p.shown) || p.shown[p.Sel].item.off {
		return nil
	}
	it := p.shown[p.Sel].item
	cmd := it.pick(m)
	if p.answers {
		m.recordAnswer(it.title, false)
	}
	return cmd
}

// close closes the picker without picking, which cancels the question
// it asks.
func (p *picker) close(m *Model) {
	m.closeOverlay()
	if p.answers {
		m.recordAnswer("", true)
	}
}

// answer picks the item titled title (in any case), as a script answers
// the picker's question, and reports whether there was one.
func (p *picker) answer(m *Model, title string) (tea.Cmd, bool) {
	for i := range p.items {
		if it := &p.items[i]; strings.EqualFold(it.title, title) && !it.off {
			return it.pick(m), true
		}
	}
	return nil, false
}

// Rows of the box: the top border, the search field, a separator, then
// the results.
const pickerFirstRow = 3

func (p *picker) Mouse(e overlay.MouseEvent) tea.Cmd {
	m := p.m
	if e.Box != pickerID {
		if e.Kind == overlay.MousePress {
			p.close(m)
		}
		return nil
	}
	i := p.Top + e.Row - pickerFirstRow
	switch {
	case e.Kind == overlay.MouseWheel && e.Button == tea.MouseWheelUp:
		p.Move(-1, len(p.shown))
	case e.Kind == overlay.MouseWheel && e.Button == tea.MouseWheelDown:
		p.Move(1, len(p.shown))
	case e.Row < pickerFirstRow || i >= len(p.shown) || i >= p.Top+p.rows(m):
	case e.Kind == overlay.MouseMotion:
		p.Sel = i
	case e.Kind == overlay.MousePress && e.Button == tea.MouseLeft:
		p.Sel = i
		return p.pick(m)
	}
	return nil
}

func (p *picker) Status() (string, string) {
	m := p.m
	keys := m.th.KeyHints("Up/Down", "move", "Enter", cmp.Or(p.action, "run"), "Esc", "close")
	if p.Sel >= len(p.shown) {
		return "", keys
	}
	it := p.shown[p.Sel].item
	if it.off {
		return it.desc + " (not available now)", keys
	}
	return it.desc, keys
}

// rows is how many results show. The box shrinks from the bottom as the
// search narrows, so the search field never moves, and it leaves the
// status line visible.
func (p *picker) rows(m *Model) int {
	return max(min(len(p.shown), 12, m.height-1-menuLine-1-4), 1)
}

func (p *picker) box(m *Model) (x, y, inner int) {
	tw, dw, kw := 0, 0, 0
	for _, it := range p.items {
		tw = max(tw, ansi.StringWidth(it.title))
		dw = max(dw, ansi.StringWidth(it.detail))
		kw = max(kw, ansi.StringWidth(it.key)+2)
	}
	inner = min(1+tw+3+dw+3+kw+1, p.maxW, m.width-2)
	inner = max(inner, min(40, m.width-2))
	return (m.width - inner - 2) / 2, menuLine + 1, inner
}

func (p *picker) Cursor() (int, int) {
	m := p.m
	x, y, _ := p.box(m)
	return x + 1 + ansi.StringWidth(searchPrompt) + ansi.StringWidth(m.line.Head()), y + 1
}

func (p *picker) Layout() []overlay.Box {
	m := p.m
	x, y, inner := p.box(m)
	rows := p.rows(m)
	if len(p.shown) > 0 {
		p.Show(rows)
	}
	input := m.th.Title.Render(searchPrompt) + m.line.Text()
	if len(m.line.Buf) == 0 {
		input += m.th.Muted.Render(p.placeholder)
	}
	lines := []string{theme.Cells(m.th.MenuBar, input, inner), theme.SepRow}
	lines = append(lines, p.resultRows(m, inner, rows)...)
	footer := strconv.Itoa(len(p.shown)) + " of " + strconv.Itoa(len(p.items))
	return []overlay.Box{{ID: pickerID, X: x, Y: y, Lines: m.th.Frame(inner, p.title, footer, lines)}}
}

// resultRows lays out the visible results in columns: title, detail and
// the shortcut chip at the right. The detail column is dropped when the
// box is too narrow for it.
func (p *picker) resultRows(m *Model, inner, rows int) []string {
	tw, kw := 0, 0
	for _, it := range p.items {
		tw = max(tw, ansi.StringWidth(it.title))
		if it.key != "" {
			kw = max(kw, ansi.StringWidth(it.key)+2)
		}
	}
	tw = min(tw, inner*3/5)
	dw := inner - 1 - tw - 3 - kw - 2
	out := make([]string, 0, rows)
	for r := range rows {
		i := p.Top + r
		if i >= len(p.shown) {
			text := ""
			if r == 0 {
				text = m.th.Muted.Render(" No matches")
			}
			out = append(out, theme.Cells(m.th.MenuBar, text, inner))
			continue
		}
		pm := p.shown[i]
		base, dim, hl := m.th.MenuBar, m.th.Muted, m.th.Match
		switch {
		case i == p.Sel:
			base, dim, hl = m.th.MenuSelected, m.th.MenuSelected, m.th.MatchSelected
		case pm.item.off:
			base, dim, hl = m.th.Disabled, m.th.Disabled, m.th.Disabled
		}
		row := base.Render(" ") + theme.HighlightMatches(pm.item.title, pm.inTitle, tw, base, hl)
		if dw >= 8 {
			row += base.Render("   ") + theme.HighlightMatches(pm.item.detail, pm.inDesc, dw, dim, hl)
		}
		k := ""
		switch {
		case pm.item.key == "":
		case i == p.Sel:
			k = base.Render(" " + pm.item.key + " ")
		case pm.item.off:
			k = m.th.Disabled.Render(" " + pm.item.key + " ")
		default:
			k = m.th.Chip(pm.item.key)
		}
		row += base.Render(strings.Repeat(" ", max(inner-ansi.StringWidth(row)-ansi.StringWidth(k)-1, 0))) + k + base.Render(" ")
		out = append(out, ansi.Truncate(row, inner, ""))
	}
	return out
}
