// Package mcp is 012 mcp: a Model Context Protocol server on one
// workbook (see docs/agents/mcp.md). Its tools, resources and prompts
// are a thin layer over internal/headless, the same code 012 get, set
// and describe run, so agents' changes go through the workbook's one
// mutation path (Batch) and the same checks as a person's, and are
// shown by 012 diff. The workbook stands behind a Backend: a file here,
// and for live mode a session.
package mcp

import (
	"context"
	"encoding/json"
	"reflect"
	"sync"

	"github.com/google/jsonschema-go/jsonschema"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/FelineStateMachine/012/internal/headless"
	"github.com/FelineStateMachine/012/internal/sheet"
)

// Options say what the server may do beyond reading and writing cells.
type Options struct {
	// Version is 012's, which the server reports.
	Version string
	// ReadOnly leaves out every tool that changes the workbook.
	ReadOnly bool
	// Force lets writes change protected ranges, as 012 set --force.
	Force bool
	// Notebooks, when set, lets run_notebook_cell run cells with nu, as
	// --notebooks does; nil leaves the tool out. MayRun says why a
	// workbook's cells may not run here (trust, the shell setting), and
	// may mark it trusted, as the command line's --trust does.
	Notebooks *headless.NotebookOptions
	MayRun    func(*sheet.Workbook) error
}

// Server is the MCP server on a backend.
type Server struct {
	*sdk.Server
	b    Backend
	opts Options

	mu        sync.Mutex
	resources map[string]bool // the URIs listed now
}

// New makes the server with its tools, resources and prompts.
func New(b Backend, o Options) *Server {
	s := &Server{b: b, opts: o, resources: map[string]bool{}}
	s.Server = sdk.NewServer(&sdk.Implementation{Name: "012", Title: "012 spreadsheet", Version: orDevel(o.Version)},
		&sdk.ServerOptions{Instructions: instructions, Capabilities: &sdk.ServerCapabilities{Extensions: map[string]any{uiExtension: map[string]any{}}}})
	s.addReadTools()
	if !o.ReadOnly {
		s.addWriteTools()
	}
	s.addResources()
	s.addPrompts()
	s.addViews()
	s.refreshResources(context.Background())
	return s
}

func orDevel(v string) string {
	if v == "" {
		return "(devel)"
	}
	return v
}

// instructions tell the model how to use the server.
const instructions = `This server works on one 012 spreadsheet workbook (.012 file).
Call describe first: it lists the sheets, their used ranges, guessed header rows and column names, tables, named ranges, charts and notebooks.
References are written as in formulas: B7, A1:C9, Q3!B7, 'Q3 plan'!A1:C9, a named range, a table (Sales, Sales[Amount]) or a sheet name for the whole sheet; without a sheet name, the sheet shown when the file was saved.
Inputs are what a person types, in en-US form: 1.5, =SUM(A1:A6), $1,200, 12%, 2026-09-29. Formulas are Google Sheets'.
Writes are checked as typing is (formulas must parse, validation rules, protected ranges) and each call is one change, saved at once; pass dry_run to see the change without making it. Call evaluate to try a formula without writing it.`

// rawSchemas infer json.RawMessage, a value already in JSON, as any
// value.
var rawSchemas = &jsonschema.ForOptions{TypeSchemas: map[reflect.Type]*jsonschema.Schema{
	reflect.TypeFor[json.RawMessage](): {},
}}

// tool adds a tool whose handler takes In and returns Out, with the
// output schema inferred as rawSchemas says.
func tool[In, Out any](s *Server, t *sdk.Tool, h func(context.Context, *sdk.CallToolRequest, In) (*sdk.CallToolResult, Out, error)) {
	if t.OutputSchema == nil {
		schema, err := jsonschema.For[Out](rawSchemas)
		if err != nil {
			panic(err)
		}
		t.OutputSchema = schema
	}
	sdk.AddTool(s.Server, t, h)
}
