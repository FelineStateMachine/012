package sheet

import (
	"slices"
	"testing"
)

// Linked files and command regions are one set: a formula names a
// linked file as nu.name, a command reads it as $name, and refreshing it
// runs what reads it, while the linked file itself never runs.
func TestLinkedAmongRegions(t *testing.T) {
	s := notebook(t)
	w := s.Book()
	id, err := s.AddLinked(Addr{}, LinkSource{Path: "logs/app-2026.csv"})
	if err != nil || id != "app_2026" {
		t.Fatalf("name %q, %v", id, err)
	}
	apply(t, s, LiveOp{Region: id, Reset: true, Header: liveRow("n"), Rows: []LiveRow{liveRow("2"), liveRow("3")}})
	addShown(t, s, "r1", "$app_2026 | first", table([]string{"n"}, []string{"2"}))
	if _, r, _ := w.Region("r1"); !slices.Equal(r.Deps, []string{"app_2026"}) {
		t.Fatalf("r1 reads %v", r.Deps)
	}
	// Stacked on a notebook, the command region goes below the file.
	if _, r, _ := w.Region("r1"); r.At.Row <= 2 {
		t.Errorf("r1 at %v, over the linked file", r.At)
	}
	other, _ := w.AddSheet("Sheet2", 1)
	other.Set(Addr{}, "=SUM(nu.app_2026)")
	if v := other.Value(Addr{}).Num; v != 5 {
		t.Fatalf("SUM(nu.app_2026) = %v", other.Value(Addr{}))
	}
	apply(t, s, LiveOp{Region: id, Rows: []LiveRow{liveRow("4")}})
	if v := other.Value(Addr{}).Num; v != 9 {
		t.Fatalf("after a row arrived, SUM = %v", v)
	}
	if got := w.RunOrder(); !slices.Equal(got, []string{"r1"}) {
		t.Errorf("run all: %v", got)
	}
	if got := w.RefreshOrder(id); !slices.Equal(got, []string{"r1"}) {
		t.Errorf("refreshing the linked file runs %v", got)
	}
	if err := s.EditRegion(id, "ls", nil, ""); err == nil {
		t.Error("a linked file took a command")
	}
	if err := s.ShowRegion(id, table([]string{"x"})); err == nil {
		t.Error("a linked file was shown a command's table")
	}
	if got := w.linkName("logs/app-2026.csv"); got != "app_2026_2" {
		t.Errorf("a second link to the file is named %q", got)
	}
	if got := w.linkName("9.csv"); got != "file_9" {
		t.Errorf("a file named 9 is %q", got)
	}
}

// A linked file's rows are applied as ops outside the undo history,
// while a command region's run is an undo step: the policies region.go
// gives the two kinds.
func TestRegionKindsUndo(t *testing.T) {
	s := New()
	id, _ := s.AddLinked(Addr{}, LinkSource{Path: "log.csv"})
	apply(t, s, LiveOp{Region: id, Reset: true, Header: liveRow("n"), Rows: []LiveRow{liveRow("1")}})
	if s.Book().UndoLabel() != "add log" {
		t.Errorf("rows arriving made a step: %q", s.Book().UndoLabel())
	}
	s.AddRegion(Region{Name: "r1", Command: "ls", At: Addr{Col: 3}})
	s.ShowRegion("r1", table([]string{"n"}, []string{"7"}))
	if s.Book().UndoLabel() != "run r1" {
		t.Errorf("a run isn't a step: %q", s.Book().UndoLabel())
	}
	s.Book().Undo()
	wantShown(t, s, map[string]string{"D1": "r1  ls   not run", "D3": "", "A2": "1"})
}
