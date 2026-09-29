package sheet

import (
	"bytes"
	"flag"
	"fmt"
	"maps"
	"math"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"

	"github.com/FelineStateMachine/012/internal/notebook"
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
//
// A region's rows come from outside the undo history, as a notebook
// cell's output or a linked file does: the edits send a region its
// rows (feed), and after undoing, redoing or reopening, each region is
// sent the rows its source gave at that point, as the UI would.
func randomEdits(data []byte, steps int) (string, []string) {
	e := &edits{data: data}
	r := newRandBook(e)
	start, startVals, startFeeds := r.save(), bookValues(r.wb), maps.Clone(r.feeds)
	var log []string
	for range steps {
		if e.done() {
			break
		}
		log = append(log, r.edit(e))
		feedStale(r.wb, r.feeds)
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
	feedAll(r.wb, startFeeds)
	feedStale(r.wb, startFeeds)
	if got := r.save(); !bytes.Equal(got, start) {
		return fmt.Sprintf("undoing %d steps saves differently:\n%s", undone, lineDiff(string(start), string(got))), log
	}
	if d := diffValues(startVals, bookValues(r.wb)); d != "" {
		return fmt.Sprintf("undoing %d steps: %s", undone, d), log
	}
	for r.wb.CanRedo() {
		r.wb.Redo()
	}
	feedAll(r.wb, r.feeds)
	feedStale(r.wb, r.feeds)
	if got := r.save(); !bytes.Equal(got, end) {
		return "redoing saves differently:\n" + lineDiff(string(end), string(got)), log
	}
	if d := diffValues(endVals, bookValues(r.wb)); d != "" {
		return "redoing: " + d, log
	}
	r.wb.RecalcAll()
	feedStale(r.wb, r.feeds)
	if d := diffValues(endVals, bookValues(r.wb)); d != "" {
		return "recalculating everything: " + d, log
	}
	return "", log
}

// randBook is the workbook edited, its notebook tab, and the rows each
// region's source gives now, by region.
type randBook struct {
	wb    *Workbook
	nb    *Sheet
	feeds map[string]LiveOp
}

// newRandBook makes two sheets of values and formulas drawn from e, a
// named range, a notebook tab of a note and a code cell, the code
// cell's output sent to S2, then forgets the history, so undoing
// everything comes back here.
func newRandBook(e *edits) *randBook {
	s := New()
	wb := s.Book()
	s2, _ := wb.AddSheet("S2", 1)
	nb, _ := wb.AddNotebook("N", s2)
	r := &randBook{wb: wb, nb: nb, feeds: map[string]LiveOp{}}
	for _, sh := range []*Sheet{s, s2} {
		for range 14 {
			sh.Set(e.addr(), r.input(e))
		}
	}
	wb.DefineName("Total", s, NewRect(Addr{}, Addr{Col: 1, Row: 9}))
	nb.SetNotebookCells("add cells", []notebook.Cell{{Kind: notebook.Note, Source: "# Files"}, {Source: "r1 = ls"}})
	r.addOutput(s2, "r1", e)
	feedStale(wb, r.feeds)
	wb.ClearHistory()
	return r
}

// addOutput sends the output of the notebook cell name to s at an empty
// cell, and gives it rows.
func (r *randBook) addOutput(s *Sheet, name string, e *edits) error {
	a := e.addr()
	for tries := 0; s.Filled(a) && tries < 20; tries++ {
		a = e.addr()
	}
	if err := s.AddRegion(Region{Name: name, At: a, Output: true}); err != nil {
		return err
	}
	r.feed(name, e)
	return nil
}

// feed has a region's source give it a random table of numbers under a
// header, as running its cell would.
func (r *randBook) feed(name string, e *edits) {
	rows, cols := e.n(5), 1+e.n(3)
	op := LiveOp{Region: name, Reset: true}
	for c := range cols {
		op.Header = append(op.Header, LiveCell{V: Value{Kind: Text, Str: fmt.Sprintf("c%d", c)}})
	}
	for range rows {
		row := make(LiveRow, cols)
		for c := range row {
			row[c] = LiveCell{V: Value{Kind: Number, Num: float64(e.n(20))}}
		}
		op.Rows = append(op.Rows, row)
	}
	r.feeds[name] = op
	sendRows(r.wb, op)
}

// feedStale sends the regions to be sent again (moved, put back by
// undo, no longer blocked) their source's rows, as the UI does after
// every change, until none is: a region sent may free cells another
// was blocked by, or take cells another gives way with.
func feedStale(wb *Workbook, feeds map[string]LiveOp) {
	for range 8 {
		stale := wb.StaleOutputs()
		if len(stale) == 0 {
			return
		}
		for _, name := range stale {
			if op, ok := feeds[name]; ok {
				sendRows(wb, op)
			}
		}
	}
}

// feedAll sends each region wb holds its source's rows.
func feedAll(wb *Workbook, feeds map[string]LiveOp) {
	for _, name := range slices.Sorted(maps.Keys(feeds)) {
		sendRows(wb, feeds[name])
	}
}

// sendRows sends a region its rows. A table widens its columns to its
// text the first time it shows, outside the undo history as its rows
// are, so undoing the region leaves them wide, as intended
// (docs/files/following.md); the edits change widths themselves
// instead.
func sendRows(wb *Workbook, op LiveOp) {
	if s, r, ok := wb.Region(op.Region); ok {
		s.meta(nameKey(r.Name)).fitted = true
	}
	wb.ApplyLive(op)
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

// reopenDiff reads the saved file, sends its regions their rows again,
// and says how it differs from the workbook saved.
func (r *randBook) reopenDiff(file []byte, vals map[string]cellValue) string {
	wb, err := ReadBook(bytes.NewReader(file))
	if err != nil {
		return "reopening: " + err.Error()
	}
	if again := saveBook(wb); !bytes.Equal(again, file) {
		return "saves differently:\n" + lineDiff(string(file), string(again))
	}
	feedAll(wb, r.feeds)
	feedStale(wb, r.feeds)
	return diffValues(vals, bookValues(wb))
}

// cellValue is a cell's value, and what was typed in it to say which.
type cellValue struct {
	v     Value
	input string
}

// bookValues are every filled cell's value, by sheet and address.
func bookValues(wb *Workbook) map[string]cellValue {
	out := map[string]cellValue{}
	for _, s := range wb.sheets {
		for _, a := range s.Addrs() {
			out[s.name+"!"+a.String()] = cellValue{s.Value(a), s.Cell(a).Input}
		}
	}
	return out
}

func diffValues(want, got map[string]cellValue) string {
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
