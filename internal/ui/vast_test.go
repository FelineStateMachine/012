package ui

import (
	"fmt"
	"maps"
	"os"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// The guardrail against work proportional to the grid: with the whole
// sheet (or a whole column, or a whole row) selected on a nearly empty
// sheet of a million rows by 16,384 columns, every command that can run
// does, and the frame after it draws, in a small bound of time and
// memory. A command that walked every address of its selection would
// visit 17 billion cells here, and one that walked a whole column a
// million: either fails.
func TestCommandsCostTheDataNotTheGrid(t *testing.T) {
	if testing.Short() {
		t.Skip("runs every command three times")
	}
	skip := map[string]bool{
		"quit":                 true, // the session, not the sheet
		"settings.config_edit": true, "settings.config_reload": true, "settings.jev_key": true,
	}
	const maxTime, maxAlloc = 250 * time.Millisecond, 8 << 20 // a whole column of 8-byte entries is 8 MB
	selections := []struct {
		name string
		keys []string
	}{
		{"sheet", []string{"<ctrl+a>", "<ctrl+a>"}},
		{"column", []string{"<ctrl+space>"}},
		{"row", []string{"<shift+space>"}},
	}
	var report strings.Builder
	for _, id := range slices.Sorted(maps.Keys(commands)) {
		if skip[id] || strings.HasPrefix(id, "file.") {
			continue
		}
		for _, sel := range selections {
			m := vastModel(t)
			press(t, m, sel.keys...)
			if !commands[id].available(m) {
				continue
			}
			if r := m.selection(); sel.name != "row" && !r.AllRows() || sel.name != "column" && !r.AllCols() {
				t.Fatalf("%s selected %v", sel.name, r)
			}
			// Then Enter, which accepts what a dialog the command opened
			// offers (a sort, a filter, a chart), and a frame.
			d, alloc := measure(func() {
				run(m, commands[id].run(m))
				m.View()
				press(t, m, "<enter>")
				m.View()
			})
			fmt.Fprintf(&report, "%-28s %-7s %8.2f ms %9d B\n", id, sel.name, float64(d.Microseconds())/1000, alloc)
			if d > maxTime || alloc > maxAlloc {
				t.Errorf("%s over the whole %s took %v and allocated %d bytes", id, sel.name, d, alloc)
			}
		}
	}
	if testing.Verbose() {
		t.Log("\n" + report.String())
	}
}

// vastModel is a model on a sheet with the full grid and a few cells,
// clear of the top row and the first column (so filling down or right
// from them has nothing to copy), in a temporary directory.
func vastModel(t *testing.T) *Model {
	t.Helper()
	dir := t.TempDir()
	wd, _ := os.Getwd()
	os.Chdir(dir)
	t.Cleanup(func() { os.Chdir(wd) })
	s := sheet.New()
	for _, in := range []struct{ a, v string }{{"B2", "Name"}, {"C2", "Qty"}, {"B3", "pen"}, {"C3", "3"}, {"B4", "ink"}, {"C4", "=C3*2"}, {"D5", "=SUM(C:C)"}} {
		s.Set(addr(in.a), in.v)
	}
	s.ClearHistory()
	m := New(s, "")
	m.Update(tea.WindowSizeMsg{Width: 200, Height: 60})
	return m
}

// measure runs fn and returns how long it took and how much it
// allocated.
func measure(fn func()) (time.Duration, uint64) {
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	start := time.Now()
	fn()
	d := time.Since(start)
	runtime.ReadMemStats(&after)
	return d, after.TotalAlloc - before.TotalAlloc
}
