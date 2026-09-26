package e2e

import (
	"flag"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	ghostty "go.mitchellh.com/libghostty"
)

var update = flag.Bool("update", false, "rewrite golden screens in testdata/screens")

// A screen is a named UI state captured as styled HTML. Goldens make every
// visual change show up in review; `make screens` renders them to a gallery
// with dark and light palettes for looking at.
type screen struct {
	name  string
	opts  options
	setup func(s *session)
}

// budget types a small sheet with text, numbers, formulas and an error the
// way a Sheets user would (Tab across, Enter back), then selects the total.
func budget(s *session) {
	s.keys("Household budget 2026", "<enter>", "<down>")
	s.keys("Rent", "<tab>", "1450", "<enter>")
	s.keys("Groceries", "<tab>", "612.4", "<enter>")
	s.keys("Transit", "<tab>", "96", "<enter>")
	s.keys("Savings rate", "<tab>", "=B3/0", "<enter>")
	s.keys("Total", "<tab>", "=SUM(B3:B5)", "<enter>")
	s.keys("<up>", "<right>")
	s.waitForBar("B7", "=SUM(B3:B5)")
}

// formatted types a bill schedule with dates, currency and percentages,
// then styles it with Sheets' shortcuts: bold title and totals, headers
// aligned over their numbers, a struck-out cancelled bill and an italic
// note. It ends by bolding the totals, so line 3 shows the feedback.
func formatted(s *session) {
	s.keys("Bills for October", "<enter>")
	s.keys("Item", "<tab>", "Due", "<tab>", "Amount", "<tab>", "Share", "<enter>")
	s.keys("Rent", "<tab>", "10/1/2026", "<tab>", "$1,450.00", "<tab>", "=C3/C$7", "<enter>")
	s.keys("Food", "<tab>", "9/28/2026", "<tab>", "612.4", "<tab>", "=C4/C$7", "<enter>")
	s.keys("Transit", "<tab>", "9/30/2026", "<tab>", "96", "<tab>", "=C5/C$7", "<enter>")
	s.keys("Gym", "<tab>", "10/5/2026", "<tab>", "40", "<tab>", "=C6/C$7", "<enter>")
	s.keys("Total", "<tab>", "<tab>", "=SUM(C3:C6)", "<tab>", "=SUM(D3:D6)", "<enter>")
	s.keys("<down>", "<left>", "<left>", "Gym cancelled from November", "<enter>")
	// Currency for the amounts, percent for the shares.
	s.keys("<ctrl+home>", "<down>", "<down>", "<down>", "<right>", "<right>", "<shift+down>", "<shift+down>", "<ctrl+shift+4>")
	s.keys("<right>", "<up>", "<shift+down>", "<shift+down>", "<shift+down>", "<shift+down>", "<ctrl+shift+5>")
	// Bold title, bold headers with the number headers on the right.
	s.keys("<ctrl+home>", "<ctrl+b>", "<down>", "<shift+right>", "<shift+right>", "<shift+right>", "<ctrl+b>")
	s.keys("<right>", "<shift+right>", "<shift+right>", "<ctrl+shift+r>")
	// The cancelled bill is struck out; the note is italic.
	s.keys("<ctrl+home>", "<down>", "<down>", "<down>", "<down>", "<down>", "<shift+right>", "<shift+right>", "<shift+right>", "<alt+shift+5>")
	s.keys("<down>", "<down>", "<down>", "<ctrl+i>")
	s.keys("<up>", "<up>", "<shift+right>", "<shift+right>", "<shift+right>", "<ctrl+b>")
	s.waitFor("Bold on for A7:D7")
}

