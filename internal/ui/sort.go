package ui

import (
	"log/slog"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/ui/sortbar"
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
			},
			answer: func(m *Model, text string) (tea.Cmd, error) { return m.openSortBar().Answer(text) }},
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
	if _, reg, ok := m.sheet.InRegion(r); ok {
		m.note = "A region keeps its source's order: sort it in its cell (sort-by), or freeze it first"
		if reg.Linked() {
			m.note = "A linked file's rows keep the file's order: unlink it to sort them"
		}
		return nil
	}
	if m.refuseEdit(r, false) {
		return nil
	}
	if len(m.sheet.MergesIn(r)) > 0 {
		m.fail(sheet.ErrSortMerged.Error())
		return nil
	}
	retry := func(m *Model) tea.Cmd { return m.sort(r, keys) }
	if m.askProtected(r, retry) || m.askUndoCost(r, retry) {
		return nil
	}
	span := m.spans.Start("sort", slog.Int("rows", r.To.Row-r.From.Row+1), slog.Int("cols", r.To.Col-r.From.Col+1), slog.Int("keys", len(keys)))
	m.sheet.SortRange(r, keys)
	span.End()
	m.changed = true
	var by []string
	for _, k := range keys {
		by = append(by, sheet.ColName(k.Col)+" "+sortbar.OrderName(k.Desc))
	}
	m.note = "Sorted " + r.String() + " by " + strings.Join(by, ", then ")
	return nil
}

// headerRows is how many rows at the top of r are headers that sorting
// leaves in place: frozen rows, the filter's header row, and, for a range
// found around the active cell, a first row of text over numbers.
func (g *grid) headerRows(r sheet.Rect, selected bool) int {
	n := 0
	if fr, _ := g.sheet.Frozen(); fr > r.From.Row {
		n = fr - r.From.Row
	}
	if f, ok := g.sheet.FilterRange(); ok && f.From.Row == r.From.Row {
		n = max(n, 1)
	}
	if n == 0 && !selected && g.looksLikeHeader(r) {
		n = 1
	}
	return min(n, r.To.Row-r.From.Row)
}

// looksLikeHeader reports whether r's first row is all text above a row
// with numbers, like "Item, Amount" above "Rent, 1450".
func (g *grid) looksLikeHeader(r sheet.Rect) bool {
	if r.To.Row == r.From.Row {
		return false
	}
	text, numbers := false, false
	for c := r.From.Col; c <= r.To.Col; c++ {
		switch g.sheet.Value(sheet.Addr{Col: c, Row: r.From.Row}).Kind {
		case sheet.Empty:
			continue
		case sheet.Text:
			text = true
		default:
			return false
		}
		switch g.sheet.Value(sheet.Addr{Col: c, Row: r.From.Row + 1}).Kind {
		case sheet.Number, sheet.Bool:
			numbers = true
		}
	}
	return text && numbers
}

// openSortBar opens the sort bar over the selection, or the data around
// the active cell.
func (m *Model) openSortBar() *sortbar.Bar {
	r := m.dataRange()
	b := sortbar.New(m.host(), r, m.headerRows(r, m.hasRange()), m.cur.Col)
	m.openOverlay(b)
	return b
}

// barRange is the range highlighted while a data bar is open.
func (m *Model) barRange() (sheet.Rect, bool) {
	if b, ok := m.overlay.(*sortbar.Bar); ok {
		return b.Data(), true
	}
	return sheet.Rect{}, false
}

// barActive is the cell drawn as active while a data bar is open: the top
// of the focused sort column, so its header lights up.
func (m *Model) barActive() (sheet.Addr, bool) {
	if b, ok := m.overlay.(*sortbar.Bar); ok {
		return b.Active(), true
	}
	return sheet.Addr{}, false
}

// Sort sorts from the sort bar, which a macro records as the bar's
// command answered with its choices.
func (h host) Sort(data sheet.Rect, keys []sheet.SortKey, answer string) tea.Cmd {
	cmd := h.m.sort(data, keys)
	h.m.recordDialog("data.sort_range", answer)
	return cmd
}
