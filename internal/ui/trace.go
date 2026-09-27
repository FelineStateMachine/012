package ui

import (
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// Tracing precedents and dependents, after Excel's Ctrl+[ and Ctrl+]: the
// cells a formula reads, or the formulas that read a cell, light up and
// the active cell jumps to the first; pressing the key again steps
// through the rest. Ctrl+[ is Esc to most terminals, so the keys are
// Alt+, and Alt+. (the < and > keys: back to the inputs, on to the
// results). The highlight is transient, as a search's is: any other key
// or click ends it, and Esc returns to the traced cell.

type trace struct {
	dependents bool
	origin     sheet.Addr
	targets    []sheet.Rect
	at         int // the target the active cell is on
}

func init() {
	register(
		&command{id: "data.precedents", title: "Trace precedents", desc: "Highlight the cells the formula reads; press again to jump through them", run: func(m *Model) tea.Cmd {
			m.stepTrace(false)
			return nil
		}},
		&command{id: "data.dependents", title: "Trace dependents", desc: "Highlight the formulas that read the cell; press again to jump through them", run: func(m *Model) tea.Cmd {
			m.stepTrace(true)
			return nil
		}},
	)
	keymap["alt+,"] = "data.precedents"
	keymap["alt+."] = "data.dependents"
}

// stepTrace starts a trace from the active cell, or moves to the next
// target of the one showing.
func (m *Model) stepTrace(dependents bool) {
	t := m.trace
	if t == nil || t.dependents != dependents {
		t = &trace{dependents: dependents, origin: m.cur, at: -1}
		if dependents {
			for _, a := range m.sheet.Dependents(m.cur) {
				t.targets = append(t.targets, sheet.Rect{From: a, To: a})
			}
		} else {
			t.targets = m.sheet.Precedents(m.cur)
		}
		if len(t.targets) == 0 {
			m.trace = nil
			m.note = "No formulas read " + m.cur.String()
			if !dependents {
				m.note = m.cur.String() + " has no formula reading other cells"
			}
			return
		}
		m.trace = t
	}
	t.at = (t.at + 1) % len(t.targets)
	m.clearSelection()
	m.cur = t.targets[t.at].From
}

// traceKey ends the trace on any key but the trace commands, and returns
// to the traced cell on Esc. It reports whether it used the key.
func (m *Model) traceKey(k tea.KeyPressMsg) bool {
	if m.trace == nil {
		return false
	}
	id := keymap[canonicalKey(k.String())]
	if m.mode == modeReady && (id == "data.precedents" || id == "data.dependents") {
		return false
	}
	origin := m.trace.origin
	m.trace = nil
	if m.mode == modeReady && k.String() == "esc" {
		m.cur = origin
		return true
	}
	return false
}

// traced reports whether a is in a range being traced.
func (m *Model) traced(a sheet.Addr) bool {
	if m.trace == nil {
		return false
	}
	for _, r := range m.trace.targets {
		if r.Contains(a) {
			return true
		}
	}
	return false
}

// traceLine is the context line during a trace, e.g. "2 precedents of
// C7: B3, Sales", the current one emphasized, and the keys.
func (m *Model) traceLine() (left, right string) {
	t := m.trace
	noun, id := "precedent", "data.precedents"
	if t.dependents {
		noun, id = "dependent", "data.dependents"
	}
	if len(t.targets) != 1 {
		noun += "s"
	}
	right = m.keyHints(shortcut(id), "next", "Esc", "back")
	left = strconv.Itoa(len(t.targets)) + " " + noun + " of " + t.origin.String() + ": "
	room := m.width - ansi.StringWidth(right) - 3
	if room < ansi.StringWidth(left)+12 {
		right, room = "", m.width
	}
	var b strings.Builder
	b.WriteString(left)
	for i, r := range t.targets {
		part := m.rangeLabel(r)
		if i == t.at {
			part = m.th.key.Render(part)
		}
		if i > 0 {
			part = ", " + part
		}
		more := " +" + strconv.Itoa(len(t.targets)-i) + " more"
		if ansi.StringWidth(b.String()+part)+len(more) > room && i < len(t.targets)-1 {
			b.WriteString(m.th.muted.Render(more))
			break
		}
		b.WriteString(part)
	}
	return b.String(), right
}

// rangeLabel names a range the way formulas refer to it: by its name if
// it has one.
func (m *Model) rangeLabel(r sheet.Rect) string {
	for _, n := range m.sheet.Names() {
		if !n.Lost && n.Range == r && r.From != r.To {
			return n.Name
		}
	}
	return r.String()
}
