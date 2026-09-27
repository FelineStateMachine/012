package e2e

import (
	"strings"
	"testing"

	ghostty "go.mitchellh.com/libghostty"
)

// spending types a small table of two series with labels and headers.
func spending(s *session) {
	s.keys("Month", "<tab>", "Rent", "<tab>", "Food", "<enter>")
	s.keys("Jan", "<tab>", "$1,450", "<tab>", "$600", "<enter>")
	s.keys("Feb", "<tab>", "$1,450", "<tab>", "$640", "<enter>")
	s.keys("Mar", "<tab>", "$1,500", "<tab>", "$612", "<enter>")
	s.keys("Apr", "<tab>", "$1,500", "<tab>", "$700", "<enter>")
	s.keys("<ctrl+home>")
	s.waitForBar("A1", "Month")
}

// insertChart charts the table around A1 from the Insert menu and waits
// for the editor.
func insertChart(s *session) {
	s.keys("<alt+i>")
	s.waitFor("│ Chart")
	s.keys("<up>", "<up>", "<up>", "<enter>") // past Dropdown and Checkbox, at the end
	s.waitFor("Series in columns")
}

func TestInsertEditMoveDeleteChart(t *testing.T) {
	dir := t.TempDir()
	s := start(t, dir)
	spending(s)
	insertChart(s)
	s.waitFor("Rent and Food")
	s.waitFor("$1,500 ┤")
	s.waitFor("■ Rent   ■ Food")
	if !strings.Contains(s.line(0), "CHART") {
		t.Errorf("indicator: %q", s.line(0))
	}

	// Line chart, then keep it: the chart stays selected.
	s.keys("<right>", "<right>", "<enter>")
	s.waitFor("Line chart of A1:C5")
	// Arrows move it down a row; Esc deselects.
	if !strings.Contains(s.line(gridRow1), "┌─ Rent and Food") {
		t.Fatalf("chart not at D1:\n%s", s.screen())
	}
	s.keys("<down>", "<esc>")
	s.waitFor("READY")
	s.eventually("chart moved", func() bool { return strings.Contains(s.line(gridRow1+1), "┌─ Rent and Food") })

	// The chart follows its data.
	s.keys("<f5>", "B3", "<enter>", "3000", "<enter>")
	s.waitFor("$3,000 ┤")

	// Saved with the sheet and back on reopening.
	s.keys("<ctrl+s>", "charts", "<enter>")
	s.eventually("saved", func() bool { return s.title() == "012 - charts.012" })
	s.keys("<ctrl+q>")
	s.waitExit()
	r := start(t, dir, "charts.012")
	r.waitFor("Rent and Food")
	r.waitFor("$3,000 ┤")

	// Click the chart to select it; Del deletes it and Ctrl+Z restores it.
	r.click(ghostty.MouseButtonLeft, 50, 10)
	r.waitFor("Line chart of A1:C5")
	r.keys("<delete>")
	r.waitFor("Deleted the chart")
	if strings.Contains(r.screen(), "Rent and Food") {
		t.Error("chart still shown after Del")
	}
	r.keys("<ctrl+z>")
	r.waitFor("Rent and Food")
}

func TestDragChart(t *testing.T) {
	s := start(t, "")
	spending(s)
	insertChart(s)
	s.keys("<enter>", "<esc>")
	s.waitFor("READY")
	// The chart's top-left corner is at D1: x 6+30, y 4. Drag its body
	// two rows down and one column right.
	s.mouse(ghostty.MouseActionPress, ghostty.MouseButtonLeft, 45, 8, 0)
	s.mouse(ghostty.MouseActionMotion, ghostty.MouseButtonLeft, 55, 10, 0)
	s.mouse(ghostty.MouseActionRelease, ghostty.MouseButtonLeft, 55, 10, 0)
	s.eventually("chart at E3", func() bool {
		l := s.line(gridRow1 + 2)
		return len(l) > 46 && strings.HasPrefix(l[46:], "┌─ Rent and Food")
	})
}

