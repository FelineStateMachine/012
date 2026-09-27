package ui

import (
	"log/slog"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/telemetry"
)

// Sorting follows Sheets' Data menu: sort the sheet or a range by the
// active column, A to Z or Z to A, or pick the columns and order on a bar
// on the context line (Sheets' "Advanced range sorting options" dialog).
// The bar shows each sort column as a chip; arrows change the focused
// one, and the range being sorted is highlighted with the sort column's
// header lit.

func init() {
	register(
		&command{id: "data.sort_sheet_az", title: "Sort sheet A to Z", desc: "Sort every row by the active column, A to Z; frozen rows stay on top",
			run: func(m *Model) tea.Cmd { return m.sortSheet(false) }},
		&command{id: "data.sort_sheet_za", title: "Sort sheet Z to A", desc: "Sort every row by the active column, Z to A; frozen rows stay on top",
			run: func(m *Model) tea.Cmd { return m.sortSheet(true) }},
		&command{id: "data.sort_range_az", title: "Sort range A to Z", desc: "Sort the selection, or the data around the active cell, by the active column, A to Z",
			run: func(m *Model) tea.Cmd { return m.sortRange(false) }},
		&command{id: "data.sort_range_za", title: "Sort range Z to A", desc: "Sort the selection, or the data around the active cell, by the active column, Z to A",
			run: func(m *Model) tea.Cmd { return m.sortRange(true) }},
		&command{id: "data.sort_range", title: "Sort range", desc: "Sort the selection by one or more columns, with or without a header row",
			run: func(m *Model) tea.Cmd {
				m.openSortBar()
				return nil
			}},
	)
}

// sortSheet sorts all rows below the frozen ones by the active column.
func (m *Model) sortSheet(desc bool) tea.Cmd {
	used, ok := m.sheet.UsedRange()
	fr, _ := m.sheet.Frozen()
	if !ok || used.To.Row <= fr {
		m.note = "Nothing to sort"
		return nil
	}
	r := sheet.Rect{From: sheet.Addr{Row: fr}, To: used.To}
	return m.sort(r, []sheet.SortKey{{Col: m.cur.Col, Desc: desc}})
}

// sortRange sorts the selection, or the data around the active cell, by
// the active column, leaving any header row in place.
func (m *Model) sortRange(desc bool) tea.Cmd {
	r := m.dataRange()
	r.From.Row += m.headerRows(r, m.hasRange())
	col := clamp(m.cur.Col, r.From.Col, r.To.Col)
	return m.sort(r, []sheet.SortKey{{Col: col, Desc: desc}})
}

func (m *Model) sort(r sheet.Rect, keys []sheet.SortKey) tea.Cmd {
	if r.To.Row <= r.From.Row {
		m.note = "Nothing to sort"
		return nil
	}
	span := telemetry.Start("sort", slog.Int("rows", r.To.Row-r.From.Row+1), slog.Int("cols", r.To.Col-r.From.Col+1), slog.Int("keys", len(keys)))
	m.sheet.SortRange(r, keys)
	span.End()
	m.changed = true
	var by []string
	for _, k := range keys {
		by = append(by, sheet.ColName(k.Col)+" "+orderName(k.Desc))
	}
	m.note = "Sorted " + r.String() + " by " + strings.Join(by, ", then ")
	return nil
}

func orderName(desc bool) string {
	if desc {
		return "Z→A"
	}
	return "A→Z"
}

// headerRows is how many rows at the top of r are headers that sorting
// leaves in place: frozen rows, the filter's header row, and, for a range
// found around the active cell, a first row of text over numbers.
func (m *Model) headerRows(r sheet.Rect, selected bool) int {
	n := 0
	if fr, _ := m.sheet.Frozen(); fr > r.From.Row {
		n = fr - r.From.Row
	}
	if f, ok := m.sheet.FilterRange(); ok && f.From.Row == r.From.Row {
		n = max(n, 1)
	}
	if n == 0 && !selected && m.looksLikeHeader(r) {
		n = 1
	}
	return min(n, r.To.Row-r.From.Row)
}

// looksLikeHeader reports whether r's first row is all text above a row
// with numbers, like "Item, Amount" above "Rent, 1450".
func (m *Model) looksLikeHeader(r sheet.Rect) bool {
	if r.To.Row == r.From.Row {
		return false
	}
	text, numbers := false, false
	for c := r.From.Col; c <= r.To.Col; c++ {
		switch m.sheet.Value(sheet.Addr{Col: c, Row: r.From.Row}).Kind {
		case sheet.Empty:
			continue
		case sheet.Text:
			text = true
		default:
			return false
		}
		switch m.sheet.Value(sheet.Addr{Col: c, Row: r.From.Row + 1}).Kind {
		case sheet.Number, sheet.Bool:
			numbers = true
		}
	}
	return text && numbers
}

// sortBar picks the columns and order to sort a range by.
type sortBar struct {
	rng     sheet.Rect // the whole range, header rows included
	headers int        // header rows at the top of rng; 0 when switched off
	guess   int        // header rows to use when switched on
	keys    []sheet.SortKey
	cur     int // the key being changed
}

func (m *Model) openSortBar() {
	r := m.dataRange()
	b := &sortBar{rng: r, headers: m.headerRows(r, m.hasRange())}
	b.guess = max(b.headers, 1)
	b.keys = []sheet.SortKey{{Col: clamp(m.cur.Col, r.From.Col, r.To.Col)}}
	m.openOverlay(b)
}

// data is the range the sort moves: rng without its headers.
func (b *sortBar) data() sheet.Rect {
	r := b.rng
	r.From.Row = min(r.From.Row+b.headers, r.To.Row)
	return r
}

