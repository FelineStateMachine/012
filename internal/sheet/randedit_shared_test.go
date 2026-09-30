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
// them. Then the authors undo, in turns drawn from the seed, each only
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

// randAuthors is how many participants edit.
const randAuthors = 3

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
		a := 1 + e.n(randAuthors)
		r.wb.SetAuthor(a)
		log = append(log, fmt.Sprintf("%d: %s", a, r.edit(e)))
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
	first := rng.IntN(randAuthors)
	left := false
	for i := range randAuthors {
		a := 1 + (first+i)%randAuthors
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
