package ui

import (
	"strings"
	"testing"

	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/ui/theme"
)

// Every state reads without color (docs/contributing/ux.md#visual-rules):
// drawn with a monochrome theme, where every role has the same color,
// each state still differs from a plain cell in its text or attributes.

// attrs are the attributes a terminal cell is drawn with, colors aside.
type attrs struct {
	bold, faint, italic, reverse, strike bool
	underline                            string // "" none, or the SGR 4 sub-parameter: 1 single, 3 curly, 4 dotted, 5 dashed
}

// monoCell is one terminal cell: its text and attributes.
type monoCell struct {
	text string
	attrs
}

// monoCells splits a rendered line into cells, reading SGR attributes
// and skipping colors and other escape sequences.
func monoCells(line string) []monoCell {
	var out []monoCell
	var a attrs
	for i := 0; i < len(line); {
		switch {
		case strings.HasPrefix(line[i:], "\x1b["):
			j := i + 2
			for j < len(line) && (line[j] < 0x40 || line[j] > 0x7e) {
				j++
			}
			if j < len(line) && line[j] == 'm' {
				sgr(line[i+2:j], &a)
			}
			i = j + 1
		case strings.HasPrefix(line[i:], "\x1b]"): // OSC, e.g. a hyperlink, up to ST or BEL
			end := strings.Index(line[i:], "\x1b\\")
			bel := strings.IndexByte(line[i:], '\a')
			switch {
			case bel >= 0 && (end < 0 || bel < end):
				i += bel + 1
			case end >= 0:
				i += end + 2
			default:
				i = len(line)
			}
		default:
			r := []rune(line[i:])[0]
			out = append(out, monoCell{text: string(r), attrs: a})
			i += len(string(r))
		}
	}
	return out
}

// sgr applies one SGR sequence's parameters to a.
func sgr(params string, a *attrs) {
	fields := strings.Split(params, ";")
	for i := 0; i < len(fields); i++ {
		p, sub, _ := strings.Cut(fields[i], ":")
		switch p {
		case "", "0":
			*a = attrs{}
		case "1":
			a.bold = true
		case "2":
			a.faint = true
		case "3":
			a.italic = true
		case "4":
			a.underline = sub
			if sub == "" {
				a.underline = "1"
			} else if sub == "0" {
				a.underline = ""
			}
		case "7":
			a.reverse = true
		case "9":
			a.strike = true
		case "22":
			a.bold, a.faint = false, false
		case "23":
			a.italic = false
		case "24":
			a.underline = ""
		case "27":
			a.reverse = false
		case "29":
			a.strike = false
		case "38", "48", "58":
			if sub == "" && i+1 < len(fields) {
				switch fields[i+1] {
				case "5":
					i += 2
				case "2":
					i += 4
				}
			}
		}
	}
}

// monoLine is screen line y drawn in monochrome, as cells.
func monoLine(m *Model, y int) []monoCell {
	m.th = theme.Monochrome(m.th)
	return monoCells(strings.Split(m.View().Content, "\n")[y])
}

// cellAt is the terminal cells of sheet cell a (in the first screen of
// an unscrolled sheet).
func cellAt(m *Model, a sheet.Addr) []monoCell {
	x := m.hdrW() + a.Col*sheet.DefaultWidth
	cells := monoLine(m, gridTop+a.Row)
	return cells[x : x+sheet.DefaultWidth]
}

func cellText(cs []monoCell) string {
	var b strings.Builder
	for _, c := range cs {
		b.WriteString(c.text)
	}
	return b.String()
}

// every reports whether f holds for every cell of cs; with text, only for
// those that aren't blank.
func every(cs []monoCell, text bool, f func(monoCell) bool) bool {
	for _, c := range cs {
		if text && strings.TrimSpace(c.text) == "" {
			continue
		}
		if !f(c) {
			return false
		}
	}
	return true
}

func isReverse(c monoCell) bool { return c.reverse }
func plain(c monoCell) bool     { return c.attrs == attrs{} }

