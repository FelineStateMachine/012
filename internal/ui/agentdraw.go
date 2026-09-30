package ui

import (
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/cowork"
	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/ui/theme"
)

// Drawing agents' suggestions: each cell a suggestion waiting would set
// has a ◇ in its top-left corner (theme.Suggested), the context line
// says what the agent would put there and why when the pointer is on
// one, and the status line counts those waiting. Agents themselves are
// drawn as the others are (sharedraw.go), their initial a ◆.

// agentFrame is what a frame draws of suggestions, worked out once as
// it starts: the cells of the sheet shown that suggestions waiting set,
// and how many suggestions wait.
type agentFrame struct {
	cells   map[sheet.Addr]suggestedCell
	waiting int
}

// suggestedCell is a cell of a suggestion.
type suggestedCell struct {
	s *cowork.Suggestion
	i int
}

// frameAgents works out the suggestions for the frame about to be
// drawn.
func (m *Model) frameAgents() {
	f := &m.agents.frame
	clear(f.cells)
	f.waiting = 0
	if m.share.seat == nil {
		return
	}
	pending := m.board().Pending()
	f.waiting = len(pending)
	name := m.sheet.Name()
	for _, s := range pending {
		for i, c := range s.Cells {
			if c.Sheet != name || !s.Whole() && s.States[i] != cowork.CellPending {
				continue
			}
			if f.cells == nil {
				f.cells = map[sheet.Addr]suggestedCell{}
			}
			f.cells[c.At] = suggestedCell{s, i}
		}
	}
}

// suggestedMark draws the ◇ of a cell a suggestion would set over the
// first column of its text.
func (m *Model) suggestedMark(text string, a sheet.Addr, base lipgloss.Style, colored bool) string {
	if _, ok := m.agents.frame.cells[a]; !ok || m.sheet.ColWidth(a.Col) < 2 {
		return text
	}
	mark := m.th.Suggested
	if colored {
		mark = base.Bold(true)
	}
	return mark.Render("◇") + ansi.Cut(text, 1, m.sheet.ColWidth(a.Col))
}

// agentLine is the context line's word on a cell a suggestion would
// set: whose, what it would put there, what's there now and why.
func (m *Model) agentLine() string {
	sc, ok := m.agents.frame.cells[m.cur]
	if !ok {
		return ""
	}
	s, c := sc.s, sc.s.Cells[sc.i]
	what := shownInput(c.Now)
	if c.Look && c.Was == c.Now {
		what = "a new format"
	}
	line := m.th.Suggested.Render("◇ ") + m.th.Peer[s.Color%theme.Peers].Render(" ◆ "+s.Agent+" ") +
		m.th.Muted.Render(" suggests ") + m.th.Key.Render(what) + m.th.Muted.Render(", now "+shownInput(c.Was))
	if s.Message != "" {
		line += m.th.Muted.Render(": " + s.Message)
	}
	return line + "  " + m.th.KeyHints(keyLabel(keyFor("agent.review")), "review")
}

// keyFor is the first key bound to a command.
func keyFor(id string) string {
	if keys := keysFor(id, false); len(keys) > 0 {
		return keys[0]
	}
	return ""
}

// shownInput is an input as live mode shows it: blank for none.
func shownInput(in string) string {
	if in == "" {
		return "blank"
	}
	return in
}

// agentsStatus is the status line's count of suggestions waiting.
func (m *Model) agentsStatus() string {
	n := m.agents.frame.waiting
	if n == 0 {
		return ""
	}
	what := strconv.Itoa(n) + " suggestions"
	if n == 1 {
		what = "1 suggestion"
	}
	return "  " + m.th.Suggested.Render("◇ "+what)
}

// peerInitial is what stands for a participant on a row header: an
// agent's ◆, a person's initial.
func peerInitial(name string, agent bool) string {
	if agent {
		return "◆"
	}
	return initial(name)
}

// peerLabel is a participant's name as the status line shows it, an
// agent's after its ◆.
func peerLabel(name string, agent bool) string {
	name = ansi.Truncate(name, 10, "…")
	if agent {
		return "◆ " + name
	}
	return strings.TrimSpace(name)
}
