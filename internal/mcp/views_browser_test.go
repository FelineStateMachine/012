package mcp

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// TestViewInBrowser draws the view as the OpenAI Apps SDK's hosts and
// MCP Apps hosts do, in Chromium through Playwright (testdata/view.mjs),
// and checks the grid, the pointer and formula bar, scrolling, the
// theme and the frame's height. It needs Node and the playwright
// package, found through PLAYWRIGHT_DIR (a folder whose node_modules
// holds it) or NODE_PATH, and is skipped without them.
// VIEW_SHOTS names a folder for screenshots of each case.
func TestViewInBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("-short")
	}
	nodePath := playwrightPath(t)
	w := sheet.NewBook()
	s := w.Sheet(0)
	s.Set(sheet.Addr{}, "Month")
	s.Set(sheet.Addr{Col: 1}, "Revenue")
	s.Set(sheet.Addr{Col: 2}, "Cost")
	s.Set(sheet.Addr{Col: 3}, "Profit")
	for r := 1; r <= 60; r++ {
		s.Set(sheet.Addr{Row: r}, fmt.Sprintf("Week %d", r))
		s.Set(sheet.Addr{Col: 1, Row: r}, fmt.Sprintf("$%d,%03d", 10+r%7, r*37%1000))
		s.Set(sheet.Addr{Col: 2, Row: r}, fmt.Sprintf("$%d,%03d", 6+r%3, r*53%1000))
		s.Set(sheet.Addr{Col: 3, Row: r}, fmt.Sprintf("=B%d-C%d", r+1, r+1))
	}
	path := filepath.Join(t.TempDir(), "weeks.012")
	writeBook(t, w, path)
	cs := connect(t, Options{Default: path})
	result := func(name string, args map[string]any) *sdk.CallToolResult {
		return call(t, cs, name, args, nil)
	}
	gridArgs := map[string]any{"ref": "A1:D61"}
	chartArgs := map[string]any{"data": "A1:C9", "type": "line", "title": "Revenue and cost"}
	cases := map[string]any{
		"page": viewPage(),
		"cases": map[string]any{
			"grid":  map[string]any{"args": gridArgs, "result": result("read_range", gridArgs), "grid": map[string]string{"a1": "Month", "cell": "D2", "input": "=B2-C2", "below": "D3"}},
			"chart": map[string]any{"args": chartArgs, "result": result("create_chart", chartArgs)},
		},
	}
	data, err := json.Marshal(cases)
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "cases.json")
	if err := os.WriteFile(file, data, 0o644); err != nil {
		t.Fatal(err)
	}
	args := []string{filepath.Join("testdata", "view.mjs"), file}
	if shots := os.Getenv("VIEW_SHOTS"); shots != "" {
		args = append(args, shots)
	}
	cmd := exec.Command("node", args...)
	cmd.Env = append(os.Environ(), "NODE_PATH="+nodePath)
	out, err := cmd.CombinedOutput()
	var exit *exec.ExitError
	switch {
	case errors.As(err, &exit) && exit.ExitCode() == 3:
		t.Skipf("no browser for Playwright: %s", out)
	case err != nil:
		t.Fatalf("%v\n%s", err, out)
	}
}

// playwrightPath is the NODE_PATH that finds the playwright package,
// skipping the test when there's none.
func playwrightPath(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("no node")
	}
	var dirs []string
	if d := os.Getenv("PLAYWRIGHT_DIR"); d != "" {
		dirs = append(dirs, filepath.Join(d, "node_modules"), d)
	}
	if d := os.Getenv("NODE_PATH"); d != "" {
		dirs = append(dirs, d)
	}
	nodePath := strings.Join(dirs, string(os.PathListSeparator))
	cmd := exec.Command("node", "-e", "require('playwright')")
	cmd.Env = append(os.Environ(), "NODE_PATH="+nodePath)
	if err := cmd.Run(); err != nil {
		t.Skip("no playwright package: set PLAYWRIGHT_DIR to a folder whose node_modules holds it")
	}
	return nodePath
}
