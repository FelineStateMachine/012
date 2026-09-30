package sheet

import (
	"bytes"
	"fmt"
	"maps"
	"math/rand/v2"
	"testing"
)

// Random edits by several participants: the edits of TestRandomEdits,
// each made by one of three authors drawn from the seed, interleaved in
// one workbook whose history is shared, as a room of 012 serve orders
// them, and among them the agent's (live mode): its edits are proposals
// (Propose), accepted whole, cell by cell or not at all, as drawn from
// the seed. Then the authors, the agent among them, undo, in turns drawn from the seed, each only
// their own steps, from under the others' where nothing later overlaps
// them. What must hold:
//
//   - someone can always undo (the latest step's author, at least), so
//     every step is undone in the end;
//   - an undo that goes ahead, redone at once by its author, gives back
//     exactly the file it undid;
//   - once every step is undone, whatever the order, the workbook is
//     back where it started: the file it saves and every value.
func TestRandomSharedEdits(t *testing.T) {
	seeds := max(*randSeeds/4, 1)
	if testing.Short() {
		seeds = min(seeds, 20)
	}
	for seed := range seeds {
		rng := rand.New(rand.NewPCG(uint64(seed), 34))
		data := make([]byte, 64+(*randSteps)*10)
		for i := range data {
			data[i] = byte(rng.Uint32())
		}
		t.Run(fmt.Sprint(seed), func(t *testing.T) {
			msg, log := randomSharedEdits(data, *randSteps, rng)
			if msg != "" {
				t.Fatalf("%s\nafter %d edits:\n  %s", msg, len(log), joinLog(log))
			}
		})
	}
}

// randAuthors is how many people edit; the agent is one more.
const (
	randAuthors = 3
	randAgent   = randAuthors + 1
)

// suggest has the agent propose an edit, made on a copy, and accepts it
// whole, some of its cells or none, as rng draws.
func (r *randBook) suggest(e *edits, rng *rand.Rand) string {
	from := *e
	var label string
	edit := func(w *Workbook) error {
		b := r
		if w != r.wb {
			b = &randBook{wb: w, nb: w.Lookup(r.nb.Name()), feeds: maps.Clone(r.feeds)}
		}
		d := from
		label = b.edit(&d)
		*e = d
		return nil
	}
	p, err := Propose(r.wb, "suggestion", edit)
	if err != nil {
		return fmt.Sprintf("%d suggests %q: %v", randAgent, label, err)
	}
	var pick func(int) bool
	what := "accepted"
	switch rng.IntN(3) {
	case 0:
		return fmt.Sprintf("%d suggests %q: rejected", randAgent, label)
	case 1:
		if !p.Whole() {
			pick, what = func(int) bool { return rng.IntN(2) == 0 }, "accepted in part"
		}
	}
	if err := p.Apply(r.wb, pick); err != nil {
		what = err.Error()
	}
	return fmt.Sprintf("%d suggests %q: %s", randAgent, label, what)
}

func randomSharedEdits(data []byte, steps int, rng *rand.Rand) (string, []string) {
	e := &edits{data: data}
	r := newRandBook(e)
	r.wb.ShareHistory()
	start, startVals, startFeeds := r.save(), bookValues(r.wb), maps.Clone(r.feeds)
	var log []string
	for range steps {
		if e.done() {
			break
		}
		a := 1 + e.n(randAuthors+1)
		r.wb.SetAuthor(a)
		if a == randAgent {
			log = append(log, r.suggest(e, rng))
		} else {
			log = append(log, fmt.Sprintf("%d: %s", a, r.edit(e)))
		}
		feedStale(r.wb, r.feeds)
	}
	for undone := 0; ; undone++ {
		a, ok := r.nextUndo(rng)
		if !ok {
			break
		}
		r.wb.SetAuthor(a)
		label := r.wb.UndoLabel()
		before := r.save()
		if _, ok := r.wb.Undo(); !ok {
			return fmt.Sprintf("author %d's undo of %q refused though nothing blocks it", a, label), log
		}
		feedStale(r.wb, r.feeds)
		log = append(log, fmt.Sprintf("%d undoes %q", a, label))
		if rng.IntN(3) > 0 {
			continue
		}
		if _, ok := r.wb.Redo(); !ok {
			return fmt.Sprintf("author %d can't redo %q at once", a, label), log
		}
		feedStale(r.wb, r.feeds)
		if got := r.save(); !bytes.Equal(got, before) {
			return fmt.Sprintf("author %d's redo of %q saves differently:\n%s", a, label, lineDiff(string(before), string(got))), log
		}
		r.wb.Undo()
		feedStale(r.wb, r.feeds)
	}
	feedAll(r.wb, startFeeds)
	feedStale(r.wb, startFeeds)
	if got := r.save(); !bytes.Equal(got, start) {
		return "undoing every step saves differently:\n" + lineDiff(string(start), string(got)), log
	}
	if d := diffValues(startVals, bookValues(r.wb)); d != "" {
		return "undoing every step: " + d, log
	}
	return "", log
}

// nextUndo picks an author who can undo a step now, in a turn drawn
// from rng, or reports false when nobody has a step left. It fails the
// run when steps are left but nobody can undo one.
func (r *randBook) nextUndo(rng *rand.Rand) (int, bool) {
	first := rng.IntN(randAgent)
	left := false
	for i := range randAgent {
		a := 1 + (first+i)%randAgent
		r.wb.SetAuthor(a)
		if !r.wb.CanUndo() {
			continue
		}
		left = true
		if _, blocked := r.wb.UndoBlocked(); !blocked {
			return a, true
		}
	}
	if left {
		panic("steps are left that nobody can undo")
	}
	return 0, false
}

func joinLog(log []string) string {
	var b bytes.Buffer
	for i, l := range log {
		if i > 0 {
			b.WriteString("\n  ")
		}
		b.WriteString(l)
	}
	return b.String()
}
