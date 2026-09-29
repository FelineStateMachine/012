package sheet

import (
	"slices"
	"testing"
)

// table makes region data of text rows: the first is the header, and
// cells that read as numbers are numbers.
func table(rows ...[]string) *RegionData {
	d := &RegionData{Rows: len(rows)}
	for _, r := range rows {
		d.Cols = max(d.Cols, len(r))
	}
	d.Values = make([]Value, d.Rows*d.Cols)
	for i, r := range rows {
		for j, text := range r {
			v, _, ok := ParseValue(text)
			switch {
			case text == "":
				continue
			case ok && i > 0:
				d.Values[i*d.Cols+j] = Value{Kind: Number, Num: v}
			default:
				d.Values[i*d.Cols+j] = Value{Kind: Text, Str: text}
			}
		}
	}
	return d
}

func notebook(t *testing.T) *Sheet {
	t.Helper()
	s := New()
	s.MakeNotebook()
	return s
}

func addShown(t *testing.T, s *Sheet, name, command string, d *RegionData) {
	t.Helper()
	if err := s.AddRegion(Region{Name: name, Command: command, Deps: s.Book().RegionDeps(command)}); err != nil {
		t.Fatal(err)
	}
	if err := s.ShowRegion(name, d); err != nil {
		t.Fatal(err)
	}
}

func TestRegionsStack(t *testing.T) {
	s := notebook(t)
	addShown(t, s, "r1", "ls", table([]string{"name", "size"}, []string{"a", "1"}, []string{"b", "2"}))
	wantShown(t, s, map[string]string{"A1": "r1  ls", "A2": "name", "B2": "size", "A3": "a", "B4": "2", "A5": ""})
	addShown(t, s, "r2", "$r1 | where size > 1", table([]string{"name", "size"}, []string{"b", "2"}))
	// A gap row, then r2's label.
	wantShown(t, s, map[string]string{"A5": "", "A6": "r2  $r1 | where size > 1", "A7": "name", "A8": "b"})
	if r, ok := s.RegionTable("r2"); !ok || r != rect("A7:B8") {
		t.Errorf("r2's table %v %v", r, ok)
	}
	// r1 growing pushes r2 down; shrinking pulls it back.
	if err := s.ShowRegion("r1", table([]string{"name", "size"}, []string{"a", "1"}, []string{"b", "2"}, []string{"c", "3"})); err != nil {
		t.Fatal(err)
	}
	wantShown(t, s, map[string]string{"A5": "c", "A6": "", "A7": "r2  $r1 | where size > 1", "A9": "b"})
	s.ShowRegion("r1", table([]string{"name"}, []string{"a"}))
	wantShown(t, s, map[string]string{"A3": "a", "B2": "", "A4": "", "A5": "r2  $r1 | where size > 1", "A7": "b"})
	// Undo shows the table it replaced, with r2 where it was.
	s.Book().Undo()
	wantShown(t, s, map[string]string{"A5": "c", "A7": "r2  $r1 | where size > 1"})
	s.Book().Undo()
	s.Book().Undo() // r2's run
	wantShown(t, s, map[string]string{"A6": "r2  $r1 | where size > 1   not run", "A7": ""})
	s.Book().Undo() // r2 itself
	wantShown(t, s, map[string]string{"A6": ""})
}

func TestRegionGuards(t *testing.T) {
	s := notebook(t)
	addShown(t, s, "r1", "ls", table([]string{"n"}, []string{"1"}))
	for _, a := range []string{"A1", "A2", "A3"} {
		if err := s.Set(at(a), "x"); err != ErrRegionEdit {
			t.Errorf("Set %s: %v, want ErrRegionEdit", a, err)
		}
	}
	if a, r, ok := s.InRegion(rect("A3:C9")); !ok || a != at("A3") || r.Name != "r1" {
		t.Errorf("InRegion = %v %v %v", a, r.Name, ok)
	}
	// Formatting stays, and the value with it.
	s.Batch(Change{Label: "bold"}, func() error {
		c := s.Cell(at("A3")).clone()
		c.Style.Bold = true
		s.place(at("A3"), c)
		return nil
	})
	if c := s.Cell(at("A3")); c == nil || !c.Style.Bold || c.Value.Num != 1 {
		t.Errorf("A3 after bold: %+v", c)
	}
	// Saved: the definition, not the table.
	t2 := roundTrip(t, s)
	if !t2.Notebook() || len(t2.Regions()) != 1 || t2.RegionShown("r1") {
		t.Fatalf("read back: notebook %v, regions %+v", t2.Notebook(), t2.Regions())
	}
	wantShown(t, t2, map[string]string{"A1": "r1  ls   not run", "A3": ""})
	if !t2.Cell(at("A3")).Style.Bold {
		t.Error("the table's formatting wasn't kept")
	}
}

func TestRegionFormulaName(t *testing.T) {
	s := notebook(t)
	w := s.Book()
	other, _ := w.AddSheet("Sheet2", 1)
	if err := other.Set(at("A1"), "=SUM(nu.r1)"); err != nil {
		t.Fatal(err)
	}
	wantShown(t, other, map[string]string{"A1": "#NAME?"})
	addShown(t, s, "r1", "ls", table([]string{"n"}, []string{"1"}, []string{"2"}))
	wantShown(t, other, map[string]string{"A1": "3"})
	s.ShowRegion("r1", table([]string{"n"}, []string{"5"}))
	wantShown(t, other, map[string]string{"A1": "5"})
	if err := w.DefineName("nu.r2", s, rect("A1")); err != nil {
		t.Fatal(err)
	}
	if err := s.AddRegion(Region{Name: "r2", Command: "x"}); err == nil {
		t.Error("a region took a named range's name")
	}
}

