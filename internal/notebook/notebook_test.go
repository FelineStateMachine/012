package notebook

import (
	"slices"
	"testing"
)

func TestName(t *testing.T) {
	for _, tc := range []struct{ src, name string }{
		{"sales = open sales.csv", "sales"},
		{"big=\n$sales | first", "big"},
		{"ls | where name == x", ""},
		{"a == b", ""},
		{"a =~ b", ""},
		{"in = ls", ""},
		{"9x = ls", ""},
		{"let x = 1", ""},
		{"__s = 1", ""},
		{"files = ls\n$files | length", ""},
		{"files = ls\nn = $files | length", "n"},
		{"n = ls\n| where size > 1kb", "n"},
		{"n = ls |\n  where size > 1kb", "n"},
		{"n = ls\n# keep the big ones\n| where size > 1kb", "n"},
		{"n = ls\n\n| length", ""},
		{"n = ls; $n | length", ""},
		{"n = [\n1\n2\n]", "n"},
		{"n = ls # all of them", "n"},
		{"# n = ls", ""},
	} {
		if got := (Cell{Source: tc.src}).Name(); got != tc.name {
			t.Errorf("Name(%q) = %q, want %q", tc.src, got, tc.name)
		}
	}
	if (Cell{Kind: Note, Source: "x = y"}).Name() != "" {
		t.Error("a note cell has a name")
	}
}

func TestWithName(t *testing.T) {
	for _, tc := range []struct{ src, name, want string }{
		{"ls", "files", "files = ls"},
		{"files = ls", "", "ls"},
		{"files = ls", "all", "all = ls"},
		{"files = ls\n$files | length", "n", "files = ls\nn = $files | length"},
		{"files = ls\nn = $files | length", "", "files = ls\n$files | length"},
		{"", "x", "x = "},
		{"# just a note", "x", "# just a note\nx = "},
	} {
		if got := WithName(tc.src, tc.name); got != tc.want {
			t.Errorf("WithName(%q, %q) = %q, want %q", tc.src, tc.name, got, tc.want)
		}
	}
}

// The owner's cell: files is the cell's own, read by its second line, not
// a cell it reads; the cell has no name, and hands files to later cells.
func TestOwnersCell(t *testing.T) {
	src := "files = ls | where type == file\n$files | where size > 1kb | sort-by size --reverse"
	p := Parse(src)
	if p.Name() != "" || len(p.Refs()) != 0 || !slices.Equal(p.Assigned(), []string{"files"}) {
		t.Errorf("name %q refs %v assigned %v", p.Name(), p.Refs(), p.Assigned())
	}
	cmd, exports, _ := p.Command()
	want := "let files = ls | where type == file\n{__out: ($files | where size > 1kb | sort-by size --reverse), files: $files}"
	if cmd != want || !slices.Equal(exports, []string{"files"}) {
		t.Errorf("command %q exports %v", cmd, exports)
	}
	if got := p.StreamCommand(); got != "let files = ls | where type == file\n$files | where size > 1kb | sort-by size --reverse" {
		t.Errorf("stream command %q", got)
	}
	cs := cells(src, "$files | length")
	order, cycle := Order(cs, []int{0, 1})
	if !slices.Equal(order, []int{0, 1}) || len(cycle) != 0 {
		t.Errorf("order %v cycle %v", order, cycle)
	}
	run, err := Prepare(cs, 0, func(int) *Output { return nil })
	if err != nil || run.Command != want || len(run.Tables) != 0 {
		t.Errorf("prepare %+v %v", run, err)
	}
	// The second cell reads files from the first's run.
	outs := map[int]*Output{1: {NUON: []byte("[]"), Seq: 4, Vars: map[string][]byte{"files": []byte("[[name]; [a]]")}}}
	out := func(id int) *Output { return outs[id] }
	run, err = Prepare(cs, 1, out)
	if err != nil || string(run.Tables["files"]) != "[[name]; [a]]" || run.Reads["files"] != 4 {
		t.Errorf("prepare the reader %+v %v", run, err)
	}
	// An output read from a file keeps no variables: the first cell runs
	// before the second.
	outs[1].Vars = nil
	if got := Inputs(cs, 1, out); !slices.Equal(got, []int{0}) {
		t.Errorf("inputs %v", got)
	}
	if _, err := Prepare(cs, 1, out); err == nil {
		t.Error("prepared a cell reading a variable its cell didn't hand back")
	}
}

