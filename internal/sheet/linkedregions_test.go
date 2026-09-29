package sheet

import "testing"

// Linked files and sent outputs are one set: a formula names either as
// nu.name, and their names are one namespace.
func TestLinkedAmongRegions(t *testing.T) {
	s := New()
	w := s.Book()
	id, err := s.AddLinked(Addr{}, LinkSource{Path: "logs/app-2026.csv"})
	if err != nil || id != "app_2026" {
		t.Fatalf("name %q, %v", id, err)
	}
	apply(t, s, LiveOp{Region: id, Reset: true, Header: liveRow("n"), Rows: []LiveRow{liveRow("2"), liveRow("3")}})
	sent(t, s, "r1", "D1")
	other, _ := w.AddSheet("Sheet2", 2)
	other.Set(Addr{}, "=SUM(nu.app_2026)")
	if v := other.Value(Addr{}).Num; v != 5 {
		t.Fatalf("SUM(nu.app_2026) = %v", other.Value(Addr{}))
	}
	apply(t, s, LiveOp{Region: id, Rows: []LiveRow{liveRow("4")}})
	if v := other.Value(Addr{}).Num; v != 9 {
		t.Fatalf("after a row arrived, SUM = %v", v)
	}
	if got := w.linkName("logs/app-2026.csv"); got != "app_2026_2" {
		t.Errorf("a second link to the file is named %q", got)
	}
	if got := w.linkName("9.csv"); got != "file_9" {
		t.Errorf("a file named 9 is %q", got)
	}
	if got := w.linkName("r1.csv"); got != "r1_2" {
		t.Errorf("a file named like a cell is %q", got)
	}
	if len(w.LinkedRegions()) != 1 {
		t.Errorf("linked regions %+v", w.LinkedRegions())
	}
}

// Rows arriving are never undo steps, for either kind.
func TestRegionKindsUndo(t *testing.T) {
	s := New()
	id, _ := s.AddLinked(Addr{}, LinkSource{Path: "log.csv"})
	apply(t, s, LiveOp{Region: id, Reset: true, Header: liveRow("n"), Rows: []LiveRow{liveRow("1")}})
	if s.Book().UndoLabel() != "add log" {
		t.Errorf("rows arriving made a step: %q", s.Book().UndoLabel())
	}
	sent(t, s, "r1", "D1")
	apply(t, s, LiveOp{Region: "r1", Reset: true, Header: liveRow("n"), Rows: []LiveRow{liveRow("7")}})
	if s.Book().UndoLabel() != "add r1" {
		t.Errorf("an output arriving made a step: %q", s.Book().UndoLabel())
	}
	s.Book().Undo()
	wantShown(t, s, map[string]string{"D1": "", "D2": "", "A2": "1"})
}
