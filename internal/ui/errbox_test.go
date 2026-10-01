package ui

import (
	"os"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// errorCases are formulas whose errors are explained, short and long:
// arithmetic, unknown names, lookups, a chain through other cells and
// sheets, a cycle, and a function past max-cells over a linked source.
var errorCases = []string{
	"=1/0", `="a"+1`, "=SQRT(-1)", "=nosuchname+1", `=VLOOKUP("x",B20:C21,2,FALSE)`,
	"='Quarterly results by region'!B3", "=nowhere!A1+'Another sheet that is not there at all'!B2",
	"=E1", "=TEXTJOIN(\",\",TRUE,sales[region])", "=SORT(sales[amount])", "=SUM(sales[amount]*2)",
}

// TestErrorExplainedAt80Columns checks that at 80 columns the context
// line explains every error whole, or says how to read the rest: F1,
// which shows all of it.
func TestErrorExplainedAt80Columns(t *testing.T) {
	t.Chdir(t.TempDir())
	defer sheet.SetMaxCells(0)
	sheet.SetMaxCells(1000)
	m := newModel()
	linkSales(t, m, 3000)
	m.runCommand("sheet.prev")
	press(t, m, "<ctrl+home>")
	s := m.sheet
	long, err := s.Book().AddSheet("Quarterly results by region", -1)
	if err != nil {
		t.Fatal(err)
	}
	long.Set(addr("B3"), "=1/0")
	for c, f := range map[string]string{"E1": "=F1", "F1": "=G1", "G1": "=H1", "H1": "=E1"} {
		s.Set(addr(c), f) // a cycle
	}
	for i, f := range errorCases {
		if err := s.Set(sheet.Addr{Row: i}, f); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
	}
	pumpSources(t, m)
	cut := 0
	for i, f := range errorCases {
		m.cur = sheet.Addr{Row: i}
		why := s.ExplainError(m.cur)
		if why == "" {
			t.Errorf("%s: %v unexplained", f, s.Value(m.cur))
			continue
		}
		ctx := line(m, contextLine)
		if strings.Contains(ctx, why) {
			continue
		}
		cut++
		if !strings.Contains(ctx, "…") || !strings.HasSuffix(ctx, "F1  more") {
			t.Errorf("%s: cut without F1: %q (%s)", f, ctx, why)
			continue
		}
		press(t, m, "<f1>")
		box := errorBoxText(m)
		if box != why {
			t.Errorf("%s: F1 shows %q, want %q", f, box, why)
		}
		press(t, m, "<esc>")
		if m.overlay != nil {
			t.Fatalf("%s: Esc left the box open", f)
		}
	}
	if cut == 0 {
		t.Error("no explanation was long enough to cut")
	}
}

// errorBoxText is the open error box's text, its lines joined.
func errorBoxText(m *Model) string {
	b, ok := m.overlay.(*errorBox)
	if !ok {
		return ""
	}
	var words []string
	for _, l := range b.Layout()[0].Lines[1:] {
		l = strings.Trim(ansi.Strip(l), "│ ")
		if !strings.HasPrefix(l, "└") {
			words = append(words, l)
		}
	}
	return strings.Join(words, " ")
}

func TestF1ElsewhereShowsShortcuts(t *testing.T) {
	m := newModel()
	press(t, m, "<f1>")
	if m.overlay == nil || strings.Contains(screen(m), " in A1") {
		t.Fatalf("F1 on a blank cell: %T", m.overlay)
	}
	press(t, m, "<esc>", "=1/0", "<enter>", "<up>", "<f1>")
	if box := errorBoxText(m); box != "Division by zero in 1/0" {
		t.Errorf("F1 on #DIV/0!: %q", box)
	}
	press(t, m, "<down>")
	if m.overlay != nil || m.cur != addr("A2") {
		t.Errorf("a key closes the box and does what it does: %T at %v", m.overlay, m.cur)
	}
}

func TestF1OnAMissingSource(t *testing.T) {
	t.Chdir(t.TempDir())
	m := newModel()
	linkSales(t, m, 300)
	if err := os.Remove("sales.parquet"); err != nil {
		t.Fatal(err)
	}
	m.runCommand("data.source_reload")
	pumpSources(t, m)
	m.note = ""
	if l := line(m, contextLine); !strings.Contains(l, "! sales.parquet: the file isn't there") {
		t.Fatalf("context line %q", l)
	}
	press(t, m, "<f1>")
	if box := errorBoxText(m); box != "! sales.parquet: the file isn't there" {
		t.Errorf("F1 on the tab: %q", box)
	}
}

func TestCutExplanation(t *testing.T) {
	for _, c := range []struct {
		why  string
		room int
		want string
	}{
		{"Raise max-cells to 2,000 to SORT x (it's 1,000). SORT holds what it reads.", 60, "Raise max-cells to 2,000 to SORT x (it's 1,000). …"},
		{"Raise max-cells to 2,000 to SORT x (it's 1,000). SORT holds what it reads.", 30, "Raise max-cells to 2,000 to S…"},
		{"One. Two. Three is long.", 12, "One. Two. …"},
		{"x", 1, ""},
	} {
		if got := cutExplanation(c.why, c.room); got != c.want || ansi.StringWidth(got) > c.room {
			t.Errorf("cut %q to %d: %q, want %q", c.why, c.room, got, c.want)
		}
	}
}
