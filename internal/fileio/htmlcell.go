package fileio

import (
	"fmt"
	"html"
	"strings"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// htmlCell is what a td says about its cell: classes, inline styles,
// attributes and the text shown.
type htmlCell struct {
	class []string
	style []string
	attrs string
	text  string
	icon  string // an icon set's glyph, before the text
}

// writeCell writes the td of the cell at a: spanning a merge, with the
// text as the grid shows it at the cell's width.
func (l *htmlLayout) writeCell(b *strings.Builder, a sheet.Addr) {
	snap := l.snap
	var h htmlCell
	width := htmlColWidth(snap, a.Col)
	if m, ok := l.merged[a]; ok {
		width = 0
		for col := m.From.Col; col <= m.To.Col; col++ {
			width += htmlColWidth(snap, col)
		}
		h.attrs += fmt.Sprintf(` colspan="%d" rowspan="%d"`, m.To.Col-m.From.Col+1, l.visibleRows(m))
	}
	c, filled := snap.Cells[a]
	if filled {
		h.content(snap, c, width)
		h.textStyle(c.Style)
		h.borders(c.Style.Borders)
		if c.Style.Wrap == sheet.WrapOverflow && h.text != "" && l.nextFilled(a) {
			h.class = append(h.class, "clip")
		}
	}
	if look, ok := snap.Looks[a]; ok {
		h.look(look)
	}
	if role := l.tableRole[a]; role != "" {
		h.class = append(h.class, role)
	}
	if note, ok := snap.Notes[a]; ok {
		h.class = append(h.class, "note")
		h.attrs += ` title="` + html.EscapeString(note) + `"`
	}
	fmt.Fprintf(b, `<td data-a="%s"`, a)
	if len(h.class) > 0 {
		b.WriteString(` class="` + strings.Join(h.class, " ") + `"`)
	}
	if len(h.style) > 0 {
		b.WriteString(` style="` + html.EscapeString(strings.Join(h.style, ";")) + `"`)
	}
	b.WriteString(h.attrs + ">")
	if h.icon != "" {
		b.WriteString(h.icon)
	}
	b.WriteString(html.EscapeString(h.text) + "</td>")
}

// visibleRows is how many drawn rows a merge spans.
func (l *htmlLayout) visibleRows(m sheet.Rect) int {
	n := 0
	for row := m.From.Row; row <= m.To.Row; row++ {
		if !l.snap.HiddenRows[row] {
			n++
		}
	}
	return max(n, 1)
}

// nextFilled reports whether the cell right of a holds something, which
// a's text stops at rather than running on, as in Sheets.
func (l *htmlLayout) nextFilled(a sheet.Addr) bool {
	next := sheet.Addr{Col: a.Col + 1, Row: a.Row}
	c, ok := l.snap.Cells[next]
	return ok && (c.Input != "" || c.Value.Kind != sheet.Empty) || l.covered[next]
}

// content is the cell's text as the grid shows it in width columns,
// where it goes, and what was typed when that isn't the text.
func (h *htmlCell) content(snap *Snapshot, c SnapCell, width int) {
	text, align := sheet.DisplayIn(c.Value, c.Format, width, snap.Locale)
	h.text = text
	switch c.Style.Align {
	case sheet.AlignLeft:
		align = sheet.AlignLeft
	case sheet.AlignCenter:
		align = sheet.AlignCenter
	case sheet.AlignRight:
		align = sheet.AlignRight
	}
	switch align {
	case sheet.AlignRight:
		h.class = append(h.class, "r")
	case sheet.AlignCenter:
		h.class = append(h.class, "c")
	}
	if c.Value.Kind == sheet.Error {
		h.class = append(h.class, "err")
	}
	if c.Input != "" && c.Input != text {
		h.attrs += ` data-f="` + html.EscapeString(c.Input) + `"`
	}
	switch c.Style.Wrap {
	case sheet.WrapOn:
		h.class = append(h.class, "wrap")
	case sheet.WrapClip:
		h.class = append(h.class, "clip")
	}
	switch c.Style.VAlign {
	case sheet.VAlignTop:
		h.class = append(h.class, "t")
	case sheet.VAlignMiddle:
		h.class = append(h.class, "m")
	}
}

// textStyle adds bold, italic, underline and strikethrough.
func (h *htmlCell) textStyle(st sheet.Style) {
	for _, f := range []struct {
		on    bool
		class string
	}{{st.Bold, "b"}, {st.Italic, "i"}, {st.Underline, "u"}, {st.Strikethrough, "s"}} {
		if f.on {
			h.class = append(h.class, f.class)
		}
	}
}

// borders draws each edge's line in its color, or the border role's
// when it has none.
func (h *htmlCell) borders(b sheet.Borders) {
	if b.IsZero() {
		return
	}
	for _, e := range []struct {
		edge sheet.Edge
		side string
	}{{sheet.EdgeTop, "top"}, {sheet.EdgeBottom, "bottom"}, {sheet.EdgeLeft, "left"}, {sheet.EdgeRight, "right"}} {
		st := b.Stroke(e.edge)
		line := ""
		switch st.Line {
		case sheet.LineThin:
			line = "1px solid"
		case sheet.LineThick:
			line = "2px solid"
		case sheet.LineDouble:
			line = "3px double"
		default:
			continue
		}
		color := "var(--border)"
		if st.Color != sheet.ColorNone {
			color = ruleVar(st.Color)
		}
		h.style = append(h.style, "border-"+e.side+":"+line+" "+color)
	}
}

func ruleVar(c sheet.Color) string { return fmt.Sprintf("var(--r%d)", int(c)) }
func inkVar(c sheet.Color) string  { return fmt.Sprintf("var(--i%d)", int(c)) }

// look draws what the sheet's rules show: a conditional format's
// colors and text style, a color scale, a data bar, an icon, a
// checkbox, a dropdown's marker and an invalid entry's underline.
func (h *htmlCell) look(l sheet.Look) {
	if l.Styled {
		h.ruleStyle(l.Style)
	}
	if l.Scaled {
		pct := int(l.Pos*100 + 0.5)
		h.style = append(h.style, fmt.Sprintf("background:color-mix(in srgb,%s %d%%,%s)", ruleVar(l.To), pct, ruleVar(l.From)))
		ink := l.From
		if l.Pos >= 0.5 {
			ink = l.To
		}
		h.style = append(h.style, "color:"+inkVar(ink))
	}
	if l.Bar {
		pct := int(l.BarLen*100 + 0.5)
		h.style = append(h.style, fmt.Sprintf("background:linear-gradient(to right,color-mix(in srgb,%s 55%%,transparent) %d%%,transparent %d%%)",
			ruleVar(l.BarColor), pct, pct))
	}
	if l.ValueHidden {
		h.text = ""
	}
	if l.Icon != "" {
		h.icon = fmt.Sprintf(`<span class="icon" style="color:%s">%s</span>`, ruleVar(l.IconColor), html.EscapeString(l.Icon))
	}
	switch {
	case l.Checkbox && l.Checked:
		h.text, h.class = "[✓]", append(h.class, "c")
	case l.Checkbox:
		h.text, h.class = "[ ]", append(h.class, "c")
	case l.Dropdown:
		h.text += " ▾"
	}
	if l.Invalid {
		h.class = append(h.class, "bad")
	}
}

// ruleStyle is a single-color rule's fill, text color and text style.
func (h *htmlCell) ruleStyle(st sheet.RuleStyle) {
	switch {
	case st.Fill != sheet.ColorNone && (st.Text == sheet.ColorNone || st.Text == st.Fill):
		h.style = append(h.style, "background:"+ruleVar(st.Fill), "color:"+inkVar(st.Fill))
	case st.Fill != sheet.ColorNone:
		h.style = append(h.style, "background:"+ruleVar(st.Fill), "color:"+ruleVar(st.Text))
	case st.Text != sheet.ColorNone:
		h.style = append(h.style, "color:"+ruleVar(st.Text))
	}
	h.textStyle(sheet.Style{Bold: st.Bold, Italic: st.Italic, Underline: st.Underline, Strikethrough: st.Strikethrough})
}