var screens = []screen{
	{name: "ready-empty", setup: func(s *session) {}},
	{name: "budget", setup: budget},
	{name: "entry-formula", setup: func(s *session) {
		budget(s)
		s.keys("<down>", "=B7*12")
		s.waitFor("ENTER")
	}},
	{name: "point-range", setup: func(s *session) {
		budget(s)
		s.keys("<right>", "=AVERAGE(", "<left>", "<up>", "<up>", "<up>", "<up>", "<shift+down>", "<shift+down>")
		s.waitFor("=AVERAGE(B3:B5")
	}},
	{name: "selection-stats", setup: func(s *session) {
		budget(s)
		s.keys("<up>", "<up>", "<up>", "<up>", "<shift+down>", "<shift+down>")
		s.waitFor("Sum 2158.4")
	}},
	{name: "column-select", setup: func(s *session) {
		budget(s)
		s.keys("<ctrl+space>")
		s.waitFor("B1:B8192")
	}},
	{name: "edit-error", setup: func(s *session) {
		s.keys("=SUM(A1", "<enter>")
		s.waitFor("Expected , or ) in SUM")
	}},
	{name: "menu", setup: func(s *session) {
		budget(s)
		s.keys("<alt+f>", "<down>", "<down>")
		s.waitFor("Save the sheet")
	}},
	{name: "palette", setup: func(s *session) {
		budget(s)
		s.keys("<ctrl+k>")
		s.waitFor("Search the menus")
	}},
	{name: "palette-search", setup: func(s *session) {
		budget(s)
		s.keys("<alt+/>", "col")
		s.waitFor("│ › col")
	}},
	{name: "palette-narrow", opts: options{cols: 60, rows: 16}, setup: func(s *session) {
		budget(s)
		s.keys("<ctrl+k>", "sa")
		s.waitFor("│ › sa")
	}},
	{name: "context-menu", setup: func(s *session) {
		budget(s)
		s.click(ghostty.MouseButtonRight, 6+10+4, 4+3)
		s.waitFor("│ Clear")
	}},
	{name: "context-menu-column", setup: func(s *session) {
		budget(s)
		s.click(ghostty.MouseButtonRight, 6+10+4, 3)
		s.waitFor("│ Resize column")
	}},
	{name: "menu-narrow", opts: options{cols: 60, rows: 16}, setup: func(s *session) {
		budget(s)
		s.keys("<alt+h>")
		s.waitFor("│ About one23")
	}},
	{name: "functions", setup: func(s *session) {
		budget(s)
		s.keys("<alt+h>", "f", "if")
		s.waitFor("│ › if")
	}},
	{name: "about", setup: func(s *session) {
		s.keys("<alt+h>", "a")
		s.waitFor("Google Sheets keys")
	}},
	{name: "help-narrow", opts: options{cols: 60, rows: 16}, setup: func(s *session) {
		s.keys("<f1>")
		s.waitFor("Keyboard shortcuts")
	}},
	{name: "palette-wide", opts: options{cols: 200, rows: 30}, setup: func(s *session) {
		budget(s)
		s.keys("<ctrl+k>", "sel")
		s.waitFor("│ › sel")
	}},
	{name: "help-wide", opts: options{cols: 200, rows: 45}, setup: func(s *session) {
		s.keys("<f1>")
		s.waitFor("Keyboard shortcuts")
	}},
	{name: "quit-confirm", setup: func(s *session) {
		budget(s)
		s.keys("<ctrl+q>")
		s.waitFor("unsaved changes")
	}},
	{name: "prompt-width", setup: func(s *session) {
		budget(s)
		s.keys("<ctrl+home>", "<alt+o>", "c", "<enter>", "<right>", "<right>", "<right>")
		s.waitFor("Column width (1-240): 13")
	}},
	{name: "error-goto", setup: func(s *session) {
		s.keys("<f5>", "nope", "<enter>")
		s.waitFor("Not a cell address")
	}},
	{name: "help", setup: func(s *session) {
		s.keys("<f1>")
		s.waitFor("HELP")
	}},
	{name: "narrow", opts: options{cols: 60, rows: 16}, setup: budget},
	{name: "hover-resize-handle", setup: func(s *session) {
		budget(s)
		s.mouse(ghostty.MouseActionMotion, ghostty.MouseButtonUnknown, 6+10-1, 3, 0)
		s.waitFor("▐")
	}},
	{name: "resizing-column", setup: func(s *session) {
		budget(s)
		s.mouse(ghostty.MouseActionPress, ghostty.MouseButtonLeft, 6+10-1, 3, 0)
		s.mouse(ghostty.MouseActionMotion, ghostty.MouseButtonLeft, 6+10+5, 3, 0)
		s.waitFor("Column A width 16")
	}},
	{name: "copy-marker", setup: func(s *session) {
		budget(s)
		s.keys("<up>", "<up>", "<up>", "<up>", "<shift+down>", "<shift+down>", "<ctrl+c>", "<right>", "<up>")
		s.waitFor("Copied B3:B5")
	}},
	{name: "copy-marker-selected", setup: func(s *session) {
		budget(s)
		s.keys("<left>", "<up>", "<up>", "<up>", "<up>", "<shift+down>", "<shift+down>", "<shift+right>", "<ctrl+x>")
		s.waitFor("Cut A3:B5")
	}},
	{name: "undo-note", setup: func(s *session) {
		budget(s)
		s.keys("<up>", "<shift+up>", "<delete>", "<ctrl+z>")
		s.waitFor("Undid: clear B5:B6")
	}},
	{name: "narrow-copy", opts: options{cols: 60, rows: 16}, setup: func(s *session) {
		budget(s)
		s.keys("<ctrl+c>", "<down>")
		s.waitFor("Copied B7")
	}},
	{name: "formats", setup: formatted},
	{name: "menu-format-number", setup: func(s *session) {
		formatted(s)
		s.keys("<alt+o>", "<right>")
		s.waitFor("Currency rounded")
	}},
	{name: "formats-narrow", opts: options{cols: 60, rows: 16}, setup: formatted},
}

