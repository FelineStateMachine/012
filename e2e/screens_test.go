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
	s.waitFor("B7   =SUM(B3:B5)")
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
		s.keys("<f10>")
		s.waitFor("File  Edit  Format")
	}},
	{name: "quit-confirm", setup: func(s *session) {
		budget(s)
		s.keys("<ctrl+q>", "<right>")
		s.waitFor("unsaved changes")
	}},
	{name: "prompt-width", setup: func(s *session) {
		budget(s)
		s.keys("<ctrl+home>", "<f10>", "f", "<enter>", "c", "<right>", "<right>", "<right>")
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
		s.mouse(ghostty.MouseActionMotion, 0, 6+10-1, 3, 0)
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
}

// Key screens are also recorded on a light terminal, where the app picks
// its light theme from the reported background color.
func init() {
	for _, name := range []string{"budget", "point-range", "selection-stats", "quit-confirm", "help", "resizing-column", "copy-marker", "copy-marker-selected"} {
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
