package sheet

import (
	"fmt"
	"strings"

	"github.com/FelineStateMachine/012/internal/functions"
)

// What formulas and pivot tables ask of linked sources (source.go). The
// engine never reads a file: it describes each question and asks the
// workbook's SourceHost, which answers from what it keeps and queues
// the rest, worked out in the background by streaming the source (the
// UI's; headless answers at once). Until an answer arrives the cell
// shows Loading…, as a JEV question does, and when answers arrive the
// UI calls RecalcSourced, which recomputes what waited for them. A
// source's answers are kept until its file changes (SourceChanged).

// SourceHost answers the questions of a workbook's formulas and pivot
// tables about its linked sources.
type SourceHost interface {
	// Answer returns the answer to q, or false while it isn't known,
	// queuing q to be worked out.
	Answer(q SourceQuestion) (SourceAnswer, bool)
}

// AskKind is what a SourceQuestion asks.
type AskKind uint8

const (
	// AskCell asks a cell's value: At, on the source's tab.
	AskCell AskKind = iota
	// AskRange asks the values of a range, R, as an array: read whole,
	// so no bigger than the host's budget.
	AskRange
	// AskCall asks what a function computes, Call, its ranges named by
	// their sources' names.
	AskCall
	// AskPivot asks a pivot table's groups over the source, Pivot.
	AskPivot
)

// SourceQuestion is one question about a linked source. Source is the
// source's name, the one whose file changing makes the answer stale; a
// call may read others too, named in its arguments.
type SourceQuestion struct {
	Kind   AskKind
	Source string
	At     Addr
	R      Rect
	Call   functions.StreamCall
	Pivot  Pivot
}

// Key identifies the question by its content.
func (q SourceQuestion) Key() string {
	switch q.Kind {
	case AskCell:
		return fmt.Sprintf("cell\x1f%s\x1f%d:%d", nameKey(q.Source), q.At.Col, q.At.Row)
	case AskRange:
		return fmt.Sprintf("range\x1f%s\x1f%d:%d:%d:%d", nameKey(q.Source), q.R.From.Col, q.R.From.Row, q.R.To.Col, q.R.To.Row)
	case AskPivot:
		return "pivot\x1f" + nameKey(q.Source) + "\x1f" + pivotKey(q.Pivot)
	}
	return "call\x1f" + q.Call.Key()
}

// Sources names every source the question reads.
func (q SourceQuestion) Sources() []string {
	if q.Kind != AskCall {
		return []string{q.Source}
	}
	var out []string
	for _, a := range q.Call.Args {
		if a.Sheet != "" && !containsFold(out, a.Sheet) {
			out = append(out, a.Sheet)
		}
	}
	return out
}

func containsFold(list []string, s string) bool {
	for _, x := range list {
		if strings.EqualFold(x, s) {
			return true
		}
	}
	return false
}

// SourceAnswer is what a SourceQuestion is answered: a value, an array
// of them, or a pivot table's groups; for an error that needs one, why.
type SourceAnswer struct {
	V     Value
	A     *functions.Array
	Why   string
	Pivot *PivotGroups
}

// SetSources sets what answers the workbook's questions about linked
// sources, nil for nothing (they then say so), and recomputes what
// reads them.
func (w *Workbook) SetSources(h SourceHost) {
	w.sources = h
	w.recalc(w.src.all())
}

// sourceAsks keeps who asked what of linked sources: the formulas that
// asked each source anything, so its file changing recalculates them,
// the formulas and pivot sheets waiting for each question's answer, and
// why each formula whose answer was an error shows it.
type sourceAsks struct {
	users   map[string]map[loc]struct{} // by source key
	waiting map[string]map[loc]struct{} // by question key
	pivots  map[string][]*Sheet         // by question key
	why     map[loc]string
}

// all is every formula that asked a source something.
func (a *sourceAsks) all() []loc {
	var out []loc
	for _, set := range a.users {
		for l := range set {
			out = append(out, l)
		}
	}
	return out
}

