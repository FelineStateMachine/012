// Package picker is the searchable list behind the command palette and
// every other picker of the UI (functions, sheets, named ranges, macros,
// themes, files to import): a search field on top, results below with
// matched characters highlighted, in the manner of fzf. It knows the UI
// only through Host.
package picker

import (
	"cmp"
	"maps"
	"slices"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sahilm/fuzzy"

	"github.com/FelineStateMachine/012/internal/ui/lineedit"
	"github.com/FelineStateMachine/012/internal/ui/overlay"
	"github.com/FelineStateMachine/012/internal/ui/theme"
)

// Host is what a picker needs of the UI.
type Host interface {
	Theme() *theme.Theme
	Size() (width, height int)
	// Line is the shared edit line the search is typed in.
	Line() *lineedit.Line
	// Close closes the open overlay.
	Close()
	// RecordAnswer tells a macro recording the answer to the question of
	// the command that opened the picker, or that it was cancelled.
	RecordAnswer(answer string, cancelled bool)
}

// Item is one row of a picker.
type Item struct {
	Title  string // e.g. "Save as"
	Name   int    // bytes of Title that are searched, e.g. just "SUM" of "SUM(value, ...)"
	Detail string // a dimmed second column, also searched, e.g. the menu path
	Key    string // shortcut, shown as a key chip
	Desc   string // what it does, for the status line
	Off    bool   // unavailable right now
	Pick   func() tea.Cmd
}

// haystack is what a search matches against.
func (it Item) haystack() string { return it.Title[:it.Name] + "  " + it.Detail }

// Match is an item that matches the search, with the matched byte
// offsets in its title and detail.
type Match struct {
	Item              *Item
	InTitle, InDetail []int
}

// Picker is a searchable list in a box: a search field on top, results
// below with matched characters highlighted, and the highlighted result's
// description on the status line. The search is edited in the host's
// edit line with the usual line-editing keys.
type Picker struct {
	h           Host
	title       string
	placeholder string
	maxW        int
	shown       []Match
	overlay.List

	// Action is what Enter does, for the key hints ("run" by default).
	Action string
	Items  []Item

	// Narrow, when set, sees the search first and can narrow the items by
	// it, as "dark" does the themes: it returns the rest of the search,
	// matched against the kept items' titles only, and which items to
	// keep, or a nil keep to search every item as usual.
	Narrow func(query string) (rest string, keep func(*Item) bool)

	// Enter, when set, gets the first look at Enter with the search text,
	// e.g. to take a typed path; it reports whether it handled it.
	Enter func(query string) (tea.Cmd, bool)

	// Answers marks a picker that is the question of a command that
	// changes the workbook: a recording keeps the title picked as the
	// command's answer, and scripts answer it with a title.
	Answers bool

	// At, when set, is where the box's top-left corner goes, as a
	// dropdown opens under its cell; the box stays on screen. Pickers
	// open centered under the menu bar otherwise.
	At *[2]int
}

// ID identifies the picker's box in mouse events.
const ID = "picker"

// New returns a picker of items titled title, at most maxW columns wide
// inside its frame, with the edit line cleared for the search.
func New(h Host, title, placeholder string, maxW int, items []Item) *Picker {
	p := &Picker{h: h, title: title, placeholder: placeholder, maxW: maxW, Items: items}
	h.Line().Clear()
	p.Changed()
	return p
}

// Title is the picker's title, on its frame.
func (p *Picker) Title() string { return p.title }

// Shown are the items that match the search, best first.
func (p *Picker) Shown() []Match { return p.shown }

// Selected is the highlighted item, or nil when nothing matches.
func (p *Picker) Selected() *Item {
	if p.Sel >= len(p.shown) {
		return nil
	}
	return p.shown[p.Sel].Item
}

func (p *Picker) Indicator() string { return "MENU" }

// Changed filters the items by the search. Best matches come first; ties
// keep menu order.
func (p *Picker) Changed() {
	p.Sel, p.Top = 0, 0
	p.shown = p.shown[:0]
	q := strings.TrimSpace(p.h.Line().Text())
	var keep func(*Item) bool
	if p.Narrow != nil {
		if rest, k := p.Narrow(q); k != nil {
			q, keep = strings.TrimSpace(rest), k
		}
	}
	if q == "" {
		for i := range p.Items {
			if keep == nil || keep(&p.Items[i]) {
				p.shown = append(p.shown, Match{Item: &p.Items[i]})
			}
		}
		return
	}
	p.rank(q, keep)
}

