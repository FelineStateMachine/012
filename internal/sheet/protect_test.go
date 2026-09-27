package sheet

import (
	"bytes"
	"strings"
	"testing"
)

func TestProtect(t *testing.T) {
	s := New()
	s.Protect(Protection{Range: rng("B2:C4"), Desc: "Totals"})
	if p, ok := s.Protecting(rng("C4:D9")); !ok || p.Desc != "Totals" {
		t.Errorf("Protecting overlap = %+v, %v", p, ok)
	}
	if _, ok := s.Protecting(rng("A1:A9")); ok {
		t.Error("A1:A9 protected")
	}
	// Protections follow inserted and deleted rows, and undo.
	s.InsertRows(0, 1)
	if got := s.Protections(); len(got) != 1 || got[0].Range != rng("B3:C5") {
		t.Errorf("after insert %+v", got)
	}
	s.DeleteRows(2, 3)
	if got := s.Protections(); len(got) != 0 {
		t.Errorf("all rows deleted, still %+v", got)
	}
	s.Undo()
	s.Undo()
	if got := s.Protections(); len(got) != 1 || got[0].Range != rng("B2:C4") {
		t.Errorf("after undo %+v", got)
	}
	// The whole sheet: everything is protected, once.
	s.Protect(Protection{Sheet: true})
	s.Protect(Protection{Sheet: true, Desc: "Final"})
	if got := s.Protections(); len(got) != 2 || got[1].Label() != "Final" {
		t.Errorf("sheet protections %+v", got)
	}
	if _, ok := s.Protecting(rng("Z99")); !ok {
		t.Error("sheet protection doesn't cover Z99")
	}
	if n := s.UnprotectRange(rng("A1")); n != 1 {
		t.Errorf("UnprotectRange(A1) = %d", n)
	}
	s.Unprotect(0)
	if len(s.Protections()) != 0 {
		t.Errorf("left %+v", s.Protections())
	}
	s.Undo()
	if len(s.Protections()) != 1 {
		t.Errorf("undo unprotect %+v", s.Protections())
	}
}

func TestProtectFile(t *testing.T) {
	s := New()
	s.Set(at("A1"), "x")
	s.Protect(Protection{Range: rng("A1:B2"), Desc: "Inputs"})
	s.Protect(Protection{Sheet: true})
	var b bytes.Buffer
	if err := s.Write(&b); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), `"protected": [{"range":"A1:B2","description":"Inputs"},{"sheet":true}]`) || !strings.Contains(b.String(), `"version": 2`) {
		t.Errorf("file:\n%s", b.String())
	}
	got, err := Read(&b)
	if err != nil {
		t.Fatal(err)
	}
	if ps := got.Protections(); len(ps) != 2 || ps[0].Range != rng("A1:B2") || !ps[1].Sheet {
		t.Errorf("read back %+v", ps)
	}
}
