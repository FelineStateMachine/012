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

// whyIn is a change's message, in live mode.
type whyIn struct {
	Message string `json:"message,omitempty" jsonschema:"live mode: why, shown to the person with your suggestion"`
}

type writeIn struct {
	bookIn
	whyIn
	Entries []headless.Entry `json:"entries" jsonschema:"the cells to set, in order: each a ref (one cell) and its input as a person types it, or its value with its type, and a format; an empty input or a null value clears the cell"`
	DryRun  bool             `json:"dry_run,omitempty" jsonschema:"check the entries and return the change without making it"`
}

type tableIn struct {
	bookIn
	whyIn
	headless.TableSpec
	DryRun bool `json:"dry_run,omitempty" jsonschema:"check the rows and return the change without making it"`
}

type opsIn struct {
	bookIn
	whyIn
	Operations []headless.Operation `json:"operations" jsonschema:"the operations, made in order as one change"`
	DryRun     bool                 `json:"dry_run,omitempty" jsonschema:"return the change without making it"`
}

type sortIn struct {
	bookIn
	whyIn
	Ref    string             `json:"ref" jsonschema:"the range to sort"`
	Keys   []headless.SortKey `json:"keys" jsonschema:"the columns to sort by, first first"`
	Header bool               `json:"header,omitempty" jsonschema:"the first row is a header and stays in place"`
	DryRun bool               `json:"dry_run,omitempty"`
}

type filterIn struct {
	bookIn
	whyIn
	Ref     string                  `json:"ref" jsonschema:"the range to filter, its first row the header"`
	Columns []headless.FilterColumn `json:"columns,omitempty" jsonschema:"each filtered column's criteria"`
	Remove  bool                    `json:"remove,omitempty" jsonschema:"take the sheet's filter off instead"`
	DryRun  bool                    `json:"dry_run,omitempty"`
}

type chartIn struct {
	bookIn
	whyIn
	headless.ChartSpec
	DryRun bool `json:"dry_run,omitempty"`
}

type pivotIn struct {
	bookIn
	whyIn
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
	// Suggestion and Notices are live mode's: the change waiting for
	// the person, and what became of earlier ones.
	Suggestion int      `json:"suggestion,omitempty" jsonschema:"live mode: the suggestion's number, waiting for the person to accept or reject it"`
	Notices    []string `json:"notices,omitempty" jsonschema:"live mode: what became of your earlier suggestions"`
}

type chartOut struct {
	Chart headless.ChartMade `json:"chart"`
	Changed
}

type pivotOut struct {
	Sheet string `json:"sheet"`
	Changed
}

var writes = &sdk.ToolAnnotations{DestructiveHint: ptr(false), OpenWorldHint: ptr(false)}

