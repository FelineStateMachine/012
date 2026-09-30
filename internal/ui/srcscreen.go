package ui

import (
	"path/filepath"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/locale"
	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/ui/srcview"
	"github.com/FelineStateMachine/012/internal/ui/theme"
	"github.com/FelineStateMachine/012/internal/ui/transfer"
)

// A linked source's tab on the screen: the menu bar with SOURCE as the
// mode, the formula bar naming the active cell as the source's sheet
// does and showing its value, the context line saying what the source
// is and how it's ordered, then the source's rows where a sheet's grid
// would be (package srcview). Keys move the active cell; commands that
// read a tab's data (sort, filter, pivot tables, copy) act on the
// source, and those that would change cells say it's read-only.

// srcView is the view of the source tab shown, or nil on another.
func (m *Model) srcView() *srcview.View {
	if !m.sheet.IsSource() {
		return nil
	}
	v := m.src.views[m.sheet]
	if v == nil {
		if m.src.views == nil {
			m.src.views = map[*sheet.Sheet]*srcview.View{}
		}
		v = srcview.New(srcHost{m, m.sheet})
		m.src.views[m.sheet] = v
	}
	v.Resize(m.width, m.height-headerLine-1)
	return v
}

// srcHost is a source tab's srcview.Host.
type srcHost struct {
	m *Model
	s *sheet.Sheet
}

func (h srcHost) Theme() *theme.Theme    { return &h.m.th }
func (h srcHost) Locale() *locale.Locale { return h.m.locale() }

func (h srcHost) Columns() []srcview.Column {
	info, _ := h.s.Source()
	cols := make([]srcview.Column, len(info.Shape.Cols))
	for i, name := range info.Shape.Cols {
		cols[i].Name = name
		if i < len(info.Shape.Formats) {
			cols[i].Format = info.Shape.Formats[i]
		}
	}
	return cols
}

func (h srcHost) Rows() (int64, bool) {
	if p := h.pages(); p != nil {
		return p.Rows()
	}
	return 0, false
}

func (h srcHost) Row(i int64) (int64, []sheet.LiveCell, bool) {
	if p := h.pages(); p != nil {
		return p.Row(i)
	}
	return 0, nil, false
}

func (h srcHost) Want(from, to int64) {
	if p := h.pages(); p != nil {
		p.Want(from, to)
	}
}

func (h srcHost) pages() interface {
	Rows() (int64, bool)
	Row(int64) (int64, []sheet.LiveCell, bool)
	Want(int64, int64)
} {
	info, ok := h.s.Source()
	if !ok {
		return nil
	}
	if p := h.m.src.pages[info.Name]; p != nil {
		return p
	}
	return nil
}

// sourceLines are the screen's lines on a source tab, from the menu bar
// down to the line above the status line.
func (m *Model) sourceLines(v *srcview.View) []string {
	return append([]string{m.menuBarLine(), m.formulaBar(), m.contextLineText()}, v.Draw()...)
}

// sourceName is the active cell as the source's tab names it, and its
// value as text: C5 is the source's fourth row, under the header.
func (m *Model) sourceCell(v *srcview.View) (string, string) {
	num, cell, ok := v.Cell()
	_, col := v.Active()
	if !ok {
		return sheet.ColName(col) + "…", ""
	}
	a := sheet.Addr{Col: col, Row: int(num) + 1}
	name := a.String()
	if a.Row >= sheet.MaxRows {
		name = sheet.ColName(col) + strconv.FormatInt(num+2, 10)
	}
	if cell.V.Kind == sheet.Empty {
		return name, ""
	}
	f := cell.F
	if f.IsZero() {
		if cols := (srcHost{m, m.sheet}).Columns(); col < len(cols) {
			f = cols[col].Format
		}
	}
	return name, sheet.FormatTextIn(cell.V, f, m.locale())
}

