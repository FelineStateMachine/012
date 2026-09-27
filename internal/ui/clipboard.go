package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// Copy, cut and paste follow Sheets: Ctrl+C copies the selection and
// marks it until Esc or the next edit, Ctrl+V pastes with references
// adjusted, Ctrl+X then Ctrl+V moves. The copied values also go to the
// system clipboard as TSV (OSC 52), and a multi-cell paste from the
// terminal fills a block of cells.

// clipboard is what Ctrl+V pastes.
type clipboard struct {
	clip   *sheet.Clip
	sheet  *sheet.Sheet // where the clip was copied from
	cut    bool         // pasting moves the cells rather than copying them
	marked bool         // the source shows the copy marker
	keep   bool         // the last edit was a paste, which keeps the marker
}

func init() {
	register(
		&command{id: "edit.copy", title: "Copy", desc: "Copy the selection", run: func(m *Model) tea.Cmd { return m.copy(false) }},
		&command{id: "edit.cut", title: "Cut", desc: "Cut the selection, to move it where you paste", edits: (*Model).selection, run: func(m *Model) tea.Cmd { return m.copy(true) }},
		&command{id: "edit.paste", title: "Paste", desc: "Paste into the selection, adjusting references", edits: (*Model).pasteTarget, run: func(m *Model) tea.Cmd { return m.paste(false) }},
		&command{id: "edit.paste_values", title: "Paste values only", desc: "Paste the copied values without formulas", edits: (*Model).pasteTarget, run: func(m *Model) tea.Cmd { return m.paste(true) }},
	)
	keymap["ctrl+c"] = "edit.copy"
	keymap["ctrl+x"] = "edit.cut"
	keymap["ctrl+v"] = "edit.paste"
	keymap["ctrl+shift+v"] = "edit.paste_values"
}

// copy puts the selection on the clipboard and the system clipboard.
func (m *Model) copy(cut bool) tea.Cmd {
	clip := m.sheet.Copy(m.copyRange())
	m.copied = clipboard{clip: clip, sheet: m.sheet, cut: cut, marked: true}
	text := clip.Text()
	if text == nil {
		m.note = "Copied; too many cells for the system clipboard, but Ctrl+V pastes them here"
		return nil
	}
	return tea.SetClipboard(formatTSV(text))
}

// copyRange is the selection, trimmed to the data when whole rows or
// columns are selected, as there's no point copying thousands of blanks.
func (g *grid) copyRange() sheet.Rect {
	r := g.selection()
	if g.whole == wholeNone {
		return r
	}
	used, ok := g.sheet.UsedRange()
	r.To = sheet.Addr{Col: min(r.To.Col, used.To.Col), Row: min(r.To.Row, used.To.Row)}
	if !ok || r.To.Col < r.From.Col || r.To.Row < r.From.Row {
		r.To = r.From
	}
	return r
}

// paste pastes the clipboard into the selection and selects what it
// wrote.
func (m *Model) paste(values bool) tea.Cmd {
	c := m.copied
	if c.clip == nil {
		m.note = "Nothing to paste: copy with Ctrl+C first"
		return nil
	}
	var r sheet.Rect
	var err error
	if c.cut && !values {
		if !c.sheet.Live() {
			m.copied = clipboard{}
			m.note = "Nothing to move: the cut cells' sheet was deleted"
			return nil
		}
		r, err = c.sheet.MoveTo(m.sheet, c.clip.Src, m.cur)
		m.copied = clipboard{}
	} else {
		r, err = m.sheet.Paste(c.clip, m.selection(), values)
		m.copied.keep = true
	}
	if err != nil {
		m.fail(err.Error())
		return nil
	}
	m.selectRect(r)
	switch {
	case c.cut && !values:
		m.note = "Moved " + c.label(m.sheet) + " to " + r.String()
	case values:
		m.note = "Pasted values into " + countCells(r) + " at " + r.String()
	default:
		m.note = "Pasted " + countCells(r) + " at " + r.String()
	}
	return nil
}

// marks reports whether a, on the sheet shown, shows the copy marker.
func (c *clipboard) marks(shown *sheet.Sheet, a sheet.Addr) bool {
	return c.marked && c.sheet == shown && c.clip.Src.Contains(a)
}

// label names the copied range, with its sheet when that isn't the
// one shown: A1:B3, or Sheet1!A1:B3.
func (c *clipboard) label(shown *sheet.Sheet) string {
	if c.sheet != shown {
		return sheet.Qualified(c.sheet.Name(), c.clip.Src)
	}
	return c.clip.Src.String()
}

