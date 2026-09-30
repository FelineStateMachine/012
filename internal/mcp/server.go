// Package mcp is 012 mcp: a Model Context Protocol server on the
// workbooks in the folders open to it (see docs/agents/mcp.md). Its
// tools, resources and prompts are a thin layer over internal/headless,
// the same code 012 get, set and describe run, so agents' changes go
// through the workbook's one mutation path (Batch) and the same checks
// as a person's, and are shown by 012 diff. Each workbook stands behind
// a Backend: a file here, and for live mode a session.
package mcp

import (
	"context"
	"slices"
	"sync"

	"github.com/google/jsonschema-go/jsonschema"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/FelineStateMachine/012/internal/confine"
	"github.com/FelineStateMachine/012/internal/headless"
	"github.com/FelineStateMachine/012/internal/sheet"
)

// Options say where the server's workbooks are and what it may do
// beyond reading and writing cells.
type Options struct {
	// Version is 012's, which the server reports.
	Version string
	// ReadOnly leaves out every tool that changes a workbook.
	ReadOnly bool
	// Force lets writes change protected ranges, as 012 set --force.
	Force bool
	// Notebooks, when set, lets run_notebook_cell run cells with nu, as
	// --notebooks does; nil leaves the tool out. MayRun says why a
	// workbook's cells may not run here (trust, the shell setting), and
	// may mark it trusted, as the command line's --trust does.
	Notebooks *headless.NotebookOptions
	MayRun    func(path string, w *sheet.Workbook) error
	// Roots are the folders workbooks may be in when the client shares
	// none of its own (NewRoots makes them).
	Roots []confine.Root
	// Default, an absolute path, is the workbook of tools called without
	// a path, open wherever it is; "" makes the path required.
	Default string
	// Prepare, when set, runs on every workbook once opened (answering
	// JEV functions); Save writes a changed one, nil being File.Save.
	Prepare func(context.Context, *headless.File) error
	Save    func(*headless.File) error
	// Live, when set, is the one workbook of a server attached to a
	// session (live mode, live.go): every tool works on it, and the
	// live tools are added. Roots, Default and Notebooks go unused.
	Live Live
}

// Server is the MCP server on the workbooks open to it.
type Server struct {
	*sdk.Server
	opts     Options
	fallback []confine.Root // Options.Roots

	mu          sync.Mutex
	backends    map[string]Backend // by absolute path
	clientRoots map[*sdk.ServerSession][]confine.Root
	resources   map[string]bool // the URIs listed now
	recent      []*book         // the workbooks whose resources are listed, most recent last
}

// New makes the server with its tools, resources and prompts.
func New(o Options) *Server {
	if o.Default != "" {
		o.Default = realPath(o.Default)
	}
	s := &Server{opts: o, fallback: o.Roots, backends: map[string]Backend{},
		clientRoots: map[*sdk.ServerSession][]confine.Root{}, resources: map[string]bool{}}
	s.Server = sdk.NewServer(&sdk.Implementation{Name: "012", Title: "012 spreadsheet", Version: orDevel(o.Version)}, s.serverOptions())
	s.addReadTools()
	if !o.ReadOnly {
		s.addWriteTools()
	}
	if o.Live != nil {
		s.addLiveTools(o.Live)
	}
	s.addResources()
	s.addPrompts()
	s.addViews()
	if o.Default != "" || o.Live != nil {
		if b, err := s.book(context.Background(), nil, ""); err == nil {
			s.used(context.Background(), b)
		}
	}
	return s
}

func orDevel(v string) string {
	if v == "" {
		return "(devel)"
	}
	return v
}

// instructions tell the model how to use the server.
const instructions = `This server works on 012 spreadsheet workbooks (.012 files) in the folders open to it, and reads the files 012 imports (CSV, XLSX, JSON, SQLite and more).
Every tool takes the workbook's path. Call describe without one to list the workbooks; describe with a path lists its sheets, their used ranges, guessed header rows and column names, tables, named ranges, charts and notebooks. create_workbook makes a new workbook, empty, from a file 012 imports, or holding a table of data.
References are written as in formulas: B7, A1:C9, Q3!B7, 'Q3 plan'!A1:C9, a named range, a table (Sales, Sales[Amount]) or a sheet name for the whole sheet; without a sheet name, the sheet shown when the file was saved.
Inputs are what a person types, in en-US form: 1.5, =SUM(A1:A6), $1,200, 12%, 2026-09-29. Formulas are Google Sheets'.
Keep values' meaning: write money, percentages, dates, times, durations and sizes with their types, never as bare numbers, either typed as a person types them ($3.50, 12%) or as values with their types ({"currency": 3.5}, {"percent": 0.12}, {"date": "2026-09-29"}, {"duration": "90min"}, {"size": "1.5kb"}); write rows of records with write_table, which formats each column from its values. Reads return values the same way, with each cell as shown.
Writes are checked as typing is (formulas must parse, validation rules, protected ranges) and each call is one change, saved at once; pass dry_run to see the change without making it. Call evaluate to try a formula without writing it.`

// tool adds a tool whose handler takes In and returns Out, with the
// schemas inferred as rawSchemas says. Its path is required
// unless the server has a default workbook, or the tool is describe,
// which lists the workbooks without one.
func tool[In, Out any](s *Server, t *sdk.Tool, h func(context.Context, *sdk.CallToolRequest, In) (*sdk.CallToolResult, Out, error)) {
	if t.InputSchema == nil {
		schema, err := jsonschema.For[In](rawSchemas)
		if err != nil {
			panic(err)
		}
		if _, ok := schema.Properties["path"]; ok && s.opts.Default == "" && s.opts.Live == nil && t.Name != "describe" && !slices.Contains(schema.Required, "path") {
			schema.Required = append(schema.Required, "path")
		}
		t.InputSchema = schema
	}
	if t.OutputSchema == nil {
		schema, err := jsonschema.For[Out](rawSchemas)
		if err != nil {
			panic(err)
		}
		t.OutputSchema = schema
	}
	sdk.AddTool(s.Server, t, h)
}
