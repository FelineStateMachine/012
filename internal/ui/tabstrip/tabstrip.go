// Package tabstrip is the sheet tabs at the left of the status line, the
// way tmux lists windows: the sheet shown is highlighted, a + adds a
// sheet, and when the tabs don't all fit, ‹ and › step through them. It
// lays the strip out and remembers where each sheet was left; what a
// click on it does is the UI's (showing, renaming, moving a sheet are
// commands). It needs no host: the UI hands it a View.
package tabstrip

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/ui/theme"
)

// Strip is the sheet tabs' state: where each sheet was left, and how far
// the strip is scrolled.
type Strip struct {
	Places map[*sheet.Sheet]Place // where each sheet's cursor and scroll were left
	left   int                    // the first tab shown when they don't all fit
}

// Place is where a sheet's cursor and scroll position were when it was
// last shown.
type Place struct {
	Cur       sheet.Addr
	Top, Left int
}

// Kind is a part of the strip.
type Kind int

const (
	None Kind = iota
	Tab       // a sheet's tab
	Add       // the + after the tabs
	Prev      // the ‹ before tabs scrolled off to the left
	Next      // the › after tabs scrolled off to the right
)

// View is what the strip shows: the sheets, the one shown, and what the
// mouse is over.
type View struct {
	Sheets []*sheet.Sheet
	Active int
	// Hover is the part under the mouse, and HoverIndex the tab for Tab.
	Hover      Kind
	HoverIndex int
	// Busy is set while the mouse drags something other than a tab, so
	// passing over a tab doesn't highlight it.
	Busy bool
}

// Span is a clickable part of the strip, at x on the status line.
type Span struct {
	Kind  Kind
	Index int // the sheet, for Tab
	X, W  int
}

// maxName is how much of a long sheet name a tab shows.
const maxName = 20

// The widths of the strip's fixed parts: " + ", and "‹" or "›" with the
// space after it.
const addW, arrowW = 3, 2

// NotebookMark starts a notebook tab's label, so it reads apart from
// the sheets' without color.
const NotebookMark = "❯ "

// SourceMark starts a linked source's tab's label; ! follows the name
// when its file can't be read.
const SourceMark = "▦ "

// Label is a sheet's name as its tab shows it, a notebook's after its
// mark, with the mark of its linked regions, if it has any.
func Label(s *sheet.Sheet) string {
	label := " " + ansi.Truncate(s.Name(), maxName, "…") + " "
	if s.IsNotebook() {
		label = " " + NotebookMark + ansi.Truncate(s.Name(), maxName, "…") + " "
	}
	if info, ok := s.Source(); ok {
		label = " " + SourceMark + ansi.Truncate(s.Name(), maxName, "…") + " "
		if info.Err != "" {
			label += Mark(false, true) + " "
		}
	}
	if s.HasLinked() {
		label += liveMark(s) + " "
	}
	return label
}

// Mark is the glyph that shows how a linked region follows its file:
// ● following, ‖ paused, ! when the file can't be read. It reads without
// color.
func Mark(paused, failed bool) string {
	switch {
	case failed:
		return "!"
	case paused:
		return "‖"
	}
	return "●"
}

// liveMark is the mark of a sheet's linked regions: a failing one's
// first, then a following one's.
func liveMark(s *sheet.Sheet) string {
	paused := true
	for _, r := range s.LinkedRegions() {
		if r.Err != "" {
			return Mark(false, true)
		}
		paused = paused && r.Paused
	}
	return Mark(paused, false)
}

// MinWidth is the narrowest the strip gets: the shown sheet's tab, the
// arrows and the +.
func (v View) MinWidth() int {
	w := ansi.StringWidth(Label(v.Sheets[v.Active])) + 4
	if len(v.Sheets) > 1 {
		w += 4
	}
	return w
}

// FullWidth is how wide the strip is with every tab showing.
func (v View) FullWidth() int {
	w := addW
	for _, s := range v.Sheets {
		w += ansi.StringWidth(Label(s)) + 1
	}
	return w
}

// style is the style of tab i: the one shown, one under the mouse (or
// where a dragged tab would go), or plain.
func (v View) style(th *theme.Theme, i int) lipgloss.Style {
	switch {
	case i == v.Active:
		return th.TabActive
	case v.Hover == Tab && v.HoverIndex == i && !v.Busy:
		return th.TabHover
	}
	return th.Tab
}

// Layout lays out the tabs in room columns: as many as fit, always the
// shown one, the first shown staying put until the shown one would fall
// off, ‹ and › where tabs are hidden, then the +.
func (t *Strip) Layout(th *theme.Theme, v View, room int) (string, []Span) {
	widths := make([]int, len(v.Sheets))
	for i, s := range v.Sheets {
		widths[i] = ansi.StringWidth(Label(s))
	}
	first, last := t.scroll(widths, v.Active, room)
	var w writer
	if first > 0 {
		w.part(Prev, 0, th.Muted.Render("‹"))
	}
	for i := first; i <= last; i++ {
		label := Label(v.Sheets[i])
		if i == v.Active && w.x+ansi.StringWidth(label)+addW > room {
			label = ansi.Truncate(label, max(room-w.x-addW, 3), "…")
		}
		w.part(Tab, i, v.style(th, i).Render(label))
	}
	if last < len(v.Sheets)-1 {
		w.part(Next, 0, th.Muted.Render("›"))
	}
	addStyle := th.Muted
	if v.Hover == Add {
		addStyle = th.TabHover
	}
	w.part(Add, 0, addStyle.Render(" + "))
	return strings.TrimSuffix(w.b.String(), " "), w.spans
}

// scroll picks the first and last tab shown, keeping the first where it
// was unless the active tab would fall off, and showing tabs to the left
// again when there's room, e.g. once the screen is wider.
func (t *Strip) scroll(widths []int, active, room int) (first, last int) {
	first = max(0, min(t.left, active))
	last, ok := fit(widths, first, active, room)
	for !ok && first < active {
		first++
		last, ok = fit(widths, first, active, room)
	}
	for first > 0 {
		l, ok := fit(widths, first-1, active, room)
		if !ok || l < last {
			break
		}
		first, last = first-1, l
	}
	t.left = first
	return first, max(last, active) // a name too long for the room is cut
}

// fit returns the last tab that fits in room when the strip starts at
// tab first, and whether the active tab is among those shown.
func fit(widths []int, first, active, room int) (last int, ok bool) {
	w := addW
	if first > 0 {
		w += arrowW
	}
	last = first - 1
	for i := first; i < len(widths); i++ {
		more := 0
		if i < len(widths)-1 {
			more = arrowW
		}
		if w+widths[i]+1+more > room {
			break
		}
		w += widths[i] + 1
		last = i
	}
	return last, last >= active
}

// writer writes the parts of the strip, noting where each is.
type writer struct {
	b     strings.Builder
	spans []Span
	x     int
}

func (w *writer) part(kind Kind, index int, text string) {
	width := ansi.StringWidth(text)
	w.spans = append(w.spans, Span{Kind: kind, Index: index, X: w.x, W: width})
	w.b.WriteString(text + " ")
	w.x += width + 1
}
