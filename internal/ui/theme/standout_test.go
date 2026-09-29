package theme

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

// The standout roles are drawn in reverse video with their colors
// swapped, so they look as defined in color; Drawable undoes that where
// lipgloss would draw spaces without reverse video.
func TestStandouts(t *testing.T) {
	th := New(true)
	p := th.Pointer
	if !p.GetReverse() || p.GetForeground() != lipgloss.Cyan || p.GetBackground() != lipgloss.Black {
		t.Errorf("pointer: reverse %v fg %v bg %v", p.GetReverse(), p.GetForeground(), p.GetBackground())
	}
	if !strings.Contains(p.Render("x"), "7") {
		t.Errorf("no SGR 7: %q", p.Render("x"))
	}
	if Drawable(p).Render("x") != p.Render("x") {
		t.Error("Drawable changed a plain standout")
	}
	d := Drawable(p.Underline(true))
	if d.GetReverse() || d.GetForeground() != lipgloss.Black || d.GetBackground() != lipgloss.Cyan {
		t.Errorf("underlined: reverse %v fg %v bg %v", d.GetReverse(), d.GetForeground(), d.GetBackground())
	}
	for _, s := range []lipgloss.Style{th.Selection, th.HeaderSel, th.HeaderActive, th.Found, th.Precedent, th.Dependent, th.MenuSelected} {
		if !s.GetReverse() {
			t.Errorf("%v isn't reversed", s)
		}
	}
	// Monochrome keeps attributes and makes every color one.
	m := Monochrome(th)
	if !m.Pointer.GetReverse() || m.Pointer.GetForeground() != m.Error.GetForeground() || !m.Spilled.GetItalic() {
		t.Error("monochrome lost an attribute or kept a color")
	}
}
