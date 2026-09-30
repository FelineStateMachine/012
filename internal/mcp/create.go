package mcp

import (
	"context"
	"fmt"
	"os"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/FelineStateMachine/012/internal/confine"
	"github.com/FelineStateMachine/012/internal/headless"
	"github.com/FelineStateMachine/012/internal/sheet"
)

// create_workbook makes a new .012 file in a folder open to the server:
// empty, or from a file 012 imports (which the other tools only read),
// or a copy of another workbook. It never replaces a file.

type createIn struct {
	Path string `json:"path" jsonschema:"the new workbook's file, ending in .012; relative to the folder open to the server, or absolute inside it"`
	From string `json:"from,omitempty" jsonschema:"a file to make it from: one 012 imports (CSV, XLSX, JSON, SQLite...) or another workbook; empty for an empty workbook"`
}

func (s *Server) addCreateTool() {
	tool(s, &sdk.Tool{Name: "create_workbook", Annotations: writes,
		Description: "Make a new .012 workbook, empty or from a file 012 imports (CSV, XLSX, JSON, SQLite...), which can then be changed; it describes the workbook made. It won't replace a file that exists."},
		s.createWorkbook)
}

func (s *Server) createWorkbook(ctx context.Context, req *sdk.CallToolRequest, in createIn) (*sdk.CallToolResult, describeOut, error) {
	roots := s.roots(ctx, req.Session)
	path, err := resolve(roots, in.Path)
	if err != nil {
		return nil, describeOut{}, err
	}
	if own, _ := isWorkbook(path); !own {
		return nil, describeOut{}, fmt.Errorf("%s: a workbook's file name ends in %s", in.Path, sheet.FileExt)
	}
	if _, err := os.Lstat(path); err == nil {
		return nil, describeOut{}, fmt.Errorf("%s exists: describe it, or name a new file", in.Path)
	}
	f, err := headless.Open(path, true)
	if err != nil {
		return nil, describeOut{}, err
	}
	if in.From != "" {
		if f.Book, err = s.source(ctx, roots, in.From); err != nil {
			return nil, describeOut{}, err
		}
	}
	save := f.Save
	if s.opts.Save != nil {
		save = func() error { return s.opts.Save(f) }
	}
	if err := save(); err != nil {
		return nil, describeOut{}, err
	}
	b := &book{Backend: s.backend(path), path: path, name: displayName(roots, path)}
	s.used(ctx, b)
	d := headless.Describe(f.Book)
	return nil, describeOut{Path: b.name, Description: &d}, nil
}

// source is the workbook a new one is made from: a file imported, or
// another workbook's contents.
func (s *Server) source(ctx context.Context, roots []confine.Root, name string) (*sheet.Workbook, error) {
	path, err := resolve(roots, name)
	if err != nil {
		return nil, err
	}
	own, readable := isWorkbook(path)
	switch {
	case own:
		f, err := headless.Open(path, false)
		if err != nil {
			return nil, err
		}
		return f.Book, nil
	case readable:
		return importBook(ctx, path)
	}
	return nil, fmt.Errorf("%s isn't a file 012 imports", name)
}
