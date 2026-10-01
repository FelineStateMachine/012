package theme

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"unicode"
)

// A Palette is a terminal color scheme, the kind every terminal ships
// and Ghostty, kitty, iTerm2 and VHS share: the 16 ANSI colors, the
// background and foreground, and optionally the selection color. 012's
// roles are defined on these slots (see New), so any scheme works.
type Palette struct {
	Name       string
	ANSI       [16]color.Color
	Background color.Color
	Foreground color.Color
	Selection  color.Color // may be nil
	Dark       bool
	File       string // the file it came from; empty for built-ins
	// HighContrast holds every role to WCAG AAA (7:1 for text) instead
	// of AA: the built-in high-contrast schemes.
	HighContrast bool
}

// Terminal is the theme name that keeps the terminal's own palette.
const Terminal = "terminal"

// themesJSON is VHS's theme catalog (MIT, Charmbracelet; see NOTICE),
// so names match VHS's Set Theme.
//
//go:embed themes.json
var themesJSON []byte

// classic is 1-2-3's DOS look: CGA colors on blue.
const classic = `# 1-2-3 Classic: CGA colors on a blue screen.
background = #0000aa
foreground = #ffffff
selection-background = #00aaaa
palette = 0=#000000
palette = 1=#aa0000
palette = 2=#00aa00
palette = 3=#aa5500
palette = 4=#0000aa
palette = 5=#aa00aa
palette = 6=#00aaaa
palette = 7=#aaaaaa
palette = 8=#555555
palette = 9=#ff5555
palette = 10=#55ff55
palette = 11=#ffff55
palette = 12=#5555ff
palette = 13=#ff55ff
palette = 14=#55ffff
palette = 15=#ffffff
`

// ClassicName is the built-in 1-2-3 scheme.
const ClassicName = "1-2-3 Classic"

var builtins = sync.OnceValue(func() []Palette {
	ps, err := parseJSON(themesJSON)
	if err != nil {
		panic("theme: embedded themes.json: " + err.Error())
	}
	c, _, err := parseText(classic)
	if err != nil {
		panic("theme: classic: " + err.Error())
	}
	c.Name = ClassicName
	return append(append([]Palette{c}, highContrastPalettes()...), ps...)
})

// Builtins returns the built-in schemes.
func Builtins() []Palette { return builtins() }

// Entry is a theme a picker can offer.
type Entry struct {
	Name string
	Dark bool
	User bool // a file in the themes directory
}

// List returns the themes available: terminal and high-contrast first,
// then the files in dir (the user's themes directory, may be ""), then
// the built-ins, each group sorted by name case-insensitively. A file
// with a built-in's name hides the built-in.
func List(dir string) []Entry {
	out := []Entry{{Name: Terminal, Dark: true}, {Name: HighContrast, Dark: true}}
	seen := map[string]bool{}
	var user []Entry
	for _, p := range userPalettes(dir) {
		if key := strings.ToLower(p.Name); !seen[key] {
			seen[key] = true
			user = append(user, Entry{Name: p.Name, Dark: p.Dark, User: true})
		}
	}
	sortEntries(user)
	var bi []Entry
	for _, p := range Builtins() {
		if !seen[strings.ToLower(p.Name)] {
			bi = append(bi, Entry{Name: p.Name, Dark: p.Dark})
		}
	}
	sortEntries(bi)
	return append(append(out, user...), bi...)
}

func sortEntries(es []Entry) {
	slices.SortStableFunc(es, func(a, b Entry) int {
		return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	})
}

// userPalettes reads every theme file in dir, skipping ones that don't
// parse.
func userPalettes(dir string) []Palette {
	if dir == "" {
		return nil
	}
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []Palette
	for _, f := range files {
		if f.IsDir() || strings.HasPrefix(f.Name(), ".") {
			continue
		}
		ps, err := ReadFile(filepath.Join(dir, f.Name()))
		if err == nil {
			out = append(out, ps...)
		}
	}
	return out
}

