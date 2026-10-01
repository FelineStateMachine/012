package theme

import (
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestBuiltins(t *testing.T) {
	bs := Builtins()
	if len(bs) < 300 {
		t.Fatalf("only %d built-in themes", len(bs))
	}
	if bs[0].Name != ClassicName {
		t.Errorf("first is %q", bs[0].Name)
	}
	for _, name := range []string{"Dracula", "Catppuccin Mocha", "Builtin Solarized Light", "nord", "GruvboxDark", "TokyoNight"} {
		if _, err := Lookup(name, ""); err != nil {
			t.Error(err)
		}
	}
	p, err := Lookup("tokyo-night", "")
	if err != nil || p.Name != "TokyoNight" {
		t.Errorf("loose match: %q %v", p.Name, err)
	}
	if p, err := Lookup("tokyonight", ""); err != nil || p.Name != "tokyonight" {
		t.Errorf("exact name: %q %v", p.Name, err)
	}
	if _, err := Lookup("No Such Theme", ""); err == nil || !strings.Contains(err.Error(), "012 config themes") {
		t.Errorf("unknown: %v", err)
	}
}

// requireContrast fails when fg on bg is below min.
func requireContrast(t *testing.T, theme, what string, fg, bg color.Color, min float64) {
	t.Helper()
	if isNoColor(fg) || isNoColor(bg) {
		t.Errorf("%s: %s has no color (fg %v, bg %v)", theme, what, fg, bg)
		return
	}
	if r := contrast(fg, bg); r < min-0.01 {
		t.Errorf("%s: %s contrast %.2f < %.1f", theme, what, r, min)
	}
}

// Every built-in scheme, run through the role derivation, keeps text
// readable: on cells, on the bars, in the selection and the headers, and
// every other role at its minimum. The high-contrast schemes are held to
// WCAG AAA.
func TestEveryThemeReadable(t *testing.T) {
	for _, p := range Builtins() {
		requireReadable(t, p.Name, FromPalette(p))
	}
}

// requireReadable checks a scheme's roles against its minimums.
func requireReadable(t *testing.T, name string, th Theme) {
	t.Helper()
	lv := th.levels
	screenBg, text := th.Screen.GetBackground(), th.Screen.GetForeground()
	requireContrast(t, name, "cell text", text, screenBg, lv.text)
	for _, bar := range []struct {
		name string
		s    lipgloss.Style
	}{{"menu bar", th.MenuBarRow}, {"status bar", th.StatusBarRow}, {"column header row", th.ColumnHeaderRow}} {
		requireContrast(t, name, bar.name+" text", bar.s.GetForeground(), bar.s.GetBackground(), lv.text)
		if c := contrast(bar.s.GetBackground(), screenBg); c < minDistinct-0.01 {
			t.Errorf("%s: %s doesn't stand out from the screen (%.2f)", name, bar.name, c)
		}
	}
	for _, r := range []struct {
		name string
		s    lipgloss.Style
	}{
		{"selection", th.Selection}, {"header", th.Header}, {"row header", th.RowHeader},
		{"active header", th.HeaderActive}, {"selected header", th.HeaderSel}, {"pointer", th.Pointer},
		{"key chip", th.KeyChip}, {"menu selection", th.MenuSelected}, {"indicator", th.Indicator},
	} {
		requireContrast(t, name, r.name, r.s.GetForeground(), r.s.GetBackground(), lv.text)
	}
	// Muted text and hints on the bars, where the status line shows them.
	requireContrast(t, name, "muted text", th.Muted.GetForeground(), screenBg, lv.secondary)
	eachRole(&th, func(role string, s *lipgloss.Style) {
		fg, bg := s.GetForeground(), s.GetBackground()
		if role == "SeriesBg" || isNoColor(fg) {
			return
		}
		if isNoColor(bg) {
			bg = screenBg
		}
		requireContrast(t, name, role, fg, bg, minContrast(role, lv))
	})
}

// The high-contrast theme follows the terminal's background, and its
// schemes are AAA: 7:1 for every text role, hints included.
func TestHighContrast(t *testing.T) {
	for _, dark := range []bool{true, false} {
		th, err := Resolve("high-contrast", dark, "")
		if err != nil || th.Name != HighContrast || th.Palette == nil || th.Palette.Dark != dark || th.levels != aaa {
			t.Fatalf("dark %v: %q %v", dark, th.Name, err)
		}
		requireReadable(t, th.Palette.Name, th)
		for _, role := range []string{"Hint", "Muted", "Warning", "Error", "ErrorCell", "Link", "Spilled", "NoteMark"} {
			if minContrast(role, th.levels) < 7 {
				t.Errorf("%s held to %.1f", role, minContrast(role, th.levels))
			}
		}
	}
	for _, name := range []string{"High Contrast Dark", "high-contrast-light", "HIGH-CONTRAST"} {
		if _, err := Resolve(name, true, ""); err != nil {
			t.Error(err)
		}
	}
	if l := List(""); l[1].Name != HighContrast {
		t.Errorf("list: %v", l[:3])
	}
}

func TestSchemeUsesPaletteColors(t *testing.T) {
	p, _ := Lookup("Dracula", "")
	th := FromPalette(p)
	if !th.Palette.Dark || th.Name != "Dracula" {
		t.Errorf("%+v", th.Name)
	}
	// Every role is in true color: no ANSI index is left to the terminal.
	eachRole(&th, func(role string, s *lipgloss.Style) {
		for _, c := range []color.Color{s.GetForeground(), s.GetBackground(), s.GetUnderlineColor()} {
			switch c.(type) {
			case ansi.BasicColor, ansi.IndexedColor:
				t.Errorf("%s keeps ANSI color %v", role, c)
			}
		}
	})
	// The selection is in reverse video: its background is its foreground.
	if bg := th.Selection.GetForeground(); fmt.Sprint(bg) != fmt.Sprint(p.Selection) || !th.Selection.GetReverse() {
		t.Errorf("selection %v, scheme's %v", bg, p.Selection)
	}
	// The terminal theme keeps ANSI colors and no bands.
	term := New(true)
	if _, ok := term.Pointer.GetBackground().(ansi.BasicColor); !ok {
		t.Error("terminal theme lost its ANSI colors")
	}
	if !isNoColor(term.MenuBarRow.GetBackground()) || !isNoColor(term.Screen.GetBackground()) {
		t.Error("terminal theme has bands")
	}
}

func TestReadThemeFiles(t *testing.T) {
	dir := t.TempDir()
	ghostty := "# Ghostty\npalette = 0=#1d1f21\npalette = 4=81a2be\npalette = 200=#ffffff\nbackground = #fafafa\nforeground = #333\ncursor-color = #000000\nselection-background = #cccccc\nfont-family = x\n"
	kitty := "# kitty\ncolor0 #000000\ncolor12   #5c5cff\nbackground #101010\nforeground #e0e0e0\nselection_background #444444\ncursor_text_color background\n"
	vhs := `{"name": "Mine", "black": "#000000", "red": "#ff0000", "green": "#00ff00", "yellow": "#ffff00", "blue": "#0000ff",
"purple": "#ff00ff", "cyan": "#00ffff", "white": "#eeeeee", "brightBlack": "#555555", "brightRed": "#ff5555",
"brightGreen": "#55ff55", "brightYellow": "#ffff55", "brightBlue": "#5555ff", "brightPurple": "#ff55ff",
"brightCyan": "#55ffff", "brightWhite": "#ffffff", "background": "#202020", "foreground": "#dddddd",
"selectionBackground": "#333333", "meta": {"isDark": true}}`
	os.WriteFile(filepath.Join(dir, "My Light"), []byte(ghostty), 0o644)
	os.WriteFile(filepath.Join(dir, "kitty.conf"), []byte(kitty), 0o644)
	os.WriteFile(filepath.Join(dir, "mine.json"), []byte(vhs), 0o644)
	os.WriteFile(filepath.Join(dir, "broken"), []byte("palette = 1=#ff0000\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "Dracula"), []byte(ghostty), 0o644)

	p, err := Lookup("My Light", dir)
	if err != nil || p.Dark || fmt.Sprint(p.ANSI[4]) != fmt.Sprint(color.RGBA{0x81, 0xa2, 0xbe, 0xff}) ||
		fmt.Sprint(p.Foreground) != fmt.Sprint(color.RGBA{0x33, 0x33, 0x33, 0xff}) || p.Selection == nil {
		t.Errorf("ghostty: %+v %v", p, err)
	}
	if fmt.Sprint(p.ANSI[1]) != fmt.Sprint(xterm[1]) {
		t.Error("slots left out should be xterm's")
	}
	p, err = Lookup("kitty", dir)
	if err != nil || !p.Dark || fmt.Sprint(p.ANSI[12]) != fmt.Sprint(color.RGBA{0x5c, 0x5c, 0xff, 0xff}) {
		t.Errorf("kitty: %+v %v", p, err)
	}
	p, err = Lookup("mine", dir)
	if err != nil || p.Name != "Mine" || fmt.Sprint(p.ANSI[5]) != fmt.Sprint(color.RGBA{0xff, 0, 0xff, 0xff}) || p.Selection == nil {
		t.Errorf("json: %+v %v", p, err)
	}
	if _, err := Lookup("broken", dir); err == nil || !strings.Contains(err.Error(), "background and foreground") {
		t.Errorf("broken: %v", err)
	}
	p, _ = Lookup("dracula", dir)
	if p.File == "" {
		t.Error("a user file should hide the built-in of the same name")
	}
	names := map[string]Entry{}
	list := List(dir)
	for _, e := range list {
		names[e.Name] = e
	}
	if list[0].Name != Terminal || !names["My Light"].User || names["My Light"].Dark || !names["Mine"].User || !names["Nord"].Dark && names["nord"].Name == "" {
		t.Errorf("list: %v", list[:6])
	}
	if _, ok := names["broken"]; ok {
		t.Error("a file that doesn't parse is listed")
	}
	if _, err := Lookup("../etc", dir); err == nil {
		t.Error("path in a name")
	}
}

func TestFill(t *testing.T) {
	band := lipgloss.NewStyle().Background(lipgloss.Color("#102030")).Foreground(lipgloss.Color("#eeeeee"))
	chip := lipgloss.NewStyle().Background(lipgloss.Color("#ff0000")).Render("F1")
	line := "File " + chip + " x" + lipgloss.NewStyle().Foreground(lipgloss.Red).Render("warn") + " end"
	got := Fill(line, 30, band)
	if w := ansi.StringWidth(got); w != 30 {
		t.Errorf("width %d", w)
	}
	if ansi.Strip(got) != ansi.Strip(line)+strings.Repeat(" ", 30-ansi.StringWidth(line)) {
		t.Errorf("text changed: %q", ansi.Strip(got))
	}
	// The band's background comes back after the chip's reset.
	after := got[strings.Index(got, "F1"):]
	if !strings.Contains(after, "48;2;16;32;48") {
		t.Errorf("band not restored after a reset: %q", got)
	}
	if Fill(line, 30, lipgloss.NewStyle()) != line {
		t.Error("an empty band changed the line")
	}
	// Params of 38;2;r;g;b aren't mistaken for resets.
	bg, fg := sgrResets(params(t, "\x1b[38;2;0;39;49m"))
	if bg || fg {
		t.Errorf("true color read as reset: bg %v fg %v", bg, fg)
	}
	bg, fg = sgrResets(params(t, "\x1b[0;31m"))
	if !bg || fg {
		t.Errorf("0;31: bg %v fg %v", bg, fg)
	}
}

func params(t *testing.T, seq string) ansi.Params {
	p := ansi.NewParser()
	_, _, _, _ = ansi.DecodeSequence(seq, 0, p)
	return append(ansi.Params(nil), p.Params()...)
}
