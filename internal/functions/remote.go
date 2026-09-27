package functions

import "github.com/FelineStateMachine/012/internal/value"

// Some functions (JEV.*) are answered by a hosted model. The library
// never talks to the network: it describes each question as a RemoteCall
// and asks the Book for the answer, which the engine looks up in the
// workbook's RemoteSource. Until the answer arrives the cell shows
// Loading…, and the source queues the call.

// RemoteCall is one question for the model, built from a function's
// arguments. It isn't comparable: Key identifies it by its content.
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

var (
	// Pending is shown while an answer is on its way.
	Pending = Value{Kind: value.Error, Str: "Loading…"}
	// ErrNoRemote means JEV functions can't run: there is no API key.
	ErrNoRemote = Value{Kind: value.Error, Str: "#N/A"}
)

// IsPending reports whether v is waiting for a remote answer.
func IsPending(v Value) bool { return v == Pending }
