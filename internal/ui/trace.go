package ui

import (
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/ui/theme"
)

// Stepping through precedents and dependents, after Excel's Ctrl+[ and
// Ctrl+]: the cells a formula reads, or the formulas that read a cell,
// light up and the active cell jumps to the first; pressing the key
// again steps through the rest. Ctrl+[ is Esc to most terminals, so the
// keys are Alt+, and Alt+. (the < and > keys: back to the inputs, on to
// the results). The highlight is transient, as a search's is: any other
// key or click ends it, and Esc returns to the traced cell. Tracing that
// stays on as the pointer moves is traceview.go.

type trace struct {
	dependents bool
	home       *sheet.Sheet // the traced cell's sheet
	origin     sheet.Addr
	targets    []sheet.Link // on any sheet: stepping to one shows its sheet
	at         int          // the target the active cell is on
}

// maxListed caps the dependents found for a cell, so one read by a
// million formulas lists a thousand and says there are more.
const maxListed = 1000

func init() {
	register(
		&command{id: "data.precedents", macro: macroView, title: "Trace precedents", desc: "Highlight the cells the formula reads; press again to jump through them", run: func(m *Model) tea.Cmd {
			m.stepTrace(false)
			return nil
		}},
		&command{id: "data.dependents", macro: macroView, title: "Trace dependents", desc: "Highlight the formulas that read the cell; press again to jump through them", run: func(m *Model) tea.Cmd {
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
			t.targets, _ = m.sheet.DependentLinks(m.cur, maxListed)
		} else {
			t.targets = m.sheet.PrecedentLinks(m.cur)
		}
		// Cells on hidden sheets can't be shown, nor a notebook's cell
		// among the grid's, so the trace skips them.
		hidden := hiddenSheets(t.targets)
		t.targets = slices.DeleteFunc(t.targets, func(l sheet.Link) bool { return l.Sheet.Hidden() || !l.OnGrid() })
		if len(t.targets) == 0 {
			m.trace = nil
			m.note = noTrace(dependents, m.cur, hidden)
			return
		}
		m.trace = t
	}
	t.at = (t.at + 1) % len(t.targets)
	m.showSheet(t.targets[t.at].Sheet)
	m.clearSelection()
	m.cur = t.targets[t.at].Range.From
}

// hiddenSheets names the hidden sheets links are on, in order.
func hiddenSheets(links []sheet.Link) []string {
	var out []string
	for _, l := range links {
		if l.Sheet.Hidden() && !slices.Contains(out, l.Sheet.Name()) {
			out = append(out, l.Sheet.Name())
		}
	}
	return out
}

// noTrace says why a trace from a found nothing to show: nothing to
// find, or only cells on hidden sheets, which it names.
func noTrace(dependents bool, a sheet.Addr, hidden []string) string {
	if len(hidden) == 0 {
		if dependents {
			return "No formulas read " + a.String()
		}
		return a.String() + " has no formula reading other cells"
	}
	names, what, it := hidden[0], "a hidden sheet", "it"
	if n := len(hidden); n > 1 {
		names = strings.Join(hidden[:n-1], ", ") + " and " + hidden[n-1]
		what, it = "hidden sheets", "them"
	}
	if dependents {
		return "Only formulas on " + names + ", " + what + ", read " + a.String() + "; View > Hidden sheets shows " + it
	}
	return "Reads only " + names + ", " + what + "; View > Hidden sheets shows " + it
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

// role is the role a, on the sheet shown, is drawn in while it's being
// traced, or nil.
func (t *trace) role(th *theme.Theme, shown *sheet.Sheet, a sheet.Addr) *lipgloss.Style {
	if t == nil {
		return nil
	}
	for _, tg := range t.targets {
		if tg.Sheet == shown && tg.Range.Contains(a) {
			if t.dependents {
				return &th.Dependent
			}
			return &th.Precedent
		}
	}
	return nil
}

// line is the context line during a trace, e.g. "2 precedents of
// C7: B3, Sales", the current one emphasized, and the keys.
func (t *trace) line(th *theme.Theme, width int, shown *sheet.Sheet) (left, right string) {
	noun, id := "precedent", "data.precedents"
	if t.dependents {
		noun, id = "dependent", "data.dependents"
	}
	if len(t.targets) != 1 {
		noun += "s"
	}
	right = th.KeyHints(shortcut(id), "next", "Esc", "back")
	origin := t.origin.String()
	if t.home != shown {
		origin = sheet.Qualified(t.home.Name(), sheet.Rect{From: t.origin, To: t.origin})
	}
	left = strconv.Itoa(len(t.targets)) + " " + noun + " of " + origin + ": "
	room := width - ansi.StringWidth(right) - 3
	if room < ansi.StringWidth(left)+12 {
		right, room = "", width
	}
	labels := make([]string, len(t.targets))
	for i, tg := range t.targets {
		labels[i] = linkLabel(shown, tg, t.home)
		if i == t.at {
			labels[i] = th.Key.Render(labels[i])
		}
	}
	return left + listFit(th, labels, len(labels), room-ansi.StringWidth(left), false), right
}

// listFit joins labels, the first of total, with commas in room
// columns, ending in "+3 more" when they don't all fit, or "and more"
// when there are more than total (more).
func listFit(th *theme.Theme, labels []string, total, room int, more bool) string {
	var b strings.Builder
	for i, part := range labels {
		if i > 0 {
			part = ", " + part
		}
		rest := " +" + strconv.Itoa(total-i) + " more"
		if more {
			rest = " and more"
		}
		if ansi.StringWidth(b.String()+part)+len(rest) > room && (i < total-1 || more) {
			b.WriteString(th.Muted.Render(rest))
			return b.String()
		}
		b.WriteString(part)
	}
	switch {
	case more:
		b.WriteString(th.Muted.Render(" and more"))
	case len(labels) < total:
		b.WriteString(th.Muted.Render(" +" + strconv.Itoa(total-len(labels)) + " more"))
	}
	return b.String()
}

// linkLabel names a link the way a formula on from refers to it: by the
// name of its named range or region, with its sheet when it's on
// another, or by what a region's cells come from.
func linkLabel(shown *sheet.Sheet, l sheet.Link, from *sheet.Sheet) string {
	switch l.Kind {
	case sheet.LinkName, sheet.LinkRegion:
		return l.Name
	case sheet.LinkFile:
		return filepath.Base(l.Name)
	case sheet.LinkOutput:
		return "notebook cell " + l.Name
	}
	for _, n := range shown.Names() {
		if !n.Gone() && n.Sheet == l.Sheet && n.Range == l.Range && l.Range.From != l.Range.To {
			return n.Name
		}
	}
	if l.Sheet != from {
		return sheet.Qualified(l.Sheet.Name(), l.Range)
	}
	return l.Range.String()
}
