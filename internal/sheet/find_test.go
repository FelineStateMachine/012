package sheet

import (
	"slices"
	"testing"
)

func findSheet() *Sheet {
	s := New()
	for a, in := range map[string]string{
		"A1": "Rent", "B1": "1450",
		"A2": "rental car", "B2": "=B1*2",
		"A3": "'2900", "B3": "=SUM(B1:B2)",
		"C1": "Rent", "C2": "Total rent: $5",
	} {
		s.Set(at(a), in)
	}
	return s
}

func names(as []Addr) []string {
	out := make([]string, len(as))
	for i, a := range as {
		out[i] = a.String()
	}
	return out
}

func TestFind(t *testing.T) {
	s := findSheet()
	b1b2 := NewRect(at("B1"), at("B2"))
	tests := []struct {
		query string
		o     FindOptions
		want  []string
	}{
		{"rent", FindOptions{}, []string{"A1", "C1", "A2", "C2"}}, // reading order
		{"Rent", FindOptions{MatchCase: true}, []string{"A1", "C1"}},
		{"rent", FindOptions{WholeCell: true}, []string{"A1", "C1"}},
		{"2900", FindOptions{}, []string{"B2", "A3"}}, // formula results and forced text
		{"B1", FindOptions{}, nil},                    // formula text isn't searched by default
		{"B1", FindOptions{InFormulas: true}, []string{"B2", "B3"}},
		{`^r\w+`, FindOptions{Regex: true}, []string{"A1", "C1", "A2"}},
		{"$5", FindOptions{}, []string{"C2"}}, // plain queries are literal
		{"1450", FindOptions{Within: &b1b2}, []string{"B1"}},
		{"", FindOptions{}, nil},
	}
	for _, tt := range tests {
		got, err := s.Find(tt.query, tt.o)
		if err != nil {
			t.Errorf("Find(%q): %v", tt.query, err)
			continue
		}
		if !slices.Equal(names(got), tt.want) {
			t.Errorf("Find(%q, %+v) = %v, want %v", tt.query, tt.o, names(got), tt.want)
		}
	}
	if _, err := s.Find("(", FindOptions{Regex: true}); err == nil {
		t.Error("invalid regex accepted")
	}
}

func TestReplace(t *testing.T) {
	s := findSheet()
	n, err := s.ReplaceAll("rent", "Lease", FindOptions{})
	if err != nil || n != 4 {
		t.Fatalf("ReplaceAll = %d, %v", n, err)
	}
	for a, want := range map[string]string{"A1": "Lease", "A2": "Leaseal car", "C2": "Total Lease: $5"} {
		if got := s.Cell(at(a)).Input; got != want {
			t.Errorf("%s = %q, want %q", a, got, want)
		}
	}

	// Formulas only change when searching within them, and keep working.
	if n, _ := s.ReplaceAll("B1", "A9", FindOptions{}); n != 0 {
		t.Errorf("replaced %d formula results", n)
	}
	s.Set(at("A9"), "10")
	if n, _ := s.ReplaceAll("B1", "A9", FindOptions{InFormulas: true}); n != 2 {
		t.Errorf("replaced %d formulas", n)
	}
	if got := s.Value(at("B2")).Num; got != 20 {
		t.Errorf("B2 = %v after rewriting its reference", got)
	}

	// Regex groups; forced text stays text; literal $ in plain mode.
	s.Set(at("D1"), "2026-09-26")
	s.Replace(at("D1"), `(\d+)-(\d+)-(\d+)`, "$3/$2/$1", FindOptions{Regex: true})
	if got := s.Cell(at("D1")).Input; got != "26/09/2026" {
		t.Errorf("regex replace = %q", got)
	}
	s.Replace(at("A3"), "2900", "3000", FindOptions{})
	if c := s.Cell(at("A3")); c.Input != "'3000" || c.Value.Kind != Text {
		t.Errorf("forced text = %+v", c)
	}
	s.Set(at("C1"), "Rent due")
	s.Replace(at("C1"), "Rent", "$1", FindOptions{})
	if got := s.Cell(at("C1")).Input; got != "$1 due" {
		t.Errorf("plain replacement = %q, want a literal $1", got)
	}

	// A replacement that breaks a formula is an error and changes nothing.
	s.Set(at("E1"), "=SUM(B1:B2)")
	if _, err := s.Replace(at("E1"), ")", "", FindOptions{InFormulas: true}); err == nil {
		t.Error("broken formula accepted")
	}
	if got := s.Cell(at("E1")).Input; got != "=SUM(B1:B2)" {
		t.Errorf("E1 changed to %q", got)
	}
}
