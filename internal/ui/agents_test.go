package ui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/FelineStateMachine/012/internal/cowork"
	"github.com/FelineStateMachine/012/internal/headless"
	"github.com/FelineStateMachine/012/internal/mcp"
	"github.com/FelineStateMachine/012/internal/sheet"
)

// liveSession is a local session listening for agents, and the agent in
// its room, as 012 --listen and 012 mcp --attach make them.
type liveSession struct {
	*seshion
	agent *cowork.Agent
}

// testAgent is one of the agents in the room without a connection.
type testAgent struct{ kick chan struct{} }

func (a *testAgent) Name() string { return "claude" }
func (a *testAgent) Agent()       {}
func (a *testAgent) Notify() {
	select {
	case a.kick <- struct{}{}:
	default:
	}
}

// startLive opens a session on a workbook of 40, 2 and their sum in
// A1:A3, listening for agents, with the agent attached.
func startLive(t *testing.T) *liveSession {
	t.Helper()
	run, err := os.MkdirTemp("", "o12")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(run) })
	t.Setenv("XDG_RUNTIME_DIR", run)
	s := sheet.New()
	for i, in := range []string{"40", "2", "=A1+A2"} {
		s.Set(sheet.Addr{Row: i}, in)
	}
	s.Book().ClearHistory()
	path := filepath.Join(t.TempDir(), "budget.012")
	m := New(s, path)
	m.SetMachine("here")
	m.nb.still = true
	sh := InRooms(Guard(m))
	ls := &liveSession{seshion: &seshion{t: t, sh: sh, m: m}}
	m.Listen()
	m.share.still = true
	ls.run(sh.Init())
	ls.send(tea.WindowSizeMsg{Width: 80, Height: 20})
	t.Cleanup(m.StopListening)
	if m.share.seat == nil || m.agents.listener == nil {
		t.Fatalf("not listening: %s", m.warn)
	}
	seat, ok := m.share.reg.JoinOpen(m.share.seat.Key(), &testAgent{kick: make(chan struct{}, 1)})
	if !ok {
		t.Fatal("the agent couldn't join")
	}
	ls.agent = cowork.NewAgent(seat)
	ls.sync()
	return ls
}

// sync hands the session the room's changes.
func (ls *liveSession) sync() {
	for range 10 {
		select {
		case <-ls.m.share.link.kick:
			ls.send(roomMsg{})
		default:
			return
		}
	}
}

// suggest has the agent write entries, pairs of a reference and an
// input, as write_cells does.
func (ls *liveSession) suggest(message string, entries ...string) mcp.Made {
	ls.t.Helper()
	var es []headless.Entry
	for i := 0; i+1 < len(entries); i += 2 {
		es = append(es, headless.Entry{Ref: entries[i], Input: entries[i+1]})
	}
	made, err := ls.agent.Make(context.Background(), mcp.Making{Label: "set", Message: message, Fn: func(w *sheet.Workbook) error {
		_, err := headless.Set(w, es, headless.SetOptions{})
		return err
	}})
	if err != nil {
		ls.t.Fatal(err)
	}
	ls.sync()
	return made
}

func TestListenSharesTheWorkbookAsItIs(t *testing.T) {
	ls := startLive(t)
	ls.press("<down>", "<down>", "<down>", "7", "<enter>")
	if !ls.m.book().CanUndo() {
		t.Fatal("the person can't undo their step")
	}
	if !ls.m.changed || ls.m.filename == "" {
		t.Fatal("the session lost its file")
	}
	var peers string
	_, peers = ls.m.Others()
	if peers != "claude" {
		t.Fatalf("others: %q", peers)
	}
	if got := line(ls.m, ls.m.height-1); !strings.Contains(got, "◆ claude") {
		t.Errorf("status line %q", got)
	}
	eps, _ := cowork.Endpoints(filepath.Join(os.Getenv("XDG_RUNTIME_DIR"), "012"))
	if len(eps) != 1 || eps[0].Workbook != "budget" {
		t.Fatalf("endpoints %+v", eps)
	}
}

func TestSuggestionMarkedAndAccepted(t *testing.T) {
	ls := startLive(t)
	made := ls.suggest("keep the total at 50", "A1", "48")
	if made.Suggestion != 1 || made.Applied {
		t.Fatalf("made %+v", made)
	}
	if got := ls.m.sheet.Value(addr("A3")).Num; got != 42 {
		t.Fatalf("the suggestion was made: A3 = %v", got)
	}
	if !strings.Contains(ls.screen(), "◇40") && !strings.Contains(ls.screen(), "◇ ") {
		t.Errorf("no mark on A1:\n%s", ls.screen())
	}
	ls.press("<down>", "<up>")
	ctx := line(ls.m, contextLine)
	if !strings.Contains(ctx, "◆ claude") || !strings.Contains(ctx, "suggests 48, now 40: keep the total at 50") {
		t.Errorf("context line %q", ctx)
	}
	if got := line(ls.m, ls.m.height-1); !strings.Contains(got, "◇ 1 suggestion") {
		t.Errorf("status line %q", got)
	}
	ls.press("<ctrl+alt+a>")
	if !strings.Contains(ls.screen(), "Suggestions") || !strings.Contains(ls.screen(), "A1  40 → 48") {
		t.Fatalf("the panel:\n%s", ls.screen())
	}
	ls.press("<enter>")
	if got := ls.m.sheet.Value(addr("A3")).Num; got != 50 {
		t.Fatalf("A3 = %v after accepting", got)
	}
	if ls.m.overlay != nil {
		t.Error("the panel stayed open with nothing waiting")
	}
	if !strings.Contains(ls.m.note, "Accepted all of claude's suggestion") {
		t.Errorf("note %q", ls.m.note)
	}
	if ls.m.book().CanUndo() {
		t.Error("the person's undo takes back the agent's step")
	}
	if label, err := ls.agent.Undo(context.Background()); err != nil || label != "set" {
		t.Fatalf("the agent's undo: %q %v", label, err)
	}
	ls.sync()
	if got := ls.m.sheet.Value(addr("A3")).Num; got != 42 {
		t.Fatalf("A3 = %v after the agent's undo", got)
	}
}

