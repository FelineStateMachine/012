// Package paged reads linked sources in place: Parquet files and SQLite
// tables or queries too big for any grid, linked as tables on tabs of
// their own (the engine's source.go). Its Host answers what a
// workbook's formulas and pivot tables ask of them (sheet.SourceHost)
// by streaming the sources in the background, keeping the answers until
// a source's file changes; its Pages are the rows a source's tab shows,
// read a page at a time as it scrolls.
//
// The Host and Pages belong to the goroutine that owns the workbook (the
// UI's). What reads files runs as Jobs on other goroutines, over what
// the Job took with it, and hands its result back to be stored, so the
// owner never waits on a file.
package paged

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/FelineStateMachine/012/internal/fileio"
	"github.com/FelineStateMachine/012/internal/sheet"
)

// Linked is a source as the workbook links it, its file resolved.
type Linked struct {
	Name string
	Spec fileio.SourceSpec
}

// Opener opens what a spec names: fileio.OpenSource, or a stand-in.
type Opener func(ctx context.Context, spec fileio.SourceSpec) (fileio.Source, error)

// Host answers a workbook's questions about its linked sources.
type Host struct {
	// Budget is how many cells a function that holds what it reads may
	// read of a source, and a range read whole may have: max-cells.
	Budget int
	// Open opens a source; fileio.OpenSource unless replaced.
	Open Opener
	// Debounce is how long a changed file must hold still before it's
	// read again.
	Debounce time.Duration
	// Now is the clock; tests replace it.
	Now func() time.Time

	srcs    map[string]*source // by name key
	answers map[string]answer  // by question key
	asked   map[string]bool    // questions queued or running
	fresh   []*Job             // jobs not yet handed out
}

// source is a linked source as the host has it.
type source struct {
	name    string
	spec    fileio.SourceSpec
	h       fileio.Source // nil until opened
	shape   sheet.SourceShape
	err     string
	gen     int   // counts openings; answers of earlier ones are stale
	opening bool  // a job is opening it
	read    stamp // the file when opened
	seen    stamp // the file when last polled, different from read
	seenAt  time.Time
}

// stamp is what tells a file changed.
type stamp struct {
	size int64
	mod  time.Time
}

// answer is a stored answer, with the generation of each source it read.
type answer struct {
	a    sheet.SourceAnswer
	gens map[string]int
}

// NewHost is a host with budget max-cells.
func NewHost(budget int) *Host {
	return &Host{Budget: budget, Open: fileio.OpenSource, Debounce: 300 * time.Millisecond, Now: time.Now,
		srcs: map[string]*source{}, answers: map[string]answer{}, asked: map[string]bool{}}
}

func key(name string) string { return strings.ToLower(name) }

// Link makes the host's sources the workbook's: new ones and those
// reading something else are opened, those gone are closed. It returns
// whether anything changed.
func (h *Host) Link(list []Linked) bool {
	changed := false
	keep := map[string]bool{}
	for _, l := range list {
		k := key(l.Name)
		keep[k] = true
		s := h.srcs[k]
		if s != nil && s.spec == l.Spec {
			continue
		}
		if s != nil && s.h != nil {
			s.h.Close()
		}
		gen := 0
		if s != nil {
			gen = s.gen + 1
		}
		s = &source{name: l.Name, spec: l.Spec, gen: gen}
		h.srcs[k] = s
		h.reopen(s)
		changed = true
	}
	for k, s := range h.srcs {
		if !keep[k] {
			if s.h != nil {
				s.h.Close()
			}
			delete(h.srcs, k)
			changed = true
		}
	}
	return changed
}

// reopen queues a job opening s.
func (h *Host) reopen(s *source) {
	s.opening = true
	h.fresh = append(h.fresh, &Job{kind: jobOpen, name: s.name, spec: s.spec, gen: s.gen, open: h.Open})
}

// Reload reads the source named name again, as when its file changed:
// what was worked out of it is worked out again.
func (h *Host) Reload(name string) {
	s := h.srcs[key(name)]
	if s == nil || s.opening {
		return
	}
	if s.h != nil {
		s.h.Close()
		s.h = nil
	}
	s.gen++
	h.reopen(s)
}

// Close lets every source go.
func (h *Host) Close() {
	for _, s := range h.srcs {
		if s.h != nil {
			s.h.Close()
		}
	}
	clear(h.srcs)
}

// Shape is what the host found of the source named name: its shape, or
// why it can't be read, and whether it has been opened.
func (h *Host) Shape(name string) (sheet.SourceShape, string, bool) {
	s := h.srcs[key(name)]
	if s == nil || s.h == nil && s.err == "" {
		return sheet.SourceShape{}, "", false
	}
	return s.shape, s.err, true
}

