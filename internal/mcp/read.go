package mcp

import (
	"context"
	"slices"

	"github.com/google/jsonschema-go/jsonschema"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/FelineStateMachine/012/internal/headless"
	"github.com/FelineStateMachine/012/internal/sheet"
)

// The tools that read a workbook, or compute on a copy thrown away.

type describeIn struct{ bookIn }

// describeOut is a workbook described, or with no path the workbooks
// open to the server.
type describeOut struct {
	Path string `json:"path,omitempty" jsonschema:"the workbook described"`
	*headless.Description
	Roots     []string     `json:"roots,omitempty" jsonschema:"without a path: the folders open to the server"`
	Workbooks []listedFile `json:"workbooks,omitempty" jsonschema:"without a path: the workbooks in them"`
	More      bool         `json:"more,omitempty" jsonschema:"there were more workbooks than listed"`
}

type readIn struct {
	bookIn
	Ref  string `json:"ref" jsonschema:"a cell, range, named range, table or sheet, as formulas write it: Q3!A1:D40, Sales, Sales[Amount], Q3"`
	Text *bool  `json:"text,omitempty" jsonschema:"false leaves out each cell as the sheet shows it ($1,200.00, 9/29/2026), which comes back beside the values otherwise"`
	Max  int    `json:"max_cells,omitempty" jsonschema:"the most cells to return, whole rows at a time; 10000 when 0"`
}

// RangeRead is headless.Range under another name, which a struct
// embedding it can have beside its own field Range.
type RangeRead = headless.Range

type evaluateIn struct {
	bookIn
	Formula string `json:"formula" jsonschema:"the formula, with or without its =: =SUMIF(B:B, \"North\", C:C)"`
	At      string `json:"at,omitempty" jsonschema:"the cell it's computed in, which relative references are relative to; below the shown sheet's data when empty"`
}

type findIn struct {
	bookIn
	Query string `json:"query" jsonschema:"what to look for"`
	headless.FindOptions
}

type errorsIn struct{ bookIn }

// errorsOut lists the formulas showing errors.
type errorsOut struct {
	Errors   []headless.ProblemRecord `json:"errors"`
	Circular bool                     `json:"circular"`
}

var readOnly = &sdk.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true, OpenWorldHint: ptr(false)}

func ptr[T any](v T) *T { return &v }

func (s *Server) addReadTools() {
	tool(s, &sdk.Tool{Name: "describe", Annotations: readOnly, OutputSchema: describeSchema(),
		Description: "Without a path: the workbooks (.012 files, and files 012 imports) in the folders open to the server. With one: what the workbook holds, each sheet's used range, guessed header row and column names, tables, notebook outputs and linked files, charts, pivot tables, notebook cells, and the named ranges. Call it first."},
		s.describe)
	tool(s, &sdk.Tool{Name: "read_range", Annotations: readOnly, Meta: viewMeta("Reading the range", "Read the range"),
		Description: "The values of a range as rows, typed by their formats (numbers, strings, booleans, null for blank, {\"currency\": 3.5}, {\"percent\": 0.12}, {\"date\": \"2026-09-29\"}, {\"duration\": \"90min\"}, {\"size\": 1500}, errors as #DIV/0!), each cell as the sheet shows it ($3.50, 12%, 9/29/2026), and the formulas in it by cell. Hosts that draw views show it as 012's grid."},
		s.readRange)
	tool(s, &sdk.Tool{Name: "evaluate", Annotations: readOnly,
		Description: "Compute a formula in the workbook without writing it: its value, as the cell would show it, why it's an error, and an array's spilled values."},
		func(ctx context.Context, req *sdk.CallToolRequest, in evaluateIn) (*sdk.CallToolResult, headless.Evaluated, error) {
			var out headless.Evaluated
			err := s.scratch(ctx, req, in.Path, func(w *sheet.Workbook) (err error) {
				out, err = headless.Evaluate(w, in.Formula, in.At)
				return err
			})
			return nil, out, err
		})
	tool(s, &sdk.Tool{Name: "find", Annotations: readOnly,
		Description: "Find cells by what they show (or their formulas' text), as Edit > Find does: the cells, their text and what was typed in them."},
		func(ctx context.Context, req *sdk.CallToolRequest, in findIn) (*sdk.CallToolResult, headless.Found, error) {
			var out headless.Found
			err := s.view(ctx, req, in.Path, func(_ *book, w *sheet.Workbook) (err error) {
				out, err = headless.Find(w, in.Query, in.FindOptions)
				return err
			})
			return nil, out, err
		})
	tool(s, &sdk.Tool{Name: "list_errors", Annotations: readOnly,
		Description: "The formulas showing errors after recalculating everything, with what the screen says about each (as 012 recalc lists them), without saving."},
		func(ctx context.Context, req *sdk.CallToolRequest, in errorsIn) (*sdk.CallToolResult, errorsOut, error) {
			var out errorsOut
			err := s.scratch(ctx, req, in.Path, func(w *sheet.Workbook) error {
				problems, circular := headless.Recalc(w)
				out = errorsOut{Errors: headless.Problems(problems), Circular: circular}
				return nil
			})
			return nil, out, err
		})
}

