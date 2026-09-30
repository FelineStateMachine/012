package mcp

import (
	"context"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/FelineStateMachine/012/internal/headless"
	"github.com/FelineStateMachine/012/internal/sheet"
)

// The tools that change a workbook. Each is one call to the backend's
// Change, so one undo step and one save, and returns what changed as
// 012 diff lists it; dry_run returns it without keeping it.

type writeIn struct {
	bookIn
	Entries []headless.Entry `json:"entries" jsonschema:"the cells to set, in order: each a ref (one cell) and its input, typed as a person types it in en-US form; an empty input clears the cell"`
	DryRun  bool             `json:"dry_run,omitempty" jsonschema:"check the entries and return the change without making it"`
}

type opsIn struct {
	bookIn
	Operations []headless.Operation `json:"operations" jsonschema:"the operations, made in order as one change"`
	DryRun     bool                 `json:"dry_run,omitempty" jsonschema:"return the change without making it"`
}

type sortIn struct {
	bookIn
	Ref    string             `json:"ref" jsonschema:"the range to sort"`
	Keys   []headless.SortKey `json:"keys" jsonschema:"the columns to sort by, first first"`
	Header bool               `json:"header,omitempty" jsonschema:"the first row is a header and stays in place"`
	DryRun bool               `json:"dry_run,omitempty"`
}

type filterIn struct {
	bookIn
	Ref     string                  `json:"ref" jsonschema:"the range to filter, its first row the header"`
	Columns []headless.FilterColumn `json:"columns,omitempty" jsonschema:"each filtered column's criteria"`
	Remove  bool                    `json:"remove,omitempty" jsonschema:"take the sheet's filter off instead"`
	DryRun  bool                    `json:"dry_run,omitempty"`
}

type chartIn struct {
	bookIn
	headless.ChartSpec
	DryRun bool `json:"dry_run,omitempty"`
}

type pivotIn struct {
	bookIn
	headless.PivotSpec
	DryRun bool `json:"dry_run,omitempty"`
}

type runCellIn struct {
	bookIn
	Notebook string `json:"notebook" jsonschema:"the notebook tab's name"`
	Cell     int    `json:"cell" jsonschema:"the cell's number, 1 for the first, as describe lists them"`
}

// Changed is what a write returns: whether it was kept, what changed
// as 012 diff lists it, and warnings of entries a rule marks invalid.
type Changed struct {
	Saved    bool              `json:"saved"`
	Changes  []headless.Change `json:"changes"`
	Warnings []string          `json:"warnings"`
}

type chartOut struct {
	Chart headless.ChartMade `json:"chart"`
	Changed
	View *View `json:"view,omitempty"`
}

type pivotOut struct {
	Sheet string `json:"sheet"`
	Changed
}

var writes = &sdk.ToolAnnotations{DestructiveHint: ptr(false), OpenWorldHint: ptr(false)}

