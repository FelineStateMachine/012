package rules

import (
	"image/color"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/ui/lineedit"
	"github.com/FelineStateMachine/012/internal/ui/overlay"
	"github.com/FelineStateMachine/012/internal/ui/theme"
)

// fakeHost is a sheet and an edit line, recording what the panel asks.
type fakeHost struct {
	th     theme.Theme
	line   lineedit.Line
	s      *sheet.Sheet
	sel    sheet.Rect
	closed bool
	edits  int
	saves  []string
}

func newHost() *fakeHost {
	s := sheet.New()
	for i, v := range []string{"5", "150", "20"} {
		s.Set(sheet.Addr{Row: i}, v)
	}
	return &fakeHost{th: theme.New(true), s: s, sel: sheet.Rect{To: sheet.Addr{Row: 2}}}
}

func (h *fakeHost) Theme() *theme.Theme    { return &h.th }
func (h *fakeHost) Size() (int, int)       { return 100, 30 }
func (h *fakeHost) Line() *lineedit.Line   { return &h.line }
func (h *fakeHost) Close()                 { h.closed = true }
func (h *fakeHost) Sheet() *sheet.Sheet    { return h.s }
func (h *fakeHost) Selection() sheet.Rect  { return h.sel }
func (h *fakeHost) Reworked()              { h.edits++ }
func (h *fakeHost) Slot(i int) color.Color { return color.Gray{Y: uint8(i * 16)} }
func (h *fakeHost) SaveFormat(i int, f sheet.CondFormat) error {
	h.saves = append(h.saves, f.JSON())
	if i < 0 {
		return h.s.AddCondFormat(f)
	}
	return h.s.SetCondFormat(i, f)
}
func (h *fakeHost) SaveValidation(i int, v sheet.Validation) error {
	h.saves = append(h.saves, v.JSON())
	if i < 0 {
		return h.s.AddValidation(v)
	}
	return h.s.SetValidation(i, v)
}

var keys = map[string]tea.Key{"enter": {Code: tea.KeyEnter}, "esc": {Code: tea.KeyEscape},
	"up": {Code: tea.KeyUp}, "down": {Code: tea.KeyDown}, "left": {Code: tea.KeyLeft},
	"right": {Code: tea.KeyRight}, "space": {Code: tea.KeySpace}, "delete": {Code: tea.KeyDelete},
	"backspace": {Code: tea.KeyBackspace}}

// press sends named keys in angle brackets, and types other text.
func press(e *Editor, ks ...string) {
	for _, k := range ks {
		if name, ok := strings.CutPrefix(k, "<"); ok {
			name = strings.TrimSuffix(name, ">")
			key := keys[strings.TrimPrefix(name, "shift+")]
			if strings.HasPrefix(name, "shift+") {
				key.Mod = tea.ModShift
			}
			e.Key(tea.KeyPressMsg(key))
			continue
		}
		for _, r := range k {
			e.Key(tea.KeyPressMsg{Code: r, Text: string(r)})
		}
	}
}

func text(e *Editor) string {
	var b strings.Builder
	for _, box := range e.Layout() {
		for _, l := range box.Lines {
			b.WriteString(ansi.Strip(l) + "\n")
		}
	}
	return b.String()
}