func TestMonochromePointer(t *testing.T) {
	m := newModel()
	m.sheet.Set(addr("B1"), "text")
	if p := cellAt(m, addr("A1")); !every(p, false, isReverse) {
		t.Errorf("the pointer isn't in reverse video: %+v", p)
	}
	if c := cellAt(m, addr("B1")); !every(c, false, plain) {
		t.Errorf("a plain cell has attributes: %+v", c)
	}
}

func TestMonochromeSelection(t *testing.T) {
	m := newModel()
	press(t, m, "<shift+right>", "<shift+down>")
	for _, a := range []string{"A1", "B1", "A2", "B2"} {
		if c := cellAt(m, addr(a)); !every(c, false, isReverse) {
			t.Errorf("%s in the selection isn't in reverse video", a)
		}
	}
	if c := cellAt(m, addr("C3")); !every(c, false, plain) {
		t.Error("a cell outside the selection has attributes")
	}
	// Column headers: the active cell's (the selection's anchor, A1)
	// bold and reversed, another selected one reversed, the rest plain.
	hdr := monoLine(m, headerLine)
	x := m.hdrW()
	col := func(c int) []monoCell { return hdr[x+c*sheet.DefaultWidth : x+(c+1)*sheet.DefaultWidth] }
	if !every(col(0), true, func(c monoCell) bool { return c.reverse && c.bold }) {
		t.Errorf("the active column's header: %+v", col(0))
	}
	if !every(col(1), true, func(c monoCell) bool { return c.reverse && !c.bold }) {
		t.Errorf("a selected column's header: %+v", col(1))
	}
	if every(col(2), true, isReverse) {
		t.Error("an unselected column's header is reversed")
	}
	if !strings.Contains(cellText(monoLine(m, formulaLine)), "A1:B2") {
		t.Error("the name box doesn't name the selection")
	}
}

func TestMonochromeError(t *testing.T) {
	m := newModel()
	m.sheet.Set(addr("B1"), "=1/0")
	c := cellAt(m, addr("B1"))
	if !strings.Contains(cellText(c), "#DIV/0!") || !every(c, true, func(c monoCell) bool { return c.underline == "3" }) {
		t.Errorf("an error cell: %q %+v", cellText(c), c)
	}
}

func TestMonochromeInvalid(t *testing.T) {
	m := newModel()
	mustValidate(t, m, `{"ranges":"B1:B9","criteria":"number","condition":"between","values":["1","10"]}`)
	m.sheet.Set(addr("B1"), "42")
	c := cellAt(m, addr("B1"))
	if !every(c, true, func(c monoCell) bool { return c.underline == "4" }) {
		t.Errorf("an invalid cell isn't dotted: %+v", c)
	}
	m.cur = addr("B1")
	if l := cellText(monoLine(m, contextLine)); !strings.Contains(l, "Invalid:") {
		t.Errorf("no warning in words: %q", l)
	}
}

func TestMonochromeCircular(t *testing.T) {
	m := newModel()
	m.sheet.Set(addr("A1"), "=A1+1")
	if l := cellText(monoLine(m, m.height-1)); !strings.Contains(l, "Circular reference") {
		t.Errorf("status line: %q", l)
	}
}

func TestMonochromeSpill(t *testing.T) {
	m := newModel()
	m.sheet.Set(addr("B1"), "=SEQUENCE(3)")
	m.cur = addr("D1")
	c := cellAt(m, addr("B2"))
	if strings.TrimSpace(cellText(c)) != "2" || !every(c, true, func(c monoCell) bool { return c.italic }) {
		t.Errorf("a spilled value isn't italic: %+v", c)
	}
	m.cur = addr("B2")
	if l := cellText(monoLine(m, contextLine)); !strings.Contains(l, "Spilled from B1") {
		t.Errorf("context line: %q", l)
	}
}

func TestMonochromeNote(t *testing.T) {
	m := newModel()
	m.sheet.Set(addr("B1"), "x")
	m.sheet.SetNote(addr("B1"), "Checked")
	if c := cellAt(m, addr("B1")); c[len(c)-1].text != "▝" {
		t.Errorf("no note mark: %q", cellText(c))
	}
}

