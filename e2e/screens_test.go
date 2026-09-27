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
	files func(t *testing.T, dir string) // fills the working directory first
	setup func(s *session)
}

func TestScreens(t *testing.T) {
	dir := filepath.Join("testdata", "screens")
	if *update {
		os.MkdirAll(dir, 0o755)
	}
	for _, sc := range screens {
		t.Run(sc.name, func(t *testing.T) {
			opts := sc.opts
			if opts.jev {
				srv, _ := fakeTypeSafe(t)
				opts.env = []string{"TYPESAFE_API_KEY=test-key", "TYPESAFE_BASE_URL=" + srv.URL}
			}
			if sc.files != nil {
				opts.dir = t.TempDir()
				sc.files(t, opts.dir)
			}
			s := startWith(t, opts)
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
	b.WriteString(`<!doctype html><meta charset="utf-8"><title>012 screens</title>
<style>
body{margin:0;padding:32px;background:#0f1012;color:#ddd;font:14px system-ui,sans-serif}
h1{font-weight:600;margin:0 0 24px}
section{margin:0 0 40px}
h2{font:500 13px ui-monospace,monospace;color:#aaa;margin:0 0 8px}
.term{display:inline-block;padding:14px 16px;border-radius:10px;box-shadow:0 8px 30px #0008}
.screen{margin:0;font:13px/1.2 "JetBrains Mono","SF Mono",Menlo,monospace;color:var(--fg)}
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
	b.WriteString("</style><h1>012 screens</h1><div class=grid>\n")
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
