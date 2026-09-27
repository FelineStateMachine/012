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
	home       *sheet.Sheet // the traced cell's sheet
	origin     sheet.Addr
	targets    []sheet.Target // on any sheet: stepping to one shows its sheet
	at         int            // the target the active cell is on
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
		t = &trace{dependents: dependents, home: m.sheet, origin: m.cur, at: -1}
		if dependents {
			t.targets = m.sheet.Dependents(m.cur)
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
	m.showSheet(t.targets[t.at].Sheet)
	m.clearSelection()
	m.cur = t.targets[t.at].Range.From
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
	t := m.trace
	m.trace = nil
	if m.mode == modeReady && k.String() == "esc" {
		m.showSheet(t.home)
		m.cur = t.origin
		return true
	}
	return false
}

// traced reports whether a is in a range being traced.
func (m *Model) traced(a sheet.Addr) bool {
	if m.trace == nil {
		return false
	}
	for _, t := range m.trace.targets {
		if t.Sheet == m.sheet && t.Range.Contains(a) {
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
	origin := t.origin.String()
	if t.home != m.sheet {
		origin = sheet.Qualified(t.home.Name(), sheet.Rect{From: t.origin, To: t.origin})
	}
	left = strconv.Itoa(len(t.targets)) + " " + noun + " of " + origin + ": "
	room := m.width - ansi.StringWidth(right) - 3
	if room < ansi.StringWidth(left)+12 {
		right, room = "", m.width
	}
	var b strings.Builder
	b.WriteString(left)
	for i, tg := range t.targets {
		part := m.rangeLabel(tg, t.home)
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

// rangeLabel names a range the way a formula on from refers to it: by
// its name if it has one, with its sheet if it's on another.
func (m *Model) rangeLabel(t sheet.Target, from *sheet.Sheet) string {
	for _, n := range m.sheet.Names() {
		if !n.Gone() && n.Sheet == t.Sheet && n.Range == t.Range && t.Range.From != t.Range.To {
			return n.Name
		}
	}
	if t.Sheet != from {
		return sheet.Qualified(t.Sheet.Name(), t.Range)
	}
	return t.Range.String()
}