func TestComments(t *testing.T) {
	src := "# x = ls\n# $y | $sheet.A1:B2 | $selection\nls # $z\n| where name == \"a#b\" # $w\n| where name != 'c # $v'\n| where name != r#'d # $u'#\n| append [1#2]\n| append $\"($q # $t\n)\""
	p := Parse(src)
	if p.Name() != "" || !slices.Equal(p.Refs(), []string{"q"}) || p.ReadsSelection() || len(p.Ranges) != 0 {
		t.Errorf("name %q refs %v selection %v ranges %v", p.Name(), p.Refs(), p.ReadsSelection(), p.Ranges)
	}
	var comments []string
	for _, c := range p.Comments {
		comments = append(comments, src[c[0]:c[1]])
	}
	if !slices.Equal(comments, []string{"# x = ls", "# $y | $sheet.A1:B2 | $selection", "# $z", "# $w", "# $t"}) {
		t.Errorf("comments %q", comments)
	}
	if len(p.Stmts) != 1 {
		t.Errorf("statements %+v", p.Stmts)
	}
	cmd, _, _ := p.Command()
	if len(cmd) != len(src) || cmd[:8] != "        " {
		t.Errorf("command %q", cmd)
	}
	// Inside brackets a comment may follow a , or a :.
	if got := Parse("[1,#x\n2]").Comments; len(got) != 1 {
		t.Errorf("comments in a list %v", got)
	}
	if got := Parse("echo x,#y").Comments; len(got) != 0 {
		t.Errorf("a comment in a word %v", got)
	}
}

func TestStatements(t *testing.T) {
	src := "a = ls\nb = $a | length; c = $b + 1\nlet d = 2\n$c + $d + $e"
	p := Parse(src)
	if !slices.Equal(p.Assigned(), []string{"a", "b", "c"}) || !slices.Equal(p.Refs(), []string{"e"}) {
		t.Errorf("assigned %v refs %v", p.Assigned(), p.Refs())
	}
	cmd, exports, _ := p.Command()
	want := "let a = ls\nlet b = $a | length; let c = $b + 1\nlet d = 2\n{__out: ($c + $d + $e), a: $a, b: $b, c: $c}"
	if cmd != want || !slices.Equal(exports, []string{"a", "b", "c"}) {
		t.Errorf("command %q exports %v", cmd, exports)
	}
	// The last statement's name is the output: it's not exported apart.
	cmd, exports, _ = Parse("x = 1\nx = $x + 1").Command()
	if cmd != "let x = 1\n$x + 1" || len(exports) != 0 {
		t.Errorf("command %q exports %v", cmd, exports)
	}
	// $x on the line assigning x reads the x from before.
	if got := Parse("x = $x | first").Refs(); !slices.Equal(got, []string{"x"}) {
		t.Errorf("refs %v", got)
	}
}

// A variable of the cell's own shadows another cell's name: the cell
// doesn't read that cell.
func TestLocalShadows(t *testing.T) {
	cs := cells("sales = open sales.csv", "sales = [[a]; [1]]\n$sales | length", "n = $sales | length")
	if got := Reads(cs, Vars(cs), 1); len(got) != 0 {
		t.Errorf("the shadowing cell reads %v", got)
	}
	if got := Reads(cs, Vars(cs), 2); !slices.Equal(got, []int{0}) {
		t.Errorf("the third cell reads %v", got)
	}
	if got := Dependents(cs, 0); !slices.Equal(got, []int{2}) {
		t.Errorf("dependents %v", got)
	}
	if Vars(cs)["sales"] != 0 {
		t.Errorf("sales is the first cell's, as its name: %v", Vars(cs))
	}
	// A cell reading another's name before assigning it reads that cell.
	cs = cells("sales = ls", "big = $sales | first\nsales = 1\n$sales")
	if got := Reads(cs, Vars(cs), 1); !slices.Equal(got, []int{0}) {
		t.Errorf("reads %v", got)
	}
}

func TestRanges(t *testing.T) {
	src := "$sheet.A1:C9 | append $sheet.'Q1 data'!B2:B5 | append $sheet.A1:C9.name | $x$sheet.A1 | $sheet. | append \"$sheet.A1\""
	p := Parse(src)
	var spans []string
	for _, s := range p.Ranges {
		spans = append(spans, src[s[0]:s[1]])
	}
	if !slices.Equal(spans, []string{"$sheet.A1:C9", "$sheet.'Q1 data'!B2:B5", "$sheet.A1:C9"}) {
		t.Errorf("ranges %q", spans)
	}
	got, _, refs := p.Command()
	want := "$__sheet1 | append $__sheet2 | append $__sheet1.name | $x$sheet.A1 | $sheet. | append \"$sheet.A1\""
	if got != want {
		t.Errorf("command = %q", got)
	}
	if len(refs) != 2 || refs[0].Ref != "A1:C9" || refs[1].Ref != "'Q1 data'!B2:B5" || refs[1].Var != "__sheet2" {
		t.Errorf("refs %+v", refs)
	}
	if !slices.Equal(p.Refs(), []string{"x"}) {
		t.Errorf("refs %v", p.Refs())
	}
	if !Parse("$selection | math sum").ReadsSelection() || Parse("$selections").ReadsSelection() {
		t.Error("ReadsSelection")
	}
	if got := Parse("$r1 | where x == $in.a and $nu.home | $r_2.x | $selection | $sheet.A1").Refs(); !slices.Equal(got, []string{"r1", "r_2"}) {
		t.Errorf("refs %v", got)
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
		for _, n := range cs[i].Parse().Refs() {
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
