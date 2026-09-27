package ui

import (
	"cmp"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/macro"
	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/ui/choicebar"
	"github.com/FelineStateMachine/012/internal/ui/picker"
)

// Recording a macro, as Sheets' Extensions > Macros > Record macro: while
// recording, what the user does becomes a list of actions (the command
// log), written out as a script when the recording is saved.
//
// What's recorded:
//   - commands that change the workbook, by id, where runCommand runs
//     them, with the answer to the question they ask (a width, a name, a
//     confirmation) once it's given;
//   - entries, when they're stored (enter), and pastes of text;
//   - the fill handle, column borders dragged or fitted, and tabs dragged;
//   - the selection, not the keys or clicks that made it: just before
//     something is recorded, the selection it acts on is, as select() with
//     absolute references or as move() and extend() from the last one with
//     relative references. So pressing Down five times is one move(0, 5),
//     and a replay doesn't depend on the window's size.
//
// With relative references, Ctrl+arrows, Home, Ctrl+Home and Ctrl+End are
// recorded as jump(), since where they land depends on the data. Anything
// else that changes the workbook (a dialog such as sorting by several
// columns, the filter picker, dragging a chart, undo) becomes a comment
// saying it wasn't recorded, so the script never silently differs from
// what was done.

// recorder is a recording in progress.
type recorder struct {
	relative bool
	actions  []macro.Action
	base     selState // the selection the recording has reached
	pending  string   // a command waiting for the answer to its question
	acted    bool     // something was recorded during this input event
	depth    int      // commands running inside a recorded one
}

// selState is the selection as a recording sees it.
type selState struct {
	sheet     *sheet.Sheet
	cur, ext  sheet.Addr
	selecting bool
	whole     wholeKind
}

func (g *grid) selState() selState {
	return selState{g.sheet, g.cur, g.ext, g.selecting, g.whole}
}

func (s selState) rect() sheet.Rect {
	return (&grid{cur: s.cur, ext: s.ext, selecting: s.selecting, whole: s.whole}).corners()
}

// startRecording starts recording, from the selection as it is.
func (m *Model) startRecording(relative bool) {
	m.rec = &recorder{relative: relative, base: m.selState()}
}

// add records a, taking the selection after it as the new base.
func (r *recorder) add(m *Model, a macro.Action) {
	r.actions = append(r.actions, a)
	r.base = m.selState()
	r.acted = true
}

// flush records the selection, if it moved since the last action.
func (r *recorder) flush(m *Model) {
	now := m.selState()
	if now == r.base {
		return
	}
	r.actions = append(r.actions, r.selection(r.base, now)...)
	r.base = now
}

// selection records the move from one selection to another.
func (r *recorder) selection(from, to selState) []macro.Action {
	if !r.relative || from.sheet != to.sheet || to.whole == wholeAll {
		return []macro.Action{absoluteSelect(from, to)}
	}
	var out []macro.Action
	dc, dr := to.cur.Col-from.cur.Col, to.cur.Row-from.cur.Row
	if dc != 0 || dr != 0 || from.selecting && !to.selecting {
		out = append(out, macro.Call("move", dc, dr))
	}
	if to.selecting && (from.cur != to.cur || from.ext != to.ext || from.whole != to.whole || !from.selecting) {
		ext := macro.Call("extend", to.ext.Col-to.cur.Col, to.ext.Row-to.cur.Row)
		switch to.whole {
		case wholeCols:
			ext = ext.With("whole", "columns")
		case wholeRows:
			ext = ext.With("whole", "rows")
		}
		out = append(out, ext)
	}
	return out
}

// absoluteSelect is select() of the selection to, naming its sheet when
// it's another than from's, and the active cell when it isn't where
// select() puts it.
func absoluteSelect(from, to selState) macro.Action {
	r := to.rect()
	ref := wholeRef(r)
	if to.sheet != from.sheet {
		ref = sheet.QuoteSheet(to.sheet.Name()) + "!" + ref
	}
	a := macro.Call("select", ref)
	if to.whole != wholeNone || to.cur != r.From {
		a = a.With("active", to.cur.String())
	}
	return a
}

// recordingCommand runs c, recording it if it changes the workbook.
func (m *Model) recordingCommand(c *command) tea.Cmd {
	r := m.rec
	if r.depth > 0 || c.macro != macroRecord {
		return c.run(m)
	}
	r.flush(m)
	r.depth++
	cmd := c.run(m)
	r.depth--
	_, choosing := m.overlay.(*choicebar.Bar)
	if p, ok := m.overlay.(*picker.Picker); ok && p.Answers {
		choosing = true
	}
	switch {
	case m.mode == modePrompt || choosing:
		r.pending = c.id // recorded once answered
	case m.overlay != nil:
		// A dialog or Picker: what it does isn't recorded (see observe).
	case m.mode != modeError:
		r.add(m, macro.Call("run", c.id))
	}
	return cmd
}