func TestAddAndEditFormat(t *testing.T) {
	h := newHost()
	e := Formats(h)
	if out := text(e); !strings.Contains(out, "Conditional format rules") || !strings.Contains(out, "+ Add rule") || !strings.Contains(out, "0 rules") {
		t.Fatalf("list:\n%s", out)
	}
	press(e, "<enter>") // add
	out := text(e)
	for _, want := range []string{"Add conditional format", "Apply to    A1:A3", "‹ Single color ›", "‹ Is not empty ›", "‹ Green ›", "[ ] Bold"} {
		if !strings.Contains(out, want) {
			t.Errorf("form lacks %q:\n%s", want, out)
		}
	}
	// Condition: Greater than is ten steps on; a value row appears.
	press(e, "<down>", "<down>")
	for range 9 {
		press(e, "<right>")
	}
	if !strings.Contains(text(e), "‹ Greater than ›") {
		t.Fatalf("condition:\n%s", text(e))
	}
	press(e, "<down>", "100", "<down>", "<right>") // text color red
	press(e, "<down>", "<down>", "<space>")        // bold
	press(e, "<enter>")
	fs := h.s.CondFormats()
	if len(fs) != 1 || fs[0].Op != sheet.RuleGreater || fs[0].Args[0] != "100" || fs[0].Style.Text != sheet.ColorRed || !fs[0].Style.Bold {
		t.Fatalf("saved %+v", fs)
	}
	if !h.s.Look(sheet.Addr{Row: 1}).Styled || h.s.Look(sheet.Addr{}).Styled {
		t.Error("the rule doesn't apply")
	}
	if out := text(e); !strings.Contains(out, "A1:A3  Greater than 100") || !strings.Contains(out, "1 rule") {
		t.Errorf("list after save:\n%s", out)
	}
	// Editing it: a bad value keeps the form open with why.
	press(e, "<down>", "<enter>", "<down>", "<down>", "<down>")
	h.line.Set("")
	e.Changed()
	press(e, "<enter>")
	if e.msg != "Enter a value" || e.f == nil {
		t.Errorf("msg %q", e.msg)
	}
	desc, _ := e.Status()
	if !strings.Contains(desc, "Enter a value") {
		t.Errorf("status %q", desc)
	}
	press(e, "<esc>") // back to the list, unchanged
	if e.f != nil || h.s.CondFormats()[0].Args[0] != "100" {
		t.Error("Esc didn't discard the form")
	}
	press(e, "<esc>")
	if !h.closed {
		t.Error("Esc on the list didn't close")
	}
}

func TestColorScaleForm(t *testing.T) {
	h := newHost()
	e := Formats(h)
	press(e, "<enter>", "<down>", "<right>") // Color scale
	out := text(e)
	for _, want := range []string{"Minpoint    ‹ Min value ›", "Midpoint    ‹ None ›", "Maxpoint    ‹ Max value ›", "‹ Red ›", "‹ Green ›", "Preview"} {
		if !strings.Contains(out, want) {
			t.Errorf("scale form lacks %q:\n%s", want, out)
		}
	}
	// A percentile midpoint in yellow.
	press(e, "<down>", "<down>", "<down>", "<left>")
	if !strings.Contains(text(e), "‹ Percentile ›") {
		t.Fatalf("midpoint:\n%s", text(e))
	}
	press(e, "<enter>")
	f := h.s.CondFormats()[0]
	if len(f.Scale) != 3 || f.Scale[1].Kind != sheet.PointPercentile || f.Scale[1].Value != "50" || f.Scale[1].Color != sheet.ColorYellow {
		t.Fatalf("scale %+v", f.Scale)
	}
	if l := h.s.Look(sheet.Addr{Row: 2}); !l.Scaled || l.To != sheet.ColorYellow || l.Pos != 1 {
		t.Errorf("look %+v", l)
	}
}

func TestRemoveAndReorder(t *testing.T) {
	h := newHost()
	for _, v := range []string{"1", "2"} {
		h.s.AddCondFormat(sheet.CondFormat{Ranges: []sheet.Rect{h.sel}, Op: sheet.RuleEqual, Args: [2]string{v}, Style: sheet.RuleStyle{Bold: true}})
	}
	e := Formats(h)
	press(e, "<down>", "<shift+down>")
	if fs := h.s.CondFormats(); fs[1].Args[0] != "1" || e.list.Sel != 2 || h.edits != 1 {
		t.Errorf("reorder: %+v sel %d", fs, e.list.Sel)
	}
	press(e, "<delete>")
	if fs := h.s.CondFormats(); len(fs) != 1 || fs[0].Args[0] != "2" || e.list.Sel != 1 {
		t.Errorf("remove: %+v sel %d", fs, e.list.Sel)
	}
}