// clearMark hides the copy marker; a pending cut is cancelled, as in
// Sheets, since the cells it would move may have changed.
func (c *clipboard) clearMark() {
	if c.cut {
		*c = clipboard{}
	}
	c.marked = false
}

// pasteText fills a block of cells from pasted text with tabs or line
// breaks, such as cells copied from another spreadsheet, and reports
// whether it did. Each value is entered as if typed.
func (m *Model) pasteText(content string) bool {
	content = strings.TrimSuffix(strings.TrimSuffix(content, "\n"), "\r")
	if !strings.ContainsAny(content, "\t\n") {
		return false
	}
	rows := parseTSV(content)
	width := 0
	for _, row := range rows {
		width = max(width, len(row))
	}
	r := sheet.Rect{From: m.cur, To: sheet.Addr{Col: m.cur.Col + width - 1, Row: m.cur.Row + len(rows) - 1}}
	if width*len(rows) > sheet.MaxCells() {
		m.fail(sheet.ErrFillTooBig.Error())
		return true
	}
	if !r.To.Valid() {
		m.fail(sheet.ErrPasteEdge.Error())
		return true
	}
	if m.refusePivot(r) {
		return true
	}
	m.sheet.Batch(sheet.Change{Label: "paste into " + r.String(), Focus: r}, func() error {
		for i, row := range rows {
			for j := range width {
				var v string
				if j < len(row) {
					v = row[j]
				}
				a := sheet.Addr{Col: r.From.Col + j, Row: r.From.Row + i}
				if m.sheet.Set(a, v) != nil {
					m.sheet.Set(a, "'"+v) // a broken formula stays as text
				}
			}
		}
		return nil
	})
	m.selectRect(r)
	m.note = "Pasted " + countCells(r) + " at " + r.String()
	return true
}

// formatTSV writes rows as tab-separated values the way Sheets puts them
// on the clipboard: fields with tabs, line breaks or a leading quote are
// quoted, with quotes doubled.
func formatTSV(rows [][]string) string {
	var b strings.Builder
	for i, row := range rows {
		if i > 0 {
			b.WriteByte('\n')
		}
		for j, f := range row {
			if j > 0 {
				b.WriteByte('\t')
			}
			if strings.ContainsAny(f, "\t\n\r") || strings.HasPrefix(f, `"`) {
				f = `"` + strings.ReplaceAll(f, `"`, `""`) + `"`
			}
			b.WriteString(f)
		}
	}
	return b.String()
}

// parseTSV reads tab-separated rows, keeping blank lines and fields, and
// unquoting fields written by formatTSV (or Sheets and Excel).
func parseTSV(s string) [][]string {
	var rows [][]string
	var row []string
	var field strings.Builder
	quoted, fieldStart := false, true
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case quoted && c == '"' && i+1 < len(s) && s[i+1] == '"':
			field.WriteByte('"')
			i++
		case quoted && c == '"':
			quoted = false
		case quoted:
			field.WriteByte(c)
		case c == '"' && fieldStart:
			quoted = true
		case c == '\t':
			row = append(row, field.String())
			field.Reset()
			fieldStart = true
			continue
		case c == '\n':
			row = append(row, strings.TrimSuffix(field.String(), "\r"))
			rows = append(rows, row)
			row, fieldStart = nil, true
			field.Reset()
			continue
		default:
			field.WriteByte(c)
		}
		fieldStart = false
	}
	return append(rows, append(row, strings.TrimSuffix(field.String(), "\r")))
}

// readyLine is the context line in READY mode: feedback on the last
// action, or what to do with a copied range.
func (m *Model) readyLine() string {
	switch {
	case m.warn != "":
		return m.th.Warning.Render(m.warn)
	case m.note != "":
		return m.note
	case m.copied.marked && m.copied.cut:
		return "Cut " + m.copied.label(m.sheet) + "   " + m.th.KeyHints("Ctrl+V", "move here", "Esc", "cancel")
	case m.copied.marked:
		text := "Copied " + m.copied.label(m.sheet) + "   "
		full := text + m.th.KeyHints("Ctrl+V", "paste", "Ctrl+Shift+V", "paste values", "Esc", "clear")
		if ansi.StringWidth(full) <= m.width {
			return full
		}
		return text + m.th.KeyHints("Ctrl+V", "paste", "Esc", "clear")
	}
	return ""
}
