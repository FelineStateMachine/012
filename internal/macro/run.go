package macro

import (
	"errors"
	"fmt"
	"strings"
	"sync/atomic"

	"go.starlark.net/resolve"
	"go.starlark.net/starlark"
	"go.starlark.net/syntax"
)

// DefaultMaxSteps bounds a run: enough for loops over tens of thousands
// of cells, few enough that a script that never ends stops within a
// second or so.
const DefaultMaxSteps = 10_000_000

// Env is where a run happens.
type Env struct {
	Host Host
	// Do runs fn where Host may be used and returns once it has; nil
	// runs it directly. The UI hands calls to its own goroutine here, so
	// the script can run on another.
	Do func(fn func())
	// Print shows a line printed by the script; called inside Do.
	Print func(msg string)
	// MaxSteps bounds the run; 0 means DefaultMaxSteps.
	MaxSteps uint64
}

// Stats describe a finished run.
type Stats struct {
	Steps uint64 // Starlark computation steps
	Calls int    // calls to the spreadsheet
}

// ErrCancelled is the reason a cancelled run gives.
var ErrCancelled = errors.New("stopped")

// Error is a script's failure at a position in it, e.g.
// "Totals:3:5: set: not a cell or range: ZZ".
type Error struct {
	Pos    string // name:line:column, or "" when not at a place in the script
	Msg    string
	Cancel bool // the run was cancelled
	Limit  bool // the run hit its step limit
}

func (e *Error) Error() string {
	if e.Pos == "" {
		return e.Msg
	}
	return e.Pos + ": " + e.Msg
}

// options are the language options: top-level if, for and while, so a
// recording can be a plain list of statements and scripts don't need a
// main function, and reassigning globals, as in Python.
var options = &syntax.FileOptions{Set: true, While: true, TopLevelControl: true, GlobalReassign: true}

// Run is one run of a script.
type Run struct {
	name, src string
	env       Env
	thread    *starlark.Thread
	calls     int
	cancelled atomic.Bool
}

// New prepares a run of src, named name in error positions.
func New(name, src string, env Env) *Run {
	if env.Do == nil {
		env.Do = func(fn func()) { fn() }
	}
	if env.Print == nil {
		env.Print = func(string) {}
	}
	r := &Run{name: name, src: src, env: env}
	r.thread = &starlark.Thread{Name: name}
	r.thread.Print = func(_ *starlark.Thread, msg string) { r.env.Do(func() { r.env.Print(msg) }) }
	// No Load: scripts can't load modules, so nothing but the builtins
	// below is reachable.
	max := env.MaxSteps
	if max == 0 {
		max = DefaultMaxSteps
	}
	r.thread.SetMaxExecutionSteps(max)
	return r
}

// Cancel stops the run as soon as the script next takes a step. It is
// safe to call from any goroutine.
func (r *Run) Cancel() {
	r.cancelled.Store(true)
	r.thread.Cancel(ErrCancelled.Error())
}

// Exec runs the script to its end on the calling goroutine. Host calls
// go through Env.Do.
func (r *Run) Exec() (Stats, error) {
	_, err := starlark.ExecFileOptions(options, r.thread, r.name, r.src, r.builtins())
	st := Stats{Steps: r.thread.ExecutionSteps(), Calls: r.calls}
	return st, r.explain(err)
}

// Check parses and resolves src without running it, reporting the first
// problem with its position.
func Check(name, src string) error {
	_, _, err := starlark.SourceProgramOptions(options, name, src, func(n string) bool { return predeclared[n] })
	return (&Run{name: name}).explain(err)
}

// predeclared are the names scripts see besides Starlark's own.
var predeclared = func() map[string]bool {
	out := map[string]bool{}
	for n := range (&Run{}).builtins() {
		out[n] = true
	}
	return out
}()

// explain turns an error from Starlark into an Error with the script
// position it happened at.
func (r *Run) explain(err error) error {
	if err == nil {
		return nil
	}
	var se syntax.Error
	var re resolve.ErrorList
	var ee *starlark.EvalError
	switch {
	case errors.As(err, &se):
		return &Error{Pos: position(se.Pos), Msg: se.Msg}
	case errors.As(err, &re):
		return &Error{Pos: position(re[0].Pos), Msg: re[0].Msg}
	case errors.As(err, &ee):
		return r.evalError(ee)
	}
	return &Error{Msg: err.Error()}
}

// evalError places an evaluation error at the innermost line of the
// script, naming the function that failed when it's one of ours.
func (r *Run) evalError(ee *starlark.EvalError) *Error {
	e := &Error{Msg: ee.Msg}
	stack := ee.CallStack
	if n := len(stack); n > 0 && stack[n-1].Pos.Filename() == "<builtin>" {
		if name := stack[n-1].Name; !strings.HasPrefix(e.Msg, name+":") {
			e.Msg = name + ": " + e.Msg
		}
		stack = stack[:n-1]
	}
	if n := len(stack); n > 0 {
		e.Pos = position(stack[n-1].Pos)
	}
	switch {
	case r.cancelled.Load() && strings.Contains(ee.Msg, ErrCancelled.Error()):
		e.Msg, e.Cancel = "stopped with Esc", true
	case strings.Contains(ee.Msg, "too many steps"):
		e.Msg = fmt.Sprintf("stopped after %d steps: does it loop forever?", r.thread.ExecutionSteps())
		e.Limit = true
	}
	return e
}

func position(p syntax.Position) string {
	return fmt.Sprintf("%s:%d:%d", p.Filename(), p.Line, p.Col)
}
