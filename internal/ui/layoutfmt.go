package ui

import (
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// Format > Wrapping, Borders and Merge cells, as in Sheets: each applies
// to the selection as one undo step and says what it did on the context
// line. Borders draw with the line style last chosen (thin at first),
// as Sheets' border style picker keeps its choice.

var wrappings = []struct {
	id, title, desc string
	w               sheet.Wrap
}{
	{"format.wrap_overflow", "Overflow", "Let text run on into the blank cells beside it", sheet.WrapOverflow},
	{"format.wrap", "Wrap", "Break text into lines that fit the column; the row grows to fit them", sheet.WrapOn},
	{"format.wrap_clip", "Clip", "Cut text off at the cell's edge", sheet.WrapClip},
}

var borderKinds = []struct {
	id, title, desc string
	k               sheet.BorderKind
}{
	{"format.borders_all", "All borders", "Draw every edge of the selected cells", sheet.BorderAll},
	{"format.borders_outer", "Outer borders", "Draw a line around the selection", sheet.BorderOuter},
	{"format.borders_inner", "Inner borders", "Draw the lines between the selected cells", sheet.BorderInner},
	{"format.border_top", "Top border", "Draw a line along the top of the selection", sheet.BorderTop},
	{"format.border_bottom", "Bottom border", "Draw a line along the bottom of the selection", sheet.BorderBottom},
	{"format.border_left", "Left border", "Draw a line down the left of the selection", sheet.BorderLeft},
	{"format.border_right", "Right border", "Draw a line down the right of the selection", sheet.BorderRight},
	{"format.borders_clear", "Clear borders", "Remove the lines of the selected cells", sheet.BorderNone},
}

var borderLines = []struct {
	id, title, desc string
	l               sheet.Line
}{
	{"format.border_thin", "Thin lines", "Draw borders with thin lines: ─", sheet.LineThin},
	{"format.border_thick", "Thick lines", "Draw borders with thick lines: ━", sheet.LineThick},
	{"format.border_double", "Double lines", "Draw borders with double lines: ═", sheet.LineDouble},
}

var mergeKinds = []struct {
	id, title, desc string
	k               sheet.MergeKind
}{
	{"format.merge_all", "Merge all", "Join the selection into one cell showing its top-left value", sheet.MergeAll},
	{"format.merge_horizontal", "Merge horizontally", "Join each row of the selection into one cell", sheet.MergeHorizontally},
	{"format.merge_vertical", "Merge vertically", "Join each column of the selection into one cell", sheet.MergeVertically},
}

func init() {
	for _, wr := range wrappings {
		w, title := wr.w, wr.title
		register(&command{id: wr.id, title: title, desc: wr.desc, edits: (*Model).selection, keepsSpills: true,
			checked: func(m *Model) bool { return m.sheet.CellStyle(m.cur).Wrap == w },
			run: func(m *Model) tea.Cmd {
				r := m.selection()
				m.sheet.Batch(sheet.Change{Label: strings.ToLower(title) + " " + r.String(), Focus: r}, func() error {
					m.sheet.SetStyle(r, func(s *sheet.Style) { s.Wrap = w })
					return nil
				})
				m.formatted("Wrapping: " + strings.ToLower(title))
				return nil
			}})
	}
	for _, bk := range borderKinds {
		k, title := bk.k, bk.title
		register(&command{id: bk.id, title: title, desc: bk.desc, edits: (*Model).selection, keepsSpills: true,
			run: func(m *Model) tea.Cmd {
				m.sheet.SetBorderStroke(m.selection(), k, sheet.Stroke{Line: m.lineStyle(), Color: m.borderColor})
				m.formatted(title)
				return nil
			}})
	}
	for _, bl := range borderLines {
		l := bl.l
		register(&command{id: bl.id, title: bl.title, desc: bl.desc,
			checked: func(m *Model) bool { return m.lineStyle() == l },
			run: func(m *Model) tea.Cmd {
				m.borderLine = l
				m.note = "Borders draw " + strings.ToLower(bl.title) + " now"
				return nil
			}})
	}
	for _, mk := range mergeKinds {
		k := mk.k
		register(&command{id: mk.id, title: mk.title, desc: mk.desc, edits: (*Model).selection,
			enabled: (*Model).hasRange,
			run:     func(m *Model) tea.Cmd { return m.merge(k) }})
	}
	register(&command{id: "format.unmerge", title: "Unmerge", desc: "Split the merged cells in the selection back into cells", edits: (*Model).selection, keepsSpills: true,
		enabled: func(m *Model) bool { return len(m.sheet.MergesIn(m.selection())) > 0 },
		run: func(m *Model) tea.Cmd {
			n := m.sheet.Unmerge(m.selection())
			m.changed = m.changed || n > 0
			m.note = "Unmerged " + strconv.Itoa(n) + plural(n, " merged cell", " merged cells")
			return nil
		}})
	// Sheets' border shortcuts; terminals without the kitty keyboard
	// protocol send Alt+Shift+1 as alt+!, which keyAliases maps.
	for key, id := range map[string]string{
		"alt+shift+1": "format.border_top",
		"alt+shift+2": "format.border_right",
		"alt+shift+3": "format.border_bottom",
		"alt+shift+4": "format.border_left",
		"alt+shift+6": "format.borders_clear",
		"alt+shift+7": "format.borders_outer",
	} {
		keymap[key] = id
	}
}

// lineStyle is the line Format > Borders draws with.
func (m *Model) lineStyle() sheet.Line {
	if m.borderLine == sheet.LineNone {
		return sheet.LineThin
	}
	return m.borderLine
}

// merge joins the selection as k says, asking first when that clears
// values other than the top-left ones, as Sheets warns.
func (m *Model) merge(k sheet.MergeKind) tea.Cmd {
	r := m.selection()
	do := func(m *Model) tea.Cmd {
		if err := m.sheet.Merge(r, k); err != nil {
			m.fail(err.Error())
			return nil
		}
		m.changed = true
		m.clearSelection()
		m.cur = r.From
		m.note = "Merged " + r.String()
		return nil
	}
	lost, loses := m.sheet.MergeLoses(r, k)
	if !loses {
		return do(m)
	}
	m.openOverlay(&choiceBar{m: m, msg: "Merging keeps only the top-left value.", warn: true,
		desc: "Merging clears " + lost.String() + " and the other values; undo brings them back",
		choices: []choice{
			{key: "enter", label: "Merge", run: do},
			{key: "esc", label: "Cancel", run: func(*Model) tea.Cmd { return nil }},
		}})
	return nil
}
