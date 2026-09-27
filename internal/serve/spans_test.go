package serve

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FelineStateMachine/012/internal/telemetry"
)

// Each session's commands nest under its own span, in a trace of its
// own, though the sessions run in one process at once.
func TestSessionSpans(t *testing.T) {
	path := filepath.Join(t.TempDir(), "log.jsonl")
	stop, err := telemetry.Setup(telemetry.Config{LogPath: path})
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	key := newKey(t)
	srv, addr, _ := testServer(t, nil, key)
	c, err := dial(addr, key, srv.HostKey())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	a, b := openTerm(t, c, 80, 24), openTerm(t, c, 80, 24)
	a.waitFor(t, "READY")
	b.waitFor(t, "READY")
	for range 3 {
		io.WriteString(a.stdin, "\x02") // Ctrl+B: format.bold
		io.WriteString(b.stdin, "\x02")
	}
	for _, tm := range []*term{a, b} {
		io.WriteString(tm.stdin, "\x11") // Ctrl+Q, then discard the bold
		tm.waitFor(t, "unsaved changes")
		io.WriteString(tm.stdin, "d")
		tm.waitClosed(t)
	}
	srv.Close()
	stop()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sessions := map[string]string{} // span id -> trace id
	var commands []map[string]any
	for _, l := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var ev map[string]any
		if err := json.Unmarshal([]byte(l), &ev); err != nil {
			t.Fatal(err)
		}
		switch ev["event"] {
		case "serve.session":
			sessions[ev["span_id"].(string)] = ev["trace_id"].(string)
		case "command":
			commands = append(commands, ev)
		}
	}
	if len(sessions) != 2 {
		t.Fatalf("%d session spans: %s", len(sessions), data)
	}
	per := map[string]int{}
	for _, cmd := range commands {
		parent, _ := cmd["parent_id"].(string)
		trace, ok := sessions[parent]
		if !ok || cmd["trace_id"] != trace {
			t.Errorf("command %v is not under a session", cmd)
		}
		per[parent]++
	}
	for id := range sessions {
		if per[id] == 0 {
			t.Errorf("session %s ran no commands: %v", id, per)
		}
	}
}