func (s *Server) addWriteTools() {
	set := headless.SetOptions{Force: s.opts.Force}
	tool(s, &sdk.Tool{Name: "write_cells", Annotations: writes,
		Description: "Set cells as one change, as 012 set does: each checked as typing it is (a formula must parse, validation rules, protected ranges), stopping at the first a cell can't take. " +
			"Write money, percentages, dates, times, durations and sizes with their types, never as bare numbers: an input as a person types it ($3.50, 12%, 2026-09-29), or a value ({\"currency\": 3.5}, {\"percent\": 0.12}, {\"date\": \"2026-09-29\"}, {\"duration\": \"90min\"}, {\"size\": \"1.5kb\"}); \"00123\" as a value stays text. " +
			"format sets a number format code on the cell, or alone on a range. For rows of records, write_table."},
		func(ctx context.Context, req *sdk.CallToolRequest, in writeIn) (*sdk.CallToolResult, Changed, error) {
			var warnings []string
			return s.change(ctx, req, in.Path, "set", in.Message, in.DryRun, &warnings, func(w *sheet.Workbook) (err error) {
				warnings, err = headless.Set(w, in.Entries, set)
				return err
			})
		})
	tool(s, &sdk.Tool{Name: "write_table", Annotations: writes,
		Description: "Write a table as one change: a header row at `at` naming the records' fields, then a row for each record, its values typed as write_cells takes them, " +
			"each column formatted as its first typed value is (or as formats says), as importing a NUON table does. " +
			"Example rows: [{\"Item\": \"Tea\", \"Price\": {\"currency\": 3.5}, \"Share\": {\"percent\": 0.12}, \"Bought\": {\"date\": \"2026-09-29\"}, \"Code\": \"00123\"}]."},
		func(ctx context.Context, req *sdk.CallToolRequest, in tableIn) (*sdk.CallToolResult, Changed, error) {
			var warnings []string
			return s.change(ctx, req, in.Path, "write table", in.Message, in.DryRun, &warnings, func(w *sheet.Workbook) (err error) {
				warnings, err = headless.WriteTable(w, in.TableSpec, set)
				return err
			})
		})
	tool(s, &sdk.Tool{Name: "apply_operations", Annotations: &sdk.ToolAnnotations{DestructiveHint: ptr(true), OpenWorldHint: ptr(false)},
		Description: "Make several operations as one change: set (input or typed value, and format, as write_cells), write_table (rows at ref, as write_table), clear, insert_rows, delete_rows, insert_columns, delete_columns, add_sheet, rename_sheet, delete_sheet, define_name, sort."},
		func(ctx context.Context, req *sdk.CallToolRequest, in opsIn) (*sdk.CallToolResult, Changed, error) {
			var warnings []string
			return s.change(ctx, req, in.Path, "apply operations", in.Message, in.DryRun, &warnings, func(w *sheet.Workbook) (err error) {
				warnings, err = headless.Apply(w, in.Operations, set)
				return err
			})
		})
	tool(s, &sdk.Tool{Name: "sort", Annotations: writes, Description: "Sort a range's rows by columns, as Data > Sort range does."},
		func(ctx context.Context, req *sdk.CallToolRequest, in sortIn) (*sdk.CallToolResult, Changed, error) {
			return s.change(ctx, req, in.Path, "sort", in.Message, in.DryRun, nil, func(w *sheet.Workbook) error { return headless.Sort(w, in.Ref, in.Keys, in.Header) })
		})
	tool(s, &sdk.Tool{Name: "filter", Annotations: writes,
		Description: "Put a filter on a range, hiding the rows its columns' conditions or values leave out, as Data > Create a filter does; or remove the sheet's filter."},
		func(ctx context.Context, req *sdk.CallToolRequest, in filterIn) (*sdk.CallToolResult, Changed, error) {
			return s.change(ctx, req, in.Path, "filter", in.Message, in.DryRun, nil, func(w *sheet.Workbook) error { return headless.Filter(w, in.Ref, in.Columns, in.Remove) })
		})
	tool(s, &sdk.Tool{Name: "create_chart", Annotations: writes, Meta: viewMeta("Adding the chart", "Added the chart"),
		Description: "Add a chart of a range, as Insert > Chart does, with the header row, labels and title guessed unless given."},
		s.createChart)
	tool(s, &sdk.Tool{Name: "create_pivot", Annotations: writes,
		Description: "Add a pivot table on a new sheet, summarizing a range whose first row names its columns."},
		func(ctx context.Context, req *sdk.CallToolRequest, in pivotIn) (*sdk.CallToolResult, pivotOut, error) {
			var out pivotOut
			_, changed, err := s.change(ctx, req, in.Path, "create pivot table", in.Message, in.DryRun, nil, func(w *sheet.Workbook) (err error) {
				out.Sheet, err = headless.AddPivot(w, in.PivotSpec)
				return err
			})
			out.Changed = changed
			return nil, out, err
		})
	if s.opts.Live != nil {
		return // the live tools run notebook cells; there are no files to create
	}
	s.addCreateTool()
	if s.opts.Notebooks != nil {
		tool(s, &sdk.Tool{Name: "run_notebook_cell", Annotations: &sdk.ToolAnnotations{DestructiveHint: ptr(false), OpenWorldHint: ptr(true)},
			Description: "Run a notebook tab's code cell with nushell, as Run does on the screen: its output replaces the one kept and goes on to the sheet it was sent to."},
			s.runCell)
	}
}

func (s *Server) createChart(ctx context.Context, req *sdk.CallToolRequest, in chartIn) (*sdk.CallToolResult, chartOut, error) {
	var out chartOut
	var view *View
	b, err := s.open(ctx, req, in.Path)
	if err != nil {
		return nil, out, err
	}
	_, changed, err := s.change(ctx, req, in.Path, "insert chart", in.Message, in.DryRun, nil, func(w *sheet.Workbook) (err error) {
		if out.Chart, err = headless.AddChart(w, in.ChartSpec); err != nil {
			return err
		}
		view = chartView(b, w, out.Chart)
		return nil
	})
	if err != nil {
		return nil, chartOut{}, err
	}
	out.Changed = changed
	return viewResult(view), out, nil
}

func (s *Server) runCell(ctx context.Context, req *sdk.CallToolRequest, in runCellIn) (*sdk.CallToolResult, headless.CellRun, error) {
	var out headless.CellRun
	b, err := s.open(ctx, req, in.Path)
	if err != nil {
		return nil, out, err
	}
	_, _, err = s.change(ctx, req, in.Path, "run notebook cell", "", false, nil, func(w *sheet.Workbook) error {
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
func (s *Server) change(ctx context.Context, req *sdk.CallToolRequest, path, label, message string, dryRun bool, warnings *[]string, fn func(*sheet.Workbook) error) (*sdk.CallToolResult, Changed, error) {
	b, err := s.open(ctx, req, path)
	if err != nil {
		return nil, Changed{}, err
	}
	var made Made
	if l, ok := b.Backend.(Live); ok {
		made, err = l.Make(ctx, Making{Label: label, Message: message, DryRun: dryRun, Fn: fn})
	} else {
		made.Changes, err = b.Change(ctx, label, dryRun, fn)
		made.Applied = !dryRun
	}
	if err != nil {
		return nil, Changed{}, err
	}
	out := Changed{Saved: made.Applied && len(made.Changes) > 0, Changes: headless.Changes(made.Changes), Warnings: []string{},
		Suggestion: made.Suggestion, Notices: made.Notices}
	if warnings != nil && *warnings != nil {
		out.Warnings = *warnings
	}
	if out.Saved {
		s.refreshResources(ctx)
	}
	return nil, out, nil
}
