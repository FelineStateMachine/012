package sheet

import (
	"github.com/FelineStateMachine/012/internal/formula"
	"github.com/FelineStateMachine/012/internal/functions"
)

// Some functions (JEV.*) are answered by a hosted model. The engine never
// talks to the network: it describes each question as a RemoteCall and
// asks the workbook's RemoteSource for the answer. Until the answer arrives the cell shows
// Loading…, and the source queues the call; when answers arrive the UI
// calls RecalcAnswered, which recomputes the formulas waiting for them.
// The functions are volatile, so any other change looks again too.

// The questions and answers are the function library's
// (functions/remote.go); the engine's API names them too.
type (
	RemoteCall   = functions.RemoteCall
	RemoteAnswer = functions.RemoteAnswer
	RemoteSource = functions.RemoteSource
)

// SetRemote sets what answers the workbook's JEV functions: nil when no
// API key is configured, and the functions then evaluate to ErrNoRemote.
// It recomputes them, so a loaded file's questions are asked.
func (w *Workbook) SetRemote(r RemoteSource) {
	w.remote = r
	w.recalc(nil)
}

// SetRemote on a sheet is its workbook's.
func (s *Sheet) SetRemote(r RemoteSource) { s.wb.SetRemote(r) }

var (
	// Pending is shown while an answer is on its way.
	Pending = functions.Pending
	// ErrNoRemote means JEV functions can't run: there is no API key.
	ErrNoRemote = functions.ErrNoRemote
	// ErrRemote is a question the model couldn't answer.
	ErrRemote = functions.ErrRemote
)

// IsPending reports whether v is waiting for a remote answer.
func IsPending(v Value) bool { return functions.IsPending(v) }

// RecalcVolatile recomputes volatile formulas and their dependents on
// every sheet, e.g. after remote answers arrive. It doesn't touch the
// undo history.
func (s *Sheet) RecalcVolatile() { s.wb.recalc(nil) }

// RemoteCalls returns the questions the formula at a asks, with their
// current inputs, so the UI can show details or re-ask them.
func (s *Sheet) RemoteCalls(a Addr) []RemoteCall {
	c := s.cells.get(a)
	if c == nil || c.expr == nil {
		return nil
	}
	get := s.wb.values(s).lib
	var calls []RemoteCall
	var walk func(Node)
	walk = func(n Node) {
		if call, ok := n.(formula.Call); ok {
			if f := funcOf(call); f.Remote() {
				if q, err := f.Question(call.Args, get); err == nil {
					calls = append(calls, q)
				}
			}
		}
		formula.EachChild(n, walk)
	}
	walk(s.bound(c))
	return calls
}

// wait notes that the formula being evaluated is waiting for the answer
// to c.
func (w *Workbook) wait(c RemoteCall) {
	if w.evaluating.s == nil {
		return
	}
	k := c.Key()
	if w.waiting == nil {
		w.waiting = map[string]map[loc]struct{}{}
	}
	if w.waiting[k] == nil {
		w.waiting[k] = map[loc]struct{}{}
	}
	w.waiting[k][w.evaluating] = struct{}{}
}

// RecalcAnswered recomputes the formulas that were waiting for the
// answers to calls, and what depends on them, on every sheet: with
// thousands of JEV cells, an answer costs the cells that asked it, not
// all of them. It doesn't touch the undo history.
func (w *Workbook) RecalcAnswered(calls []RemoteCall) {
	var changed []loc
	for _, c := range calls {
		k := c.Key()
		for l := range w.waiting[k] {
			if l.s.live && l.s.cells.get(l.a) != nil {
				changed = append(changed, l)
			}
		}
		delete(w.waiting, k)
	}
	if len(changed) > 0 {
		w.recalcFrom(changed, false)
	}
}
