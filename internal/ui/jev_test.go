package ui

import (
	"context"
	"strings"
	"sync"
	"testing"

	tea "charm.land/bubbletea/v2"
	typesafe "github.com/FelineStateMachine/typesafe-go"

	"github.com/FelineStateMachine/012/internal/jev"
	"github.com/FelineStateMachine/012/internal/sheet"
)

// fakeJEV answers every question the same way and counts requests.
type fakeJEV struct {
	mu    sync.Mutex
	calls int
	noul  float64
}

func (f *fakeJEV) SystemOne(_ context.Context, r typesafe.SystemOneRequest) (*typesafe.SystemOneResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	var a typesafe.Answer = typesafe.NoulAnswer{Noul: f.noul}
	if _, ok := r.Questions["answer"].(typesafe.ChoiceQuestion); ok {
		a = typesafe.ChoiceAnswer{Choice: "negative", Confidence: 0.8}
	}
	return &typesafe.SystemOneResponse{Answers: map[string]typesafe.Answer{"answer": a}}, nil
}

// jevModel returns a model with JEV on, answered by a fake.
func jevModel(t *testing.T) (*Model, *fakeJEV) {
	t.Helper()
	fake := &fakeJEV{noul: 0.9}
	cache := jev.NewCache()
	m := tallModel()
	m.EnableJEV(fake, cache)
	return m, fake
}

func TestJEVAnswersArriveInBackground(t *testing.T) {
	m, fake := jevModel(t)
	press(t, m, "The box was crushed", "<enter>")
	// Typing the formula queues a question; its answer comes back as a
	// message, which the press helper feeds in.
	press(t, m, `=JEV.TEST(A1, "Is this a complaint?")`, "<enter>", "<up>")
	if got := m.sheet.Value(addr("A2")); got.Kind != sheet.Bool || got.Num != 1 {
		t.Fatalf("A2 = %+v", got)
	}
	if !strings.Contains(line(m, contextLine), "JEV: 90% likely yes") {
		t.Errorf("context line %q", line(m, contextLine))
	}
	// Asking again with the same inputs is answered from the cache.
	press(t, m, "<down>", `=JEV.PROB(A1, "Is this a complaint?")`, "<enter>")
	if fake.calls != 1 {
		t.Errorf("%d requests for one question", fake.calls)
	}
	if got := strings.TrimSpace(rowCells(m, 3, 1)[0]); got != "90%" {
		t.Errorf("JEV.PROB shows %q", got)
	}
	// Ask JEV again forgets the answers for the selection.
	press(t, m, "<up>", "<shift+down>", "<ctrl+k>", "ask jev", "<enter>")
	if fake.calls != 2 {
		t.Errorf("refresh made %d requests in total", fake.calls)
	}
}

func TestJEVPendingAndBusy(t *testing.T) {
	m, _ := jevModel(t)
	// Send the formula without running the resulting commands, so the
	// question stays in flight.
	for _, r := range `=JEV.CLASSIFY("meh", "Sentiment", "positive, negative")` {
		m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !sheet.IsPending(m.sheet.Value(addr("A1"))) {
		t.Fatalf("A1 = %+v", m.sheet.Value(addr("A1")))
	}
	status := line(m, m.height-1)
	if !strings.Contains(status, "JEV answering 1") || m.View().ProgressBar == nil {
		t.Errorf("status %q, progress %v", status, m.View().ProgressBar)
	}
	if got := strings.TrimSpace(rowCells(m, 1, 1)[0]); got != "Loading…" {
		t.Errorf("pending cell shows %q", got)
	}
	run(m, cmd)
	if got := m.sheet.Value(addr("A1")); got.Str != "negative" {
		t.Errorf("A1 = %+v", got)
	}
	if strings.Contains(line(m, m.height-1), "JEV") || m.View().ProgressBar != nil {
		t.Error("still busy after the answer")
	}
}

func TestJEVWithoutKeyExplains(t *testing.T) {
	m := tallModel()
	press(t, m, `=JEV.TEST("x", "Q")`, "<enter>", "<up>")
	if !strings.Contains(line(m, contextLine), "Settings > JEV API key") {
		t.Errorf("context line %q", line(m, contextLine))
	}
	if commands["jev.refresh"].enabled(m) {
		t.Error("Ask JEV again is available without a key")
	}
}

// Answers that arrive within a frame are recalculated together, once.
func TestJEVAnswersRecalculateOncePerFrame(t *testing.T) {
	m, _ := jevModel(t)
	var cmds []tea.Cmd
	for _, q := range []string{"Q1", "Q2", "Q3"} {
		for _, r := range `=JEV.TEST("x", "` + q + `")` {
			m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		}
		_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		cmds = append(cmds, cmd)
	}
	if in, _ := m.jev.cache.Busy(); in != 3 {
		t.Fatalf("%d questions in flight, want 3", in)
	}
	recalcs := 0
	sheet.OnRecalc = func(any, sheet.RecalcInfo) { recalcs++ }
	defer func() { sheet.OnRecalc = nil }()
	var answers []jevAnswerMsg
	for _, cmd := range cmds {
		for _, msg := range collect(cmd) {
			if a, ok := msg.(jevAnswerMsg); ok {
				answers = append(answers, a)
			}
		}
	}
	if len(answers) != 3 {
		t.Fatalf("%d answers", len(answers))
	}
	var tick tea.Cmd
	for i, a := range answers {
		_, cmd := m.Update(a)
		if i == 0 {
			tick = cmd
		}
	}
	if recalcs != 0 {
		t.Errorf("%d recalculations before the frame", recalcs)
	}
	run(m, tick)
	if recalcs != 1 {
		t.Errorf("%d recalculations for 3 answers, want 1", recalcs)
	}
	for _, a := range []string{"A1", "A2", "A3"} {
		if got := m.sheet.Value(addr(a)); got.Kind != sheet.Bool {
			t.Errorf("%s = %+v", a, got)
		}
	}
}

// collect runs cmd and returns the messages it and its batches produce,
// without feeding them to a model.
func collect(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	switch msg := cmd().(type) {
	case nil:
		return nil
	case tea.BatchMsg:
		var out []tea.Msg
		for _, c := range msg {
			out = append(out, collect(c)...)
		}
		return out
	default:
		return []tea.Msg{msg}
	}
}

// Starting a new sheet or opening a file keeps JEV on.
func TestJEVSurvivesNewSheet(t *testing.T) {
	m, fake := jevModel(t)
	run(m, m.runCommand("file.new"))
	press(t, m, `=JEV.TEST("x", "Is this on?")`, "<enter>")
	if got := m.sheet.Value(addr("A1")); got.Kind != sheet.Bool || fake.calls != 1 {
		t.Errorf("after File > New: A1 %+v, %d requests", got, fake.calls)
	}
}
