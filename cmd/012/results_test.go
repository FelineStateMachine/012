package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FelineStateMachine/012/internal/headless"
)

// TestSetDryRun prints the change as 012 diff would and leaves the
// file as it was.
func TestSetDryRun(t *testing.T) {
	e, out, _ := scriptEnv(t)
	path := filepath.Join(t.TempDir(), "book.012")
	status(e, "set", path, "A1", "1")
	before, _ := os.ReadFile(path)
	out.Reset()
	if code, err := status(e, "set", path, "A1", "2", "B1", "=A1*2", "--dry-run"); code != 0 {
		t.Fatal(err)
	}
	if got := out.String(); !strings.Contains(got, "Sheet1!A1  input  1 → 2") || !strings.Contains(got, "Sheet1!B1  input  + =A1*2") {
		t.Errorf("dry run printed %q", got)
	}
	if after, _ := os.ReadFile(path); string(after) != string(before) {
		t.Errorf("dry run saved")
	}
	out.Reset()
	if code, err := status(e, "set", path, "A1", "3", "--format", "json"); code != 0 {
		t.Fatal(err)
	}
	var res headless.SetResult
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if !res.Saved || len(res.Changes) == 0 || res.Changes[0].Item != "A1" || string(res.Changes[0].New) != `"3"` {
		t.Errorf("set result %+v", res)
	}
	// A new file's dry run lists what it would hold, and makes no file.
	fresh := filepath.Join(t.TempDir(), "new.012")
	out.Reset()
	status(e, "set", fresh, "A1", "x", "--dry-run", "--format", "nuon")
	if _, err := os.Stat(fresh); err == nil || !strings.Contains(out.String(), "saved: false") {
		t.Errorf("dry run on a new file: %s", out)
	}
}

// TestResultFormats checks the other commands' --format json results.
func TestResultFormats(t *testing.T) {
	e, out, _ := scriptEnv(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "book.012")
	status(e, "set", path, "A1", "Item", "B1", "Price", "A2", "Pen", "B2", "=1/0")
	for _, c := range []struct {
		args []string
		code int
		want string
	}{
		{[]string{"recalc", path, "--format", "json"}, 1, `"value": "#DIV/0!"`},
		{[]string{"describe", path, "--format", "json"}, 0, `"columns": [`},
		{[]string{"describe", path}, 0, "Sheet1 (shown)  A1:B2, 2 rows x 2 columns, 4 cells, 1 formula (1 showing errors)"},
		{[]string{"export", path, filepath.Join(dir, "out.csv"), "--report", "json"}, 0, `"format": "csv"`},
		{[]string{"version", "--format", "json"}, 0, `"go": "go`},
	} {
		out.Reset()
		code, err := status(e, c.args...)
		if code != c.code || !strings.Contains(out.String(), c.want) {
			t.Errorf("%v: %d %v %s, want %q", c.args, code, err, out, c.want)
		}
	}
	if code, _ := status(e, "describe", path, "--format", "csv"); code != 2 {
		t.Errorf("describe --format csv: status %d", code)
	}
}
