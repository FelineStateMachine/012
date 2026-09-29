package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FelineStateMachine/012/internal/notebook"
	"github.com/FelineStateMachine/012/internal/nushell"
	"github.com/FelineStateMachine/012/internal/sheet"
)

// scriptEnv is testEnv with standard error apart from standard output.
func scriptEnv(t *testing.T) (env, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	e, out, _ := testEnv(t, nil)
	errOut := &bytes.Buffer{}
	e.stderr = errOut
	return e, out, errOut
}

// status runs 012 with args and returns its exit status, as main would.
func status(e env, args ...string) (int, error) {
	err := run(args, e)
	if err == nil {
		return 0, nil
	}
	code, _ := exitCode(err)
	return code, err
}

func TestGetSetRecalc(t *testing.T) {
	e, out, errOut := scriptEnv(t)
	path := filepath.Join(t.TempDir(), "book.012")
	if code, err := status(e, "set", path, "A1", "Item", "B1", "Price", "A2", "Apple", "B2", "-1.5", "B3", "=SUM(B2:B2)*2"); code != 0 {
		t.Fatal(err)
	}
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"B3"}, "-3\n"},
		{[]string{"Sheet1!B3", "--input"}, "=SUM(B2:B2)*2\n"},
		{[]string{"A1:B2", "--format", "nuon"}, "[[Item, Price]; [Apple, -1.5]]\n"},
		{[]string{"--format=json", "B3"}, "-3\n"},
	} {
		out.Reset()
		if code, err := status(e, append([]string{"get", path}, c.args...)...); code != 0 || out.String() != c.want {
			t.Errorf("get %v: %d %v %q, want %q", c.args, code, err, out, c.want)
		}
	}
	for _, c := range []struct {
		args []string
		code int
		want string
	}{
		{[]string{"get", path, "Q3!A1"}, 1, `no sheet named "Q3"`},
		{[]string{"get", path, "A1", "--format", "xml"}, 2, "--format xml: 012 get writes text, csv, tsv, json, nuon"},
		{[]string{"get", path, "A1", "--bogus"}, 2, "unknown flag --bogus"},
		{[]string{"get", path, "A1", "--trust"}, 2, "--trust goes with --notebooks"},
		{[]string{"get", filepath.Join(t.TempDir(), "none.012"), "A1"}, 1, "no such file"},
		{[]string{"set", path, "A1"}, 2, "usage: 012 set"},
		{[]string{"set", path, "B4", "=SUM(B1"}, 1, "Sheet1!B4: Expected , or ) in SUM, at character 8 of =SUM(B1; " + path + " is unchanged"},
		{[]string{"recalc"}, 2, "usage: 012 recalc"},
	} {
		if code, err := status(e, c.args...); code != c.code || err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%v: %d %v, want %d and %q", c.args, code, err, c.code, c.want)
		}
	}

	out.Reset()
	if code, err := status(e, "recalc", path); code != 0 || out.Len() > 0 {
		t.Errorf("recalc without errors: %d %v %q", code, err, out)
	}
	status(e, "set", path, "C1", "=1/0", "--", "C2", "--")
	code, err := status(e, "recalc", path)
	if code != 1 || err.Error() != "1 cell showing errors" || !strings.HasPrefix(out.String(), "Sheet1!C1  #DIV/0!  ") {
		t.Errorf("recalc with an error: %d %v %q", code, err, out)
	}
	out.Reset()
	status(e, "get", path, "C2", "--input")
	if out.String() != "--\n" {
		t.Errorf("-- as a value after --: %q", out)
	}
	if errOut.Len() > 0 {
		t.Errorf("stderr: %s", errOut)
	}
}

func TestExport(t *testing.T) {
	e, _, errOut := scriptEnv(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "book.012")
	status(e, "set", path, "A1", "n", "A2", "1", "A3", "=A2+1")
	if code, err := status(e, "export", path, filepath.Join(dir, "out.csv")); code != 0 {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(filepath.Join(dir, "out.csv")); string(data) != "n\n1\n2\n" {
		t.Errorf("out.csv: %q", data)
	}
	if !strings.Contains(errOut.String(), "1 formula saved as values") {
		t.Errorf("notes: %q", errOut)
	}
	if code, err := status(e, "export", path, filepath.Join(dir, "out.txt"), "A2:A3", "--format", "json"); code != 0 {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(filepath.Join(dir, "out.txt")); string(data) != "[\n{\"1\":2}\n]\n" {
		t.Errorf("out.txt: %q", data)
	}
	if code, err := status(e, "export", path, filepath.Join(dir, "out.xlsx")); code != 0 {
		t.Fatal(err)
	}
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{path, filepath.Join(dir, "copy.012")}, "is a workbook: copy the file instead"},
		{[]string{path, filepath.Join(dir, "out.wk1")}, "012 reads WK1 files but doesn't write them"},
		{[]string{path, filepath.Join(dir, "out")}, "name the format with --format"},
		{[]string{path, path, "--format", "csv"}, "is the workbook being exported"},
	} {
		if _, err := status(e, append([]string{"export"}, c.args...)...); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("export %v: %v, want %q", c.args, err, c.want)
		}
	}
}

