package main

import (
	"context"
	"io"
	"path/filepath"
	"slices"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestMCPOverStdio runs 012 mcp on pipes standing for standard input
// and output, as a host runs it, and lists its tools.
func TestMCPOverStdio(t *testing.T) {
	e, _, _ := scriptEnv(t)
	path := filepath.Join(t.TempDir(), "book.012")
	status(e, "set", path, "A1", "1")
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	e.stdin, e.stdout = inR, outW
	done := make(chan error, 1)
	go func() {
		done <- run([]string{"mcp", path, "--read-only"}, e)
		outW.Close()
	}()
	ctx := context.Background()
	cs, err := sdk.NewClient(&sdk.Implementation{Name: "test", Version: "1"}, nil).Connect(ctx, &sdk.IOTransport{Reader: outR, Writer: inW}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for tl, err := range cs.Tools(ctx, nil) {
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, tl.Name)
	}
	if !slices.Contains(names, "read_range") || slices.Contains(names, "write_cells") {
		t.Errorf("tools of a read-only server: %v", names)
	}
	cs.Close()
	inW.Close()
	if err := <-done; err != nil {
		t.Errorf("012 mcp: %v", err)
	}
	if code, _ := status(e, "mcp", path, "--trust"); code != 2 {
		t.Errorf("--trust without --notebooks: status %d", code)
	}
}
