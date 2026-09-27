package ui

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/jev"
	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/telemetry"
)

// JEV functions are answered in the background: the engine queues
// questions in the cache, Update sends them as commands (a few at a time),
// and each answer triggers a recalculation of the JEV cells.

const (
	jevParallel = 8
	jevTimeout  = 60 * time.Second
)

type jevRunner struct {
	client jev.Client
	cache  *jev.Cache
}

type jevAnswerMsg struct {
	call   sheet.RemoteCall
	answer sheet.RemoteAnswer
}

// EnableJEV turns on JEV functions. The cache must already be installed
// as sheet.Remote so loading a file queues its questions.
func (m *Model) EnableJEV(client jev.Client, cache *jev.Cache) {
	m.jev = &jevRunner{client: client, cache: cache}
}

func init() {
	register(&command{id: "jev.refresh", title: "Ask JEV again",
		desc:    "Ask the JEV model again for the selected cells, replacing its answers",
		enabled: func(m *Model) bool { return m.jev != nil },
		run: func(m *Model) tea.Cmd {
			r := m.selection()
			var calls []sheet.RemoteCall
			for _, a := range m.sheet.Addrs() {
				if r.Contains(a) {
					calls = append(calls, m.sheet.RemoteCalls(a)...)
				}
			}
			if len(calls) == 0 {
				m.note = "No JEV functions in " + r.String()
				return nil
			}
			m.jev.cache.Forget(calls)
			m.sheet.RecalcVolatile()
			m.note = fmt.Sprintf("Asking JEV again for %d %s", len(calls), plural(len(calls), "question", "questions"))
			return nil
		}})
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// sendJEV starts queued questions, keeping at most jevParallel in flight
// so a filled-down column doesn't open hundreds of connections at once.
func (m *Model) sendJEV() tea.Cmd {
	if m.jev == nil {
		return nil
	}
	inFlight, queued := m.jev.cache.Busy()
	telemetry.Set("jev_in_flight", int64(inFlight))
	telemetry.Set("jev_queued", int64(queued))
	calls := m.jev.cache.Take(jevParallel - inFlight)
	cmds := make([]tea.Cmd, len(calls))
	for i, call := range calls {
		client := m.jev.client
		cmds[i] = func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), jevTimeout)
			defer cancel()
			span := telemetry.Start("jev", slog.String("kind", call.Kind), slog.Int("queued", queued))
			answer := jev.Ask(ctx, client, call)
			outcome := "ok"
			if answer.Failed != "" {
				outcome = "failed"
			}
			span.End(slog.String("outcome", outcome))
			return jevAnswerMsg{call: call, answer: answer}
		}
	}
	return tea.Batch(cmds...)
}

// handleJEVAnswer stores an answer and recalculates the JEV cells.
func (m *Model) handleJEVAnswer(msg jevAnswerMsg) {
	if m.jev == nil {
		return
	}
	m.jev.cache.Store(msg.call, msg.answer)
	m.sheet.RecalcVolatile()
}

// jevBusy describes questions still being answered, for the status line.
func (m *Model) jevBusy() string {
	if m.jev == nil {
		return ""
	}
	inFlight, queued := m.jev.cache.Busy()
	switch {
	case inFlight+queued == 0:
		return ""
	case queued == 0:
		return fmt.Sprintf("JEV answering %d", inFlight)
	}
	return fmt.Sprintf("JEV answering %d, %d waiting", inFlight, queued)
}

// jevLine explains the JEV answer in the active cell, for the context line.
func (m *Model) jevLine() string {
	calls := m.sheet.RemoteCalls(m.cur)
	if len(calls) == 0 {
		return ""
	}
	if m.jev == nil {
		return m.th.warning.Render("JEV functions need TYPESAFE_API_KEY, in the environment or a .env file")
	}
	var parts []string
	for _, c := range calls {
		a, ok := m.jev.cache.Answer(c)
		if !ok {
			parts = append(parts, "JEV: asking…")
			continue
		}
		parts = append(parts, jev.Describe(c, a))
	}
	line := parts[0]
	for _, p := range parts[1:] {
		line += "   " + p
	}
	return m.th.muted.Render(line)
}
