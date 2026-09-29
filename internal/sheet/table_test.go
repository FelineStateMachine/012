package sheet

import (
	"bytes"
	"strings"
	"testing"
)

// salesSheet is a sheet with a table Sales over A1:C4: Region, Units and
// Amount, three rows of data.
func salesSheet(t *testing.T) *Sheet {
	t.Helper()
	s := sheetOf(t, map[string]string{
		"A1": "Region", "B1": "Units", "C1": "Amount",
		"A2": "North", "B2": "2", "C2": "10",
		"A3": "South", "B3": "3", "C3": "20",
		"A4": "East", "B4": "5", "C4": "30",
	})
	if err := s.CreateTable("Sales", rect("A1:C4")); err != nil {
		t.Fatal(err)
	}
	return s
}

// entry is the formula or entry at a.
func entry(s *Sheet, a string) string {
	if c := s.Cell(at(a)); c != nil {
		return c.Input
	}
	return ""
}

func TestTableStructuredReferences(t *testing.T) {
	s := salesSheet(t)
	for a, f := range map[string]string{
		"E1": "=SUM(Sales[Amount])",
		"E2": "=ROWS(Sales[#All])",
		"E3": "=Sales[[#Headers],[Units]]",
		"E4": "=COLUMNS(Sales)*10+ROWS(Sales)",
		"E5": "=SUM(Sales[[Units]:[Amount]])",
		"D2": "=Sales[@Units]*Sales[@Amount]",
		"E6": "=Sales[@Amount]",
		"E7": "=SUM(Sales[Nope])",
		"E8": "=SUM(Nope[Amount])",
		"E9": "=ROWS(Sales[#Data])+ROWS(sales[#headers])",
	} {
		if err := s.Set(at(a), f); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
	}
	wantShown(t, s, map[string]string{
		"E1": "60", "E2": "4", "E3": "Units", "E4": "33", "E5": "70", "D2": "20",
		"E6": "#REF!", "E7": "#REF!", "E8": "#REF!", "E9": "4",
	})
	// Changing a cell of the table recalculates what reads it.
	s.Set(at("C2"), "100")
	wantShown(t, s, map[string]string{"E1": "150", "D2": "200"})
	if deps := s.Dependents(at("C3")); len(deps) == 0 {
		t.Error("no dependents traced through the table")
	}
}

// Rows typed or pasted just below a table join it, in the same undo
// step as the entry.
func TestTableGrows(t *testing.T) {
	s := salesSheet(t)
	s.Set(at("E1"), "=SUM(Sales[Amount])")
	s.Set(at("C5"), "40")
	if tb, _ := s.TableAt(at("C5")); tb.Range != rect("A1:C5") {
		t.Fatalf("table %v", tb.Range)
	}
	wantShown(t, s, map[string]string{"E1": "100"})
	s.Set(at("A7"), "not next to it")
	if _, ok := s.TableAt(at("A7")); ok {
		t.Error("a row with a gap joined the table")
	}
	s.Undo()
	s.Undo()
	if tb, _ := s.TableAt(at("A1")); tb.Range != rect("A1:C4") {
		t.Errorf("undo left %v", tb.Range)
	}
	wantShown(t, s, map[string]string{"E1": "60"})
	clip := s.Copy(rect("A2:C3"))
	if _, err := s.Paste(clip, rect("A5"), false); err != nil {
		t.Fatal(err)
	}
	if tb, _ := s.TableAt(at("A1")); tb.Range != rect("A1:C6") {
		t.Errorf("paste below grew it to %v", tb.Range)
	}
	wantShown(t, s, map[string]string{"E1": "90"})
}

