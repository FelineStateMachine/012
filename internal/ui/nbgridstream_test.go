package ui

import (
	"context"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/live"
	"github.com/FelineStateMachine/012/internal/notebook"
	"github.com/FelineStateMachine/012/internal/sheet"
)

// streamed is a notebook of one cell run as a stream, its output
// selected, and a function printing lines as the stream's pipeline and
// showing them as the output, as a poll and the next update of the
// output do.
func streamed(t *testing.T) (*Model, func(lines ...string)) {
	t.Helper()
	m := sized(sheet.New(), 100, 30)
	m.OpenNotebook()
	id := m.book().NewCellID()
	m.sheet.SetNotebookCells("cells", []notebook.Cell{{ID: id, Source: "log = tail -f app.log | lines | parse '{level} {msg}'"}})
	printed := make(chan string)
	src := live.NewStream(func(ctx context.Context, w io.Writer) error {
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case l := <-printed:
				io.WriteString(w, l+"\n")
			}
		}
	})
	src.Wait = 5 * time.Second
	t.Cleanup(src.Close)
	st := &nbStream{nbRun: &nbRun{nbQueued: nbQueued{m.sheet, id}, start: time.Now()}, src: src, count: 1}
	m.nb.runs.streams = map[int]*nbStream{id: st}
	m.showStream(st)
	return m, func(lines ...string) {
		t.Helper()
		printed <- strings.Join(lines, "\n")
		if _, ok := src.Poll(context.Background()); !ok {
			t.Fatal("the poll found nothing")
		}
		m.showStream(st)
		if m.nbView().SelectedGrid() == nil {
			m.nbView().Select(0, true)
		}
		screen(m)
	}
}

// selectedGrid is the selected output's grid.
func selectedGrid(t *testing.T, m *Model) *outGrid {
	t.Helper()
	g, ok := m.nbView().SelectedGrid().(*outGrid)
	if !ok || g.child == nil {
		t.Fatalf("no grid:\n%s", screen(m))
	}
	return g
}

// A stream's grid takes the rows as they arrive, its columns widening
// to fit a longer row, but not while the reader is in it; the pointer
// and the filter stay.
func TestStreamGridWidens(t *testing.T) {
	m, feed := streamed(t)
	feed(`{level: info, msg: started}`, `{level: info, msg: "GET /"}`)
	g := selectedGrid(t, m)
	msg := g.child.sheet.ColWidth(1)
	if msg != sheet.DefaultWidth || g.Rows() != 2 {
		t.Fatalf("msg %d wide, %d rows", msg, g.Rows())
	}
	long := "slow query on the orders table, 812ms"
	feed(`{level: warn, msg: "` + long[:20] + `"}`)
	if selectedGrid(t, m) != g || g.Rows() != 3 {
		t.Fatalf("the grid was made again, or has %d rows", g.Rows())
	}
	if got := g.child.sheet.ColWidth(1); got != sheet.FitWidth(long[:20]) {
		t.Errorf("msg is %d wide, want %d", got, sheet.FitWidth(long[:20]))
	}
	if s := screen(m); !strings.Contains(s, long[:20]) || !strings.Contains(s, "● live, 3 rows") {
		t.Errorf("the new row:\n%s", s)
	}

	// In the grid, a filter hiding warnings, the pointer on row 2.
	press(t, m, "<enter>", "<down>")
	g.child.sheet.CreateFilter(g.table())
	g.child.sheet.FilterColumn(0, sheet.Criteria{Hidden: []string{"warn"}})
	feed(`{level: info, msg: "`+long+`"}`, `{level: warn, msg: hidden}`)
	if got := g.child.sheet.ColWidth(1); got != sheet.FitWidth(long[:20]) {
		t.Errorf("msg widened to %d under the pointer", got)
	}
	if g.child.cur != addr("A3") || g.Rows() != 3 || !g.child.sheet.RowHidden(5) {
		t.Errorf("pointer at %s, %d rows showing, row 5 hidden %v", g.child.cur, g.Rows(), g.child.sheet.RowHidden(5))
	}
	press(t, m, "<esc>")
	screen(m) // drawn once left
	if got := g.child.sheet.ColWidth(1); got != sheet.FitWidth(long) {
		t.Errorf("left, msg is %d wide, want %d", got, sheet.FitWidth(long))
	}

	// A column the reader resized keeps its width.
	g.child.sheet.SetColWidth(0, 6)
	feed(`{level: "a level wider than six", msg: x}`)
	if got := g.child.sheet.ColWidth(0); got != 6 {
		t.Errorf("level is %d wide, resized to 6", got)
	}
}

// A stream printing 10,000 rows in batches of 250, a frame drawn after
// each as the output updates, costs each batch the rows it brings, well
// within a frame, rather than the rows so far.
func TestStreamGridAtFrameSpeed(t *testing.T) {
	if testing.Short() {
		t.Skip("streams 10,000 rows")
	}
	m, feed := streamed(t)
	term := newFakeTerm(200, 60)
	m.Update(tea.WindowSizeMsg{Width: 200, Height: 60})
	batch := make([]string, 250)
	var worst, all time.Duration
	for b := range 40 {
		for i := range batch {
			n := b*len(batch) + i
			batch[i] = fmt.Sprintf(`{time: "09:%02d:%02d", level: info, msg: "GET /orders/%d 200"}`, n/60%60, n%60, n)
		}
		start := time.Now()
		feed(batch...)
		term.frame(m)
		d := time.Since(start)
		worst, all = max(worst, d), all+d
	}
	if g := selectedGrid(t, m); g.Rows() != 10000 {
		t.Fatalf("%d rows", g.Rows())
	}
	t.Logf("a batch of 250 rows through to its frame: %s on average, %s at worst", all/40, worst)
	if per := all / 40; per > 8*time.Millisecond {
		t.Errorf("a batch takes %s", per)
	}
}