// sourceContext is the context line on a source tab: what it is, how
// many rows it has, and how they're ordered.
func (m *Model) sourceContext() (string, string) {
	info, _ := m.sheet.Source()
	name := filepath.Base(info.Source.Path)
	if info.Source.Table != "" {
		name += " " + info.Source.Table
	}
	switch {
	case info.Err != "":
		return m.th.Warning.Render("! " + name + ": " + info.Err), ""
	case !info.Known:
		return m.th.Muted.Render(sourceMark+" Opening ") + m.th.Key.Render(name) + m.th.Muted.Render("…"), ""
	}
	left := m.th.Muted.Render(sourceMark+" ") + m.th.Key.Render(name) + m.th.Muted.Render("  "+m.sourceCounts(info))
	return left, m.th.KeyHints("Ctrl+↓", "last row", "Alt+D", "sort, filter")
}

// sourceMark is the glyph a source's tab shows beside its name and on
// the context line.
const sourceMark = "▦"

// sourceCounts says how many rows the tab shows of the source's, and
// how they're ordered.
func (m *Model) sourceCounts(info sheet.SourceInfo) string {
	total := int64(info.Shape.Rows)
	out := transfer.Thousands(int(total)) + plural(int(min(total, 2)), " row", " rows") + ", " +
		transfer.Thousands(len(info.Shape.Cols)) + plural(len(info.Shape.Cols), " column", " columns")
	o := info.Source.Order
	if o.IsZero() {
		return out
	}
	p := m.src.pages[info.Name]
	switch {
	case p == nil || p.Building():
		return out + "; sorting and filtering…"
	case p.Err() != "":
		return out + "; " + p.Err()
	}
	var parts []string
	for _, s := range o.Sort {
		dir := "A to Z"
		if s.Desc {
			dir = "Z to A"
		}
		parts = append(parts, "sorted by "+m.sourceCol(info, s.Col)+" "+dir)
	}
	for _, f := range o.Filter {
		parts = append(parts, m.sourceCol(info, f.Col)+" "+strings.ToLower(f.Cond.Op.Title())+" "+f.Cond.Arg)
	}
	if len(o.Filter) > 0 {
		if n, ok := p.Rows(); ok {
			out += "; showing " + transfer.Thousands(int(n))
		}
	}
	return out + "; " + strings.Join(parts, ", ")
}

// sourceCol names column col of a source: its letter and name.
func (m *Model) sourceCol(info sheet.SourceInfo, col int) string {
	if col < len(info.Shape.Cols) {
		return info.Shape.Cols[col]
	}
	return sheet.ColName(col)
}

// sourceReadyKey is a key on a source tab: the view's, or else the
// UI's (menus, other sheets, commands that read the source).
func (m *Model) sourceReadyKey(v *srcview.View, k tea.KeyPressMsg) tea.Cmd {
	if v.Key(k) {
		return nil
	}
	key := k.String()
	if i := barMenuFor(key); i >= 0 {
		m.showBarMenu(i)
		return nil
	}
	if id, ok := keymap[canonicalKey(key)]; ok && commands[id].available(m) {
		return m.runCommand(id)
	}
	cmd, _ := m.runShortcut(key)
	return cmd
}

// sourceMouse takes a click, a drag of the scrollbar or the wheel over
// the source's rows.
func (m *Model) sourceMouse(msg tea.MouseMsg) (tea.Cmd, bool) {
	v := m.srcView()
	mouse := msg.Mouse()
	if v == nil || m.mode != modeReady || m.overlay != nil || mouse.Y < headerLine || mouse.Y >= m.height-1 {
		return nil, false
	}
	hit := v.HitAt(mouse.X, mouse.Y-headerLine)
	switch msg := msg.(type) {
	case tea.MouseClickMsg:
		switch {
		case msg.Button != tea.MouseLeft:
		case hit.Bar:
			m.src.dragBar = true
			v.ScrollBar(hit.BarAt, hit.BarLines)
		case hit.Cell:
			v.MoveTo(hit.Row, hit.Col)
		}
	case tea.MouseMotionMsg:
		if m.src.dragBar {
			v.DragBar(mouse.Y - headerLine)
		}
	case tea.MouseReleaseMsg:
		m.src.dragBar = false
	case tea.MouseWheelMsg:
		switch msg.Button {
		case tea.MouseWheelUp:
			v.Scroll(-3)
		case tea.MouseWheelDown:
			v.Scroll(3)
		}
	}
	return nil, true
}
