//lint:file-ignore SA1019 roots are deprecated, and followed for the hosts that still share them

package mcp

import (
	"path/filepath"
	"strings"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestClientRoots checks the client's roots win over the server's, and
// follow the client's changes.
func TestClientRoots(t *testing.T) {
	dir, outsideBook := folder(t)
	c := sdk.NewClient(&sdk.Implementation{Name: "test", Version: "1"}, nil)
	c.AddRoots(&sdk.Root{URI: "file://" + filepath.ToSlash(filepath.Dir(outsideBook))})
	// Servers ask for roots only of clients of protocols up to 2025-11-25.
	cs := connectClient(t, Options{Roots: rootsFor(t, dir)}, c, &sdk.ClientSessionOptions{ProtocolVersion: "2025-11-25"})
	if res := call(t, cs, "describe", map[string]any{"path": filepath.Join(dir, "book.012")}, nil); !strings.Contains(errorText(res), "outside") {
		t.Errorf("the server's root with the client's given: %q", errorText(res))
	}
	var d describeOut
	call(t, cs, "describe", map[string]any{"path": "other.012"}, &d)
	if d.Path != "other.012" {
		t.Errorf("the client's root: %+v", d)
	}
	c.RemoveRoots("file://" + filepath.ToSlash(filepath.Dir(outsideBook)))
	c.AddRoots(&sdk.Root{URI: "file://" + filepath.ToSlash(dir)})
	for range 100 {
		if res := call(t, cs, "describe", map[string]any{"path": "book.012"}, &d); !res.IsError {
			return
		}
	}
	t.Error("the client's new roots weren't followed")
}