// Lookup finds a scheme by name: a file in dir first (named exactly, or
// with .json or .conf), then a built-in. Names match exactly, then
// case-insensitively, then ignoring spaces, hyphens and underscores, so
// "tokyo-night" finds "TokyoNight" and "tokyonight" the scheme of that
// name.
func Lookup(name, dir string) (Palette, error) {
	if dir != "" && !strings.ContainsAny(name, `/\`) && name != "." && name != ".." {
		for _, file := range []string{name, name + ".json", name + ".conf"} {
			if ps, err := ReadFile(filepath.Join(dir, file)); err == nil {
				return pick(ps, name), nil
			} else if !errors.Is(err, os.ErrNotExist) {
				return Palette{}, err
			}
		}
	}
	all := append(userPalettes(dir), Builtins()...)
	same := func(s string) string { return s }
	for _, match := range []func(string) string{same, strings.ToLower, squash} {
		for _, p := range all {
			if match(p.Name) == match(name) {
				return p, nil
			}
		}
	}
	return Palette{}, fmt.Errorf("no theme %q (012 config themes lists them)", name)
}

// pick returns the scheme called name from a file's schemes, or the
// first.
func pick(ps []Palette, name string) Palette {
	for _, p := range ps {
		if strings.EqualFold(p.Name, name) {
			return p
		}
	}
	return ps[0]
}

func squash(s string) string {
	return strings.Map(func(r rune) rune {
		if r == ' ' || r == '-' || r == '_' {
			return -1
		}
		return unicode.ToLower(r)
	}, s)
}

// ReadFile reads a theme file: Ghostty's format (palette = N=#rrggbb,
// background, foreground, selection-background), kitty's (color0 ...
// color15, background, foreground, selection_background), or VHS's JSON
// (one scheme or a list). A scheme without a name is named after the
// file.
func ReadFile(path string) ([]Palette, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var ps []Palette
	if t := strings.TrimSpace(string(data)); strings.HasPrefix(t, "{") || strings.HasPrefix(t, "[") {
		ps, err = parseJSON(data)
	} else {
		var p Palette
		p, _, err = parseText(string(data))
		ps = []Palette{p}
	}
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	for i := range ps {
		ps[i].File = path
		if ps[i].Name == "" {
			ps[i].Name = strings.TrimSuffix(strings.TrimSuffix(filepath.Base(path), ".json"), ".conf")
		}
	}
	return ps, nil
}

// vhsTheme is a scheme in VHS's themes.json. Puzzletea's spellings
// (purple, cursorColor, selectionBackground) are accepted too.
type vhsTheme struct {
	Name                                                  string
	Black, Red, Green, Yellow, Blue, Magenta, Cyan, White string
	BrightBlack, BrightRed, BrightGreen, BrightYellow     string
	BrightBlue, BrightMagenta, BrightCyan, BrightWhite    string
	Purple, BrightPurple                                  string
	Background, Foreground, Selection                     string
	SelectionBackground                                   string
	Meta                                                  struct {
		IsDark *bool `json:"isDark"`
	}
}

func parseJSON(data []byte) ([]Palette, error) {
	var list []vhsTheme
	if strings.HasPrefix(strings.TrimSpace(string(data)), "{") {
		var one vhsTheme
		if err := json.Unmarshal(data, &one); err != nil {
			return nil, err
		}
		list = []vhsTheme{one}
	} else if err := json.Unmarshal(data, &list); err != nil {
		return nil, err
	}
	out := make([]Palette, 0, len(list))
	for _, t := range list {
		p, err := t.palette()
		if err != nil {
			return nil, fmt.Errorf("%s: %w", t.Name, err)
		}
		out = append(out, p)
	}
	if len(out) == 0 {
		return nil, errors.New("no themes")
	}
	return out, nil
}

func (t vhsTheme) palette() (Palette, error) {
	or := func(a, b string) string {
		if a != "" {
			return a
		}
		return b
	}
	slots := [16]string{t.Black, t.Red, t.Green, t.Yellow, t.Blue, or(t.Magenta, t.Purple), t.Cyan, t.White,
		t.BrightBlack, t.BrightRed, t.BrightGreen, t.BrightYellow, t.BrightBlue, or(t.BrightMagenta, t.BrightPurple), t.BrightCyan, t.BrightWhite}
	p := Palette{Name: t.Name}
	var err error
	for i, s := range slots {
		if p.ANSI[i], err = parseHex(s); err != nil {
			return p, fmt.Errorf("color %d: %w", i, err)
		}
	}
	if p.Background, err = parseHex(t.Background); err != nil {
		return p, fmt.Errorf("background: %w", err)
	}
	if p.Foreground, err = parseHex(t.Foreground); err != nil {
		return p, fmt.Errorf("foreground: %w", err)
	}
	if sel := or(t.Selection, t.SelectionBackground); sel != "" {
		p.Selection, _ = parseHex(sel)
	}
	p.Dark = isDark(p.Background)
	if t.Meta.IsDark != nil {
		p.Dark = *t.Meta.IsDark
	}
	return p, nil
}

// parseText parses Ghostty's or kitty's format. Unknown keys (cursor
// colors, fonts) are ignored; they're reported in the warnings.
func parseText(s string) (Palette, []string, error) {
	var p Palette
	var warnings []string
	set := map[string]bool{}
	for n, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok || strings.ContainsAny(strings.TrimSpace(key), " \t") {
			key, val, ok = strings.Cut(line, " ") // kitty: "color0 #000000"
		}
		key, val = strings.TrimSpace(key), strings.TrimSpace(val)
		if !ok {
			warnings = append(warnings, fmt.Sprintf("line %d: expected key = value", n+1))
			continue
		}
		if err := p.setKey(key, val, set); err != nil {
			warnings = append(warnings, fmt.Sprintf("line %d: %v", n+1, err))
		}
	}
	if !set["background"] || !set["foreground"] {
		return p, warnings, errors.New("a theme needs background and foreground")
	}
	for i := range p.ANSI {
		if p.ANSI[i] == nil {
			p.ANSI[i] = xterm[i]
		}
	}
	p.Dark = isDark(p.Background)
	return p, warnings, nil
}

// setKey applies one line of a Ghostty or kitty theme.
func (p *Palette) setKey(key, val string, set map[string]bool) error {
	slot := -1
	switch {
	case key == "palette": // Ghostty: palette = 4=#81a2be
		i, c, ok := strings.Cut(val, "=")
		if !ok {
			return fmt.Errorf("palette %q: expected N=#rrggbb", val)
		}
		slot, val = atoiOr(strings.TrimSpace(i), -1), strings.TrimSpace(c)
	case strings.HasPrefix(key, "color"): // kitty: color4 #81a2be
		if slot = atoiOr(strings.TrimPrefix(key, "color"), -1); slot < 0 {
			return nil
		}
	case key != "background" && key != "foreground" && key != "selection-background" && key != "selection_background":
		return nil // cursor, selection-foreground and the like: not used
	}
	if slot >= 16 {
		return nil // the 256-color cube isn't used
	}
	c, err := parseHex(val)
	if err != nil {
		return fmt.Errorf("%s: %w", key, err)
	}
	switch {
	case slot >= 0:
		p.ANSI[slot] = c
	case key == "background":
		p.Background = c
	case key == "foreground":
		p.Foreground = c
	default:
		p.Selection = c
	}
	set[key] = true
	return nil
}

func atoiOr(s string, def int) int {
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return def
	}
	return n
}

// parseHex reads #rrggbb, rrggbb or #rgb.
func parseHex(s string) (color.Color, error) {
	h := strings.TrimPrefix(strings.TrimSpace(s), "#")
	if len(h) == 3 {
		h = string([]byte{h[0], h[0], h[1], h[1], h[2], h[2]})
	}
	v, err := strconv.ParseUint(h, 16, 32)
	if len(h) != 6 || err != nil {
		return nil, fmt.Errorf("%q isn't a #rrggbb color", s)
	}
	return color.RGBA{R: uint8(v >> 16), G: uint8(v >> 8), B: uint8(v), A: 0xff}, nil
}

// xterm is the palette used for slots a theme file leaves out.
var xterm = [16]color.Color{
	color.RGBA{0, 0, 0, 255}, color.RGBA{205, 0, 0, 255}, color.RGBA{0, 205, 0, 255}, color.RGBA{205, 205, 0, 255},
	color.RGBA{0, 0, 238, 255}, color.RGBA{205, 0, 205, 255}, color.RGBA{0, 205, 205, 255}, color.RGBA{229, 229, 229, 255},
	color.RGBA{127, 127, 127, 255}, color.RGBA{255, 0, 0, 255}, color.RGBA{0, 255, 0, 255}, color.RGBA{255, 255, 0, 255},
	color.RGBA{92, 92, 255, 255}, color.RGBA{255, 0, 255, 255}, color.RGBA{0, 255, 255, 255}, color.RGBA{255, 255, 255, 255},
}

func isDark(bg color.Color) bool {
	return contrast(bg, color.White) > contrast(bg, color.Black)
}