// describeSchema is describeOut's, whose workbook's sheets and names
// are there only with a path.
func describeSchema() *jsonschema.Schema {
	schema, err := jsonschema.For[describeOut](rawSchemas)
	if err != nil {
		panic(err)
	}
	schema.Required = slices.DeleteFunc(schema.Required, func(p string) bool { return p == "sheets" || p == "names" })
	return schema
}

func (s *Server) describe(ctx context.Context, req *sdk.CallToolRequest, in describeIn) (*sdk.CallToolResult, describeOut, error) {
	if in.Path == "" && s.opts.Default == "" && s.opts.Live == nil {
		roots := s.roots(ctx, req.Session)
		files, more := s.listFiles(roots)
		return nil, describeOut{Roots: rootDirs(roots), Workbooks: files, More: more}, nil
	}
	var out describeOut
	err := s.view(ctx, req, in.Path, func(b *book, w *sheet.Workbook) error {
		d := headless.Describe(w)
		out = describeOut{Path: b.name, Description: &d}
		return nil
	})
	return nil, out, err
}

func (s *Server) readRange(ctx context.Context, req *sdk.CallToolRequest, in readIn) (*sdk.CallToolResult, RangeRead, error) {
	var out RangeRead
	var view *View
	err := s.view(ctx, req, in.Path, func(b *book, w *sheet.Workbook) error {
		t, err := headless.Resolve(w, in.Ref)
		if err != nil {
			return err
		}
		out = headless.ReadRange(t, headless.ReadOptions{MaxCells: in.Max, NoText: in.Text != nil && !*in.Text})
		view = rangeView(b, t, out.Rows)
		return nil
	})
	if err != nil {
		return nil, RangeRead{}, err
	}
	return viewResult(view), out, nil
}

// view runs fn on the workbook path names, as it is.
func (s *Server) view(ctx context.Context, req *sdk.CallToolRequest, path string, fn func(*book, *sheet.Workbook) error) error {
	b, err := s.open(ctx, req, path)
	if err != nil {
		return err
	}
	return b.View(ctx, func(w *sheet.Workbook) error { return fn(b, w) })
}

// scratch runs fn on a copy of the workbook path names, thrown away.
func (s *Server) scratch(ctx context.Context, req *sdk.CallToolRequest, path string, fn func(*sheet.Workbook) error) error {
	b, err := s.open(ctx, req, path)
	if err != nil {
		return err
	}
	return b.Scratch(ctx, fn)
}

// open is the workbook path names for req's client, its resources
// listed from now on.
func (s *Server) open(ctx context.Context, req *sdk.CallToolRequest, path string) (*book, error) {
	var ss *sdk.ServerSession
	if req != nil {
		ss = req.Session
	}
	b, err := s.book(ctx, ss, path)
	if err != nil {
		return nil, err
	}
	s.used(ctx, b)
	return b, nil
}
