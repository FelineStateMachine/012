package sheet

import (
	"bytes"
	"flag"
	"fmt"
	"math"
	"math/rand/v2"
	"strings"
	"testing"
)

// Random edits check invariants that hold between features rather than
// within one: a sequence of edits, pastes, fills, sorts, inserts and
// deletes, formats, merges, moves, notes, rules, names, sheets and
// region runs, drawn from a seed, must leave a workbook that
//
//   - undoes, step by step, to exactly where it started (the file it
//     saves and every value), and redoes to where it ended;
//   - saves and reopens to the same file and the same values (reopening
//     recalculates everything from scratch, so this is also a full
//     recalculation agreeing with the incremental one);
//   - recalculates in place, everything at once, to the same values.
//
// go test runs a fixed set of seeds in a second or two; -randedit.seeds
// runs more, and FuzzRandomEdits lets the fuzzer choose the edits.
var (
	randSeeds = flag.Int("randedit.seeds", 1000, "seeds TestRandomEdits runs")
	randSteps = flag.Int("randedit.steps", 40, "edits per seed")
)

func TestRandomEdits(t *testing.T) {
	seeds := *randSeeds
	if testing.Short() {
		seeds = min(seeds, 50)
	}
	for seed := range seeds {
		rng := rand.New(rand.NewPCG(uint64(seed), 12))
		data := make([]byte, 64+(*randSteps)*8)
		for i := range data {
			data[i] = byte(rng.Uint32())
		}
		t.Run(fmt.Sprint(seed), func(t *testing.T) { checkRandomEdits(t, data, *randSteps) })
	}
}

// FuzzRandomEdits reads the edits from the fuzzer's bytes: go test
// -run '^$' -fuzz FuzzRandomEdits ./internal/sheet.
func FuzzRandomEdits(f *testing.F) {
	f.Add([]byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20})
	f.Fuzz(func(t *testing.T, data []byte) { checkRandomEdits(t, data, 60) })
}

// edits draws choices from bytes; past their end every choice is 0.
type edits struct {
	data []byte
	i    int
}

func (e *edits) n(k int) int {
	if k <= 1 || e.i >= len(e.data) {
		return 0
	}
	b := int(e.data[e.i])
	e.i++
	return b % k
}

func (e *edits) done() bool { return e.i >= len(e.data) }

// Edits stay in a small corner, so they meet: columns A to H, rows 1 to
// 20.
const randCols, randRows = 8, 20

func (e *edits) addr() Addr { return Addr{Col: e.n(randCols), Row: e.n(randRows)} }

func (e *edits) rect() Rect {
	a := e.addr()
	return NewRect(a, Addr{Col: min(a.Col+e.n(3), randCols-1), Row: min(a.Row+e.n(5), randRows-1)})
}

// checkRandomEdits applies up to steps edits from data and checks the
// invariants. When one fails, it reports the fewest of the edits that
// still fail, with the log of them.
func checkRandomEdits(t *testing.T, data []byte, steps int) {
	t.Helper()
	msg, log := randomEdits(data, steps)
	if msg == "" {
		return
	}
	for n := 1; n < len(log); n++ {
		if m, l := randomEdits(data, n); m != "" {
			msg, log = m, l
			break
		}
	}
	t.Fatalf("%s\nafter %d edits:\n  %s", msg, len(log), strings.Join(log, "\n  "))
}

// randomEdits builds a workbook, applies up to steps edits from data,
// and says which invariant fails, if one does, and the edits made.
func randomEdits(data []byte, steps int) (string, []string) {
	e := &edits{data: data}
	r := newRandBook(e)
	start, startVals := r.save(), bookValues(r.wb)
	var log []string
	for range steps {
		if e.done() {
			break
		}
		log = append(log, r.edit(e))
	}
	end, endVals := r.save(), bookValues(r.wb)
	if d := r.reopenDiff(end, endVals); d != "" {
		return "reopening differs: " + d, log
	}
	undone := 0
	for r.wb.CanUndo() {
		r.wb.Undo()
		undone++
	}
	if got := r.save(); !bytes.Equal(got, start) {
		return fmt.Sprintf("undoing %d steps saves differently:\n%s", undone, lineDiff(string(start), string(got))), log
	}
	if d := diffValues(startVals, bookValues(r.wb)); d != "" {
		return fmt.Sprintf("undoing %d steps: %s", undone, d), log
	}
	for r.wb.CanRedo() {
		r.wb.Redo()
	}
	if got := r.save(); !bytes.Equal(got, end) {
		return "redoing saves differently:\n" + lineDiff(string(end), string(got)), log
	}
	if d := diffValues(endVals, bookValues(r.wb)); d != "" {
		return "redoing: " + d, log
	}
	r.wb.RecalcAll()
	if d := diffValues(endVals, bookValues(r.wb)); d != "" {
		return "recalculating everything: " + d, log
	}
	return "", log
}