// A region pushed down onto rows its table showed, then undone and
// redone, shows its label line alone: nothing of those rows is left
// beside the label.
func TestRegionRedoneOverItsOldRows(t *testing.T) {
	s := notebook(t)
	w := s.Book()
	addShown(t, s, "r1", "ls", table([]string{"a", "b"}, []string{"14", "13"}))
	w.ClearHistory()
	addShown(t, s, "r2", "$r1 | first 2", table([]string{"x", "y", "z"}, []string{"12", "1", "7"}, []string{"2", "10", "0"}))
	s.ShowRegion("r1", table([]string{"a"}, []string{"4"}, []string{"12"}, []string{"8"}, []string{"13"}))
	want := map[string]string{"A8": "r2  $r1 | first 2", "B8": "", "C8": "", "A9": "x", "C11": "0"}
	wantShown(t, s, want)
	for w.CanUndo() {
		w.Undo()
	}
	for w.CanRedo() {
		w.Redo()
	}
	wantShown(t, s, want)
}

// A formula naming a region follows its table as it shrinks, run with
// fewer rows or undone to them, as it does as it grows.
func TestRegionFormulaFollowsShrinking(t *testing.T) {
	s := notebook(t)
	w := s.Book()
	other, _ := w.AddSheet("Sheet2", 1)
	other.Set(at("A1"), "=SUM(nu.r1)")
	addShown(t, s, "r1", "ls", table([]string{"n"}))
	s.ShowRegion("r1", table([]string{"n"}, []string{"7"}, []string{"2"}))
	wantShown(t, other, map[string]string{"A1": "9"})
	s.ShowRegion("r1", table([]string{"n"}, []string{"7"}))
	wantShown(t, other, map[string]string{"A1": "7"})
	w.Undo()
	wantShown(t, other, map[string]string{"A1": "9"})
	w.Undo()
	wantShown(t, other, map[string]string{"A1": "0"})
	w.Redo()
	wantShown(t, other, map[string]string{"A1": "9"})
}

func TestRegionGraph(t *testing.T) {
	s := notebook(t)
	w := s.Book()
	addShown(t, s, "r1", "ls", table([]string{"n"}))
	addShown(t, s, "r2", "$r1 | first", table([]string{"n"}))
	addShown(t, s, "big", "$r2 | append $r1", table([]string{"n"}))
	addShown(t, s, "r3", "$env.PATH | $in", table([]string{"n"}))
	if got := w.RefreshOrder("r1"); !slices.Equal(got, []string{"r1", "r2", "big"}) {
		t.Errorf("refresh r1: %v", got)
	}
	if got := w.RefreshOrder("r2"); !slices.Equal(got, []string{"r2", "big"}) {
		t.Errorf("refresh r2: %v", got)
	}
	if got := w.RunOrder(); !slices.Equal(got, []string{"r1", "r2", "big", "r3"}) {
		t.Errorf("run all: %v", got)
	}
	if err := s.EditRegion("r1", "$big", w.RegionDeps("$big"), ""); err == nil {
		t.Error("a cycle was accepted")
	}
	if err := s.EditRegion("r1", "$r1", w.RegionDeps("$r1"), ""); err == nil {
		t.Error("a region reading itself was accepted")
	}
	if got := RegionRefs("$r1 | where x == $in.a and $nu.home | $r_2.x"); !slices.Equal(got, []string{"r1", "r_2"}) {
		t.Errorf("refs %v", got)
	}
}

func TestRegionFreezeAndSort(t *testing.T) {
	s := notebook(t)
	addShown(t, s, "r1", "ls", table([]string{"n", "s"}, []string{"b", "2"}, []string{"a", "10"}, []string{"c", ""}))
	if err := s.SortRegion("r1", []SortKey{{Col: 1, Desc: true}}); err != nil {
		t.Fatal(err)
	}
	wantShown(t, s, map[string]string{"A3": "a", "A4": "b", "A5": "c"})
	// Running again keeps the order.
	s.ShowRegion("r1", table([]string{"n", "s"}, []string{"x", "1"}, []string{"y", "3"}))
	wantShown(t, s, map[string]string{"A3": "y", "A4": "x"})
	if err := s.FreezeRegion("r1"); err != nil {
		t.Fatal(err)
	}
	wantShown(t, s, map[string]string{"A1": "", "A2": "n", "A3": "y", "B3": "3"})
	if err := s.Set(at("A3"), "z"); err != nil {
		t.Errorf("a frozen cell can't be edited: %v", err)
	}
	s.Book().Undo()
	s.Book().Undo()
	wantShown(t, s, map[string]string{"A1": "r1  ls", "A3": "y"})
}

func TestRegionBlocked(t *testing.T) {
	s := New()
	if err := s.Set(at("B3"), "mine"); err != nil {
		t.Fatal(err)
	}
	if err := s.AddRegion(Region{Name: "feed", Command: "tail", At: at("A1")}); err != nil {
		t.Fatal(err)
	}
	s.ShowRegion("feed", table([]string{"a", "b"}, []string{"1", "2"}, []string{"3", "4"}))
	wantShown(t, s, map[string]string{"A1": "feed  tail   can't show: it would overwrite data in B3", "A2": ""})
	s.Set(at("B3"), "")
	wantShown(t, s, map[string]string{"A1": "feed  tail", "A2": "a", "B3": "2", "B4": "4"})
}
