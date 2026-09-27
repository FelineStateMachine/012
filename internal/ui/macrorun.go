package ui

import (
	"errors"
	"log/slog"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/macro"
	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/telemetry"
)

// Running a macro. The script runs on a goroutine of its own, so a long
// or runaway one never freezes the screen and Esc can stop it; every call
// it makes to the spreadsheet comes back to the UI goroutine as a message
// and runs there, on the model, like a key press would. The whole run is
// one undo step, opened when it starts and closed when it ends. While it
// runs the mode indicator says CMD, as 1-2-3 did, and only Esc is taken.

// macroState is the macros' part of the model: a run in progress, and
// whether this file's macros may run without asking.
type macroState struct {
	run *macroRun
	// cmds are commands the run's calls returned (copying sets the
	// system clipboard), handed to Bubble Tea after the call.
	cmds []tea.Cmd

	// machine identifies this computer (see SetMachine); trusted is set
	// once the user has agreed to run macros from a file made elsewhere.
	machine string
	trusted bool

	// editor allows editing scripts in the user's editor, a program of
	// their own; only the local app does (see AllowEditor).
	editor bool
}

// macroRun is a script running.
type macroRun struct {
	name  string
	run   *macro.Run
	calls chan func()   // from the script: a call to run on the UI goroutine
	ack   chan struct{} // to the script: the call has run
	done  chan macroDone
	end   func() // closes the undo step
	start time.Time
	span  telemetry.Span
	print string // the last line the script printed

	orphan *time.Timer // see do; used only on the script's goroutine
}

type macroDone struct {
	stats macro.Stats
	err   error
}

// macroCallMsg carries a call from a running script; macroDoneMsg says it
// finished.
type (
	macroCallMsg struct {
		r    *macroRun
		call func()
	}
	macroDoneMsg struct {
		r    *macroRun
		done macroDone
	}
)

// How the UI serves a script's calls: it keeps taking calls within one
// update for up to sliceBudget, so a busy script isn't held up by a frame
// per call, and gives the screen back when the script pauses for
// idleWait (computing on its own) or the slice is used up.
const (
	sliceBudget = 12 * time.Millisecond
	idleWait    = 2 * time.Millisecond
)

// macroMaxSteps bounds every run; tests lower it.
var macroMaxSteps uint64 = macro.DefaultMaxSteps

// startMacro runs mc's script as one undo step.
func (m *Model) startMacro(mc sheet.Macro) tea.Cmd {
	if mc.API > sheet.MacroAPI {
		m.fail("Macro " + mc.Name + " was written for a newer version of 012")
		return nil
	}
	r := &macroRun{
		name: mc.Name, calls: make(chan func()), ack: make(chan struct{}), done: make(chan macroDone, 1),
		start: time.Now(), span: telemetry.Start("macro"),
	}
	r.end = m.book().Begin(sheet.Change{Label: "run macro " + mc.Name, Focus: m.selection(), Sheet: m.sheet})
	r.run = macro.New(mc.Name, mc.Source, macro.Env{
		Host:     scriptHost{m},
		Do:       r.do,
		Print:    func(s string) { r.print = s },
		MaxSteps: macroMaxSteps,
	})
	m.macros.run = r
	m.note = ""
	go func() {
		st, err := r.run.Exec()
		r.done <- macroDone{st, err}
	}()
	return r.wait()
}

// orphanAfter is how long a script waits for the UI to take a call before
// deciding the program is gone (it quit, or its SSH session dropped) and
// stopping, so its goroutine doesn't outlive the program. A live UI
// always has a wait running, so it takes calls at once.
var orphanAfter = 30 * time.Second

// do hands fn to the UI goroutine and waits until it has run. It runs on
// the script's goroutine.
func (r *macroRun) do(fn func()) {
	if r.orphan == nil {
		r.orphan = time.NewTimer(orphanAfter)
	} else {
		r.orphan.Reset(orphanAfter)
	}
	select {
	case r.calls <- fn:
		<-r.ack
	case <-r.orphan.C:
		r.run.Cancel() // the call is dropped; the script stops at its next step
	}
}

// wait is the command that waits for the script's next call or its end.
func (r *macroRun) wait() tea.Cmd {
	return func() tea.Msg {
		select {
		case fn := <-r.calls:
			return macroCallMsg{r, fn}
		case d := <-r.done:
			return macroDoneMsg{r, d}
		}
	}
}

// serveMacro runs a script's call, then the calls that follow it for as
// long as the slice lasts.
func (m *Model) serveMacro(msg macroCallMsg) tea.Cmd {
	r := msg.r
	deadline := time.Now().Add(sliceBudget)
	m.serveCall(r, msg.call)
	idle := time.NewTimer(idleWait)
	defer idle.Stop()
	for time.Now().Before(deadline) {
		select {
		case fn := <-r.calls:
			m.serveCall(r, fn)
			idle.Reset(idleWait)
		case d := <-r.done:
			return m.finishMacro(r, d)
		case <-idle.C:
			return m.macroCmds(r.wait())
		}
	}
	return m.macroCmds(r.wait())
}

// serveCall runs one call with the values of formulas up to date, and
// lets the script go on.
func (m *Model) serveCall(r *macroRun, fn func()) {
	m.book().Settle()
	fn()
	r.ack <- struct{}{}
}

// macroCmds hands the commands the run's calls returned to Bubble Tea,
// with next.
func (m *Model) macroCmds(next tea.Cmd) tea.Cmd {
	cmds := append(m.macros.cmds, next)
	m.macros.cmds = nil
	return tea.Batch(cmds...)
}

// finishMacro closes the run's undo step and says how it went on the
// context line: what it printed last, or where it failed.
func (m *Model) finishMacro(r *macroRun, d macroDone) tea.Cmd {
	before := m.sheet.StateID()
	r.end()
	m.macros.run = nil
	changed := m.sheet.StateID() != before
	var me *macro.Error
	outcome := "ok"
	switch {
	case d.err == nil && r.print != "":
		m.note = r.name + ": " + r.print
	case d.err == nil:
		m.note = "Ran " + r.name
	case errors.As(d.err, &me) && me.Cancel:
		outcome = "cancelled"
		m.warn = "Stopped " + r.name + " with Esc"
	case errors.As(d.err, &me) && me.Limit:
		outcome = "limit"
		m.warn = d.err.Error()
	default:
		outcome = "error"
		m.warn = d.err.Error()
	}
	if d.err != nil && changed {
		m.warn += "   Ctrl+Z undoes what it did"
	}
	r.span.End(slog.Int64("steps", int64(d.stats.Steps)), slog.Int("calls", d.stats.Calls), slog.String("outcome", outcome))
	return m.macroCmds(nil)
}

// macroKey handles a key while a macro runs: Esc stops it, nothing else
// gets through. It reports whether a macro is running.
func (m *Model) macroKey(k tea.KeyPressMsg) bool {
	r := m.macros.run
	if r == nil {
		return false
	}
	if k.String() == "esc" {
		r.run.Cancel()
	}
	return true
}

// macroRunning reports whether msg is input to hold back while a macro
// runs, so the script's view of the selection stays its own.
func (m *Model) macroBusy(msg tea.Msg) bool {
	if m.macros.run == nil {
		return false
	}
	switch msg.(type) {
	case tea.MouseMsg, tea.PasteMsg:
		return true
	}
	return false
}