func (s *Server) addWriteTools() {
	set := headless.SetOptions{Force: s.opts.Force}
	tool(s, &sdk.Tool{Name: "write_cells", Annotations: writes,
		Description: "Type entries into cells as one change, as 012 set does: each checked as typing it is (a formula must parse, validation rules, protected ranges), stopping at the first a cell can't take."},
		func(ctx context.Context, req *sdk.CallToolRequest, in writeIn) (*sdk.CallToolResult, Changed, error) {
			var warnings []string
			return s.change(ctx, req, in.Path, "set", in.DryRun, &warnings, func(w *sheet.Workbook) (err error) {
				warnings, err = headless.Set(w, in.Entries, set)
				return err
			})
		})
	tool(s, &sdk.Tool{Name: "apply_operations", Annotations: &sdk.ToolAnnotations{DestructiveHint: ptr(true), OpenWorldHint: ptr(false)},
		Description: "Make several operations as one change: set, clear, insert_rows, delete_rows, insert_columns, delete_columns, add_sheet, rename_sheet, delete_sheet, define_name, sort."},
		func(ctx context.Context, req *sdk.CallToolRequest, in opsIn) (*sdk.CallToolResult, Changed, error) {
			var warnings []string
			return s.change(ctx, req, in.Path, "apply operations", in.DryRun, &warnings, func(w *sheet.Workbook) (err error) {
				warnings, err = headless.Apply(w, in.Operations, set)
				return err
			})
		})
	tool(s, &sdk.Tool{Name: "sort", Annotations: writes, Description: "Sort a range's rows by columns, as Data > Sort range does."},
		func(ctx context.Context, req *sdk.CallToolRequest, in sortIn) (*sdk.CallToolResult, Changed, error) {
			return s.change(ctx, req, in.Path, "sort", in.DryRun, nil, func(w *sheet.Workbook) error { return headless.Sort(w, in.Ref, in.Keys, in.Header) })
		})
	tool(s, &sdk.Tool{Name: "filter", Annotations: writes,
		Description: "Put a filter on a range, hiding the rows its columns' conditions or values leave out, as Data > Create a filter does; or remove the sheet's filter."},
		func(ctx context.Context, req *sdk.CallToolRequest, in filterIn) (*sdk.CallToolResult, Changed, error) {
			return s.change(ctx, req, in.Path, "filter", in.DryRun, nil, func(w *sheet.Workbook) error { return headless.Filter(w, in.Ref, in.Columns, in.Remove) })
		})
	tool(s, &sdk.Tool{Name: "create_chart", Annotations: writes, Meta: viewMeta("Adding the chart", "Added the chart"),
		Description: "Add a chart of a range, as Insert > Chart does, with the header row, labels and title guessed unless given."},
		s.createChart)
	tool(s, &sdk.Tool{Name: "create_pivot", Annotations: writes,
		Description: "Add a pivot table on a new sheet, summarizing a range whose first row names its columns."},
		func(ctx context.Context, req *sdk.CallToolRequest, in pivotIn) (*sdk.CallToolResult, pivotOut, error) {
			var out pivotOut
			_, changed, err := s.change(ctx, req, in.Path, "create pivot table", in.DryRun, nil, func(w *sheet.Workbook) (err error) {
				out.Sheet, err = headless.AddPivot(w, in.PivotSpec)
				return err
			})
			out.Changed = changed
			return nil, out, err
		})
	s.addCreateTool()
	if s.opts.Notebooks != nil {
		tool(s, &sdk.Tool{Name: "run_notebook_cell", Annotations: &sdk.ToolAnnotations{DestructiveHint: ptr(false), OpenWorldHint: ptr(true)},
			Description: "Run a notebook tab's code cell with nushell, as Run does on the screen: its output replaces the one kept and goes on to the sheet it was sent to."},
			s.runCell)
	}
}

func (s *Server) createChart(ctx context.Context, req *sdk.CallToolRequest, in chartIn) (*sdk.CallToolResult, chartOut, error) {
	var out chartOut
	b, err := s.open(ctx, req, in.Path)
	if err != nil {
		return nil, out, err
	}
	_, changed, err := s.change(ctx, req, in.Path, "insert chart", in.DryRun, nil, func(w *sheet.Workbook) (err error) {
		if out.Chart, err = headless.AddChart(w, in.ChartSpec); err != nil {
			return err
		}
		out.View = chartView(b, w, out.Chart)
		return nil
	})
	if err != nil {
		return nil, chartOut{}, err
	}
	out.Changed = changed
	model := out
	model.View = nil
	return forModel(model), out, nil
}

func (s *Server) runCell(ctx context.Context, req *sdk.CallToolRequest, in runCellIn) (*sdk.CallToolResult, headless.CellRun, error) {
	var out headless.CellRun
	b, err := s.open(ctx, req, in.Path)
	if err != nil {
		return nil, out, err
	}
	_, _, err = s.change(ctx, req, in.Path, "run notebook cell", false, nil, func(w *sheet.Workbook) error {
		if s.opts.MayRun != nil {
			if err := s.opts.MayRun(b.path, w); err != nil {
				return err
			}
		}
		var err error
		out, err = headless.RunCell(ctx, w, in.Notebook, in.Cell, *s.opts.Notebooks)
		return err
	})
	return nil, out, err
}

// change makes fn a change of the workbook path names and says what it
// changed; warnings, when set, are fn's.
func (s *Server) change(ctx context.Context, req *sdk.CallToolRequest, path, label string, dryRun bool, warnings *[]string, fn func(*sheet.Workbook) error) (*sdk.CallToolResult, Changed, error) {
	b, err := s.open(ctx, req, path)
	if err != nil {
		return nil, Changed{}, err
	}
	changes, err := b.Change(ctx, label, dryRun, fn)
	if err != nil {
		return nil, Changed{}, err
	}
	out := Changed{Saved: !dryRun && len(changes) > 0, Changes: headless.Changes(changes), Warnings: []string{}}
	if warnings != nil && *warnings != nil {
		out.Warnings = *warnings
	}
	if out.Saved {
		s.refreshResources(ctx)
	}
	return nil, out, nil
}
