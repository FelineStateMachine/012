package fileio

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// htmlBook is a sheet with a header, numbers, a formula, a style, a
// merge, a note and a chart.
func htmlBook(t *testing.T) *sheet.Sheet {
	t.Helper()
	s := sheet.New()
	for a, in := range map[string]string{
		"A1": "Month", "B1": "Sales", "A2": "Jan", "B2": "$1,200", "A3": "Feb", "B3": "900",
		"A4": "Total", "B4": "=SUM(B2:B3)", "A6": "<b>not markup</b>",
	} {
		addr, _ := sheet.ParseAddr(a)
		if err := s.Set(addr, in); err != nil {
			t.Fatal(err)
		}
	}
	s.SetStyle(sheet.Rect{To: sheet.Addr{Col: 1}}, func(st *sheet.Style) { st.Bold = true })
	if err := s.SetNote(sheet.Addr{Row: 3}, "adds up"); err != nil {
		t.Fatal(err)
	}
	if err := s.Merge(sheet.Rect{From: sheet.Addr{Row: 5}, To: sheet.Addr{Col: 1, Row: 5}}, sheet.MergeAll); err != nil {
		t.Fatal(err)
	}
	s.AddChart(sheet.Chart{Type: sheet.ChartColumn, Data: sheet.Rect{To: sheet.Addr{Col: 1, Row: 2}}, Header: true, Labels: true,
		Title: "Sales by month", At: sheet.Addr{Col: 3, Row: 1}, W: 40, H: 14})
	return s
}

// TestHTMLPage exports a sheet as a page that holds everything it
// needs: fonts, styles, the cells as shown, the chart as SVG.
func TestHTMLPage(t *testing.T) {
	s := htmlBook(t)
	path := filepath.Join(t.TempDir(), "sales.html")
	k, ok := ExportKindOf(path)
	if !ok || k != HTML || k.CanImport() {
		t.Fatalf("ExportKindOf = %v, %v", k, ok)
	}
	if _, ok := KindOf(path); ok {
		t.Errorf("an HTML file is offered for import")
	}
	res, err := Export(context.Background(), path, HTML, Snap(s, sheet.Rect{}, "Sales"), ExportOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Rows != 6 || len(res.Notes) != 0 {
		t.Errorf("rows %d, notes %q", res.Rows, res.Notes)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	page := string(data)
	for _, want := range []string{
		"<title>Sales</title>", "@font-face{font-family:'IBM Plex Mono'", "data:font/woff2;base64,",
		`<th data-c="A">A</th>`, `data-a="B1" class="b">Sales<`, `data-a="A2" class="clip">Jan<`, `data-a="B2" class="r">$1,200<`,
		`data-f="=SUM(B2:B3)">$2,100<`, `class="clip note" title="adds up"`, `colspan="2" rowspan="1"`, "&lt;b&gt;not markup&lt;/b&gt;",
		`<figure class="chart"`, "<svg", "<figcaption>Sales by month</figcaption>", `class="range">A1:B3<`,
		"prefers-color-scheme:light", "READY",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("page lacks %q", want)
		}
	}
	// Nothing is fetched from elsewhere.
	if m := regexp.MustCompile(`(src|href)="?https?:`).FindString(page); m != "" {
		t.Errorf("page loads %s", m)
	}
}

// TestHTMLRange exports a range, with the charts anchored in it.
func TestHTMLRange(t *testing.T) {
	s := htmlBook(t)
	var b bytes.Buffer
	snap := Snap(s, sheet.Rect{From: sheet.Addr{Row: 1}, To: sheet.Addr{Col: 1, Row: 2}}, "Sales")
	if _, err := WriteHTML(&b, HTMLPage{Title: "Sales", Snap: snap}); err != nil {
		t.Fatal(err)
	}
	page := b.String()
	if strings.Contains(page, `data-a="A1"`) || !strings.Contains(page, `data-a="B3"`) || strings.Contains(page, "<svg") {
		t.Errorf("range page wrong:\n%s", page[strings.Index(page, "<body>"):])
	}
	if !strings.Contains(page, `id="o12-context">Sales!A2:B3<`) {
		t.Errorf("context line doesn't name the range")
	}
}

// TestHTMLChartPage draws one chart on its own page.
func TestHTMLChartPage(t *testing.T) {
	s := htmlBook(t)
	c, ok := ChartSnap(s, 0)
	if !ok {
		t.Fatal("no chart")
	}
	if _, ok := ChartSnap(s, 1); ok {
		t.Error("chart 1 found")
	}
	var b bytes.Buffer
	if _, err := WriteHTML(&b, HTMLPage{Title: "Sales by month", Chart: &c}); err != nil {
		t.Fatal(err)
	}
	page := b.String()
	if !strings.Contains(page, `<figure class="chart solo"`) || !strings.Contains(page, "<rect") || strings.Contains(page, "<table") {
		t.Errorf("chart page wrong")
	}
}

// TestHTMLLooks draws a rule's fill and text color.
func TestHTMLLooks(t *testing.T) {
	s := sheet.New()
	for i, in := range []string{"5", "50"} {
		if err := s.Set(sheet.Addr{Row: i}, in); err != nil {
			t.Fatal(err)
		}
	}
	err := s.AddCondFormat(sheet.CondFormat{Ranges: []sheet.Rect{{To: sheet.Addr{Row: 1}}}, Op: sheet.RuleGreater, Args: [2]string{"10"},
		Style: sheet.RuleStyle{Fill: sheet.ColorGreen, Bold: true}})
	if err != nil {
		t.Fatal(err)
	}
	grid, _ := HTMLGrid(Snap(s, sheet.Rect{}, "S"))
	if !strings.Contains(grid, `data-a="A2" class="r b" style="background:var(--r3);color:var(--i3)"`) {
		t.Errorf("rule not drawn:\n%s", grid)
	}
	if strings.Contains(grid, `data-a="A1" class="r b"`) {
		t.Errorf("rule drawn where it doesn't apply")
	}
}