// errNoSources is what a formula reading a source shows when nothing
// answers them.
var errNoSources = ErrNA

// ask looks up the answer to q for the formula being evaluated, noting
// that it read the sources q reads and, when the answer isn't known,
// that it waits for it.
func (w *Workbook) ask(q SourceQuestion) (SourceAnswer, bool) {
	l := w.evaluating
	if l.s != nil {
		for _, src := range q.Sources() {
			addLoc(&w.src.users, nameKey(src), l)
		}
		delete(w.src.why, l)
	}
	if w.sources == nil {
		return SourceAnswer{V: errNoSources, Why: "Nothing reads linked sources here"}, true
	}
	ans, ok := w.sources.Answer(q)
	switch {
	case !ok && l.s != nil:
		addLoc(&w.src.waiting, q.Key(), l)
	case ok && ans.Why != "" && l.s != nil:
		if w.src.why == nil {
			w.src.why = map[loc]string{}
		}
		w.src.why[l] = ans.Why
	}
	return ans, ok
}

func addLoc(m *map[string]map[loc]struct{}, k string, l loc) {
	if *m == nil {
		*m = map[string]map[loc]struct{}{}
	}
	if (*m)[k] == nil {
		(*m)[k] = map[loc]struct{}{}
	}
	(*m)[k][l] = struct{}{}
}

// RecalcSourced recomputes the formulas and pivot tables that were
// waiting for the answers to the questions with keys, and what depends
// on them. It doesn't touch the undo history.
func (w *Workbook) RecalcSourced(keys []string) {
	var changed []loc
	for _, k := range keys {
		for l := range w.src.waiting[k] {
			if l.s.live && l.s.cells.has(l.a) {
				changed = append(changed, l)
			}
		}
		delete(w.src.waiting, k)
		for _, p := range w.src.pivots[k] {
			if p.live {
				p.pivot.stale = true
			}
		}
		delete(w.src.pivots, k)
	}
	w.recalcFrom(changed, false)
}

// sourceFailure says why the formula c at l shows an error that a
// linked source gave it: the answer's reason, or why a source it names
// can't be read.
func (s *Sheet) sourceFailure(l loc, c *Cell) string {
	if why := s.wb.src.why[l]; why != "" {
		return why
	}
	for _, k := range c.names {
		k = strings.TrimPrefix(k, nameKey(regionPrefix))
		if info, ok := s.wb.LookupSource(k); ok && info.Err != "" {
			return info.Name + ": " + info.Err
		}
	}
	return ""
}

// paged is the paged sheet a reference names from the formula's sheet,
// with its source, or nil.
func (rd *reader) paged(sheet string) (*Sheet, Region) {
	if sheet == "" {
		return nil, Region{}
	}
	t := rd.sheet(sheet)
	if t == nil {
		return nil, Region{}
	}
	if r, ok := t.pagedRegion(); ok {
		return t, r
	}
	return nil, Region{}
}

// Paged reports whether a sheet is a linked source's tab.
func (rd *reader) Paged(sheet string) bool {
	t, _ := rd.paged(sheet)
	return t != nil
}

// Stream answers a call over linked sources, its ranges renamed by
// their sources' names so the answer stays the same whatever the tabs
// are called.
func (rd *reader) Stream(call functions.StreamCall) (Value, *functions.Array) {
	q := SourceQuestion{Kind: AskCall, Call: call}
	q.Call.Args = append([]functions.StreamArg(nil), call.Args...)
	for i, a := range q.Call.Args {
		if a.Sheet == "" {
			continue
		}
		t, r := rd.paged(a.Sheet)
		if t == nil {
			return ErrRef, nil
		}
		q.Call.Args[i].Sheet = r.Name
		if q.Source == "" {
			q.Source = r.Name
		}
	}
	ans, ok := rd.w.ask(q)
	if !ok {
		return Pending, nil
	}
	return ans.V, ans.A
}