func TestKittyImages(t *testing.T) {
	s := startWith(t, options{graphics: true})
	spending(s)
	insertChart(s)
	s.keys("<enter>")
	// The plot is Unicode placeholders naming image 16, and the image
	// is stored with a virtual placement.
	s.eventually("placeholders", func() bool { return strings.ContainsRune(s.screen(), '\U0010EEEE') })
	s.eventually("image 16", func() bool {
		s.mu.Lock()
		defer s.mu.Unlock()
		kg, err := s.vt.KittyGraphics()
		if err != nil {
			return false
		}
		img := kg.Image(16)
		if img == nil {
			return false
		}
		w, _ := img.Width()
		h, _ := img.Height()
		it, err := ghostty.NewKittyGraphicsPlacementIterator()
		if err != nil {
			return false
		}
		defer it.Close()
		kg.PlacementIterator(it)
		for it.Next() {
			id, _ := it.ImageID()
			virtual, _ := it.IsVirtual()
			if id == 16 && virtual && w > 0 && h > 0 {
				return true
			}
		}
		return false
	})
	// Labels and legend stay text.
	s.waitFor("$1,500 ┤")
	s.waitFor("■ Rent   ■ Food")
	// Deleting the chart frees the image.
	s.keys("<delete>")
	s.eventually("image freed", func() bool {
		s.mu.Lock()
		defer s.mu.Unlock()
		kg, _ := s.vt.KittyGraphics()
		return kg.Image(16) == nil
	})
}

func TestLinksAndErrorMarks(t *testing.T) {
	s := start(t, "")
	s.keys("https://example.com/docs", "<enter>", `=HYPERLINK("example.org", "Example")`, "<enter>", "=1/0", "<enter>", "<up>")
	s.waitFor("#DIV/0!  Division by zero in 1/0")
	s.mu.Lock()
	defer s.mu.Unlock()
	uri := func(x, y int) string {
		ref, err := s.vt.GridRef(ghostty.Point{Tag: ghostty.PointTagActive, X: uint16(x), Y: uint32(y)})
		if err != nil {
			t.Fatal(err)
		}
		u, _ := ref.HyperlinkURI()
		return u
	}
	if got := uri(8, gridRow1); got != "https://example.com/docs" {
		t.Errorf("A1 links to %q", got)
	}
	if got := uri(8, gridRow1+1); got != "https://example.org" {
		t.Errorf("A2 links to %q", got)
	}
	if got := uri(8, gridRow1+2); got != "" {
		t.Errorf("error cell links to %q", got)
	}
	ref, _ := s.vt.GridRef(ghostty.Point{Tag: ghostty.PointTagActive, X: 9, Y: gridRow1 + 2})
	if st, err := ref.Style(); err != nil || st.Underline() != ghostty.UnderlineCurly {
		t.Errorf("error cell underline %v, want curly", st.Underline())
	}
}

// TestChartOptions stacks a column chart, fixes its axis minimum, moves
// its legend, saves and reopens it, and turns it into a scatter with a
// trend line.
func TestChartOptions(t *testing.T) {
	dir := t.TempDir()
	s := start(t, dir)
	spending(s)
	insertChart(s)
	s.keys("k")
	s.waitFor("Stacked")
	s.waitFor("$2,000 ┤")
	s.keys("a")
	s.waitFor("Min auto")
	s.keys("n")
	s.waitFor("Axis minimum (blank for auto):")
	s.keys("1000", "<enter>")
	s.waitFor("Min 1000")
	s.waitFor("$1,000 ┼")
	s.keys("p")
	s.waitFor("Legend right")
	s.waitFor("■ Rent")
	s.keys("<esc>")
	s.waitFor("Series in columns")
	s.keys("<enter>")
	s.waitFor("Column chart of A1:C5")
	s.keys("<esc>", "<ctrl+s>", "options", "<enter>")
	s.eventually("saved", func() bool { return s.title() == "012 - options.012" })
	s.keys("<ctrl+q>")
	s.waitExit()

	r := start(t, dir, "options.012")
	r.waitFor("$1,000 ┼")
	r.waitFor("■ Rent")
	r.click(ghostty.MouseButtonLeft, 50, 10)
	r.waitFor("Column chart of A1:C5")
	r.keys("<enter>")
	r.waitFor("Stacked")
	r.keys("6", "e")
	r.waitFor("Trend line")
	r.keys("<enter>")
	r.waitFor("Scatter chart of A1:C5")
}
