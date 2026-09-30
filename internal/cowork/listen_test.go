package cowork

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/FelineStateMachine/012/internal/room"
	"github.com/FelineStateMachine/012/internal/sheet"
)

// socketDir is a short folder for sockets: their paths are limited to
// about 100 bytes.
func socketDir(t *testing.T) string {
	t.Helper()
	d, err := os.MkdirTemp("", "o12")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(d) })
	return filepath.Join(d, "run")
}

// listening is a session of ann's on budget.012 listening for agents.
func listening(t *testing.T) (*room.Seat, *Listener) {
	t.Helper()
	g := room.NewRegistry(room.Edit)
	ann, _ := g.Join("/home/ann/budget.012", &person{"ann"}, func() *sheet.Workbook {
		w := sheet.New().Book()
		w.Sheet(0).Set(sheet.Addr{}, "1")
		w.Sheet(0).Set(sheet.Addr{Row: 1}, "=A1*2")
		return w
	})
	ann.Do(func(*sheet.Workbook) { ann.SetSaved(room.Saved{Name: "budget.012"}) })
	l, err := Listen(Options{Registry: g, Room: func(string) (string, error) { return "/home/ann/budget.012", nil },
		Kind: "session", Workbook: "budget", Dir: socketDir(t)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close(); l.Wait() })
	return ann, l
}

// attachClient connects an MCP client to the listener as 012 mcp
// --attach does.
func attachClient(t *testing.T, l *Listener, name string) *sdk.ClientSession {
	t.Helper()
	conn, err := net.Dial("unix", l.Socket())
	if err != nil {
		t.Fatal(err)
	}
	json.NewEncoder(conn).Encode(hello{Version: 1, Client: name})
	r := bufio.NewReader(conn)
	line, _ := r.ReadBytes('\n')
	var w welcome
	if err := json.Unmarshal(line, &w); err != nil || w.Error != "" || w.Workbook != "budget" || w.Scope != "the whole workbook" {
		t.Fatalf("welcome %s: %v", line, err)
	}
	c := sdk.NewClient(&sdk.Implementation{Name: name, Version: "1"}, nil)
	cs, err := c.Connect(context.Background(), &sdk.IOTransport{Reader: io.NopCloser(r), Writer: conn}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close() })
	return cs
}

func callTool(t *testing.T, cs *sdk.ClientSession, name string, args any) map[string]any {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &sdk.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	if res.IsError {
		t.Fatalf("%s: %s", name, res.Content[0].(*sdk.TextContent).Text)
	}
	var out map[string]any
	data, _ := json.Marshal(res.StructuredContent)
	json.Unmarshal(data, &out)
	return out
}

// An agent attached over the socket sits in the room, reads the
// workbook as it is, suggests, and hears what became of it.
func TestAttachedAgentWorksInTheRoom(t *testing.T) {
	ann, l := listening(t)
	info, err := os.Stat(l.Socket())
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("the socket's permissions: %v %v", info.Mode(), err)
	}
	cs := attachClient(t, l, "claude")
	var peers []room.Peer
	ann.Do(func(*sheet.Workbook) { peers = ann.Peers() })
	if len(peers) != 1 || peers[0].Name != "claude" || !peers[0].Agent {
		t.Fatalf("peers %+v", peers)
	}
	read := callTool(t, cs, "read_range", map[string]any{"ref": "A1:A2"})
	if rows := read["values"].([]any); rows[1].([]any)[0] != 2.0 {
		t.Fatalf("read %v", read)
	}
	ann.Do(func(w *sheet.Workbook) { w.Sheet(0).Set(sheet.Addr{}, "5") })
	read = callTool(t, cs, "read_range", map[string]any{"ref": "A1:A2"})
	if rows := read["values"].([]any); rows[1].([]any)[0] != 10.0 {
		t.Fatalf("the agent doesn't see ann's edit: %v", read)
	}
	wrote := callTool(t, cs, "write_cells", map[string]any{"entries": []map[string]any{{"ref": "B1", "input": "=A2+1"}}, "message": "one more"})
	if wrote["suggestion"] != 1.0 || wrote["saved"] != false {
		t.Fatalf("write %v", wrote)
	}
	focus := callTool(t, cs, "focus", map[string]any{"ref": "C3"})
	if focus["at"] != "Sheet1!C3" {
		t.Fatalf("focus %v", focus)
	}
	ann.Do(func(w *sheet.Workbook) {
		if err := BoardOf(ann).Accept(w, 1, nil); err != nil {
			t.Error(err)
		}
	})
	if v := value(ann, "Sheet1", "B1"); v.Num != 11 {
		t.Fatalf("B1 is %v", v)
	}
	list := callTool(t, cs, "suggestions", nil)
	if s := list["suggestions"].([]any)[0].(map[string]any); s["state"] != "accepted" {
		t.Fatalf("suggestions %v", list)
	}
	undone := callTool(t, cs, "undo", nil)
	if undone["undone"] != "set" {
		t.Fatalf("undo %v", undone)
	}
	cs.Close()
	deadline := time.Now().Add(5 * time.Second)
	for len(peers) > 0 && time.Now().Before(deadline) {
		ann.Do(func(*sheet.Workbook) { peers = ann.Peers() })
		time.Sleep(5 * time.Millisecond)
	}
	if len(peers) != 0 {
		t.Fatal("the agent didn't leave with its connection")
	}
}

// The endpoint is listed for 012 mcp --attach, and goes with the
// listener.
func TestEndpointsListSessions(t *testing.T) {
	_, l := listening(t)
	dir := filepath.Dir(l.Socket())
	eps, err := Endpoints(dir)
	if err != nil || len(eps) != 1 || eps[0].Workbook != "budget" || eps[0].PID != os.Getpid() {
		t.Fatalf("endpoints %+v, %v", eps, err)
	}
	if ep, _, err := Pick(eps, ""); err != nil || ep.Socket != l.Socket() {
		t.Fatalf("picking the only one: %v", err)
	}
	if ep, _, err := Pick(eps, "budget.012"); err != nil || ep.Socket != l.Socket() {
		t.Fatalf("picking by name: %v", err)
	}
	if _, _, err := Pick(eps, "plan"); err == nil || !strings.Contains(err.Error(), "budget (pid") {
		t.Fatalf("picking one not there: %v", err)
	}
	if _, _, err := Pick(append(eps, Endpoint{Workbook: "plan", Kind: "session"}), ""); err == nil || !strings.Contains(err.Error(), "several") {
		t.Fatalf("picking among two: %v", err)
	}
	if info, err := os.Stat(dir); err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("the folder: %v %v", info.Mode(), err)
	}
	l.Close()
	if eps, _ := Endpoints(dir); len(eps) != 0 {
		t.Fatalf("still listed: %+v", eps)
	}
}

// A socket others can reach isn't attached to.
func TestAttachRefusesSocketsOthersReach(t *testing.T) {
	_, l := listening(t)
	os.Chmod(l.Socket(), 0o666)
	if err := checkOwner(l.Socket()); err == nil {
		t.Fatal("a socket anyone can reach passed")
	}
}

// A hello that isn't 012 mcp --attach's is refused.
func TestListenerRefusesStrangers(t *testing.T) {
	_, l := listening(t)
	conn, err := net.Dial("unix", l.Socket())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.Write([]byte(`{"jsonrpc":"2.0","id":1,"method":"initialize"}` + "\n"))
	line, _ := bufio.NewReader(conn).ReadBytes('\n')
	if !strings.Contains(string(line), "012 mcp --attach") {
		t.Fatalf("got %s", line)
	}
}
