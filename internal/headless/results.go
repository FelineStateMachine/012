package headless

import (
	"bytes"
	"encoding/json"
	"os"

	"github.com/FelineStateMachine/012/internal/diff"
	"github.com/FelineStateMachine/012/internal/nuon"
)

// The results commands write with --format json or nuon, and the MCP
// tools return. Each is a stable schema (docs/reference/json.md): fields
// are only added, never renamed or removed.

// Change is one change as 012 diff --format json lists it: what kind
// of thing changed (cell, sheet, region, name, macro or a layout
// field), on which sheet, which item, which field of it, and its old
// and new values, null where it wasn't or isn't there.
type Change struct {
	Kind  string          `json:"kind"`
	Sheet string          `json:"sheet"`
	Item  string          `json:"item"`
	Field string          `json:"field"`
	Old   json.RawMessage `json:"old"`
	New   json.RawMessage `json:"new"`
}

// Changes are diff's changes as records.
func Changes(changes []diff.Change) []Change {
	out := make([]Change, 0, len(changes))
	for _, c := range changes {
		out = append(out, Change{Kind: c.Kind, Sheet: c.Sheet, Item: c.Item, Field: c.Field,
			Old: nuon.AppendJSON(nil, c.Old), New: nuon.AppendJSON(nil, c.New)})
	}
	return out
}

// SetResult is what 012 set writes: the file, whether it was saved (not
// with --dry-run), what changed, and the warnings of entries a rule
// marks invalid.
type SetResult struct {
	File     string   `json:"file"`
	Saved    bool     `json:"saved"`
	Changes  []Change `json:"changes"`
	Warnings []string `json:"warnings"`
}

// ProblemRecord is a formula showing an error.
type ProblemRecord struct {
	Cell  string `json:"cell"` // with its sheet: Q3!B7
	Sheet string `json:"sheet"`
	Addr  string `json:"addr"`
	Value string `json:"value"` // #DIV/0!
	Why   string `json:"why"`
	JEV   bool   `json:"jev"` // a JEV function's, answered with --jev
}

// Problems are problems as records.
func Problems(ps []Problem) []ProblemRecord {
	out := make([]ProblemRecord, 0, len(ps))
	for _, p := range ps {
		out = append(out, ProblemRecord{Cell: p.At(), Sheet: p.Sheet, Addr: p.Addr.String(), Value: p.Value, Why: p.Why, JEV: p.JEV})
	}
	return out
}

// RecalcResult is what 012 recalc writes.
type RecalcResult struct {
	File     string          `json:"file"`
	Errors   []ProblemRecord `json:"errors"`
	Circular bool            `json:"circular"`
	// NotebookFailures are the notebook cells --notebooks couldn't run.
	NotebookFailures []string `json:"notebook_failures"`
}

// ExportResult is what 012 export --report writes.
type ExportResult struct {
	File   string   `json:"file"`
	Format string   `json:"format"`
	Rows   int      `json:"rows"`
	Notes  []string `json:"notes"`
}

// Changes compares the workbook as it is with its file as it was
// opened (nothing, for a new file), before it's saved.
func (f *File) Changes() ([]diff.Change, error) {
	var old []byte
	if !f.New {
		var err error
		if old, err = os.ReadFile(f.Path); err != nil {
			return nil, err
		}
	}
	var now bytes.Buffer
	if err := f.Book.Write(&now); err != nil {
		return nil, err
	}
	a, err := diff.Read(old)
	if err != nil {
		return nil, err
	}
	b, err := diff.Read(now.Bytes())
	if err != nil {
		return nil, err
	}
	return diff.Compare(a, b), nil
}
