package mcp

import (
	"encoding/json"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/FelineStateMachine/012/internal/headless"
	"github.com/FelineStateMachine/012/internal/notebook"
	"github.com/FelineStateMachine/012/internal/nushell"
	"github.com/FelineStateMachine/012/internal/sheet"
)

// groceries are rows of every type, as a model writes them, in records
// and as a list in columns' order.
var groceries = []json.RawMessage{
	json.RawMessage(`{"Item": "Tea", "Price": {"currency": 3.5}, "Share": {"percent": 0.12}, "Bought": {"date": "2026-09-29"}, "Steep": {"duration": "4min"}, "Size": {"size": 1500}, "Code": "00123"}`),
	json.RawMessage(`["Cake", {"currency": 2}, {"percent": 0.5}, {"date": "2026-09-28"}, {"duration": "90sec"}, {"size": "250kb"}, "7"]`),
}

// columns order the table: the SDK hands a tool a record's fields in
// another order than written.
var columns = []string{"Item", "Price", "Share", "Bought", "Steep", "Size", "Code"}

// wantValues are groceries as read_range reads them back, header first.
const wantValues = `[["Item","Price","Share","Bought","Steep","Size","Code"],` +
	`["Tea",{"currency":3.5},{"percent":0.12},{"date":"2026-09-29"},{"duration":"4min"},{"size":1500},"00123"],` +
	`["Cake",{"currency":2},{"percent":0.5},{"date":"2026-09-28"},{"duration":"90sec"},{"size":250000},"7"]]`

// Values written with their types read back as the same values, shown
// as their formats show them, and keep their formats in the file.
func TestTypedValuesRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "groceries.012")
	cs := connect(t, Options{Roots: rootsFor(t, dir)})
	if res := call(t, cs, "create_workbook", map[string]any{"path": path, "columns": columns, "data": groceries}, nil); res.IsError {
		t.Fatal(errorText(res))
	}
	var r headless.Range
	call(t, cs, "read_range", map[string]any{"path": path, "ref": "A1:G3"}, &r)
	if got, _ := json.Marshal(r.Values); string(got) != wantValues {
		t.Errorf("values:\n%s\nwant\n%s", got, wantValues)
	}
	if got := strings.Join(r.Text[1], "|"); got != "Tea|$3.50|12%|9/29/2026|0:04:00|1.5 kB|00123" {
		t.Errorf("text: %s", got)
	}
	var changed Changed
	call(t, cs, "write_cells", map[string]any{"path": path, "entries": []map[string]any{
		{"ref": "B4", "value": map[string]any{"currency": 4.25, "symbol": "€"}},
		{"ref": "D4", "input": "2026-10-01"},
		{"ref": "A4", "value": "0042"},
		{"ref": "B2:B3", "format": "$#,##0"},
	}}, &changed)
	if !changed.Saved {
		t.Fatalf("write_cells: %+v", changed)
	}
	call(t, cs, "read_range", map[string]any{"path": path, "ref": "A2:D4"}, &r)
	want := `[["Tea",{"currency":3.5,"decimals":0},{"percent":0.12},{"date":"2026-09-29"}],` +
		`["Cake",{"currency":2,"decimals":0},{"percent":0.5},{"date":"2026-09-28"}],` +
		`["0042",{"currency":4.25,"symbol":"€"},null,{"date":"2026-10-01","format":"yyyy-mm-dd"}]]`
	if got, _ := json.Marshal(r.Values); string(got) != want {
		t.Errorf("after write_cells:\n%s\nwant\n%s", got, want)
	}
	f, err := headless.Open(path, false)
	if err != nil {
		t.Fatal(err)
	}
	s := f.Book.Sheet(0)
	for a, k := range map[string]sheet.FormatKind{"B3": sheet.FmtCurrency, "C2": sheet.FmtPercent, "D2": sheet.FmtDate, "E2": sheet.FmtDuration, "F2": sheet.FmtSize} {
		at, _ := sheet.ParseAddr(a)
		if got := s.DisplayFormat(at).Kind; got != k {
			t.Errorf("%s is formatted %v in the file, want %v", a, got, k)
		}
	}
}

// Values written through MCP go to nushell with their types as a
// notebook cell reads them ($sheet), and its output sent back to a
// sheet keeps the formats: currency, percentages, dates, durations and
// sizes.
func TestTypedValuesThroughNotebook(t *testing.T) {
	if _, err := exec.LookPath("nu"); err != nil {
		t.Skip("nu isn't installed")
	}
	w := sheet.NewBook()
	out, _ := w.AddSheet("Out", 1)
	nb, _ := w.AddNotebook("Notes", out)
	nb.SetNotebookCells("add cell", []notebook.Cell{{Kind: notebook.Code,
		Source: "dear = $sheet.Sheet1!A1:G3 | where Price > 3 | insert Kind {|r| $r.Size | describe} | insert Took {|r| $r.Steep | describe}"}})
	if err := out.AddRegion(sheet.Region{Name: "dear", At: sheet.Addr{}, Output: true}); err != nil {
		t.Fatal(err)
	}
	w.SetActive(w.Sheet(0))
	path := filepath.Join(t.TempDir(), "nb.012")
	writeBook(t, w, path)
	cs := connect(t, Options{Default: path, Notebooks: &headless.NotebookOptions{Runner: nushell.Nu{}, Timeout: 30 * time.Second}})
	call(t, cs, "write_table", map[string]any{"at": "Sheet1!A1", "columns": columns, "rows": groceries}, nil)
	var run headless.CellRun
	call(t, cs, "run_notebook_cell", map[string]any{"notebook": "Notes", "cell": 1}, &run)
	if run.State != "ran" {
		t.Fatalf("run: %+v", run)
	}
	var r headless.Range
	call(t, cs, "read_range", map[string]any{"ref": "Out!A1:I2"}, &r)
	want := `[["Item","Price","Share","Bought","Steep","Size","Code","Kind","Took"],` +
		`["Tea",{"currency":3.5},{"percent":0.12},{"date":"2026-09-29"},{"duration":"4min"},{"size":1500},"00123","filesize","duration"]]`
	if got, _ := json.Marshal(r.Values); string(got) != want {
		t.Errorf("the output sent to Out:\n%s\nwant\n%s\n%s", got, want, run.Output)
	}
}