// barRange is the range highlighted while a data bar is open.
func (m *Model) barRange() (sheet.Rect, bool) {
	if b, ok := m.overlay.(*sortBar); ok {
		return b.data(), true
	}
	return sheet.Rect{}, false
}

// barActive is the cell drawn as active while a data bar is open: the top
// of the focused sort column, so its header lights up.
func (m *Model) barActive() (sheet.Addr, bool) {
	if b, ok := m.overlay.(*sortBar); ok {
		return sheet.Addr{Col: b.keys[b.cur].Col, Row: b.data().From.Row}, true
	}
	return sheet.Addr{}, false
}

func (b *sortBar) indicator() string   { return "SORT" }
func (b *sortBar) layout(*Model) []box { return nil }

// setCol points the focused key at column c, if it's in the range.
func (b *sortBar) setCol(c int) {
	if c >= b.rng.From.Col && c <= b.rng.To.Col {
		b.keys[b.cur].Col = c
	}
}

// add adds a sort column after the focused one: the next column not yet
// sorted by.
func (b *sortBar) add() {
	used := map[int]bool{}
	for _, k := range b.keys {
		used[k.Col] = true
	}
	for c := b.rng.From.Col; c <= b.rng.To.Col; c++ {
		if !used[c] {
			b.keys = append(b.keys[:b.cur+1], append([]sheet.SortKey{{Col: c}}, b.keys[b.cur+1:]...)...)
			b.cur++
			return
		}
	}
}

func (b *sortBar) key(m *Model, k tea.KeyPressMsg) tea.Cmd {
	key := k.String()
	switch key {
	case "esc":
		m.closeOverlay()
	case "enter":
		keys, r := b.keys, b.data()
		m.closeOverlay()
		return m.sort(r, keys)
	case "left":
		b.setCol(b.keys[b.cur].Col - 1)
	case "right":
		b.setCol(b.keys[b.cur].Col + 1)
	case "up", "down", "space":
		b.keys[b.cur].Desc = !b.keys[b.cur].Desc
	case "tab":
		b.cur = (b.cur + 1) % len(b.keys)
	case "shift+tab":
		b.cur = (b.cur + len(b.keys) - 1) % len(b.keys)
	case "alt+a":
		b.add()
	case "alt+h":
		if b.headers > 0 {
			b.headers = 0
		} else {
			b.headers = min(b.guess, b.rng.To.Row-b.rng.From.Row)
		}
	case "backspace", "delete":
		if len(b.keys) > 1 {
			b.keys = append(b.keys[:b.cur], b.keys[b.cur+1:]...)
			b.cur = min(b.cur, len(b.keys)-1)
		}
	default:
		// Typing a column's letter sorts by it.
		if t := typed(k); len(t) == 1 {
			if c, ok := sheet.ParseCol(t); ok {
				b.setCol(c)
			}
		}
	}
	return nil
}

// sortPart is a piece of the bar: plain text, a sort key's chip, or the
// header row toggle.
type sortPart struct {
	text   string
	key    int // index of the sort key, -1 for others
	toggle bool
}

func (b *sortBar) parts(m *Model) []sortPart {
	parts := []sortPart{{text: m.th.Key.Render("Sort "+b.data().String()) + m.th.Muted.Render(" by "), key: -1}}
	for i, k := range b.keys {
		if i > 0 {
			parts = append(parts, sortPart{text: m.th.Muted.Render(" then "), key: -1})
		}
		label := sheet.ColName(k.Col)
		if b.headers > 0 {
			if h := m.sheet.ShownText(sheet.Addr{Col: k.Col, Row: b.rng.From.Row + b.headers - 1}); h != "" {
				label += " " + ansi.Truncate(h, 16, "…")
			}
		}
		style := m.th.KeyChip
		if i == b.cur {
			style = m.th.MenuSelected
		}
		parts = append(parts, sortPart{text: style.Render(" " + label + "  " + orderName(k.Desc) + " "), key: i})
	}
	style := m.th.Muted
	if b.headers > 0 {
		style = m.th.MenuSelected
	}
	parts = append(parts, sortPart{text: "   ", key: -1}, sortPart{text: style.Render(" Header row "), key: -1, toggle: true})
	return parts
}

// line renders the bar for the context line.
func (b *sortBar) line(m *Model) string {
	var s strings.Builder
	for _, p := range b.parts(m) {
		s.WriteString(p.text)
	}
	return s.String()
}

// mouse focuses a key's chip (a second click flips its order) or flips
// the header row toggle; a click anywhere else cancels.
func (b *sortBar) mouse(m *Model, e mouseEvent) tea.Cmd {
	if e.kind != mousePress {
		return nil
	}
	if e.y != contextLine {
		m.closeOverlay()
		return nil
	}
	x := 0
	for _, p := range b.parts(m) {
		w := ansi.StringWidth(p.text)
		if e.x >= x && e.x < x+w {
			switch {
			case p.toggle:
				return b.key(m, keyFor("alt+h"))
			case p.key == b.cur:
				b.keys[b.cur].Desc = !b.keys[b.cur].Desc
			case p.key >= 0:
				b.cur = p.key
			}
			return nil
		}
		x += w
	}
	return nil
}

// status shows the keys, the most useful ones first on narrow screens.
func (b *sortBar) status(m *Model) (string, string) {
	pairs := []string{"Left/Right", "column", "Space", "order", "Enter", "sort", "Esc", "cancel"}
	desc := "Alt+A add  Alt+H header  Tab next"
	for {
		keys := m.th.KeyHints(pairs...)
		switch {
		case ansi.StringWidth(desc)+3+ansi.StringWidth(keys) <= m.width:
			return m.th.Muted.Render(desc), keys
		case desc != "":
			desc = ""
		case len(pairs) > 4:
			pairs = pairs[2:]
		default:
			return "", keys
		}
	}
}