// A header changed in place renames its column in the formulas that
// read it; blank and repeated headers get names of their own.
func TestTableHeaderRenames(t *testing.T) {
	s := salesSheet(t)
	s.Set(at("E1"), "=SUM(Sales[Amount])+Sales[[#Headers],[Units]:[Amount]]")
	s.Set(at("C1"), "Revenue")
	if got := entry(s, "E1"); got != "=SUM(Sales[Revenue])+Sales[[#Headers],[Units]:[Revenue]]" {
		t.Errorf("formula after renaming: %s", got)
	}
	s.Undo()
	if got := entry(s, "E1"); got != "=SUM(Sales[Amount])+Sales[[#Headers],[Units]:[Amount]]" {
		t.Errorf("formula after undo: %s", got)
	}
	s.Set(at("B1"), "")
	s.Set(at("A1"), "Amount")
	tb, _ := s.TableAt(at("A1"))
	if strings.Join(tb.Cols, ",") != "Amount2,Column2,Amount" || entry(s, "A1") != "Amount2" || entry(s, "B1") != "Column2" {
		t.Errorf("columns %v, A1 %q, B1 %q", tb.Cols, entry(s, "A1"), entry(s, "B1"))
	}
}

// Columns inserted, deleted and moved keep formulas reading columns by
// name; a deleted column's references show #REF!.
func TestTableColumnsShift(t *testing.T) {
	s := salesSheet(t)
	s.Set(at("F1"), "=SUM(Sales[Amount])")
	s.Set(at("F2"), "=SUM(Sales[Units])")
	s.InsertCols(1, 1)
	tb, _ := s.TableAt(at("A1"))
	if tb.Range != rect("A1:D4") || strings.Join(tb.Cols, ",") != "Region,Column2,Units,Amount" {
		t.Fatalf("after insert: %v %v", tb.Range, tb.Cols)
	}
	wantShown(t, s, map[string]string{"G1": "60", "G2": "10", "B1": "Column2"})
	s.DeleteCols(2, 1)
	wantShown(t, s, map[string]string{"F1": "60", "F2": "#REF!"})
	if got := entry(s, "F2"); got != "=SUM(Sales[Units])" {
		t.Errorf("formula after delete: %s", got)
	}
	// The whole table moved: formulas follow it.
	if _, err := s.Move(rect("A1:C4"), at("A10")); err != nil {
		t.Fatal(err)
	}
	if tb, _ := s.TableAt(at("A10")); tb.Name != "Sales" || tb.Range != rect("A10:C13") {
		t.Errorf("after move: %v", tb)
	}
	wantShown(t, s, map[string]string{"F1": "60"})
	s.DeleteRows(9, 1)
	if s.HasTables() {
		t.Error("a table kept without its header row")
	}
	wantShown(t, s, map[string]string{"F1": "#REF!"})
}

// Filling a formula down a table reads each row's own cells; copying
// keeps structured references as they are.
func TestTableFillAndCopy(t *testing.T) {
	s := salesSheet(t)
	if err := s.Book().ResizeTable("Sales", rect("A1:D4")); err != nil {
		t.Fatal(err)
	}
	s.Set(at("D1"), "Total")
	s.Set(at("D2"), "=Sales[@Units]*Sales[@Amount]")
	if _, err := s.FillDown(rect("D2:D4")); err != nil {
		t.Fatal(err)
	}
	wantShown(t, s, map[string]string{"D2": "20", "D3": "60", "D4": "150"})
	if got := entry(s, "D4"); got != "=Sales[@Units]*Sales[@Amount]" {
		t.Errorf("filled %s", got)
	}
	clip := s.Copy(rect("D2"))
	s.Paste(clip, rect("H9"), false)
	if got := entry(s, "H9"); got != "=Sales[@Units]*Sales[@Amount]" {
		t.Errorf("pasted %s", got)
	}
	wantShown(t, s, map[string]string{"H9": "#REF!"})
}

