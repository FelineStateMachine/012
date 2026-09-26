package ui

import (
	"github.com/charmbracelet/x/ansi"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"012/internal/sheet"
)

// cells returns the text of the first n columns of grid row r (1-based),
// each trimmed.
func rowCells(m *Model, r, n int) []string {
	l := line(m, gridTop+r-1)
	out := make([]string, n)
	x := rowHdrW
	for i := range n {
		w := m.sheet.ColWidth(m.left + i)
		seg := ""
		if x < len(l) {
			seg = l[x:min(x+w, len(l))]
		}
		out[i] = strings.TrimSpace(seg)
		x += w
	}
	return out
}

// raw returns the columns of grid row r untrimmed, joined by |.
func raw(m *Model, r, n int) string {
	l := line(m, gridTop+r-1)
	l += strings.Repeat(" ", max(0, 400-len(l)))
	var parts []string
	x := rowHdrW
	for i := range n {
		w := m.sheet.ColWidth(m.left + i)
		parts = append(parts, l[x:x+w])
		x += w
	}
	return strings.Join(parts, "|")
}

func TestFormatShortcutsApplyToSelection(t *testing.T) {
	tests := []struct {
		keys []string
		a, b string
	}{
		{[]string{"<ctrl+shift+1>"}, "1,234.50", "0.25"},
		{[]string{"<ctrl+!>"}, "1,234.50", "0.25"}, // shifted form of Ctrl+Shift+1
		{[]string{"<ctrl+shift+4>"}, "$1,234.50", "$0.25"},
		{[]string{"<ctrl+$>"}, "$1,234.50", "$0.25"},
		{[]string{"<ctrl+shift+5>"}, "123450.00%", "25.00%"},
		{[]string{"<ctrl+shift+6>"}, "1.23E+03", "2.50E-01"},
		{[]string{"<ctrl+shift+3>"}, "5/18/1903", "12/30/1899"},
		{[]string{"<ctrl+shift+2>"}, "12:00:00 PM", "6:00:00 AM"},
	}
	for _, tt := range tests {
		m := newModel()
		m.sheet.SetColWidth(0, 12)
		m.sheet.SetColWidth(1, 12)
		press(t, m, "1234.5", "<tab>", "0.25", "<enter>", "<up>", "<shift+right>")
		press(t, m, tt.keys...)
		if got := rowCells(m, 1, 2); got[0] != tt.a || got[1] != tt.b {
			t.Errorf("%v: row 1 = %q, want %q %q", tt.keys, got, tt.a, tt.b)
		}
	}
}

func TestFormatCommandsCoverSheetsMenu(t *testing.T) {
	m := newModel()
	press(t, m, "-1450", "<enter>", "<up>")
	m.sheet.SetColWidth(0, 12)
	tests := []struct{ id, want string }{
		{"format.accounting", " $(1,450.00)"},
		{"format.financial", " (1,450.00) "},
		{"format.currency_rounded", "    -$1,450 "},
		{"format.duration", "-34800:00:00"}, // uses the padding: nothing to its right
		{"format.plain_text", " -1450      "},
		{"format.automatic", "      -1450 "},
	}
	for _, tt := range tests {
		m.runCommand(tt.id)
		if got := raw(m, 1, 1); got != tt.want {
			t.Errorf("%s: A1 shows %q, want %q", tt.id, got, tt.want)
		}
	}
}

func TestDecimalPlaces(t *testing.T) {
	m := newModel()
	press(t, m, "1.5", "<enter>", "<up>")
	m.runCommand("format.decimals_more")
	m.runCommand("format.decimals_more")
	if got := rowCells(m, 1, 1)[0]; got != "1.500" {
		t.Errorf("after two more: %q", got)
	}
	m.runCommand("format.decimals_less")
	m.runCommand("format.decimals_less")
	m.runCommand("format.decimals_less")
	if got := rowCells(m, 1, 1)[0]; got != "2" {
		t.Errorf("after three less: %q", got)
	}
}

func TestTextStylesToggle(t *testing.T) {
	m := newModel()
	press(t, m, "Rent", "<enter>", "Food", "<enter>", "<up>", "<up>", "<shift+down>")
	style := func(a string) sheet.Style {
		if c := m.sheet.Cell(addr(a)); c != nil {
			return c.Style
		}
		return sheet.Style{}
	}
	press(t, m, "<ctrl+b>")
	if !style("A1").Bold || !style("A2").Bold {
		t.Fatal("Ctrl+B didn't bold the selection")
	}
	if got := line(m, 2); got != "Bold on for A1:A2" {
		t.Errorf("line 3 = %q", got)
	}
	press(t, m, "<ctrl+b>")
	if style("A1").Bold || style("A2").Bold {
		t.Error("second Ctrl+B didn't turn bold off")
	}
	if line(m, 2) != "Bold off for A1:A2" {
		t.Errorf("line 3 = %q", line(m, 2))
	}
	press(t, m, "<ctrl+i>", "<ctrl+u>", "<alt+shift+5>")
	if st := style("A2"); !st.Italic || !st.Underline || !st.Strikethrough {
		t.Errorf("A2 style %+v", st)
	}
	press(t, m, "<ctrl+\\>")
	if m.sheet.Cell(addr("A1")).Style != (sheet.Style{}) || input(m, "A1") != "Rent" {
		t.Error("Ctrl+\\ didn't clear formatting, or cleared contents")
	}
	// The note goes away on the next key.
	press(t, m, "<down>")
	if line(m, 2) != "" {
		t.Errorf("line 3 after moving = %q", line(m, 2))
	}
}

