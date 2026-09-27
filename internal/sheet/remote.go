package sheet

import "github.com/FelineStateMachine/012/internal/formula"

// Some functions (JEV.*) are answered by a hosted model. The engine never
// talks to the network: it describes each question as a RemoteCall and
// asks the workbook's RemoteSource for the answer. Until the answer arrives the cell shows
// Loading…, and the source queues the call; when answers arrive the UI
// calls RecalcVolatile, and the functions, being volatile, look again.

// RemoteCall is one question for the model, built from a function's
// arguments. It is comparable through its JSON form, which callers use as
// a cache key.
type RemoteCall struct {
	Kind         string `json:"kind"` // "noul", "choice" or "score"
	State        any    `json:"state"`
	Instructions string `json:"instructions"`
	// Criteria depends on Kind: noul is [2]string{yes, no} (either may be
	// empty), choice is map[label]description, score is []string levels.
	Criteria any `json:"criteria"`
}

// RemoteAnswer is the model's answer. Which fields are set depends on the
// call's Kind; Failed explains an answer that couldn't be had.
type RemoteAnswer struct {
	Noul       float64 // probability of yes
	Choice     string
	Score      float64
	Confidence float64 // in Choice or Score, not a probability of truth
	Failed     string
}

// RemoteSource answers RemoteCalls. Lookup returns false while an answer
// isn't known, and queues the call.
type RemoteSource interface {
	Lookup(RemoteCall) (RemoteAnswer, bool)
}

// Remote is the source new workbooks start with.
//
// Deprecated: set each workbook's source with SetRemote.
var Remote RemoteSource

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
	Pending = Value{Kind: Error, Str: "Loading…"}
	// ErrNoRemote means JEV functions can't run: there is no API key.
	ErrNoRemote = Value{Kind: Error, Str: "#N/A"}
)

// IsPending reports whether v is waiting for a remote answer.
func IsPending(v Value) bool { return v == Pending }

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
	get := s.wb.values(s)
	var calls []RemoteCall
	var walk func(Node)
	walk = func(n Node) {
		switch n := n.(type) {
		case formula.Unary:
			walk(n.X)
		case formula.Binary:
			walk(n.L)
			walk(n.R)
		case formula.Call:
			if funcOf(n).remote != nil {
				if call, err := funcOf(n).remote(n.Args, get); err == nil {
					calls = append(calls, call)
				}
			}
			for _, arg := range n.Args {
				walk(arg)
			}
		}
	}
	walk(s.bound(c))
	return calls
}
