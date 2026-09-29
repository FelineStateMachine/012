package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/FelineStateMachine/012/internal/headless"
	"github.com/FelineStateMachine/012/internal/notebook"
	"github.com/FelineStateMachine/012/internal/nushell"
	"github.com/FelineStateMachine/012/internal/sheet"
)

// bookFile writes a workbook: a sheet of sales with a header, and a
// second sheet whose name has a space.
func bookFile(t *testing.T) string {
	t.Helper()
	w := sheet.NewBook()
	s := w.Sheet(0)
	rows := [][]string{{"Region", "Units", "Price"}, {"North", "3", "12.5"}, {"South", "5", "9.75"}, {"North", "7", "12.5"}}
	for r, row := range rows {
		for c, in := range row {
			s.Set(sheet.Addr{Col: c, Row: r}, in)
		}
	}
	other, _ := w.AddSheet("Q3 plan", 1)
	other.Set(sheet.Addr{}, "=Sheet1!B2*2")
	path := filepath.Join(t.TempDir(), "book.012")
	writeBook(t, w, path)
	return path
}

func writeBook(t *testing.T, w *sheet.Workbook, path string) {
	t.Helper()
	var b bytes.Buffer
	if err := w.Write(&b); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

// connect serves backend b with options o to a client over the SDK's
// in-memory transport.
func connect(t *testing.T, b Backend, o Options) *sdk.ClientSession {
	t.Helper()
	ctx := context.Background()
	srv := New(b, o)
	st, ct := sdk.NewInMemoryTransports()
	ss, err := srv.Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ss.Close() })
	cs, err := sdk.NewClient(&sdk.Implementation{Name: "test", Version: "1"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close() })
	return cs
}

// call calls a tool, failing on a protocol error, and decodes its
// structured result into out.
func call(t *testing.T, cs *sdk.ClientSession, name string, args any, out any) *sdk.CallToolResult {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &sdk.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	if out != nil && !res.IsError {
		data, _ := json.Marshal(res.StructuredContent)
		if err := json.Unmarshal(data, out); err != nil {
			t.Fatalf("%s: %v in %s", name, err, data)
		}
	}
	return res
}

func errorText(res *sdk.CallToolResult) string {
	if !res.IsError || len(res.Content) == 0 {
		return ""
	}
	return res.Content[0].(*sdk.TextContent).Text
}

func TestTools(t *testing.T) {
	cs := connect(t, &FileBackend{Path: bookFile(t)}, Options{})
	var names []string
	for tl, err := range cs.Tools(context.Background(), nil) {
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, tl.Name)
	}
	for _, want := range []string{"describe", "read_range", "evaluate", "find", "list_errors", "write_cells", "apply_operations",
		"sort", "filter", "create_chart", "create_pivot"} {
		if !slices.Contains(names, want) {
			t.Errorf("no tool %s in %v", want, names)
		}
	}
	if slices.Contains(names, "run_notebook_cell") {
		t.Error("run_notebook_cell offered without Notebooks")
	}
	ro := connect(t, &FileBackend{Path: bookFile(t)}, Options{ReadOnly: true})
	for tl := range ro.Tools(context.Background(), nil) {
		if !tl.Annotations.ReadOnlyHint {
			t.Errorf("read-only server offers %s", tl.Name)
		}
	}
}

func TestDescribeAndRead(t *testing.T) {
	cs := connect(t, &FileBackend{Path: bookFile(t)}, Options{})
	var d headless.Description
	call(t, cs, "describe", map[string]any{}, &d)
	if len(d.Sheets) != 2 || d.Sheets[0].Header != 1 || !slices.Equal(d.Sheets[0].Columns, []string{"Region", "Units", "Price"}) {
		t.Errorf("describe: %+v", d.Sheets)
	}
	var r headless.Range
	call(t, cs, "read_range", map[string]any{"ref": "A1:C4", "text": true}, &r)
	if r.Range != "Sheet1!A1:C4" || r.Rows != 4 || string(r.Values[1][2]) != "12.5" || r.Text[0][0] != "Region" {
		t.Errorf("read_range: %+v", r)
	}
	call(t, cs, "read_range", map[string]any{"ref": "'Q3 plan'!A1"}, &r)
	if string(r.Values[0][0]) != "6" || r.Formulas["A1"] != "=Sheet1!B2*2" {
		t.Errorf("read_range of a formula: %+v", r)
	}
	res := call(t, cs, "read_range", map[string]any{"ref": "Q4!A1"}, nil)
	if !strings.Contains(errorText(res), `no sheet named "Q4" (sheets: Sheet1, Q3 plan)`) {
		t.Errorf("bad sheet: %q", errorText(res))
	}
	var found headless.Found
	call(t, cs, "find", map[string]any{"query": "north"}, &found)
	if found.Total != 2 || found.Matches[1].Cell != "Sheet1!A4" {
		t.Errorf("find: %+v", found)
	}
}