// randBook is the workbook edited, and the tables its regions show,
// which a file doesn't keep.
type randBook struct {
	wb *Workbook
	nb *Sheet
}

// newRandBook makes two sheets of values and formulas drawn from e, a
// named range, and a notebook with a region shown, then forgets the
// history, so undoing everything comes back here.
func newRandBook(e *edits) *randBook {
	s := New()
	wb := s.Book()
	s2, _ := wb.AddSheet("S2", 1)
	nb, _ := wb.AddNotebook("N", s2)
	r := &randBook{wb: wb, nb: nb}
	for _, sh := range []*Sheet{s, s2} {
		for range 14 {
			sh.Set(e.addr(), r.input(e))
		}
	}
	wb.DefineName("Total", s, NewRect(Addr{}, Addr{Col: 1, Row: 9}))
	nb.AddRegion(Region{Name: "r1", Command: "ls"})
	r.run("r1", e)
	wb.ClearHistory()
	return r
}

// run shows a random table of numbers in a region, as running it would.
func (r *randBook) run(name string, e *edits) {
	d := &RegionData{Rows: 1 + e.n(5), Cols: 1 + e.n(3)}
	d.Values = make([]Value, d.Rows*d.Cols)
	for i := range d.Values {
		if i < d.Cols {
			d.Values[i] = Value{Kind: Text, Str: fmt.Sprintf("c%d", i)}
		} else {
			d.Values[i] = Value{Kind: Number, Num: float64(e.n(20))}
		}
	}
	r.nb.ShowRegion(name, d)
}

func (r *randBook) save() []byte { return saveBook(r.wb) }

// saveBook is the file wb saves. Saving into memory fails only on a
// bug, which the test then panics with.
func saveBook(wb *Workbook) []byte {
	var b bytes.Buffer
	if err := wb.Write(&b); err != nil {
		panic(err)
	}
	return b.Bytes()
}

// reopenDiff reads the saved file, runs its regions again with the
// tables they showed, and says how it differs from the workbook saved.
func (r *randBook) reopenDiff(file []byte, vals map[string]shown) string {
	wb, err := ReadBook(bytes.NewReader(file))
	if err != nil {
		return "reopening: " + err.Error()
	}
	if again := saveBook(wb); !bytes.Equal(again, file) {
		return "saves differently:\n" + lineDiff(string(file), string(again))
	}
	for _, s := range wb.sheets {
		for _, rg := range s.regions.list {
			if d := r.nb.regions.data[nameKey(rg.Name)]; d != nil && !rg.Linked() {
				s.ShowRegion(rg.Name, d)
			}
		}
	}
	return diffValues(vals, bookValues(wb))
}

// shown is a cell's value, and what was typed in it to say which.
type shown struct {
	v     Value
	input string
}

// bookValues are every filled cell's value, by sheet and address.
func bookValues(wb *Workbook) map[string]shown {
	out := map[string]shown{}
	for _, s := range wb.sheets {
		for _, a := range s.Addrs() {
			out[s.name+"!"+a.String()] = shown{s.Value(a), s.Cell(a).Input}
		}
	}
	return out
}

func diffValues(want, got map[string]shown) string {
	var b strings.Builder
	for k, w := range want {
		if g, ok := got[k]; !ok || !sameValue(w.v, g.v) {
			fmt.Fprintf(&b, "\n  %s: want %v (%q), got %v (%q)", k, w.v, w.input, g.v, g.input)
		}
	}
	for k, g := range got {
		if _, ok := want[k]; !ok {
			fmt.Fprintf(&b, "\n  %s: want nothing, got %v (%q)", k, g.v, g.input)
		}
	}
	return b.String()
}

func sameValue(a, b Value) bool {
	return a.Kind == b.Kind && a.Str == b.Str && (a.Num == b.Num || math.IsNaN(a.Num) && math.IsNaN(b.Num))
}
