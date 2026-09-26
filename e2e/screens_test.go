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

// budget types a small worksheet with labels, numbers, formulas and an
// error, used by several screens.
func budget(s *session) {
	s.keys("Household budget 2026", "<down>", "<down>")
	s.keys("Rent", "<right>", "1450", "<down>", "<left>")
	s.keys("Groceries", "<right>", "612.4", "<down>", "<left>")
	s.keys("Transit", "<right>", "96", "<down>", "<left>")
	s.keys("Savings rate", "<right>", "+B3/0", "<down>", "<left>")
	s.keys(`"Total`, "<right>", "@SUM(B3..B5)", "<enter>")
	s.waitFor("B7: @SUM(B3..B5)")
}

var screens = []screen{
	{name: "ready-empty", setup: func(s *session) {}},
	{name: "budget", setup: budget},
	{name: "budget-light", opts: options{light: true}, setup: budget},
	{name: "entry-formula", setup: func(s *session) {
		budget(s)
		s.keys("<down>", "+B7*12")
		s.waitFor("VALUE")
	}},
	{name: "point-range", setup: func(s *session) {
		budget(s)
		s.keys("<right>", "@AVG(", "<left>", "<up>", "<up>", "<up>", "<up>", ".", "<down>", "<down>")
		s.waitFor("@AVG(B3..B5")
	}},
	{name: "edit-error", setup: func(s *session) {
		s.keys("@SUM(A1", "<enter>")
		s.waitFor("expected , or )")
	}},
	{name: "menu-top", setup: func(s *session) {
		budget(s)
		s.keys("/")
		s.waitFor("Worksheet  Range  File  Quit")
	}},
	{name: "menu-quit-unsaved", setup: func(s *session) {
		budget(s)
		s.keys("/q", "<right>")
		s.waitFor("NOT SAVED")
	}},
	{name: "prompt-width", setup: func(s *session) {
		budget(s)
		s.keys("<home>", "/wcs", "<right>", "<right>", "<right>")
		s.waitFor("Enter column width (1..240): 12")
	}},
	{name: "range-erase", setup: func(s *session) {
		budget(s)
		s.keys("<home>", "<down>", "<down>", "/re", "<right>", "<down>", "<down>")
		s.waitFor("Enter range to erase: A3..B5")
	}},
	{name: "error-goto", setup: func(s *session) {
		s.keys("<f5>", "nope", "<enter>")
		s.waitFor("Invalid cell address")
	}},
	{name: "help", setup: func(s *session) {
		s.keys("<f1>")
		s.waitFor("HELP")
	}},
	{name: "narrow", opts: options{cols: 60, rows: 16}, setup: budget},
}

// Key screens are also recorded on a light terminal, where the app picks
// its light theme from the reported background color.
func init() {
	for _, name := range []string{"point-range", "menu-quit-unsaved", "help"} {
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
.term{display:inline-block;padding:14px 16px;border-radius:10px;box-shadow:0 8px 30px #0008;font:13px/1.25 "JetBrains Mono","SF Mono",Menlo,monospace}
.term div{font-family:inherit !important}
.grid{display:grid;grid-template-columns:repeat(auto-fill,minmax(820px,1fr));gap:8px 24px}
`)
	for _, p := range []struct {
		class  string
		ansi   [16]uint32
		bg, fg string
	}{{"dark", darkANSI, "#1d1f21", "#c5c8c6"}, {"light", lightANSI, "#fafafa", "#1d1f21"}} {
		fmt.Fprintf(&b, ".%s{background:%s;color:%s}.%s{", p.class, p.bg, p.fg, p.class)
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
		fmt.Fprintf(&b, "<section id=%q><h2>%s</h2><div>", sc.name, html.EscapeString(sc.name))
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
