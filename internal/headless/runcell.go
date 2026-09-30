package headless

import (
	"context"
	"fmt"

	"github.com/FelineStateMachine/012/internal/notebook"
	"github.com/FelineStateMachine/012/internal/sheet"
)

// CellRun is what running a notebook cell gave: its state, its output
// as NUON (cut at maxShownOutput), or why it failed.
type CellRun struct {
	Notebook string `json:"notebook"`
	Cell     int    `json:"cell"`
	Name     string `json:"name"`
	State    string `json:"state"` // "ran" or "failed"
	Error    string `json:"error"`
	Output   string `json:"output"` // NUON
	Cut      bool   `json:"cut"`    // Output was longer and is cut
}

// maxShownOutput is the most of an output CellRun returns.
const maxShownOutput = 64 << 10

// RunCell runs cell n (1 for the first) of the notebook tab named, as
// Run does on the screen: its output replaces the one kept and goes on
// to the sheet it was sent to. Whether it may run at all (trust, the
// shell setting) is the caller's to decide first.
func RunCell(ctx context.Context, w *sheet.Workbook, name string, n int, o NotebookOptions) (CellRun, error) {
	s := w.Lookup(name)
	if s == nil || !s.IsNotebook() {
		return CellRun{}, fmt.Errorf("no notebook named %q", name)
	}
	cells := s.NotebookCells()
	if n < 1 || n > len(cells) {
		return CellRun{}, fmt.Errorf("%s has %d cells: cell %d isn't one", name, len(cells), n)
	}
	c := cells[n-1]
	if c.Kind != notebook.Code {
		return CellRun{}, fmt.Errorf("%s cell %d is a note, which doesn't run", name, n)
	}
	out := CellRun{Notebook: s.Name(), Cell: n, Name: c.Name(), State: "ran"}
	if err := runCell(ctx, w, cells, n-1, 1, o); err != nil {
		out.State, out.Error = "failed", err.Error()
		return out, nil
	}
	if res := w.Output(c.ID); res != nil {
		out.Output = string(res.NUON)
		if len(out.Output) > maxShownOutput {
			out.Output, out.Cut = out.Output[:maxShownOutput], true
		}
	}
	return out, nil
}
