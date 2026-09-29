package sheet

import (
	"strings"
	"testing"
)

// links writes links as they'd be listed: the range, with its sheet when
// not s, and the kind's name where it isn't a plain reference.
func links(s *Sheet, ls []Link) string {
	var out []string
	for _, l := range ls {
		ref := l.Range.String()
		if l.Sheet != s {
			ref = Qualified(l.Sheet.Name(), l.Range)
		}
		switch l.Kind {
		case LinkName, LinkRegion, LinkFile, LinkOutput:
			ref = l.Name + "=" + ref
		case LinkSpill:
			ref = "spill " + ref
		}
		out = append(out, ref)
	}
	return strings.Join(out, " ")
}

func TestPrecedentLinks(t *testing.T) {
	s := sheetOf(t, map[string]string{
		"A1": "1", "A2": "2", "B1": "=SEQUENCE(3)", "C1": "=Rate*A1+SUM(B1:B3)", "C2": "=missing!A1+A1",
	})
	s.DefineName("Rate", rect("A2"))
	if got, want := links(s, s.PrecedentLinks(at("C1"))), "Rate=A2 A1 B1:B3"; got != want {
		t.Errorf("C1 = %q, want %q", got, want)
	}
	if got, want := links(s, s.PrecedentLinks(at("C2"))), "A1"; got != want {
		t.Errorf("an unknown sheet: %q, want %q", got, want)
	}
	// A spilled cell comes from its anchor.
	if got, want := links(s, s.PrecedentLinks(at("B3"))), "spill B1"; got != want {
		t.Errorf("B3 = %q, want %q", got, want)
	}
	if got := s.PrecedentLinks(at("A1")); got != nil {
		t.Errorf("a number: %v", got)
	}
}

func TestDependentLinks(t *testing.T) {
	s := sheetOf(t, map[string]string{
		"A1": "=SEQUENCE(3)", "C1": "=A2*2", "C2": "=SUM(A1:A9)", "C3": "=A1", "D5": "=B1",
	})
	other, _ := s.Book().AddSheet("Other", 1)
	other.Set(at("A1"), "=Sheet1!A3")
	s.DefineName("Mid", rect("A2"))
	other.Set(at("B1"), "=Mid")
	// The anchor's dependents read any cell it spills into.
	got, more := s.DependentLinks(at("A1"), 0)
	if want := "spill A1:A3 C1 C2 C3 Other!A1 Other!B1"; links(s, got) != want || more {
		t.Errorf("A1 = %q (more %v), want %q", links(s, got), more, want)
	}
	if got, _ := s.DependentLinks(at("A2"), 0); links(s, got) != "C1 C2 Other!B1" {
		t.Errorf("A2 = %q", links(s, got))
	}
	// A cap finds that many and says there are more.
	got, more = s.DependentLinks(at("A1"), 2)
	if len(got) != 3 || !more {
		t.Errorf("capped: %q, more %v", links(s, got), more)
	}
	if !s.Reads(at("C2"), s, rect("A3")) || !other.Reads(at("B1"), s, rect("A1:A2")) || s.Reads(at("D5"), s, rect("A1:A3")) {
		t.Error("Reads")
	}
	if s.Reads(at("A2"), s, rect("A1")) {
		t.Error("a spilled cell reads its anchor")
	}
}

func TestTraceRegions(t *testing.T) {
	s := New()
	nb := sent(t, s, "files", "B2")
	s.Set(at("A1"), "=ROWS(nu.files)")
	apply(t, s, LiveOp{Region: "files", Reset: true, Header: liveRow("n"), Rows: []LiveRow{liveRow("a"), liveRow("b")}})
	if got, want := links(s, s.PrecedentLinks(at("A1"))), "nu.files=B2:B4"; got != want {
		t.Errorf("A1 = %q, want %q", got, want)
	}
	// The region's cells come from the notebook cell.
	if got, want := links(s, s.PrecedentLinks(at("B3"))), "files="+Qualified(nb.Name(), rect("A1")); got != want {
		t.Errorf("B3 = %q, want %q", got, want)
	}
	if got, _ := s.DependentLinks(at("B4"), 0); links(s, got) != "A1" {
		t.Errorf("B4's dependents = %q", links(s, got))
	}
	if !s.Reads(at("A1"), s, rect("B3")) {
		t.Error("Reads through a region")
	}
	if _, err := s.AddLinked(at("F1"), LinkSource{Path: "log.csv"}); err != nil {
		t.Fatal(err)
	}
	if got, want := links(s, s.PrecedentLinks(at("F1"))), "log.csv=F1"; got != want {
		t.Errorf("F1 = %q, want %q", got, want)
	}
}
