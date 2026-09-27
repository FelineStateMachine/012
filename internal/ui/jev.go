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
	"github.com/FelineStateMachine/012/internal/ui/theme"
)

// JEV functions are answered in the background: the engine queues
// questions in the cache, Update sends them as commands (a few at a time),
// and answers are stored as they arrive and, a frame later, the cells
// that asked them are recalculated together.

const (
	jevParallel = 8
	jevTimeout  = 60 * time.Second
)

type jevRunner struct {
	client jev.Client
	cache  *jev.Cache

	// answered are the questions answered since the last recalculation,
	// which runs gather after the first of them: at most one per frame.
	answered []sheet.RemoteCall
	gather   time.Duration
}

type jevAnswerMsg struct {
	call   sheet.RemoteCall
	answer sheet.RemoteAnswer
}

// jevRecalcMsg recalculates the cells whose questions were answered.
type jevRecalcMsg struct{}

// EnableJEV turns on JEV functions: the cache answers them, for this
// workbook and those opened later, and the client asks what it lacks.
func (m *Model) EnableJEV(client jev.Client, cache *jev.Cache) {
	m.jev = &jevRunner{client: client, cache: cache, gather: FrameInterval}
	m.sheet.Book().SetRemote(cache)
}

func init() {
	register(&command{id: "jev.refresh", macro: macroNever, title: "Ask JEV again",
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

// send starts queued questions, keeping at most jevParallel in flight
// so a filled-down column doesn't open hundreds of connections at once.
// Their spans nest under parent.
func (j *jevRunner) send(parent telemetry.Parent) tea.Cmd {
	if j == nil {
		return nil
	}
	inFlight, queued := j.cache.Busy()
	telemetry.Set("jev_in_flight", int64(inFlight))
	telemetry.Set("jev_queued", int64(queued))
	calls := j.cache.Take(jevParallel - inFlight)
	cmds := make([]tea.Cmd, len(calls))
	for i, call := range calls {
		client := j.client
		cmds[i] = func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), jevTimeout)
			defer cancel()
			span := parent.Start("jev", slog.String("kind", call.Kind), slog.Int("queued", queued))
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

// answerJEV stores an answer. The first answer since the last
// recalculation schedules the next one, a frame later, so answers that
// arrive together recalculate once.
func (m *Model) answerJEV(msg jevAnswerMsg) tea.Cmd {
	j := m.jev
	if j == nil {
		return nil
	}
	j.cache.Store(msg.call, msg.answer)
	j.answered = append(j.answered, msg.call)
	if len(j.answered) > 1 {
		return nil // already scheduled
	}
	if j.gather <= 0 {
		return func() tea.Msg { return jevRecalcMsg{} }
	}
	return tea.Tick(j.gather, func(time.Time) tea.Msg { return jevRecalcMsg{} })
}

// recalcAnswered recalculates the cells waiting for the answers stored
// since the last time, telling the user when the last question is
// answered.
func (m *Model) recalcAnswered() tea.Cmd {
	j := m.jev
	if j == nil || len(j.answered) == 0 {
		return nil
	}
	calls := j.answered
	j.answered = nil
	m.sheet.Book().RecalcAnswered(calls)
	if j.busy() == "" {
		return m.term.notify("JEV finished answering in " + m.displayName())
	}
	return nil
}

// busy describes questions still being answered, for the status line.
func (j *jevRunner) busy() string {
	if j == nil {
		return ""
	}
	inFlight, queued := j.cache.Busy()
	switch {
	case inFlight+queued == 0:
		return ""
	case queued == 0:
		return fmt.Sprintf("JEV answering %d", inFlight)
	}
	return fmt.Sprintf("JEV answering %d, %d waiting", inFlight, queued)
}

// line explains the JEV answers of calls, the active cell's, for the
// context line.
func (j *jevRunner) line(th *theme.Theme, calls []sheet.RemoteCall) string {
	if len(calls) == 0 {
		return ""
	}
	if j == nil {
		return th.Warning.Render("JEV functions need an API key: File > Settings > JEV API key")
	}
	var parts []string
	for _, c := range calls {
		a, ok := j.cache.Answer(c)
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
	return th.Muted.Render(line)
}
