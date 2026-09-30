package paged

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/FelineStateMachine/012/internal/fileio"
	"github.com/FelineStateMachine/012/internal/sheet"
)

// runPages runs the pages' jobs until none is left, reporting how many
// ran.
func runPages(p *Pages) int {
	n := 0
	for jobs := p.Jobs(); len(jobs) > 0; jobs = p.Jobs() {
		for _, j := range jobs {
			j.Run(context.Background())
			p.Store(j)
			n++
		}
	}
	return n
}

// openSales writes n rows to a Parquet file and opens it as a source.
func openSales(t *testing.T, n int) fileio.Source {
	t.Helper()
	name := filepath.Join(t.TempDir(), "sales.parquet")
	writeSales(t, name, n, 0)
	h, err := fileio.OpenSource(context.Background(), fileio.SourceSpec{Path: name})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { h.Close() })
	return h
}

func TestPagesKeepTheLatest(t *testing.T) {
	h := openSales(t, 100*PageRows)
	p := NewPages(h, 0, nil)
	defer p.Close()
	if runPages(p) != 1 {
		t.Fatal("the view wasn't built once")
	}
	if n, ok := p.Rows(); !ok || n != 100*PageRows {
		t.Fatalf("%d rows, %v", n, ok)
	}
	for i := range int64(90) {
		row := i * PageRows
		p.Want(row, row)
		if got := runPages(p); got != 1 {
			t.Fatalf("page %d took %d reads", i, got)
		}
		if num, vals, ok := p.Row(row); !ok || num != row || vals[0].V.Num != float64(row) {
			t.Fatalf("row %d: %d %v %v", row, num, vals, ok)
		}
	}
	if len(p.pages) > keepPages {
		t.Errorf("%d pages kept", len(p.pages))
	}
	if _, _, ok := p.Row(0); ok {
		t.Error("the least used page is kept past the budget")
	}
}

func TestPagesSortedAndFiltered(t *testing.T) {
	h := openSales(t, 1000)
	o := &sheet.SourceOrder{
		Sort:   []sheet.SourceSort{{Col: 0, Desc: true}},
		Filter: []sheet.SourceFilter{{Col: 1, Cond: sheet.Condition{Op: sheet.CondExactly, Arg: "north"}}},
	}
	p := NewPages(h, 0, o)
	defer p.Close()
	if !p.Building() {
		t.Fatal("an ordered view isn't built")
	}
	runPages(p)
	n, ok := p.Rows()
	if !ok || n == 0 || n >= 1000 {
		t.Fatalf("%d rows pass, %v", n, ok)
	}
	p.Want(0, n-1)
	runPages(p)
	prev := int64(1 << 62)
	for i := range n {
		num, vals, ok := p.Row(i)
		if !ok || num >= prev || !(vals[1].V.Str == "north" || vals[1].V.Str == "North") {
			t.Fatalf("row %d of the view: %d %v %v", i, num, vals, ok)
		}
		prev = num
	}
	if !p.Same(h, 0, o) || p.Same(h, 0, nil) || !p.Of(h, 0) {
		t.Error("Same and Of")
	}
}
