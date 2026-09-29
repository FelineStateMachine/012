package notebook

import (
	"slices"
	"testing"
)

func TestSplitName(t *testing.T) {
	for _, tc := range []struct{ src, name, pipe string }{
		{"sales = open sales.csv", "sales", "open sales.csv"},
		{"big=\n$sales | first", "big", "\n$sales | first"},
		{"ls | where name == x", "", "ls | where name == x"},
		{"a == b", "", "a == b"},
		{"in = ls", "", "in = ls"},
		{"9x = ls", "", "9x = ls"},
		{"let x = 1", "", "let x = 1"},
		{"__s = 1", "", "__s = 1"},
	} {
		name, p := SplitName(tc.src)
		if name != tc.name || p != tc.pipe {
			t.Errorf("SplitName(%q) = %q, %q", tc.src, name, p)
		}
	}
	if got := WithName("ls", "files"); got != "files = ls" {
		t.Errorf("WithName %q", got)
	}
	if got := WithName("files = ls", ""); got != "ls" {
		t.Errorf("WithName without %q", got)
	}
	if (Cell{Kind: Note, Source: "x = y"}).Name() != "" {
		t.Error("a note cell has a name")
	}
}

func TestRefs(t *testing.T) {
	got := Refs("$r1 | where x == $in.a and $nu.home | $r_2.x | $selection | $sheet.A1")
	if !slices.Equal(got, []string{"r1", "r_2"}) {
		t.Errorf("refs %v", got)
	}
	if !ReadsSelection("$selection | math sum") || ReadsSelection("$selections") {
		t.Error("ReadsSelection")
	}
}

func TestBind(t *testing.T) {
	got, refs := Bind("$sheet.A1:C9 | append $sheet.'Q1 data'!B2:B5 | append $sheet.A1:C9.name | $x$sheet.A1 | $sheet.")
	want := "$__sheet1 | append $__sheet2 | append $__sheet1.name | $x$sheet.A1 | $sheet."
	if got != want {
		t.Errorf("Bind = %q", got)
	}
	if len(refs) != 2 || refs[0].Ref != "A1:C9" || refs[1].Ref != "'Q1 data'!B2:B5" || refs[1].Var != "__sheet2" {
		t.Errorf("refs %+v", refs)
	}
	src := "$sheet.A1:C9 | append $sheet.'Q1 data'!B2:B5 | append $sheet.A1:C9.name | $x$sheet.A1 | $sheet."
	var spans []string
	for _, s := range RangeSpans(src) {
		spans = append(spans, src[s[0]:s[1]])
	}
	if !slices.Equal(spans, []string{"$sheet.A1:C9", "$sheet.'Q1 data'!B2:B5", "$sheet.A1:C9"}) {
		t.Errorf("RangeSpans %q", spans)
	}
}

func cells(srcs ...string) []Cell {
	out := make([]Cell, len(srcs))
	for i, s := range srcs {
		out[i] = Cell{ID: i + 1, Source: s}
	}
	return out
}

func TestOrder(t *testing.T) {
	cs := cells("big = $files | first", "files = ls", "# note", "$big | length", "a = $b", "b = $a")
	cs[2].Kind = Note
	order, cycle := Order(cs, []int{0, 1, 2, 3, 4, 5})
	if !slices.Equal(order, []int{1, 0, 3}) || !slices.Equal(cycle, []int{4, 5}) {
		t.Errorf("order %v cycle %v", order, cycle)
	}
	if got := Dependents(cs, 1); !slices.Equal(got, []int{0, 3}) {
		t.Errorf("dependents %v", got)
	}
	outs := map[int]*Output{2: {NUON: []byte("x")}}
	if got := Inputs(cs, 3, func(id int) *Output { return outs[id] }); !slices.Equal(got, []int{0}) {
		t.Errorf("inputs %v", got)
	}
	dup := cells("x = ls", "X = ps")
	if Taken(dup, 1) == "" || Taken(dup, 0) != "" {
		t.Error("Taken")
	}
}

func TestStale(t *testing.T) {
	cs := cells("files = ls", "big = $files | first", "$big", "ps")
	outs := map[int]*Output{}
	out := func(id int) *Output { return outs[id] }
	run := func(i int, seq int) {
		o := &Output{NUON: []byte("x"), Seq: seq, Count: seq, Source: cs[i].Source, Reads: map[string]int{}}
		for _, n := range Refs(cs[i].Pipeline()) {
			if j, ok := Names(cs)[n]; ok && outs[cs[j].ID] != nil {
				o.Reads[n] = outs[cs[j].ID].Seq
			}
		}
		outs[cs[i].ID] = o
	}
	run(0, 1)
	run(1, 2)
	run(2, 3)
	run(3, 4)
	if st := Stale(cs, out); len(st) != 0 {
		t.Fatalf("stale after running all: %v", st)
	}
	run(0, 5) // files ran again: big and what reads it are stale
	if st := Stale(cs, out); !st[2] || !st[3] || st[1] || st[4] {
		t.Errorf("stale %v", st)
	}
	run(1, 6)
	run(2, 7)
	cs[3].Source = "ps | first" // edited since it ran
	if st := Stale(cs, out); len(st) != 1 || !st[4] {
		t.Errorf("stale after editing %v", st)
	}
}

func TestSettleAndKept(t *testing.T) {
	cs := cells("files = ls", "$files | first", "ps")
	outs := map[int]*Output{1: {NUON: []byte("12345"), Seq: 1}, 2: {NUON: []byte("123"), Seq: 2}, 3: {NUON: []byte("1234567"), Seq: 3}}
	Settle(cs, outs)
	if st := Stale(cs, func(id int) *Output { return outs[id] }); len(st) != 0 {
		t.Errorf("stale after settling: %v", st)
	}
	kept := Kept(cs, func(id int) *Output { return outs[id] }, Caps{Cell: 6, Total: 9})
	if !kept[1] || !kept[2] || kept[3] {
		t.Errorf("kept %v", kept)
	}
}
