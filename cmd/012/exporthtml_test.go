package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// chartFile writes a workbook of two columns and a chart titled Sales.
func chartFile(t *testing.T, dir string) string {
	t.Helper()
	w := sheet.NewBook()
	s := w.Sheet(0)
	for i, in := range []string{"Month", "Jan", "Feb"} {
		s.Set(sheet.Addr{Row: i}, in)
	}
	for i, in := range []string{"Sales", "10", "20"} {
		s.Set(sheet.Addr{Col: 1, Row: i}, in)
	}
	s.AddChart(sheet.Chart{Type: sheet.ChartLine, Data: sheet.Rect{To: sheet.Addr{Col: 1, Row: 2}}, Header: true, Labels: true,
		Title: "Sales", At: sheet.Addr{Col: 3}, W: 30, H: 10})
	path := filepath.Join(dir, "book.012")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := w.Write(f); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestExportHTML(t *testing.T) {
	e, _, _ := scriptEnv(t)
	dir := t.TempDir()
	path := chartFile(t, dir)
	out := filepath.Join(dir, "book.html")
	if code, err := status(e, "export", path, out); code != 0 {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(out)
	if page := string(data); !strings.Contains(page, `data-a="B3" class="r">20<`) || !strings.Contains(page, "<polyline") {
		t.Errorf("page lacks the cells or the chart")
	}
	chart := filepath.Join(dir, "chart.html")
	for _, which := range []string{"1", "sales"} {
		os.Remove(chart)
		if code, err := status(e, "export", path, chart, "--chart", which); code != 0 {
			t.Fatal(err)
		}
		data, _ := os.ReadFile(chart)
		if page := string(data); !strings.Contains(page, "<title>Sales</title>") || strings.Contains(page, "<table") {
			t.Errorf("--chart %s: not the chart alone", which)
		}
	}
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{path, chart, "--chart", "2"}, "Sheet1 has 1 chart: chart 2 isn't one"},
		{[]string{path, chart, "--chart", "Costs"}, `no chart titled "Costs" (charts: Sales)`},
		{[]string{path, filepath.Join(dir, "x.csv"), "--chart", "1"}, "--chart writes a web page"},
	} {
		if _, err := status(e, append([]string{"export"}, c.args...)...); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("export %v: %v, want %q", c.args, err, c.want)
		}
	}
}
