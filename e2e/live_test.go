package e2e

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"
)

// Live mode (docs/agents/live.md): 012 --listen in a terminal, and an
// agent attached with a real 012 mcp --attach, spoken to in MCP's
// JSON-RPC over its standard input and output as a host would.

// runDir is a short folder for sessions' sockets, whose paths are
// limited to about 100 bytes, shared by the session and its agent.
func runDir(t *testing.T) string {
	t.Helper()
	d, err := os.MkdirTemp("", "o12")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(d) })
	return d
}

// agent is 012 mcp --attach, as a host runs it.
type agent struct {
	t    *testing.T
	cmd  *exec.Cmd
	in   io.WriteCloser
	out  *bufio.Reader
	mu   sync.Mutex
	next int
}

// attach starts 012 mcp --attach target as the host named client, and
// initializes the connection.
func attach(t *testing.T, run, target, client string) *agent {
	t.Helper()
	args := []string{"mcp", "--attach"}
	if target != "" {
		args = append(args, target)
	}
	cmd := exec.Command(binPath, args...)
	cmd.Env = append(os.Environ(), "XDG_RUNTIME_DIR="+run)
	in, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	a := &agent{t: t, cmd: cmd, in: in, out: bufio.NewReader(out)}
	t.Cleanup(func() {
		in.Close()
		done := make(chan struct{})
		go func() { cmd.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			cmd.Process.Kill()
		}
	})
	res := a.request("initialize", map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{},
		"clientInfo": map[string]any{"name": client, "version": "1"}})
	if res["serverInfo"] == nil {
		t.Fatalf("initialize: %v (stderr %s)", res, stderr.String())
	}
	a.send(map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"})
	return a
}

func (a *agent) send(msg map[string]any) {
	data, _ := json.Marshal(msg)
	if _, err := a.in.Write(append(data, '\n')); err != nil {
		a.t.Fatal(err)
	}
}