// Key screens are also recorded on a light terminal, where the app picks
// its light theme from the reported background color.
func init() {
	for _, name := range []string{"budget", "point-range", "selection-stats", "menu", "palette-search", "context-menu-column", "functions", "quit-confirm", "help", "resizing-column", "copy-marker", "copy-marker-selected", "formats", "menu-format-number"} {
		for _, sc := range screens {
			if sc.name == name {
				sc.name += "-light"
				sc.opts.light = true
				screens = append(screens, sc)
			}
		}
	}
}

func TestScreens(t *testing.T) {
	dir := filepath.Join("testdata", "screens")
	if *update {
		os.MkdirAll(dir, 0o755)
	}
	for _, sc := range screens {
		t.Run(sc.name, func(t *testing.T) {
			s := startWith(t, sc.opts)
			sc.setup(s)
			got := s.stableHTML()
			path := filepath.Join(dir, sc.name+".html")
			if *update {
				if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("missing golden (run make screens): %v", err)
			}
			if got != string(want) {
				actual := filepath.Join(t.TempDir(), sc.name+".html")
				os.WriteFile(actual, []byte(got), 0o644)
				t.Errorf("screen %s changed; if intended, run make screens and review the gallery.\nplain text now:\n%s", sc.name, s.screen())
			}
		})
	}
	if *update {
		if err := writeGallery(dir); err != nil {
			t.Fatal(err)
		}
	}
}

// stableHTML waits until two captures in a row match, so a snapshot never
// catches a frame mid-render.
func (s *session) stableHTML() string {
	s.t.Helper()
	prev := s.html()
	for range 100 {
		time.Sleep(30 * time.Millisecond)
		cur := s.html()
		if cur == prev {
			return cur
		}
		prev = cur
	}
	s.t.Fatal("screen never settled")
	return ""
}

// writeGallery renders all goldens into gallery.html (git-ignored), once
// with each reference palette.
func writeGallery(dir string) error {
	var b strings.Builder
	b.WriteString(`<!doctype html><meta charset="utf-8"><title>one23 screens</title>
<style>
body{margin:0;padding:32px;background:#0f1012;color:#ddd;font:14px system-ui,sans-serif}
h1{font-weight:600;margin:0 0 24px}
section{margin:0 0 40px}
h2{font:500 13px ui-monospace,monospace;color:#aaa;margin:0 0 8px}
.term{display:inline-block;padding:14px 16px;border-radius:10px;box-shadow:0 8px 30px #0008}
.screen{margin:0;font:13px/1.3 "JetBrains Mono","SF Mono",Menlo,monospace;color:var(--fg)}
.grid{display:grid;grid-template-columns:repeat(auto-fill,minmax(820px,1fr));gap:8px 24px}
.wide{grid-column:1/-1}
`)
	for _, p := range []struct {
		class  string
		ansi   [16]uint32
		bg, fg string
	}{{"dark", darkANSI, "#1d1f21", "#c5c8c6"}, {"light", lightANSI, "#fafafa", "#1d1f21"}} {
		fmt.Fprintf(&b, ".%s{background:%s;color:%s;--bg:%s;--fg:%s}.%s{", p.class, p.bg, p.fg, p.bg, p.fg, p.class)
		for i, c := range p.ansi {
			fmt.Fprintf(&b, "--vt-palette-%d:#%06x;", i, c)
		}
		b.WriteString("}\n")
	}
	b.WriteString("</style><h1>one23 screens</h1><div class=grid>\n")
	for _, sc := range screens {
		body, err := os.ReadFile(filepath.Join(dir, sc.name+".html"))
		if err != nil {
			return err
		}
		wide := ""
		if sc.opts.cols > 120 {
			wide = " class=wide" // spans the whole gallery row
		}
		fmt.Fprintf(&b, "<section id=%q%s><h2>%s</h2><div>", sc.name, wide, html.EscapeString(sc.name))
		th := "dark"
		if sc.opts.light {
			th = "light"
		}
		fmt.Fprintf(&b, "<div class=\"term %s\">%s</div>", th, body)
		b.WriteString("</div></section>\n")
	}
	b.WriteString("</div>\n")
	return os.WriteFile(filepath.Join(dir, "gallery.html"), []byte(b.String()), 0o644)
}
