package mcp

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/FelineStateMachine/012/internal/confine"
	"github.com/FelineStateMachine/012/internal/headless"
)

// folder makes a folder of workbooks: book.012 and sub/data.csv, a
// hidden .secret.012, a note that isn't a workbook, and a link leading
// out to another folder's workbook.
func folder(t *testing.T) (dir, outsideBook string) {
	t.Helper()
	dir = t.TempDir()
	out := t.TempDir()
	outsideBook = filepath.Join(out, "other.012")
	for _, p := range []string{"book.012", ".secret.012", outsideBook} {
		if !filepath.IsAbs(p) {
			p = filepath.Join(dir, p)
		}
		data, _ := os.ReadFile(bookFile(t))
		if err := os.WriteFile(p, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{"sub/data.csv": "Name,Score\nAda,3\nBo,5\n", "notes.txt": "hello"}
	for name, text := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(outsideBook, filepath.Join(dir, "link.012")); err != nil {
		t.Fatal(err)
	}
	return dir, outsideBook
}

func rootsFor(t *testing.T, dirs ...string) []confine.Root {
	t.Helper()
	roots, err := NewRoots(dirs)
	if err != nil {
		t.Fatal(err)
	}
	return roots
}

func TestResolve(t *testing.T) {
	dir, outsideBook := folder(t)
	roots := rootsFor(t, dir)
	real := roots[0].Dir()
	for name, want := range map[string]string{
		"book.012":                              real + "/book.012",
		"sub/data.csv":                          real + "/sub/data.csv",
		filepath.Join(dir, "book.012"):          real + "/book.012",
		"file://" + filepath.Join(dir, "x.012"): real + "/x.012",
		"new.012":                               real + "/new.012",
	} {
		if got, err := resolve(roots, name); err != nil || got != filepath.FromSlash(want) {
			t.Errorf("%s: %s, %v; want %s", name, got, err, want)
		}
	}
	for _, name := range []string{"../x.012", outsideBook, ".secret.012", "link.012", "/etc/passwd", filepath.Join(dir, "..", "x.012")} {
		if got, err := resolve(roots, name); !errors.Is(err, confine.ErrOutside) {
			t.Errorf("%s: %s, %v; want it refused", name, got, err)
		}
	}
	// A relative path is looked for in each root in turn.
	other := filepath.Dir(outsideBook)
	if got, err := resolve(rootsFor(t, dir, other), "other.012"); err != nil || filepath.Base(got) != "other.012" {
		t.Errorf("second root: %s, %v", got, err)
	}
}

func TestDescribeListsWorkbooks(t *testing.T) {
	dir, _ := folder(t)
	cs := connect(t, Options{Roots: rootsFor(t, dir)})
	var d describeOut
	call(t, cs, "describe", map[string]any{}, &d)
	var paths []string
	for _, f := range d.Workbooks {
		paths = append(paths, f.Path+" "+f.Format)
	}
	slices.Sort(paths)
	if !slices.Equal(paths, []string{"book.012 012", "sub/data.csv CSV"}) || len(d.Roots) != 1 {
		t.Errorf("describe without a path: %v in %v", paths, d.Roots)
	}
	call(t, cs, "describe", map[string]any{"path": "sub/data.csv"}, &d)
	if d.Path != "sub/data.csv" || d.Description == nil || d.Sheets[0].Name != "data" || d.Sheets[0].Columns[1] != "Score" {
		t.Errorf("describe a CSV: %+v", d)
	}
	for tl, err := range cs.Tools(context.Background(), nil) {
		if err != nil {
			t.Fatal(err)
		}
		list, _ := tl.InputSchema.(map[string]any)["required"].([]any)
		required := slices.Contains(list, any("path"))
		if tl.Name != "describe" && !required {
			t.Errorf("%s doesn't require path", tl.Name)
		}
	}
	if res := call(t, cs, "read_range", map[string]any{"path": "link.012", "ref": "A1"}, nil); !strings.Contains(errorText(res), "outside") {
		t.Errorf("a link leading out: %q", errorText(res))
	}
	if res := call(t, cs, "read_range", map[string]any{"path": "notes.txt", "ref": "A1"}, nil); !strings.Contains(errorText(res), "isn't a workbook") {
		t.Errorf("a text file: %q", errorText(res))
	}
	var r RangeRead
	call(t, cs, "read_range", map[string]any{"path": filepath.Join(dir, "book.012"), "ref": "B2"}, &r)
	if string(r.Values[0][0]) != "3" {
		t.Errorf("absolute path: %+v", r)
	}
}

func TestCreateWorkbook(t *testing.T) {
	dir, _ := folder(t)
	cs := connect(t, Options{Roots: rootsFor(t, dir)})
	var d describeOut
	call(t, cs, "create_workbook", map[string]any{"path": "empty.012"}, &d)
	if d.Path != "empty.012" || len(d.Sheets) != 1 {
		t.Errorf("empty: %+v", d)
	}
	if _, err := headless.Open(filepath.Join(dir, "empty.012"), false); err != nil {
		t.Errorf("not saved: %v", err)
	}
	call(t, cs, "create_workbook", map[string]any{"path": "sub/scores.012", "from": "sub/data.csv"}, &d)
	if d.Sheets[0].Used != "A1:B3" {
		t.Errorf("from a CSV: %+v", d.Sheets)
	}
	for args, want := range map[[2]string]string{
		{"book.012", ""}:       "exists",
		{"x.csv", ""}:          "ends in .012",
		{"../x.012", ""}:       "outside",
		{"y.012", "notes.txt"}: "isn't a file 012 imports",
	} {
		res := call(t, cs, "create_workbook", map[string]any{"path": args[0], "from": args[1]}, nil)
		if !strings.Contains(errorText(res), want) {
			t.Errorf("%v: %q, want %s", args, errorText(res), want)
		}
	}
	res := call(t, cs, "write_cells", map[string]any{"path": "sub/data.csv", "entries": []map[string]string{{"ref": "A1", "input": "x"}}}, nil)
	if !strings.Contains(errorText(res), "read only") {
		t.Errorf("writing a CSV: %q", errorText(res))
	}
	var ch Changed
	call(t, cs, "write_cells", map[string]any{"path": "sub/scores.012", "entries": []map[string]string{{"ref": "C1", "input": "Rank"}}}, &ch)
	if !ch.Saved {
		t.Errorf("writing the new workbook: %+v", ch)
	}
}

func TestPathRequiredWithoutDefault(t *testing.T) {
	dir, _ := folder(t)
	cs := connect(t, Options{Roots: rootsFor(t, dir)})
	res, err := cs.CallTool(context.Background(), &sdk.CallToolParams{Name: "read_range", Arguments: map[string]any{"ref": "A1"}})
	if err == nil && !res.IsError {
		t.Error("read_range without a path")
	}
	// With a default, it's optional and names the default.
	cs = connect(t, Options{Default: filepath.Join(dir, "book.012")})
	var r RangeRead
	call(t, cs, "read_range", map[string]any{"ref": "B2"}, &r)
	if string(r.Values[0][0]) != "3" {
		t.Errorf("the default: %+v", r)
	}
}