// request sends a request and returns its result, skipping
// notifications; an error fails the test.
func (a *agent) request(method string, params any) map[string]any {
	a.t.Helper()
	a.mu.Lock()
	a.next++
	id := a.next
	a.send(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
	defer a.mu.Unlock()
	for {
		line, err := a.out.ReadBytes('\n')
		if err != nil {
			a.t.Errorf("%s: %v", method, err)
			return nil
		}
		var msg struct {
			ID     *int            `json:"id"`
			Result map[string]any  `json:"result"`
			Error  json.RawMessage `json:"error"`
		}
		if json.Unmarshal(line, &msg) != nil || msg.ID == nil || *msg.ID != id {
			continue
		}
		if msg.Error != nil {
			a.t.Errorf("%s: %s", method, msg.Error)
		}
		return msg.Result
	}
}

// tool calls a tool and returns its structured result.
func (a *agent) tool(name string, args map[string]any) map[string]any {
	a.t.Helper()
	res := a.request("tools/call", map[string]any{"name": name, "arguments": args})
	if res["isError"] == true {
		a.t.Fatalf("%s: %v", name, res["content"])
	}
	out, _ := res["structuredContent"].(map[string]any)
	return out
}

// startLive starts 012 --listen on a new budget.012 with 40, 2 and
// their sum in A1:A3, and attaches an agent called claude.
func startLive(t *testing.T, o options) (*session, *agent) {
	t.Helper()
	run := runDir(t)
	o.env = append(o.env, "XDG_RUNTIME_DIR="+run)
	s := startWith(t, o, "--listen", "budget.012")
	s.keys("40", "<enter>", "2", "<enter>", "=A1+A2", "<enter>", "<ctrl+home>")
	s.waitFor("42")
	return s, attach(t, run, "", "claude")
}

func TestLiveAgentSuggestsAndThePersonAccepts(t *testing.T) {
	s, a := startLive(t, options{})
	read := a.tool("read_range", map[string]any{"ref": "A1:A3"})
	if got := fmt.Sprint(read["values"]); got != "[[40] [2] [42]]" {
		t.Fatalf("read %s", got)
	}
	s.waitFor("◆ claude")
	wrote := a.tool("write_cells", map[string]any{"entries": []map[string]any{{"ref": "A1", "input": "48"}}, "message": "round the total"})
	if wrote["suggestion"] != 1.0 {
		t.Fatalf("write %v", wrote)
	}
	s.waitFor("◇ 1 suggestion")
	if !strings.Contains(s.screen(), "◇") || strings.Contains(s.screen(), "50") {
		t.Fatalf("the suggestion was made, or isn't marked:\n%s", s.screen())
	}
	s.keys("<ctrl+alt+a>")
	s.waitFor("A1  40 → 48")
	s.keys("<enter>")
	s.waitFor("Accepted all of claude's suggestion")
	s.eventually("A3 shows 50", func() bool { return strings.Contains(s.screen(), "50") })
	list := a.tool("suggestions", nil)
	if got := list["suggestions"].([]any)[0].(map[string]any)["state"]; got != "accepted" {
		t.Fatalf("suggestions %v", list)
	}
	if undone := a.tool("undo", nil); undone["undone"] != "set" {
		t.Fatalf("undo %v", undone)
	}
	s.eventually("A3 shows 42 again", func() bool { return strings.Contains(s.screen(), "42") })
}

func TestLiveAgentAsksThePerson(t *testing.T) {
	s, a := startLive(t, options{})
	answer := make(chan map[string]any, 1)
	go func() { answer <- a.tool("ask", map[string]any{"message": "Overwrite A1:A2?"}) }()
	s.waitFor("claude asks: Overwrite A1:A2?")
	s.keys("<enter>")
	select {
	case got := <-answer:
		if got["action"] != "accept" {
			t.Fatalf("answer %v", got)
		}
	case <-time.After(waitTimeout):
		t.Fatal("no answer")
	}
}

// startLiveScreen starts a live screen's session on the household
// budget, listening, with claude attached.
func startLiveScreen(t *testing.T, o options) *session {
	t.Helper()
	run := runDir(t)
	o.env = append(o.env, "XDG_RUNTIME_DIR="+run)
	s := startWith(t, o, "--listen", "budget.012")
	s.agent = attach(t, run, "", "claude")
	s.waitFor("◆ claude")
	return s
}

// Golden screens of live mode: claude's suggestion marked in the grid,
// with what it would put in the cell under the pointer on the context
// line; the suggestions panel; and claude's question on the context
// line.
var liveScreens = []screen{
	{name: "live-review", live: true, files: householdBudget, setup: func(s *session) {
		marchSuggestion(s)
		s.keys("<ctrl+alt+a>")
		s.waitFor("B4  450 → 480")
	}},
	{name: "live-ask", live: true, files: householdBudget, setup: func(s *session) {
		s.agent.send(map[string]any{"jsonrpc": "2.0", "id": 99, "method": "tools/call",
			"params": map[string]any{"name": "ask", "arguments": map[string]any{"message": "Overwrite B3:B5 with March's figures?"}}})
		s.waitFor("claude asks: Overwrite B3:B5")
	}},
}

// marchSuggestion has claude suggest March's food and power bills, and
// puts the pointer on the food line.
func marchSuggestion(s *session) {
	s.agent.tool("write_cells", map[string]any{"message": "March's bills came in higher",
		"entries": []map[string]any{{"ref": "B4", "input": "480"}, {"ref": "B5", "input": "95"}}})
	s.waitFor("◇ 1 suggestion")
	s.keys("<down>", "<down>", "<down>", "<right>")
	s.waitFor("suggests 480")
}

func init() {
	suggestion := screen{name: "live-suggestion", live: true, files: householdBudget, setup: marchSuggestion}
	light, hc := suggestion, suggestion
	light.name += "-light"
	light.opts.light = true
	hc.name += "-high-contrast"
	hc.opts.config = highContrastConfig
	review, ask := liveScreens[0], liveScreens[1]
	review.name, review.opts.light = review.name+"-light", true
	ask.name, ask.opts.light = ask.name+"-light", true
	screens = append(screens, suggestion, light, hc)
	screens = append(screens, liveScreens...)
	screens = append(screens, review, ask)
}