func TestStylesRender(t *testing.T) {
	m := newModel()
	press(t, m, "Total", "<enter>", "<up>", "<ctrl+b>", "<ctrl+u>", "<down>")
	grid := strings.Split(m.View().Content, "\n")[gridTop]
	// Bold and underline wrap just the text, not the padding.
	if !strings.Contains(grid, "m \x1b[1;4") || !strings.Contains(grid, "l\x1b[m      ") {
		t.Errorf("row 1 = %q", grid)
	}
}

func TestAlignment(t *testing.T) {
	m := newModel()
	press(t, m, "<right>", "abc", "<tab>", "42", "<enter>")
	press(t, m, "<up>", "<ctrl+shift+r>", "<right>", "<ctrl+shift+l>")
	if got := raw(m, 1, 3); got != "          |      abc | 42       " {
		t.Errorf("right/left = %q", got)
	}
	press(t, m, "<left>", "<ctrl+shift+e>", "<right>", "<ctrl+shift+e>")
	if got := raw(m, 1, 3); got != "          |   abc    |    42    " {
		t.Errorf("centered = %q", got)
	}
}

func TestOverflowDirections(t *testing.T) {
	m := newModel()
	// Right-aligned text runs left into blank cells; centered text runs
	// both ways; a formatted blank still lets text through.
	press(t, m, "<right>", "<right>", "Right-aligned heading", "<enter>", "<up>", "<ctrl+shift+r>")
	if got := raw(m, 1, 4); got != "        Ri|ght-aligne|d heading |          " {
		t.Errorf("right overflow = %q", got)
	}
	m.sheet.SetFormat(sheet.Rect{From: addr("B1"), To: addr("B1")}, sheet.Preset(sheet.FmtCurrency))
	press(t, m, "<ctrl+shift+e>")
	if got := raw(m, 1, 4); got != "          |     Right|-aligned h|eading    " {
		t.Errorf("centered overflow = %q", got)
	}
	// A filled neighbor stops it.
	press(t, m, "<left>", "x", "<enter>")
	if got := raw(m, 1, 4); got != "          | x        |-aligned h|eading    " {
		t.Errorf("blocked overflow = %q", got)
	}
}

func TestNumbersTooWideShowHashes(t *testing.T) {
	m := newModel()
	press(t, m, "1234567.891", "<enter>", "<up>", "<ctrl+shift+1>")
	if got := raw(m, 1, 1); got != "######### " {
		t.Errorf("A1 = %q", got)
	}
	m.sheet.SetColWidth(0, 14)
	if got := raw(m, 1, 1); got != " 1,234,567.89 " {
		t.Errorf("wider A1 = %q", got)
	}
}

func TestLongDatesUseThePadding(t *testing.T) {
	m := newModel()
	press(t, m, "12/31/2026", "<enter>")
	if got := raw(m, 1, 2); got != "12/31/2026|          " {
		t.Errorf("A1 = %q", got)
	}
	// With a neighbor there's no room, as in Sheets.
	press(t, m, "<up>", "<right>", "x", "<enter>")
	if got := raw(m, 1, 2); got != "######### | x        " {
		t.Errorf("A1 next to B1 = %q", got)
	}
}

func TestTypedDatesAndCurrencyShowFormatted(t *testing.T) {
	m := newModel()
	press(t, m, "9/26/2026", "<enter>", "$1,200", "<enter>", "12.5%", "<enter>", "14:30", "<enter>")
	for i, want := range []string{"9/26/2026", "$1,200", "12.50%", "14:30:00"} {
		if got := rowCells(m, i+1, 1)[0]; got != want {
			t.Errorf("row %d = %q, want %q", i+1, got, want)
		}
	}
	// The formula bar keeps what was typed.
	press(t, m, "<ctrl+home>")
	if l := line(m, formulaLine); !strings.HasPrefix(l, " A1") || !strings.HasSuffix(l, " 9/26/2026") {
		t.Errorf("formula bar %q", l)
	}
}

func TestFormatHelpListsShortcuts(t *testing.T) {
	m := newModel()
	m.Update(teaSize(120, 60))
	press(t, m, "<f1>")
	// Check every row of the shortcuts overlay, not just the visible ones.
	rows, _ := m.overlay.(*shortcuts).lines(m)
	s := ansi.Strip(strings.Join(rows, "\n"))
	for _, want := range []string{"Ctrl+Shift+4", "Ctrl+B", "Alt+Shift+5", "Ctrl+\\"} {
		if !strings.Contains(s, want) {
			t.Errorf("help lacks %s", want)
		}
	}
	if strings.Contains(s, "Ctrl+$") {
		t.Error("help lists key aliases")
	}
}

func teaSize(w, h int) tea.WindowSizeMsg { return tea.WindowSizeMsg{Width: w, Height: h} }