// rank shows the items matching q, best first: of those keep keeps,
// matching only their titles, when it is set.
func (p *Picker) rank(q string, keep func(*Item) bool) {
	// Rank title and path matches together by fuzzy score, so a tight
	// match like "file" on the File menu beats letters scattered across a
	// title. Title matches get a small bonus and win ties. Titles with a
	// word starting with the query ("col" in Column width) come before
	// the rest, such as "Command line", which only spells it across words.
	names, hay := make([]string, len(p.Items)), make([]string, len(p.Items))
	for i, it := range p.Items {
		names[i], hay[i] = it.Title[:it.Name], it.haystack()
	}
	const titleBonus = 10
	type ranked struct {
		pm     Match
		score  int
		order  int
		prefix bool // a word of the title starts with the query
	}
	best := map[int]*ranked{}
	for _, mt := range fuzzy.FindNoSort(q, names) {
		if keep != nil && !keep(&p.Items[mt.Index]) {
			continue
		}
		best[mt.Index] = &ranked{Match{Item: &p.Items[mt.Index], InTitle: mt.MatchedIndexes}, mt.Score + titleBonus, mt.Index, false}
	}
	if keep != nil {
		hay = nil // titles only
	}
	for _, mt := range fuzzy.FindNoSort(q, hay) {
		if r, ok := best[mt.Index]; ok && r.score >= mt.Score {
			continue
		}
		it := &p.Items[mt.Index]
		pm := Match{Item: it}
		for _, i := range mt.MatchedIndexes {
			switch {
			case i < it.Name:
				pm.InTitle = append(pm.InTitle, i)
			case i >= it.Name+2:
				pm.InDetail = append(pm.InDetail, i-it.Name-2)
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

func (p *Picker) Key(k tea.KeyPressMsg) tea.Cmd {
	rows := p.rows()
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
		return p.pick()
	case "esc":
		p.Close()
	default:
		line := p.h.Line()
		before := line.Text()
		line.Key(k)
		if line.Text() != before {
			p.Changed()
		}
	}
	return nil
}

func (p *Picker) pick() tea.Cmd {
	if p.Enter != nil {
		if cmd, ok := p.Enter(strings.TrimSpace(p.h.Line().Text())); ok {
			return cmd
		}
	}
	if p.Sel >= len(p.shown) || p.shown[p.Sel].Item.Off {
		return nil
	}
	it := p.shown[p.Sel].Item
	cmd := it.Pick()
	if p.Answers {
		p.h.RecordAnswer(it.Title, false)
	}
	return cmd
}

// Close closes the picker without picking, which cancels the question
// it asks.
func (p *Picker) Close() {
	p.h.Close()
	if p.Answers {
		p.h.RecordAnswer("", true)
	}
}

// Answer picks the item titled title (in any case), as a script answers
// the picker's question, and reports whether there was one.
func (p *Picker) Answer(title string) (tea.Cmd, bool) {
	for i := range p.Items {
		if it := &p.Items[i]; strings.EqualFold(it.Title, title) && !it.Off {
			return it.Pick(), true
		}
	}
	return nil, false
}

// FirstRow is the box's first result row, under the top border, the
// search field and a separator.
const FirstRow = 3

func (p *Picker) Mouse(e overlay.MouseEvent) tea.Cmd {
	if e.Box != ID {
		if e.Kind == overlay.MousePress {
			p.Close()
		}
		return nil
	}
	i := p.Top + e.Row - FirstRow
	switch {
	case e.Kind == overlay.MouseWheel && e.Button == tea.MouseWheelUp:
		p.Move(-1, len(p.shown))
	case e.Kind == overlay.MouseWheel && e.Button == tea.MouseWheelDown:
		p.Move(1, len(p.shown))
	case e.Row < FirstRow || i >= len(p.shown) || i >= p.Top+p.rows():
	case e.Kind == overlay.MouseMotion:
		p.Sel = i
	case e.Kind == overlay.MousePress && e.Button == tea.MouseLeft:
		p.Sel = i
		return p.pick()
	}
	return nil
}

func (p *Picker) Status() (string, string) {
	keys := p.h.Theme().KeyHints("Up/Down", "move", "Enter", cmp.Or(p.Action, "run"), "Esc", "close")
	if p.Sel >= len(p.shown) {
		return "", keys
	}
	it := p.shown[p.Sel].Item
	if it.Off {
		return it.Desc + " (not available now)", keys
	}
	return it.Desc, keys
}

// rows is how many results show. The box shrinks from the bottom as the
// search narrows, so the search field never moves, and it leaves the
// status line visible.
func (p *Picker) rows() int {
	_, height := p.h.Size()
	return max(min(len(p.shown), 12, height-1-overlay.MenuLine-1-4), 1)
}

func (p *Picker) box() (x, y, inner int) {
	width, _ := p.h.Size()
	tw, dw, kw := 0, 0, 0
	for _, it := range p.Items {
		tw = max(tw, ansi.StringWidth(it.Title))
		dw = max(dw, ansi.StringWidth(it.Detail))
		kw = max(kw, ansi.StringWidth(it.Key)+2)
	}
	inner = min(1+tw+3+dw+3+kw+1, p.maxW, width-2)
	inner = max(inner, min(40, width-2))
	if p.At != nil {
		_, height := p.h.Size()
		h := max(min(len(p.Items), 12), 1) + 4 // its tallest, so it never moves
		x := min(max(p.At[0], 0), max(width-inner-2, 0))
		y := min(max(p.At[1], 0), max(height-1-h, 0))
		return x, y, inner
	}
	return (width - inner - 2) / 2, overlay.MenuLine + 1, inner
}

func (p *Picker) Cursor() (int, int) {
	x, y, _ := p.box()
	return x + 1 + ansi.StringWidth(overlay.SearchPrompt) + ansi.StringWidth(p.h.Line().Head()), y + 1
}

func (p *Picker) Layout() []overlay.Box {
	th, line := p.h.Theme(), p.h.Line()
	x, y, inner := p.box()
	rows := p.rows()
	if len(p.shown) > 0 {
		p.Show(rows)
	}
	input := th.Title.Render(overlay.SearchPrompt) + line.Text()
	if len(line.Buf) == 0 {
		input += th.Muted.Render(p.placeholder)
	}
	lines := []string{theme.Cells(th.MenuBar, input, inner), theme.SepRow}
	lines = append(lines, p.resultRows(th, inner, rows)...)
	footer := strconv.Itoa(len(p.shown)) + " of " + strconv.Itoa(len(p.Items))
	return []overlay.Box{{ID: ID, X: x, Y: y, Lines: th.Frame(inner, p.title, footer, lines)}}
}

// resultRows lays out the visible results in columns: title, detail and
// the shortcut chip at the right. The detail column is dropped when the
// box is too narrow for it.
func (p *Picker) resultRows(th *theme.Theme, inner, rows int) []string {
	tw, kw := 0, 0
	for _, it := range p.Items {
		tw = max(tw, ansi.StringWidth(it.Title))
		if it.Key != "" {
			kw = max(kw, ansi.StringWidth(it.Key)+2)
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
				text = th.Muted.Render(" No matches")
			}
			out = append(out, theme.Cells(th.MenuBar, text, inner))
			continue
		}
		pm := p.shown[i]
		base, dim, hl := th.MenuBar, th.Muted, th.Match
		switch {
		case i == p.Sel:
			base, dim, hl = th.MenuSelected, th.MenuSelected, th.MatchSelected
		case pm.Item.Off:
			base, dim, hl = th.Disabled, th.Disabled, th.Disabled
		}
		row := base.Render(" ") + theme.HighlightMatches(pm.Item.Title, pm.InTitle, tw, base, hl)
		if dw >= 8 {
			row += base.Render("   ") + theme.HighlightMatches(pm.Item.Detail, pm.InDetail, dw, dim, hl)
		}
		k := ""
		switch {
		case pm.Item.Key == "":
		case i == p.Sel:
			k = base.Render(" " + pm.Item.Key + " ")
		case pm.Item.Off:
			k = th.Disabled.Render(" " + pm.Item.Key + " ")
		default:
			k = th.Chip(pm.Item.Key)
		}
		row += base.Render(strings.Repeat(" ", max(inner-ansi.StringWidth(row)-ansi.StringWidth(k)-1, 0))) + k + base.Render(" ")
		out = append(out, ansi.Truncate(row, inner, ""))
	}
	return out
}
