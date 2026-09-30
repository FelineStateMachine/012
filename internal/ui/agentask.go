package ui

import (
	"strconv"

	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/cowork"
)

// An agent's questions (the ask tool) and requests (to run notebook
// cells, to enter JEV formulas directly) wait on the room's board until
// the person it asks is free: in READY, nothing open. Then the question
// takes the context line, a field at a time: a yes or no, a choice, or
// a value typed as a prompt. Esc cancels; the answer goes back to the
// agent as an elicitation's result.

// agentAsks opens the next question waiting for this session, when the
// person is free, and lets go of one the agent withdrew.
func (m *Model) agentAsks() tea.Cmd {
	seat := m.share.seat
	if seat == nil {
		return nil
	}
	if q := m.agents.asking; q != nil {
		if q.Done() {
			m.agents.asking = nil
			m.closeAsk()
			m.note = q.Agent + " stopped waiting for an answer"
		}
		return nil
	}
	if m.mode != modeReady || m.overlay != nil {
		return nil
	}
	b := m.board()
	q := b.NextAsk(seat.ID(), func(id int) bool {
		_, here := seat.Peer(id)
		return here || id == seat.ID()
	})
	if q == nil {
		return nil
	}
	m.agents.asking = q
	if q.Kind == cowork.Grant {
		m.askGrant(q)
	} else {
		m.askField(q, 0, map[string]any{})
	}
	return nil
}

// closeAsk closes the question on the context line.
func (m *Model) closeAsk() {
	switch {
	case m.mode == modePrompt:
		m.closePrompt()
	case m.overlay != nil:
		m.closeOverlay()
	}
}

// answerAsk sends the person's answer to the agent.
func (m *Model) answerAsk(q *cowork.Ask, ans cowork.Answer) {
	m.agents.asking = nil
	if m.share.seat != nil {
		m.board().Answer(q, ans)
	}
	switch ans.Action {
	case "accept":
		m.note = "Answered " + q.Agent
	case "decline":
		m.note = "Told " + q.Agent + " no"
	default:
		m.note = "Didn't answer " + q.Agent
	}
}

// askHead is how a question starts on the context line.
func askHead(q *cowork.Ask) string { return "◆ " + q.Agent + " asks: " + q.Message }

// askField asks field i of q, the answers so far in content; past the
// last, the answer goes back.
func (m *Model) askField(q *cowork.Ask, i int, content map[string]any) {
	cancel := func(m *Model) tea.Cmd { m.answerAsk(q, cowork.Answer{Action: "cancel"}); return nil }
	if len(q.Fields) == 0 {
		m.ask(question{msg: askHead(q), choices: []choice{
			{key: "enter", label: "Yes", run: func(m *Model) tea.Cmd { m.answerAsk(q, cowork.Answer{Action: "accept"}); return nil }},
			{key: "n", label: "No", run: func(m *Model) tea.Cmd { m.answerAsk(q, cowork.Answer{Action: "decline"}); return nil }},
			{key: "esc", label: "Cancel", run: cancel},
		}})
		return
	}
	if i == len(q.Fields) {
		m.answerAsk(q, cowork.Answer{Action: "accept", Content: content})
		return
	}
	f := q.Fields[i]
	head := askHead(q) + " " + f.Label()
	if i > 0 {
		head = "◆ " + q.Agent + " asks: " + f.Label()
	}
	next := func(v any) func(m *Model) tea.Cmd {
		return func(m *Model) tea.Cmd {
			content[f.Name] = v
			m.askField(q, i+1, content)
			return nil
		}
	}
	switch f.Kind {
	case cowork.Boolean:
		m.ask(question{msg: head + "?", desc: f.Description, choices: []choice{
			{key: "y", label: "Yes", run: next(true)}, {key: "n", label: "No", run: next(false)}, {key: "esc", label: "Cancel", run: cancel}}})
	case cowork.Choice:
		var chs []choice
		for j := range f.Choices {
			chs = append(chs, choice{key: strconv.Itoa(j + 1), label: f.ChoiceLabel(j), run: next(f.Choices[j])})
		}
		m.ask(question{msg: head, desc: f.Description, choices: append(chs, choice{key: "esc", label: "Cancel", run: cancel})})
	default:
		m.askText(head+":", "", func(text string) {
			v, err := f.Parse(text)
			if err != nil {
				m.fail(err.Error())
				m.askField(q, i, content)
				return
			}
			next(v)(m)
		}, func() { cancel(m) })
	}
}

// askGrant asks leave for what an agent wants to do beyond changing
// cells, once or for the session.
func (m *Model) askGrant(q *cowork.Ask) {
	answer := func(ans cowork.Answer) func(m *Model) tea.Cmd {
		return func(m *Model) tea.Cmd { m.answerAsk(q, ans); return nil }
	}
	desc := "Notebook cells run with nu as you, with your files, as an untrusted macro would"
	if q.Grant == "jev" {
		desc = "JEV formulas send cells' values to TypeSafe's model with your API key"
	}
	m.ask(question{msg: "◆ " + q.Message + ".", warn: true, desc: desc, choices: []choice{
		{key: "enter", label: "Allow once", run: answer(cowork.Answer{Action: "accept"})},
		{key: "a", label: "Allow for this session", run: answer(cowork.Answer{Action: "accept", Always: true})},
		{key: "esc", label: "Deny", run: answer(cowork.Answer{Action: "decline"})},
	}})
}