func TestSuggestionByCellInThePanel(t *testing.T) {
	ls := startLive(t)
	ls.suggest("", "A1", "1", "A2", "2000")
	ls.press("<ctrl+alt+a>", "<down>", "<down>", "r")
	if got := input(ls.m, "A2"); got != "2" {
		t.Fatalf("A2 = %q after rejecting it", got)
	}
	ls.press("<enter>") // the heading again: the rest
	if got := input(ls.m, "A1"); got != "1" {
		t.Fatalf("A1 = %q after accepting the rest", got)
	}
	list, _ := ls.agent.Suggestions(context.Background())
	if list[0].State != "accepted in part" {
		t.Fatalf("suggestions %+v", list)
	}
}

func TestSuggestionRejectedTellsTheAgent(t *testing.T) {
	ls := startLive(t)
	ls.suggest("", "B1", "x")
	ls.press("<ctrl+alt+a>", "r")
	made := ls.suggest("", "B2", "y")
	if len(made.Notices) != 1 || !strings.Contains(made.Notices[0], "was rejected") {
		t.Fatalf("notices %v", made.Notices)
	}
}

func TestInviteChoosesTheScope(t *testing.T) {
	ls := startLive(t)
	ls.m.runCommand("agent.invite")
	ls.press("Read", "<enter>")
	ls.sync()
	if !strings.Contains(ls.m.note, "Agents may change read only") {
		t.Errorf("note %q", ls.m.note)
	}
	_, err := ls.agent.Make(context.Background(), mcp.Making{Label: "set", Fn: func(w *sheet.Workbook) error {
		return w.Sheet(0).Set(sheet.Addr{}, "1")
	}})
	if err == nil || !strings.Contains(err.Error(), "read only") {
		t.Fatalf("a read-only agent wrote: %v", err)
	}
}

func TestDirectEditsSetting(t *testing.T) {
	ls := startLive(t)
	ls.m.runCommand("agent.direct")
	ls.sync()
	made := ls.suggest("", "A1", "8")
	if !made.Applied || input(ls.m, "A1") != "8" {
		t.Fatalf("made %+v, A1 %q", made, input(ls.m, "A1"))
	}
}

// ask waits for the agent's question to reach the session and shows it.
func (ls *liveSession) waitAsk() {
	ls.t.Helper()
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(5 * time.Millisecond) {
		ls.sync()
		ls.send(nil)
		if ls.m.agents.asking != nil {
			return
		}
	}
	ls.t.Fatal("the question didn't come")
}

func TestAgentAsksOnTheContextLine(t *testing.T) {
	ls := startLive(t)
	got := make(chan *sdk.ElicitResult, 1)
	go func() {
		res, err := ls.agent.Ask(context.Background(), &sdk.ElicitParams{Message: "Overwrite B2:B40?"})
		if err != nil {
			t.Error(err)
		}
		got <- res
	}()
	ls.waitAsk()
	if ctx := line(ls.m, contextLine); !strings.Contains(ctx, "◆ claude asks: Overwrite B2:B40?") || !strings.Contains(ctx, "Yes") {
		t.Fatalf("context line %q", ctx)
	}
	ls.press("n")
	if res := <-got; res.Action != "decline" {
		t.Fatalf("answer %+v", res)
	}
}

func TestAgentAsksForValues(t *testing.T) {
	ls := startLive(t)
	got := make(chan *sdk.ElicitResult, 1)
	go func() {
		res, _ := ls.agent.Ask(context.Background(), &sdk.ElicitParams{Message: "Which quarter?", RequestedSchema: map[string]any{
			"type": "object", "required": []string{"quarter", "target"},
			"properties": map[string]any{
				"quarter": map[string]any{"type": "string", "enum": []string{"Q1", "Q2", "Q3"}},
				"target":  map[string]any{"type": "number", "title": "Target"}}}})
		got <- res
	}()
	ls.waitAsk()
	ls.press("3")
	if ctx := line(ls.m, contextLine); !strings.Contains(ctx, "Target:") {
		t.Fatalf("context line %q", ctx)
	}
	ls.press("lots", "<enter>")
	if !strings.Contains(ls.m.errMsg+ls.m.warn, "takes a number") {
		t.Errorf("no complaint about lots: %q %q", ls.m.errMsg, ls.m.warn)
	}
	ls.press("1200", "<enter>")
	res := <-got
	if res.Action != "accept" || res.Content["quarter"] != "Q3" || res.Content["target"] != 1200.0 {
		t.Fatalf("answer %+v", res)
	}
}