func TestMonochromeFoundTraced(t *testing.T) {
	m := traceModel(t)
	m.cur = addr("B1")
	press(t, m, "<alt+,>")
	if c := cellAt(m, addr("A2")); !every(c, false, isReverse) {
		t.Errorf("a traced cell isn't reversed: %+v", c)
	}
	m = newModel()
	m.sheet.Set(addr("B2"), "Rent")
	press(t, m, "<ctrl+f>", "Rent")
	if c := cellAt(m, addr("B2")); !every(c, false, isReverse) {
		t.Errorf("a match isn't reversed: %+v", c)
	}
}

func TestMonochromeCopied(t *testing.T) {
	m := newModel()
	m.sheet.Set(addr("A1"), "1")
	press(t, m, "<ctrl+c>", "<right>")
	c := cellAt(m, addr("A1"))
	if !every(c, false, func(c monoCell) bool { return c.underline != "" }) || !every(c, true, func(c monoCell) bool { return c.underline == "5" }) {
		t.Errorf("the copied range isn't underlined, dashed under its text: %+v", c)
	}
}

// States told in words: the mode indicator, recording, protected ranges
// and pivot tables.
func TestMonochromeStatesInWords(t *testing.T) {
	m := newModel()
	if l := cellText(monoLine(m, menuLine)); !strings.Contains(l, "READY") {
		t.Errorf("indicator: %q", l)
	}
	press(t, m, "1")
	if l := cellText(monoLine(m, menuLine)); !strings.Contains(l, "ENTER") {
		t.Errorf("indicator while typing: %q", l)
	}
	press(t, m, "<esc>")
	run(m, m.runCommand("macro.record"))
	if l := cellText(monoLine(m, menuLine)); !strings.Contains(l, "REC") {
		t.Errorf("recording: %q", l)
	}

	m = newModel()
	press(t, m, "<shift+down>")
	m.runCommand("data.protect_range")
	press(t, m, "<enter>", "<esc>", "7")
	if l := cellText(monoLine(m, contextLine)); !strings.Contains(l, "is protected") {
		t.Errorf("protected: %q", l)
	}

	m = wideModel()
	salesTable(t, m)
	m.runCommand("data.pivot")
	press(t, m, "<space>", "reg", "<enter>", "<enter>", "<ctrl+home>", "x")
	if l := cellText(monoLine(m, contextLine)); !strings.Contains(l, "Pivot table results can't be edited") {
		t.Errorf("pivot: %q", l)
	}
}

// Wrapping, borders and merged cells are drawn in characters: they read
// in monochrome as they do in color, and the pointer on a merged cell
// reverses all of it.
func TestMonochromeLayout(t *testing.T) {
	m := newModel()
	m.sheet.Set(addr("A2"), "one two three")
	m.sheet.SetStyle(rect("A2"), func(st *sheet.Style) { st.Wrap = sheet.WrapOn })
	m.sheet.SetBorders(rect("A2:B2"), sheet.BorderAll, sheet.LineThin)
	m.sheet.Set(addr("C1"), "merged")
	m.sheet.Merge(rect("C1:D1"), sheet.MergeAll)
	m.cur = addr("C1")
	y, _ := m.rowY(1)
	x := m.hdrW()
	rule := cellText(monoLine(m, y-1)[x : x+2*sheet.DefaultWidth+1])
	if rule != "┌─────────┬─────────┐" {
		t.Errorf("the rule above row 2: %q", rule)
	}
	var text []string
	for k := range m.shape(1).Lines {
		text = append(text, strings.TrimSpace(cellText(monoLine(m, y+k)[x:x+sheet.DefaultWidth])))
	}
	if strings.Join(text, "|") != "│one two|│three" {
		t.Errorf("wrapped text: %q", text)
	}
	merged := monoLine(m, gridTop)[x+2*sheet.DefaultWidth : x+4*sheet.DefaultWidth]
	if !every(merged, false, isReverse) || !strings.Contains(cellText(merged), "merged") {
		t.Errorf("the pointer on a merged cell: %q %+v", cellText(merged), merged)
	}
}