func TestTableRenameAndRemove(t *testing.T) {
	s := salesSheet(t)
	w := s.Book()
	s.Set(at("E1"), "=SUM(Sales[Amount])+ROWS(Sales)")
	s.Set(at("D3"), "=Sales[@Amount]")
	if err := w.RenameTable("sales", "Revenue"); err != nil {
		t.Fatal(err)
	}
	if got := entry(s, "E1"); got != "=SUM(Revenue[Amount])+ROWS(Revenue)" {
		t.Errorf("after rename: %s", got)
	}
	if err := w.RemoveTable("Revenue"); err != nil {
		t.Fatal(err)
	}
	if got := entry(s, "E1"); got != "=SUM($C$2:$C$4)+ROWS($A$2:$C$4)" {
		t.Errorf("after remove: %s", got)
	}
	if got := entry(s, "D3"); got != "=$C3" {
		t.Errorf("this row after remove: %s", got)
	}
	wantShown(t, s, map[string]string{"E1": "63", "D3": "20"})
	s.Undo()
	if got := entry(s, "E1"); got != "=SUM(Revenue[Amount])+ROWS(Revenue)" || !s.HasTables() {
		t.Errorf("undo remove: %s", got)
	}
}

func TestTableNames(t *testing.T) {
	s := salesSheet(t)
	w := s.Book()
	if err := s.CreateTable("Other", rect("B2:D6")); err != ErrTableOverlap {
		t.Errorf("overlap: %v", err)
	}
	for _, name := range []string{"Sales", "A1", "nu.x", "", "two words"} {
		if err := s.CreateTable(name, rect("H1:I3")); err == nil {
			t.Errorf("%q named a table", name)
		}
	}
	if err := w.DefineName("sales", s, rect("A1")); err == nil {
		t.Error("a named range took a table's name")
	}
	if w.NextTableName() != "Table1" {
		t.Errorf("next name %s", w.NextTableName())
	}
	if err := s.CreateTable("One", rect("H1:I1")); err != nil {
		t.Fatal(err)
	}
	if tb, _ := s.TableAt(at("H1")); tb.Range != rect("H1:I2") || strings.Join(tb.Cols, ",") != "Column1,Column2" {
		t.Errorf("a one-row table: %v %v", tb.Range, tb.Cols)
	}
}

// A filter on a table's range grows with the table.
func TestTableFilterFollows(t *testing.T) {
	s := salesSheet(t)
	s.CreateFilter(rect("A1:C4"))
	s.Set(at("A5"), "West")
	if r, _ := s.FilterRange(); r != rect("A1:C5") {
		t.Errorf("filter %v", r)
	}
}

func TestTableFileRoundTrip(t *testing.T) {
	s := salesSheet(t)
	w := s.Book()
	w.SetTableStyle("Sales", false, true)
	s.Set(at("E1"), "=SUM(Sales[Amount])")
	var b bytes.Buffer
	if err := s.Write(&b); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), `"version": 6`) || !strings.Contains(b.String(), `"tables": [{"name":"Sales","range":"A1:C4","columns":["Region","Units","Amount"],"header":true}]`) {
		t.Fatalf("file:\n%s", b.String())
	}
	got, err := Read(&b)
	if err != nil {
		t.Fatal(err)
	}
	tb, ok := got.TableAt(at("B2"))
	if !ok || tb.Name != "Sales" || tb.Banded || !tb.Header {
		t.Errorf("read back %+v", tb)
	}
	wantShown(t, got, map[string]string{"E1": "60"})
}

// A notebook cell's output sent to a sheet is a table by the region's
// name; nu.name stays the whole table.
func TestRegionIsTable(t *testing.T) {
	s := New()
	sent(t, s, "app", "B2")
	apply(t, s, LiveOp{Region: "app", Reset: true, Header: liveRow("status", "ms"), Rows: []LiveRow{liveRow("ok", "12"), liveRow("fail", "30")}})
	s.Set(at("A1"), "=SUM(app[ms])")
	s.Set(at("A2"), "=ROWS(app)")
	s.Set(at("A3"), "=ROWS(nu.app)")
	s.Set(at("A4"), "=COUNTIF(app[status],\"ok\")")
	wantShown(t, s, map[string]string{"A1": "42", "A2": "2", "A3": "3", "A4": "1"})
	apply(t, s, LiveOp{Region: "app", Reset: true, Header: liveRow("status", "ms"), Rows: []LiveRow{liveRow("ok", "1")}})
	wantShown(t, s, map[string]string{"A1": "1", "A2": "1"})
	if err := s.CreateTable("app", rect("H1:I3")); err == nil {
		t.Error("a table took a region's name")
	}
}