func TestEvaluateLeavesTheFile(t *testing.T) {
	path := bookFile(t)
	before, _ := os.ReadFile(path)
	cs := connect(t, &FileBackend{Path: path}, Options{})
	var ev headless.Evaluated
	call(t, cs, "evaluate", map[string]any{"formula": `SUMIF(A2:A4, "North", B2:B4)`}, &ev)
	if string(ev.Value) != "10" || ev.Cell != "Sheet1!A6" {
		t.Errorf("evaluate: %+v", ev)
	}
	call(t, cs, "evaluate", map[string]any{"formula": "=SEQUENCE(2)", "at": "E1"}, &ev)
	if ev.Spill == nil || ev.Spill.Range != "Sheet1!E1:E2" {
		t.Errorf("spill: %+v", ev.Spill)
	}
	call(t, cs, "evaluate", map[string]any{"formula": "=1/0"}, &ev)
	if string(ev.Value) != `"#DIV/0!"` || ev.Error == "" {
		t.Errorf("error: %+v", ev)
	}
	if res := call(t, cs, "evaluate", map[string]any{"formula": "=SUM(A1"}, nil); !strings.Contains(errorText(res), "Expected") {
		t.Errorf("parse error: %q", errorText(res))
	}
	var errs errorsOut
	call(t, cs, "list_errors", map[string]any{}, &errs)
	if len(errs.Errors) != 0 {
		t.Errorf("errors: %+v", errs)
	}
	if after, _ := os.ReadFile(path); !bytes.Equal(before, after) {
		t.Error("reading tools changed the file")
	}
}

func TestWrites(t *testing.T) {
	path := bookFile(t)
	cs := connect(t, &FileBackend{Path: path}, Options{})
	before, _ := os.ReadFile(path)
	var ch Changed
	call(t, cs, "write_cells", map[string]any{"entries": []map[string]string{{"ref": "D1", "input": "Total"}, {"ref": "D2", "input": "=B2*C2"}},
		"dry_run": true}, &ch)
	if ch.Saved || len(ch.Changes) != 3 || ch.Changes[1].Item != "D2" || string(ch.Changes[2].New) != "37.5" {
		t.Errorf("dry run: %+v", ch)
	}
	if after, _ := os.ReadFile(path); !bytes.Equal(before, after) {
		t.Error("dry run saved")
	}
	call(t, cs, "write_cells", map[string]any{"entries": []map[string]string{{"ref": "D2", "input": "=B2*C2"}}}, &ch)
	if !ch.Saved || len(ch.Changes) != 2 {
		t.Errorf("write: %+v", ch)
	}
	f, _ := headless.Open(path, false)
	if v := f.Book.Sheet(0).Value(sheet.Addr{Col: 3, Row: 1}); v.Num != 37.5 {
		t.Errorf("D2 = %v", v)
	}
	res := call(t, cs, "write_cells", map[string]any{"entries": []map[string]string{{"ref": "D3", "input": "=SUM(A1"}}}, nil)
	if !strings.Contains(errorText(res), "Sheet1!D3: Expected") {
		t.Errorf("bad formula: %q", errorText(res))
	}
	call(t, cs, "apply_operations", map[string]any{"operations": []map[string]any{
		{"op": "insert_rows", "ref": "A2"}, {"op": "set", "ref": "A2", "input": "East"}, {"op": "define_name", "ref": "B2:B5", "name": "Units"},
	}}, &ch)
	if !ch.Saved {
		t.Errorf("operations: %+v", ch)
	}
	call(t, cs, "sort", map[string]any{"ref": "A1:C5", "header": true, "keys": []map[string]any{{"column": "Units", "descending": true}}}, &ch)
	var r headless.Range
	call(t, cs, "read_range", map[string]any{"ref": "Units"}, &r)
	if got := string(r.Values[0][0]) + string(r.Values[1][0]); got != "75" {
		t.Errorf("sorted units start %s: %+v", got, r.Values)
	}
	call(t, cs, "filter", map[string]any{"ref": "A1:C5", "columns": []map[string]any{{"column": "Region", "condition": "eq", "value": "North"}}}, &ch)
	var d headless.Description
	call(t, cs, "describe", map[string]any{}, &d)
	if d.Sheets[0].Filter != "A1:C5" || len(d.Names) != 1 {
		t.Errorf("filter or name missing: %+v", d)
	}
}