// fakeNu answers every command with one table.
type fakeNu struct{ ran int }

func (f *fakeNu) Run(_ context.Context, _ nushell.Job, _ string, stdout io.Writer) error {
	f.ran++
	_, err := io.WriteString(stdout, "[[name]; [a.txt]]")
	return err
}

// notebookFile writes a workbook whose notebook's cell files = ls is
// sent to Sheet1!A1, made on the computer origin.
func notebookFile(t *testing.T, origin string) string {
	t.Helper()
	w := sheet.NewBook()
	nb, _ := w.AddNotebook("Notebook", w.Sheet(0))
	nb.SetNotebookCells("add cell", []notebook.Cell{{Kind: notebook.Code, Source: "files = ls"}})
	a1, _ := sheet.ParseAddr("A1")
	if err := w.Sheet(0).AddRegion(sheet.Region{Name: "files", At: a1, Output: true}); err != nil {
		t.Fatal(err)
	}
	w.SetMacroOrigin(origin)
	path := filepath.Join(t.TempDir(), "nb.012")
	var b bytes.Buffer
	w.Write(&b)
	os.WriteFile(path, b.Bytes(), 0o644)
	return path
}

func TestNotebooksNeedTrust(t *testing.T) {
	e, out, errOut := scriptEnv(t)
	nu := &fakeNu{}
	e.nu = nu
	path := notebookFile(t, "another computer")

	status(e, "get", path, "A2")
	if nu.ran != 0 || out.String() != "\n" || !strings.Contains(errOut.String(), "Sheet1 shows notebook outputs as the file kept them; --notebooks runs the cells again") {
		t.Errorf("without --notebooks: ran %d, %q %q", nu.ran, out, errOut)
	}
	if code, err := status(e, "get", path, "A2", "--notebooks"); code != 1 || nu.ran != 0 || !strings.Contains(err.Error(), "saved on another computer") {
		t.Errorf("untrusted: %d %v, ran %d", code, err, nu.ran)
	}
	out.Reset()
	if code, err := status(e, "get", path, "A2", "--notebooks", "--trust"); code != 0 || nu.ran != 1 || out.String() != "a.txt\n" {
		t.Errorf("--trust: %d %v, ran %d, %q", code, err, nu.ran, out)
	}
	// get saves nothing, so the file still asks; recalc saves the trust,
	// and the output, which get then reads without running anything.
	if _, err := status(e, "recalc", path, "--notebooks"); err == nil {
		t.Error("get --trust saved the trust")
	}
	if code, err := status(e, "recalc", path, "--notebooks", "--trust"); code != 0 {
		t.Fatal(err)
	}
	out.Reset()
	if code, err := status(e, "get", path, "A2"); code != 0 || nu.ran != 2 || out.String() != "a.txt\n" {
		t.Errorf("the output saved: %d %v, ran %d, %q", code, err, nu.ran, out)
	}
	if code, err := status(e, "get", path, "A2", "--notebooks"); code != 0 || nu.ran != 3 {
		t.Errorf("trusted by recalc: %d %v, ran %d", code, err, nu.ran)
	}

	writeConfig(t, "shell = off\n")
	if _, err := status(e, "get", path, "A1", "--notebooks", "--trust"); err == nil || !strings.Contains(err.Error(), "shell = off") {
		t.Errorf("shell = off: %v", err)
	}
	writeConfig(t, "shell = on\n")
	other := notebookFile(t, "elsewhere")
	if code, err := status(e, "get", other, "A1", "--notebooks"); code != 0 {
		t.Errorf("shell = on: %v", err)
	}
}

func TestJEVNeedsKey(t *testing.T) {
	e, _, _ := scriptEnv(t)
	path := filepath.Join(t.TempDir(), "book.012")
	status(e, "set", path, "A1", "1")
	if _, err := status(e, "get", path, "A1", "--jev"); err == nil || !strings.Contains(err.Error(), "--jev needs an API key") {
		t.Errorf("--jev without a key: %v", err)
	}
}
