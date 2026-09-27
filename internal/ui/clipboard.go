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
	cut    bool // pasting moves the cells rather than copying them
	marked bool // the source shows the copy marker
	keep   bool // the last edit was a paste, which keeps the marker
}

func init() {
	register(
		&command{id: "edit.copy", title: "Copy", desc: "Copy the selection", run: func(m *Model) tea.Cmd { return m.copy(false) }},
		&command{id: "edit.cut", title: "Cut", desc: "Cut the selection, to move it where you paste", run: func(m *Model) tea.Cmd { return m.copy(true) }},
		&command{id: "edit.paste", title: "Paste", desc: "Paste into the selection, adjusting references", run: func(m *Model) tea.Cmd { return m.paste(false) }},
		&command{id: "edit.paste_values", title: "Paste values only", desc: "Paste the copied values without formulas", run: func(m *Model) tea.Cmd { return m.paste(true) }},
	)
	keymap["ctrl+c"] = "edit.copy"
	keymap["ctrl+x"] = "edit.cut"
	keymap["ctrl+v"] = "edit.paste"
	keymap["ctrl+shift+v"] = "edit.paste_values"
}

// copy puts the selection on the clipboard and the system clipboard.
func (m *Model) copy(cut bool) tea.Cmd {
	clip := m.sheet.Copy(m.copyRange())
	m.copied = clipboard{clip: clip, cut: cut, marked: true}
	return tea.SetClipboard(formatTSV(clip.Text()))
}

// copyRange is the selection, trimmed to the data when whole rows or
// columns are selected, as there's no point copying thousands of blanks.
func (m *Model) copyRange() sheet.Rect {
	r := m.selection()
	if m.whole == wholeNone {
		return r
	}
	used, ok := m.sheet.UsedRange()
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
		r, err = m.sheet.Move(c.clip.Src, m.cur)
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
		m.note = "Moved " + c.clip.Src.String() + " to " + r.String()
	case values:
		m.note = "Pasted values into " + countCells(r) + " at " + r.String()
	default:
		m.note = "Pasted " + countCells(r) + " at " + r.String()
	}
	return nil
}

// copyMarked reports whether a shows the copy marker.
func (m *Model) copyMarked(a sheet.Addr) bool {
	return m.copied.marked && m.copied.clip.Src.Contains(a)
}

// clearCopyMark hides the copy marker; a pending cut is cancelled, as in
// Sheets, since the cells it would move may have changed.
func (m *Model) clearCopyMark() {
	if m.copied.cut {
		m.copied = clipboard{}
	}
	m.copied.marked = false
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
	if !r.To.Valid() {
		m.fail(sheet.ErrPasteEdge.Error())
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
	case m.note != "":
		return m.note
	case m.copied.marked && m.copied.cut:
		return "Cut " + m.copied.clip.Src.String() + "   " + m.keyHints("Ctrl+V", "move here", "Esc", "cancel")
	case m.copied.marked:
		text := "Copied " + m.copied.clip.Src.String() + "   "
		full := text + m.keyHints("Ctrl+V", "paste", "Ctrl+Shift+V", "paste values", "Esc", "clear")
		if ansi.StringWidth(full) <= m.width {
			return full
		}
		return text + m.keyHints("Ctrl+V", "paste", "Esc", "clear")
	}
	return ""
}
