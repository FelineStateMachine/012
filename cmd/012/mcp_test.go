package main

import (
	"context"
	"encoding/json"
	"io"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// serveMCP runs 012 mcp with args on pipes standing for standard input
// and output, as a host runs it, and connects a client; stop ends it
// and returns what 012 mcp did.
func serveMCP(t *testing.T, e env, args ...string) (cs *sdk.ClientSession, stop func() error) {
	t.Helper()
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	e.stdin, e.stdout = inR, outW
	done := make(chan error, 1)
	go func() {
		done <- run(append([]string{"mcp"}, args...), e)
		outW.Close()
	}()
	cs, err := sdk.NewClient(&sdk.Implementation{Name: "test", Version: "1"}, nil).Connect(context.Background(), &sdk.IOTransport{Reader: outR, Writer: inW}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return cs, func() error {
		cs.Close()
		inW.Close()
		return <-done
	}
}

func TestMCPOverStdio(t *testing.T) {
	e, _, _ := scriptEnv(t)
	path := filepath.Join(t.TempDir(), "book.012")
	status(e, "set", path, "A1", "1")
	cs, stop := serveMCP(t, e, path, "--read-only")
	var names []string
	for tl, err := range cs.Tools(context.Background(), nil) {
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, tl.Name)
	}
	if !slices.Contains(names, "read_range") || slices.Contains(names, "write_cells") {
		t.Errorf("tools of a read-only server: %v", names)
	}
	if err := stop(); err != nil {
		t.Errorf("012 mcp: %v", err)
	}
	if code, _ := status(e, "mcp", path, "--trust"); code != 2 {
		t.Errorf("--trust without --notebooks: status %d", code)
	}
}

// TestMCPRoots serves the workbooks of the folders --root gives, with
// no file named.
func TestMCPRoots(t *testing.T) {
	e, _, _ := scriptEnv(t)
	dir, other := t.TempDir(), t.TempDir()
	status(e, "set", filepath.Join(dir, "a.012"), "A1", "1")
	status(e, "set", filepath.Join(other, "b.012"), "A1", "2")
	cs, stop := serveMCP(t, e, "--root", dir, "--root", other)
	res, err := cs.CallTool(context.Background(), &sdk.CallToolParams{Name: "describe", Arguments: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	var d struct {
		Workbooks []struct{ Path string }
	}
	data, _ := json.Marshal(res.StructuredContent)
	json.Unmarshal(data, &d)
	if len(d.Workbooks) != 2 || d.Workbooks[0].Path != "a.012" || !strings.HasSuffix(d.Workbooks[1].Path, "b.012") {
		t.Errorf("describe: %s", data)
	}
	if err := stop(); err != nil {
		t.Errorf("012 mcp: %v", err)
	}
	if _, err := status(e, "mcp", "--root", filepath.Join(dir, "missing")); err == nil || !strings.Contains(err.Error(), "--root") {
		t.Errorf("a missing root: %v", err)
	}
}