func TestValidationForm(t *testing.T) {
	h := newHost()
	e := Validations(h).Add(true)
	out := text(e)
	for _, want := range []string{"Add data validation", "‹ Dropdown ›", "Items", "‹ Show a warning ›", "Help text"} {
		if !strings.Contains(out, want) {
			t.Errorf("form lacks %q:\n%s", want, out)
		}
	}
	press(e, "<down>", "<down>", "Yes, No,, Maybe ", "<down>", "<down>", "<right>", "<enter>")
	vs := h.s.Validations()
	if len(vs) != 1 || strings.Join(vs[0].Items, "|") != "Yes|No|Maybe" || !vs[0].Reject || vs[0].Display != sheet.DropChip {
		t.Fatalf("saved %+v", vs)
	}
	// A number between, rejecting.
	press(e, "<down>", "<enter>", "<down>")
	for range 3 {
		press(e, "<right>")
	}
	press(e, "<down>", "<down>", "1", "<down>", "10", "<enter>")
	v := h.s.Validations()[0]
	if v.Kind != sheet.ValidNumber || v.Op != sheet.RuleBetween || v.Args != [2]string{"1", "10"} {
		t.Fatalf("number rule %+v", v)
	}
	if out := text(e); !strings.Contains(out, "Number between 1 and 10") || !strings.Contains(out, "rejects") {
		t.Errorf("list:\n%s", out)
	}
	// Dates offer any date first.
	press(e, "<enter>", "<down>", "<right>")
	if !strings.Contains(text(e), "‹ Is a valid date ›") {
		t.Errorf("date:\n%s", text(e))
	}
}

func TestMouseAndCursor(t *testing.T) {
	h := newHost()
	e := Formats(h)
	x, y, _ := e.box()
	// Clicking the highlighted "Add rule" opens the form.
	e.Mouse(overlay.MouseEvent{Kind: overlay.MousePress, Button: tea.MouseLeft, Box: ID, Row: 1})
	if e.f == nil {
		t.Fatal("no form")
	}
	cx, cy := e.Cursor()
	if cx != x+1+textX()+len("A1:A3") || cy != y+1 {
		t.Errorf("cursor %d,%d box %d,%d", cx, cy, x, y)
	}
	// Clicking a choice row picks it; clicking again changes it.
	e.Mouse(overlay.MouseEvent{Kind: overlay.MousePress, Button: tea.MouseLeft, Box: ID, Row: 2})
	e.Mouse(overlay.MouseEvent{Kind: overlay.MousePress, Button: tea.MouseLeft, Box: ID, Row: 2})
	if !strings.Contains(text(e), "‹ Color scale ›") {
		t.Errorf("click:\n%s", text(e))
	}
	if cx, _ := e.Cursor(); cx >= 0 {
		t.Error("a caret on a choice row")
	}
	e.Mouse(overlay.MouseEvent{Kind: overlay.MousePress, Button: tea.MouseLeft})
	if !h.closed {
		t.Error("a click outside didn't close")
	}
}

// The form makes data bars and icon sets: the Format row's third and
// fourth choices, their rows, and a preview drawn as the cells are.
func TestBarAndIconForms(t *testing.T) {
	h := newHost()
	e := Formats(h)
	press(e, "<enter>", "<down>", "<right>", "<right>")
	out := text(e)
	for _, want := range []string{"‹ Data bar ›", "Shortest", "‹ Automatic ›", "Longest", "Bar color", "Show bar only", "Preview"} {
		if !strings.Contains(out, want) {
			t.Errorf("bar form lacks %q:\n%s", want, out)
		}
	}
	press(e, "<enter>")
	fs := h.s.CondFormats()
	if len(fs) != 1 || !fs[0].IsBar() || fs[0].Bar != sheet.ColorBlue || fs[0].Scale[0].Kind != sheet.PointMin {
		t.Fatalf("saved %+v", fs)
	}
	press(e, "<down>", "<enter>", "<down>", "<right>")
	out = text(e)
	for _, want := range []string{"‹ Icon set ›", "‹ Arrows ›", "‹ 3 icons ›", "→ from", "↑ from", "Reverse icons"} {
		if !strings.Contains(out, want) {
			t.Errorf("icon form lacks %q:\n%s", want, out)
		}
	}
	// Four icons, evenly apart.
	press(e, "<down>", "<down>", "<right>", "<enter>")
	f := h.s.CondFormats()[0]
	if !f.IsIcons() || len(f.Scale) != 3 || f.Scale[0].Value != "25" || f.Scale[2].Value != "75" {
		t.Errorf("icons %+v", f)
	}
	if !strings.Contains(text(e), "↓↘↗↑") {
		t.Errorf("list sample:\n%s", text(e))
	}
}
