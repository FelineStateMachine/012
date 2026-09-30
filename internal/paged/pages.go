package paged

import (
	"context"
	"slices"

	"github.com/FelineStateMachine/012/internal/fileio"
	"github.com/FelineStateMachine/012/internal/sheet"
)

// Pages are the rows a source's tab shows, in its view's order (a sort
// and a filter, pushed down to SQL or streamed: fileio.SourceView), read
// a page at a time as the tab scrolls, and at most keepPages of them
// kept, so memory stays bounded whatever the source's size. Building a
// sorted or filtered view reads the whole source once; until it's built,
// and while a page is on its way, the rows it would show aren't known.
type Pages struct {
	h     fileio.Source
	gen   int
	order sheet.SourceOrder

	view     fileio.SourceView
	building bool
	err      string

	pages map[int64]*page // by page number
	used  int64           // counts page reads, for dropping the least used
	want  map[int64]bool  // pages asked for or on their way
	fresh []*PageJob
}

// page is PageRows rows of the view: each one's number in the source
// and its values.
type page struct {
	nums []int64
	rows [][]sheet.LiveCell
	used int64
}

// PageRows is how many rows a page holds; keepPages is how many are
// kept.
const (
	PageRows  = 128
	keepPages = 48
)

// NewPages are the pages of the view of h, the gen'th opening of its
// source, ordered as o says.
func NewPages(h fileio.Source, gen int, o *sheet.SourceOrder) *Pages {
	p := &Pages{h: h, gen: gen, pages: map[int64]*page{}, want: map[int64]bool{}}
	if o != nil {
		p.order = *o
	}
	p.building = true
	p.fresh = append(p.fresh, &PageJob{p: p, build: true})
	return p
}

// Same reports whether the pages are of h's gen'th opening, ordered as o
// says.
func (p *Pages) Same(h fileio.Source, gen int, o *sheet.SourceOrder) bool {
	var ord sheet.SourceOrder
	if o != nil {
		ord = *o
	}
	return p.h == h && p.gen == gen && sameOrder(p.order, ord)
}

// Of reports whether the pages are of h's gen'th opening, however
// ordered.
func (p *Pages) Of(h fileio.Source, gen int) bool { return p.h == h && p.gen == gen }

func sameOrder(a, b sheet.SourceOrder) bool {
	return slices.Equal(a.Sort, b.Sort) && slices.Equal(a.Filter, b.Filter)
}

// Close lets go of the view.
func (p *Pages) Close() {
	if p.view != nil {
		p.view.Close()
	}
}

// Rows counts the rows the view shows, with false while it's built.
func (p *Pages) Rows() (int64, bool) {
	if p.view == nil {
		return 0, false
	}
	return p.view.Rows(), true
}

// Building reports whether the view is being built; Err says why it
// couldn't be.
func (p *Pages) Building() bool { return p.building }
func (p *Pages) Err() string    { return p.err }

// Row is the view's row i: its number in the source and its values,
// with false while its page isn't read, which Want asks for.
func (p *Pages) Row(i int64) (int64, []sheet.LiveCell, bool) {
	pg := p.pages[i/PageRows]
	if pg == nil || int(i%PageRows) >= len(pg.rows) {
		return 0, nil, false
	}
	p.used++
	pg.used = p.used
	return pg.nums[i%PageRows], pg.rows[i%PageRows], true
}

// Want asks for the pages holding rows from through to of the view, to
// be read unless they are.
func (p *Pages) Want(from, to int64) {
	if p.view == nil {
		return
	}
	last := p.view.Rows() - 1
	for n := max(from, 0) / PageRows; n <= min(to, last)/PageRows; n++ {
		if p.pages[n] != nil || p.want[n] {
			continue
		}
		p.want[n] = true
		p.fresh = append(p.fresh, &PageJob{p: p, n: n, view: p.view})
	}
}

// Jobs hands out the reads asked for since the last call, to run in the
// background.
func (p *Pages) Jobs() []*PageJob {
	out := p.fresh
	p.fresh = nil
	return out
}

// Store keeps what a job read, back on the owner's goroutine, and
// reports whether it changed what shows.
func (p *Pages) Store(j *PageJob) bool {
	if j.p != p {
		return false
	}
	if j.build {
		p.building = false
		p.view, p.err = j.view, ""
		if j.err != nil {
			p.err = describe(j.err)
		}
		return true
	}
	if j.view != p.view {
		delete(p.want, j.n)
		return false
	}
	if j.err != nil {
		// Still wanted, so not asked for again until the view is
		// made again: a page that can't be read would be read over
		// and over.
		p.err = describe(j.err)
		return true
	}
	delete(p.want, j.n)
	p.used++ // as the page most lately used, so it isn't the one let go
	p.pages[j.n] = &page{nums: j.nums, rows: j.rows, used: p.used}
	p.drop()
	return true
}

// drop lets the least used pages go past keepPages.
func (p *Pages) drop() {
	for len(p.pages) > keepPages {
		least, at := int64(-1), int64(0)
		for n, pg := range p.pages {
			if least < 0 || pg.used < at {
				least, at = n, pg.used
			}
		}
		delete(p.pages, least)
	}
}

// PageJob builds a view or reads a page of it, on any goroutine.
type PageJob struct {
	p     *Pages
	build bool
	n     int64
	view  fileio.SourceView
	nums  []int64
	rows  [][]sheet.LiveCell
	err   error
}

// Run does the job.
func (j *PageJob) Run(ctx context.Context) {
	if j.build {
		fo := fileio.SourceOrder{}
		for _, s := range j.p.order.Sort {
			fo.Sort = append(fo.Sort, fileio.SourceSort{Col: s.Col, Desc: s.Desc})
		}
		for _, f := range j.p.order.Filter {
			fo.Filter = append(fo.Filter, fileio.SourceFilter{Col: f.Col, Cond: f.Cond})
		}
		j.view, j.err = j.p.h.View(ctx, fo)
		return
	}
	j.nums, j.rows, j.err = j.view.Page(ctx, j.n*PageRows, PageRows)
}
