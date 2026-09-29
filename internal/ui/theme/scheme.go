package theme

import (
	"image/color"
	"reflect"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// barBlend is how far the menu and status bars move from the background
// toward the text color: enough to read as a band on any scheme, little
// enough that text on them keeps its contrast.
const barBlend = 0.12

// FromPalette draws the roles in a color scheme's colors. The roles are
// New's, on the ANSI slots; each slot becomes the scheme's color. Then,
// so every scheme reads well:
//
//   - the selection uses the scheme's selection color when it has one;
//   - the menu and status bars are bands of the background moved toward
//     the text color, and the column header row is filled with the
//     header color;
//   - a background role too close to the screen's is moved apart;
//   - text that falls short of its contrast minimum (4.5:1 for text, 3:1
//     for hints and lines, 2:1 for unavailable items) against its
//     background is moved toward the scheme's text color just far enough
//     to meet it, or toward black or white.
func FromPalette(p Palette) Theme {
	t := roles(p.Dark)
	t.Name, t.Palette = p.Name, &p
	if p.HighContrast {
		t.levels = aaa
	}
	text := readable(p.Foreground, p.Background, nil, t.levels.text)
	t.Screen = lipgloss.NewStyle().Background(p.Background).Foreground(text)
	eachRole(&t, func(_ string, s *lipgloss.Style) { *s = mapStyle(*s, &p) })
	if p.Selection != nil && contrast(p.Selection, p.Background) >= minDistinct {
		on := text
		if contrast(p.Background, p.Selection) > contrast(text, p.Selection) {
			on = p.Background
		}
		sel := lipgloss.NewStyle().Background(p.Selection).Foreground(on)
		t.Selection, t.HeaderSel = sel, sel
	}
	bar := lipgloss.NewStyle().Background(blend(p.Background, text, barBlend)).Foreground(text)
	t.MenuBarRow, t.StatusBarRow = bar, bar
	eachRole(&t, func(name string, s *lipgloss.Style) { *s = fixContrast(name, *s, text, &p, t.levels) })
	t.ColumnHeaderRow = lipgloss.NewStyle().Background(t.Header.GetBackground()).Foreground(t.Header.GetForeground())
	t.standouts()
	return t
}

// eachRole calls fn with every style role of t and its field name,
// including the chart series.
func eachRole(t *Theme, fn func(name string, s *lipgloss.Style)) {
	v := reflect.ValueOf(t).Elem()
	styleType := reflect.TypeOf(lipgloss.Style{})
	for i := range v.NumField() {
		f, name := v.Field(i), v.Type().Field(i).Name
		switch {
		case f.Type() == styleType:
			fn(name, f.Addr().Interface().(*lipgloss.Style))
		case f.Kind() == reflect.Array && f.Type().Elem() == styleType:
			for j := range f.Len() {
				fn(name, f.Index(j).Addr().Interface().(*lipgloss.Style))
			}
		}
	}
}

// mapStyle replaces ANSI colors with the scheme's.
func mapStyle(s lipgloss.Style, p *Palette) lipgloss.Style {
	if c, ok := slot(s.GetForeground(), p); ok {
		s = s.Foreground(c)
	}
	if c, ok := slot(s.GetBackground(), p); ok {
		s = s.Background(c)
	}
	if c, ok := slot(s.GetUnderlineColor(), p); ok {
		s = s.UnderlineColor(c)
	}
	return s
}

// slot returns the scheme's color for an ANSI color.
func slot(c color.Color, p *Palette) (color.Color, bool) {
	switch c := c.(type) {
	case ansi.BasicColor:
		return p.ANSI[c%16], true
	case ansi.IndexedColor:
		if c < 16 {
			return p.ANSI[c], true
		}
	}
	return nil, false
}

// levels are the contrast minimums a theme's roles are held to.
type levels struct {
	text      float64 // cell text, bars, menus, chips: every other role
	secondary float64 // hints, muted text, marks and chart labels
	lines     float64 // borders, rules, chart axes and swatches, progress
	disabled  float64 // unavailable items
}

var (
	// aa is WCAG AA, every scheme's floor.
	aa = levels{text: minText, secondary: minSecondary, lines: minSecondary, disabled: minDisabled}
	// aaa is WCAG AAA, the high-contrast schemes': 7:1 for all text,
	// hints included, and 4.5:1 for lines and unavailable items.
	aaa = levels{text: minTextAAA, secondary: minTextAAA, lines: minText, disabled: minText}
)

// minContrast is the contrast minimum for a role's text.
func minContrast(role string, lv levels) float64 {
	switch role {
	case "Disabled":
		return lv.disabled
	case "Hint", "Muted", "ChartLabel", "Match", "Dropdown", "NoteMark", "Region":
		return lv.secondary
	case "Border", "FrozenLine", "ChartFrame", "ChartAxis", "Progress", "ProgressTodo", "Copied", "Series", "CellBorder":
		return lv.lines
	}
	return lv.text
}

// fixContrast keeps a role's background apart from the screen and its
// text readable on it.
func fixContrast(role string, s lipgloss.Style, text color.Color, p *Palette, lv levels) lipgloss.Style {
	if role == "Screen" {
		return s
	}
	bg := p.Background
	if c := s.GetBackground(); !isNoColor(c) {
		bg = c
		if role != "SeriesBg" {
			bg = distinct(c, p.Background, text)
			s = s.Background(bg)
		}
	}
	if c := s.GetForeground(); !isNoColor(c) && role != "SeriesBg" {
		s = s.Foreground(readable(c, bg, text, minContrast(role, lv)))
	} else if !isNoColor(s.GetBackground()) && role != "SeriesBg" {
		// Text on a background role with no color of its own is drawn in
		// the screen's text color.
		s = s.Foreground(readable(text, bg, nil, minContrast(role, lv)))
	}
	return s
}

func isNoColor(c color.Color) bool {
	_, none := c.(lipgloss.NoColor)
	return c == nil || none
}

// Resolve returns the theme called name: the terminal theme's or the
// high-contrast scheme's variant for a dark or light terminal, or a
// scheme found by Lookup in dir. An unknown scheme gives the terminal
// theme and the error.
func Resolve(name string, dark bool, dir string) (Theme, error) {
	if name == "" || name == Terminal {
		return New(dark), nil
	}
	if isHighContrast(name) {
		t := FromPalette(highContrastFor(dark))
		t.Name = HighContrast
		return t, nil
	}
	p, err := Lookup(name, dir)
	if err != nil {
		return New(dark), err
	}
	return FromPalette(p), nil
}
