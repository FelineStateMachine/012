package headless

import (
	"context"
	"encoding/base64"
	"testing"
	"time"

	"github.com/FelineStateMachine/012/internal/notebook"
	"github.com/FelineStateMachine/012/internal/sheet"
)

// --notebooks runs a cell of several statements as the screen does: its
// own variable isn't a cell it reads, and a later cell reads it from its
// run.
func TestRunNotebookStatements(t *testing.T) {
	w := sheet.NewBook()
	nb, err := w.AddNotebook("Notebook", w.Sheet(0))
	if err != nil {
		t.Fatal(err)
	}
	src := "files = ls | where type == file # the files\n$files | where size > 1kb"
	nb.SetNotebookCells("add cells", []notebook.Cell{
		{Kind: notebook.Code, Source: "$files | length"},
		{Kind: notebook.Code, Source: src},
	})
	cmd, _, _ := notebook.Parse(src).Command()
	enc := func(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }
	nu := fakeNu{
		cmd:               "__out " + enc("[[name]; [b]]") + "\nfiles " + enc("[[name]; [a], [b]]") + "\n",
		"$files | length": "2",
	}
	if failed := RunNotebooks(context.Background(), w, NotebookOptions{Runner: nu, Timeout: time.Second}); len(failed) != 0 {
		t.Fatalf("failed: %q", failed)
	}
	cells := nb.NotebookCells()
	if o := w.Output(cells[1].ID); o == nil || string(o.NUON) != "[[name]; [b]]" || string(o.Vars["files"]) != "[[name]; [a], [b]]" {
		t.Errorf("the cell of two statements: %+v", o)
	}
	if o := w.Output(cells[0].ID); o == nil || string(o.NUON) != "2" || o.Reads["files"] == 0 {
		t.Errorf("the cell reading $files: %+v", o)
	}
}