// Source is the opened source named name, for its tab's pages.
func (h *Host) Source(name string) (fileio.Source, int, bool) {
	s := h.srcs[key(name)]
	if s == nil || s.h == nil {
		return nil, 0, false
	}
	return s.h, s.gen, true
}

// Answer returns the answer to q, or false while it isn't known, queuing
// q to be worked out (sheet.SourceHost).
func (h *Host) Answer(q sheet.SourceQuestion) (sheet.SourceAnswer, bool) {
	k := q.Key()
	if a, ok := h.answers[k]; ok && h.current(a.gens) {
		return a.a, true
	}
	if h.asked[k] {
		return sheet.SourceAnswer{}, false
	}
	j := &Job{kind: jobAnswer, q: q, key: k, budget: h.Budget, srcs: map[string]snapshot{}}
	for _, name := range q.Sources() {
		s := h.srcs[key(name)]
		switch {
		case s == nil:
			return sheet.SourceAnswer{V: sheet.ErrRef, Why: "There's no linked source named " + name}, true
		case s.err != "":
			return sheet.SourceAnswer{V: sheet.ErrRef, Why: s.name + ": " + s.err}, true
		case s.h == nil:
			return sheet.SourceAnswer{}, false // opening: asked again once it's open
		}
		j.srcs[key(name)] = snapshot{name: s.name, h: s.h, shape: s.shape, gen: s.gen}
	}
	h.asked[k] = true
	h.fresh = append(h.fresh, j)
	return sheet.SourceAnswer{}, false
}

// current reports whether answers read with the sources' generations
// gens are still the sources'.
func (h *Host) current(gens map[string]int) bool {
	for k, g := range gens {
		if s := h.srcs[k]; s == nil || s.gen != g {
			return false
		}
	}
	return true
}

// Jobs hands out the jobs queued since the last call, to run in the
// background, each once.
func (h *Host) Jobs() []*Job {
	out := h.fresh
	h.fresh = nil
	return out
}

// Pending reports whether jobs are queued or running.
func (h *Host) Pending() bool { return len(h.fresh) > 0 || len(h.asked) > 0 || h.opening() }

func (h *Host) opening() bool {
	for _, s := range h.srcs {
		if s.opening {
			return true
		}
	}
	return false
}

// Done is what storing a job's result tells the workbook.
type Done struct {
	// Keys are the questions answered.
	Keys []string
	// Opened names a source opened, or failing to, with its shape.
	Opened string
	Shape  sheet.SourceShape
	Err    string
}

// Store keeps what the job j found, run and back on the owner's
// goroutine, and says what the workbook is to be told.
func (h *Host) Store(j *Job) Done {
	if j.kind == jobOpen {
		return h.storeOpen(j)
	}
	delete(h.asked, j.key)
	gens := map[string]int{}
	for k, snap := range j.srcs {
		gens[k] = snap.gen
	}
	if !h.current(gens) {
		return Done{} // a source changed while it ran: asked again
	}
	h.answers[j.key] = answer{a: j.ans, gens: gens}
	return Done{Keys: []string{j.key}}
}

func (h *Host) storeOpen(j *Job) Done {
	s := h.srcs[key(j.name)]
	if s == nil || s.gen != j.gen {
		if j.h != nil {
			j.h.Close()
		}
		return Done{}
	}
	s.opening = false
	if s.h != nil && s.h != j.h {
		s.h.Close()
	}
	s.h, s.shape, s.err, s.read = j.h, j.shape, "", j.stamp
	if j.err != nil {
		s.h, s.shape, s.err = nil, sheet.SourceShape{}, describe(j.err)
	}
	h.forget(key(s.name))
	return Done{Opened: s.name, Shape: s.shape, Err: s.err}
}

// forget drops the answers that read the source with key k.
func (h *Host) forget(k string) {
	for qk, a := range h.answers {
		if _, ok := a.gens[k]; ok {
			delete(h.answers, qk)
		}
	}
}

// describe is an error as the context line says it.
func describe(err error) string {
	if os.IsNotExist(err) {
		return "the file isn't there"
	}
	return err.Error()
}

// Poll looks at each source's file, once they're open, and queues the
// ones that changed and have held still for Debounce to be read again,
// reporting whether it queued any. A file that goes away is noticed the
// same way.
func (h *Host) Poll() bool {
	now, queued := h.Now(), false
	for _, s := range h.srcs {
		if s.opening {
			continue
		}
		st, err := os.Stat(s.spec.Path)
		cur := stamp{}
		if err == nil {
			cur = stamp{st.Size(), st.ModTime()}
		}
		switch {
		case cur == s.read:
			s.seen = stamp{}
			continue
		case cur != s.seen:
			s.seen, s.seenAt = cur, now
			continue
		case now.Sub(s.seenAt) < h.Debounce:
			continue
		}
		h.Reload(s.name) // jobs reading it fail, and their answers are dropped
		queued = true
	}
	return queued
}