func TestChartAndPivot(t *testing.T) {
	path := bookFile(t)
	cs := connect(t, &FileBackend{Path: path}, Options{})
	var c chartOut
	call(t, cs, "create_chart", map[string]any{"data": "A1:B4", "type": "bar", "title": "Units"}, &c)
	if c.Chart.Number != 1 || c.Chart.Sheet != "Sheet1" || !c.Saved {
		t.Errorf("chart: %+v", c)
	}
	var p pivotOut
	call(t, cs, "create_pivot", map[string]any{"source": "A1:C4", "rows": []string{"Region"}, "values": []map[string]any{{"column": "Units"}}}, &p)
	if p.Sheet != "Pivot Table 1" || !p.Saved {
		t.Errorf("pivot: %+v", p)
	}
	var r headless.Range
	call(t, cs, "read_range", map[string]any{"ref": "'Pivot Table 1'!A1:B3"}, &r)
	if string(r.Values[1][0]) != `"North"` || string(r.Values[1][1]) != "10" {
		t.Errorf("pivot result: %+v", r.Values)
	}
	res := call(t, cs, "create_pivot", map[string]any{"source": "A1:C4", "values": []map[string]any{{"column": "Weight"}}}, nil)
	if !strings.Contains(errorText(res), `"Weight" isn't a column of A1:C4`) {
		t.Errorf("unknown column: %q", errorText(res))
	}
}

func TestResourcesAndPrompts(t *testing.T) {
	cs := connect(t, &FileBackend{Path: bookFile(t)}, Options{})
	ctx := context.Background()
	var uris []string
	for r, err := range cs.Resources(ctx, nil) {
		if err != nil {
			t.Fatal(err)
		}
		uris = append(uris, r.URI)
	}
	if !slices.Contains(uris, "o12://book/Sheet1!A1:C4") || !slices.Contains(uris, "o12://book/Q3%20plan!A1") {
		t.Errorf("resources: %v", uris)
	}
	for uri, want := range map[string]string{
		"o12://book/Sheet1!A1:C4":  `"range":"Sheet1!A1:C4"`,
		"o12://book/Q3%20plan!A1":  `"formulas":{"A1":"=Sheet1!B2*2"}`,
		"o12://book/Sheet1!B2:B3":  `"values":[[3],[5]]`,
		"o12://book/table/Missing": "isn't a cell",
	} {
		res, err := cs.ReadResource(ctx, &sdk.ReadResourceParams{URI: uri})
		got := ""
		if err != nil {
			got = err.Error()
		} else {
			got = res.Contents[0].Text
		}
		if !strings.Contains(got, want) {
			t.Errorf("%s: %s, want %s", uri, got, want)
		}
	}
	var names []string
	for p, err := range cs.Prompts(ctx, nil) {
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, p.Name)
	}
	if len(names) != len(prompts) {
		t.Errorf("prompts: %v", names)
	}
	res, err := cs.GetPrompt(ctx, &sdk.GetPromptParams{Name: "add_column", Arguments: map[string]string{"range": "Sheet1!A1:C4", "column": "revenue"}})
	if err != nil {
		t.Fatal(err)
	}
	if text := res.Messages[0].Content.(*sdk.TextContent).Text; !strings.Contains(text, "Add a column to Sheet1!A1:C4 that computes revenue") {
		t.Errorf("prompt: %s", text)
	}
}

var errNotTrusted = errors.New("book.012 was saved on another computer")

// fakeNu answers every cell with one table.
type fakeNu struct{ ran int }

func (f *fakeNu) Run(_ context.Context, _ nushell.Job, _ string, stdout io.Writer) error {
	f.ran++
	_, err := stdout.Write([]byte("[[name]; [a.txt]]"))
	return err
}

func TestRunNotebookCell(t *testing.T) {
	w := sheet.NewBook()
	nb, _ := w.AddNotebook("Notes", w.Sheet(0))
	nb.SetNotebookCells("add cell", []notebook.Cell{{Kind: notebook.Note, Source: "# Files"}, {Kind: notebook.Code, Source: "files = ls"}})
	path := filepath.Join(t.TempDir(), "nb.012")
	writeBook(t, w, path)
	nu := &fakeNu{}
	refuse := true
	cs := connect(t, &FileBackend{Path: path}, Options{Notebooks: &headless.NotebookOptions{Runner: nu},
		MayRun: func(*sheet.Workbook) error {
			if refuse {
				return errNotTrusted
			}
			return nil
		}})
	if res := call(t, cs, "run_notebook_cell", map[string]any{"notebook": "Notes", "cell": 2}, nil); errorText(res) != errNotTrusted.Error() || nu.ran != 0 {
		t.Errorf("untrusted: %q, ran %d", errorText(res), nu.ran)
	}
	refuse = false
	var run headless.CellRun
	call(t, cs, "run_notebook_cell", map[string]any{"notebook": "Notes", "cell": 2}, &run)
	if run.State != "ran" || run.Name != "files" || !strings.Contains(run.Output, "a.txt") {
		t.Errorf("run: %+v", run)
	}
	res, err := cs.ReadResource(context.Background(), &sdk.ReadResourceParams{URI: "o12://book/notebook/Notes/2"})
	if err != nil || !strings.Contains(res.Contents[0].Text, "a.txt") {
		t.Errorf("cell resource after the run: %v %+v", err, res)
	}
	if res := call(t, cs, "run_notebook_cell", map[string]any{"notebook": "Notes", "cell": 1}, nil); !strings.Contains(errorText(res), "is a note") {
		t.Errorf("note cell: %q", errorText(res))
	}
}
