package mcp

import (
	"context"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/FelineStateMachine/012/internal/headless"
	"github.com/FelineStateMachine/012/internal/sheet"
)

// The tools that read the workbook, or compute on a copy thrown away.

type describeIn struct{}

type readIn struct {
	Ref  string `json:"ref" jsonschema:"a cell, range, named range, table or sheet, as formulas write it: Q3!A1:D40, Sales, Sales[Amount], Q3"`
	Text bool   `json:"text,omitempty" jsonschema:"also return each cell as the sheet shows it ($1,200.00, 9/29/2026)"`
	Max  int    `json:"max_cells,omitempty" jsonschema:"the most cells to return, whole rows at a time; 10000 when 0"`
}

type evaluateIn struct {
	Formula string `json:"formula" jsonschema:"the formula, with or without its =: =SUMIF(B:B, \"North\", C:C)"`
	At      string `json:"at,omitempty" jsonschema:"the cell it's computed in, which relative references are relative to; below the shown sheet's data when empty"`
}

type findIn struct {
	Query string `json:"query" jsonschema:"what to look for"`
	headless.FindOptions
}

type errorsIn struct{}

// errorsOut lists the formulas showing errors.
type errorsOut struct {
	Errors   []headless.ProblemRecord `json:"errors"`
	Circular bool                     `json:"circular"`
}

var readOnly = &sdk.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true, OpenWorldHint: ptr(false)}

func ptr[T any](v T) *T { return &v }

func (s *Server) addReadTools() {
	tool(s, &sdk.Tool{Name: "describe", Annotations: readOnly,
		Description: "What the workbook holds: each sheet's used range, guessed header row and column names, tables, notebook outputs and linked files, charts, pivot tables, notebook cells, and the named ranges. Call it first."},
		func(ctx context.Context, _ *sdk.CallToolRequest, _ describeIn) (*sdk.CallToolResult, headless.Description, error) {
			var d headless.Description
			err := s.b.View(ctx, func(w *sheet.Workbook) error { d = headless.Describe(w); return nil })
			return nil, d, err
		})
	tool(s, &sdk.Tool{Name: "read_range", Annotations: readOnly, Meta: s.viewMeta(),
		Description: "The values of a range as rows (numbers, strings, booleans, null for blank, dates as ISO 8601, errors as #DIV/0!), with the formulas in it by cell."},
		s.readRange)
	tool(s, &sdk.Tool{Name: "evaluate", Annotations: readOnly,
		Description: "Compute a formula in the workbook without writing it: its value, as the cell would show it, why it's an error, and an array's spilled values."},
		func(ctx context.Context, _ *sdk.CallToolRequest, in evaluateIn) (*sdk.CallToolResult, headless.Evaluated, error) {
			var out headless.Evaluated
			err := s.b.Scratch(ctx, func(w *sheet.Workbook) error {
				var err error
				out, err = headless.Evaluate(w, in.Formula, in.At)
				return err
			})
			return nil, out, err
		})
	tool(s, &sdk.Tool{Name: "find", Annotations: readOnly,
		Description: "Find cells by what they show (or their formulas' text), as Edit > Find does: the cells, their text and what was typed in them."},
		func(ctx context.Context, _ *sdk.CallToolRequest, in findIn) (*sdk.CallToolResult, headless.Found, error) {
			var out headless.Found
			err := s.b.View(ctx, func(w *sheet.Workbook) error {
				var err error
				out, err = headless.Find(w, in.Query, in.FindOptions)
				return err
			})
			return nil, out, err
		})
	tool(s, &sdk.Tool{Name: "list_errors", Annotations: readOnly,
		Description: "The formulas showing errors after recalculating everything, with what the screen says about each (as 012 recalc lists them), without saving."},
		func(ctx context.Context, _ *sdk.CallToolRequest, _ errorsIn) (*sdk.CallToolResult, errorsOut, error) {
			var out errorsOut
			err := s.b.Scratch(ctx, func(w *sheet.Workbook) error {
				problems, circular := headless.Recalc(w)
				out = errorsOut{Errors: headless.Problems(problems), Circular: circular}
				return nil
			})
			return nil, out, err
		})
}

func (s *Server) readRange(ctx context.Context, req *sdk.CallToolRequest, in readIn) (*sdk.CallToolResult, headless.Range, error) {
	var out headless.Range
	var res *sdk.CallToolResult
	err := s.b.View(ctx, func(w *sheet.Workbook) error {
		t, err := headless.Resolve(w, in.Ref)
		if err != nil {
			return err
		}
		out = headless.ReadRange(t, headless.ReadOptions{MaxCells: in.Max, Text: in.Text})
		res = s.rangeView(req, t)
		return nil
	})
	return res, out, err
}