// recordAnswer records the command waiting for the answer to its
// question, now that it has one. Esc on a choice cancels instead.
func (m *Model) recordAnswer(answer string, cancelled bool) {
	r := m.rec
	if r == nil || r.pending == "" {
		return
	}
	id := r.pending
	r.pending = ""
	if !cancelled && m.mode != modeError {
		r.add(m, macro.Call("run", id).With("answer", answer))
	}
}

// recordDialog records what a dialog did once it has, as its command
// answered with the dialog's choices (see command.answer), which a
// replay makes again. The selection it acted on was recorded when its
// command ran, or by recordFlush when something else opened it: what the
// command did to the selection since (showing a new sheet) is its own.
func (m *Model) recordDialog(id, answer string) {
	r := m.rec
	if r == nil || r.depth > 0 || m.mode == modeError {
		return
	}
	r.add(m, macro.Call("run", id).With("answer", macro.JSON(answer)))
}

// recordEntry records an entry stored in the active cell, or in every
// selected cell with fill (Ctrl+Enter).
func (m *Model) recordEntry(input string, fill bool) {
	if m.rec == nil {
		return
	}
	m.rec.flush(m)
	a := macro.Call("enter", input)
	if fill {
		a = a.With("fill", true)
	}
	if m.rec.relative && sheet.IsFormulaEntry(input) {
		a = a.With("origin", m.cur.String()) // references move with the active cell
	}
	m.rec.add(m, a)
}

// recordFlush records the selection an action is about to act on.
func (m *Model) recordFlush() {
	if m.rec != nil {
		m.rec.flush(m)
	}
}

// record records a after the selection it acted on (see recordFlush).
func (m *Model) record(a macro.Action) {
	if m.rec != nil {
		m.rec.add(m, a)
	}
}

// jumpTargets are the keys recorded as jump() with relative references.
var jumpTargets = map[string]string{
	"ctrl+up": "up", "ctrl+down": "down", "ctrl+left": "left", "ctrl+right": "right", "end": "right",
	"home": "home", "ctrl+home": "start", "ctrl+end": "end",
}

// recordMove handles a movement key in READY, recording it as jump()
// when it goes to a place that depends on the data.
func (m *Model) recordMove(key string) bool {
	r := m.rec
	base, extend := extendKey(key)
	if !extend {
		base = key
	}
	to, jump := jumpTargets[base]
	if r == nil || !r.relative || !jump {
		return m.moveKey(key)
	}
	r.flush(m)
	if !m.moveKey(key) {
		return false
	}
	a := macro.Call("jump", to)
	if extend {
		a = a.With("extend", true)
	}
	r.add(m, a)
	return true
}

// observe follows an input event while recording: with relative
// references a new sheet shown is recorded right away (where its
// selection starts can't be told later), and a change to the workbook
// nothing recorded is noted as a comment. A border being dragged is
// recorded when it's let go.
func (m *Model) observeRecording(before int) {
	r := m.rec
	if r == nil || m.mode != modeReady && m.mode != modeError || m.overlay != nil || m.prompt != nil {
		return
	}
	if r.relative && m.sheet != r.base.sheet {
		r.add(m, macro.Call("activate_sheet", m.sheet.Name()))
	}
	if resizing := m.mouse.drag == dragResize || m.mouse.drag == dragRowResize; m.sheet.StateID() != before && !r.acted && !resizing {
		what := cmp.Or(m.note, m.book().UndoLabel(), "a change")
		r.actions = append(r.actions, macro.Note("Not recorded: "+strings.ToLower(what[:1])+what[1:]))
	}
}

// recordingLine is the context line in READY while recording, when
// there's nothing else to say.
func (m *Model) recordingLine() string {
	if m.rec == nil {
		return ""
	}
	refs := "absolute"
	if m.rec.relative {
		refs = "relative"
	}
	return m.th.Hint.Render("Recording (" + refs + " references). Stop it in Data > Macros.")
}

// recordingHeader is the comment a recorded script starts with.
func recordingHeader(relative bool) string {
	if relative {
		return "Recorded with relative references: moves count from the active cell.\n" +
			"Edit it in Data > Macros > Manage macros; docs/reference/macro-api.md lists what it can call."
	}
	return "Recorded with absolute references: cells are named as they were.\n" +
		"Edit it in Data > Macros > Manage macros; docs/reference/macro-api.md lists what it can call."
}
