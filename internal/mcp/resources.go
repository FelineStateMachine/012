package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/FelineStateMachine/012/internal/headless"
	"github.com/FelineStateMachine/012/internal/notebook"
	"github.com/FelineStateMachine/012/internal/sheet"
)

// Resources are the workbook's data by address, for hosts that let
// people attach them to a conversation:
//
//	o12://book/Sheet1!A1:D40         a range, as read_range returns it (JSON)
//	o12://book/table/Sales           a table, header row included (JSON)
//	o12://book/notebook/Notes/2      a notebook cell: its source and output
//
// Sheet names are percent-encoded. Each sheet's used range, each table
// and each code cell are listed; the templates reach any range. The
// scheme is o12 because a URI's scheme starts with a letter, and the
// workbook is the authority, book, so a range's colon is in the path.
const (
	scheme         = "o12://book/"
	tablePrefix    = scheme + "table/"
	notebookPrefix = scheme + "notebook/"
	maxListed      = 200
)

func (s *Server) addResources() {
	s.AddResourceTemplate(&sdk.ResourceTemplate{URITemplate: scheme + "{sheet}!{+range}", Name: "range", MIMEType: "application/json",
		Description: "A range of a sheet as rows of values, as read_range returns it; the sheet's name percent-encoded"}, s.readResource)
	s.AddResourceTemplate(&sdk.ResourceTemplate{URITemplate: tablePrefix + "{name}", Name: "table", MIMEType: "application/json",
		Description: "A named table, header row included"}, s.readResource)
	s.AddResourceTemplate(&sdk.ResourceTemplate{URITemplate: notebookPrefix + "{sheet}/{cell}", Name: "notebook cell", MIMEType: "text/plain",
		Description: "A notebook cell's source, and its output as NUON or why it failed"}, s.readResource)
}

// refreshResources lists the workbook's resources as it stands,
// telling clients when the list changes.
func (s *Server) refreshResources(ctx context.Context) {
	var want []sdk.Resource
	_ = s.b.View(ctx, func(w *sheet.Workbook) error {
		want = listResources(w)
		return nil
	})
	s.mu.Lock()
	defer s.mu.Unlock()
	keep := map[string]bool{}
	for _, r := range want {
		keep[r.URI] = true
		if !s.resources[r.URI] {
			res := r
			s.AddResource(&res, s.readResource)
		}
	}
	var gone []string
	for uri := range s.resources {
		if !keep[uri] {
			gone = append(gone, uri)
		}
	}
	if len(gone) > 0 {
		s.RemoveResources(gone...)
	}
	s.resources = keep
}

func listResources(w *sheet.Workbook) []sdk.Resource {
	var out []sdk.Resource
	add := func(r sdk.Resource) {
		if len(out) < maxListed {
			out = append(out, r)
		}
	}
	for _, sh := range w.Sheets() {
		if sh.IsNotebook() {
			for i, c := range sh.NotebookCells() {
				if c.Kind == notebook.Code {
					add(sdk.Resource{URI: fmt.Sprintf("%s%s/%d", notebookPrefix, escape(sh.Name()), i+1), Name: fmt.Sprintf("%s cell %d", sh.Name(), i+1),
						MIMEType: "text/plain", Description: firstLine(c.Source)})
				}
			}
			continue
		}
		if used, ok := sh.UsedRange(); ok {
			add(sdk.Resource{URI: rangeURI(sh.Name(), used), Name: sh.Name(), Title: sh.Name() + " " + used.String(),
				MIMEType: "application/json", Description: "The used range of " + sh.Name()})
		}
		for _, t := range sh.Tables() {
			add(sdk.Resource{URI: tablePrefix + escape(t.Name), Name: "table " + t.Name, MIMEType: "application/json",
				Description: "Table " + t.Name + " on " + sh.Name() + ", " + t.Range.String() + ": " + strings.Join(t.Cols, ", ")})
		}
	}
	return out
}

// rangeURI is the resource of a range of a sheet.
func rangeURI(sheetName string, r sheet.Rect) string {
	return scheme + escape(sheetName) + "!" + r.String()
}

func (s *Server) readResource(ctx context.Context, req *sdk.ReadResourceRequest) (*sdk.ReadResourceResult, error) {
	uri := req.Params.URI
	var text, mime string
	err := s.b.View(ctx, func(w *sheet.Workbook) error {
		var err error
		text, mime, err = resourceText(w, uri)
		return err
	})
	if err != nil {
		return nil, err
	}
	return &sdk.ReadResourceResult{Contents: []*sdk.ResourceContents{{URI: uri, MIMEType: mime, Text: text}}}, nil
}

// resourceText is what a resource's URI names, and its type.
func resourceText(w *sheet.Workbook, uri string) (string, string, error) {
	switch {
	case strings.HasPrefix(uri, notebookPrefix):
		return notebookCellText(w, strings.TrimPrefix(uri, notebookPrefix))
	case strings.HasPrefix(uri, tablePrefix):
		name, err := unescape(strings.TrimPrefix(uri, tablePrefix))
		if err != nil {
			return "", "", err
		}
		return rangeJSON(w, name)
	case strings.HasPrefix(uri, scheme):
		rest := strings.TrimPrefix(uri, scheme)
		i := strings.LastIndex(rest, "!")
		if i < 0 {
			return "", "", sdk.ResourceNotFoundError(uri)
		}
		name, err := unescape(rest[:i])
		if err != nil {
			return "", "", err
		}
		return rangeJSON(w, sheet.QuoteSheet(name)+"!"+rest[i+1:])
	}
	return "", "", sdk.ResourceNotFoundError(uri)
}

func rangeJSON(w *sheet.Workbook, ref string) (string, string, error) {
	t, err := headless.Resolve(w, ref)
	if err != nil {
		return "", "", err
	}
	data, err := json.Marshal(headless.ReadRange(t, headless.ReadOptions{}))
	return string(data), "application/json", err
}

// notebookCellText is a cell's source, then its output or why it
// failed; path is sheet/cell.
func notebookCellText(w *sheet.Workbook, path string) (string, string, error) {
	i := strings.LastIndex(path, "/")
	if i < 0 {
		return "", "", fmt.Errorf("a notebook cell's resource is %snotebook/cell", notebookPrefix)
	}
	name, err := unescape(path[:i])
	if err != nil {
		return "", "", err
	}
	n, err := strconv.Atoi(path[i+1:])
	s := w.Lookup(name)
	if s == nil || !s.IsNotebook() || err != nil || n < 1 || n > len(s.NotebookCells()) {
		return "", "", fmt.Errorf("no cell %s of a notebook %q", path[i+1:], name)
	}
	c := s.NotebookCells()[n-1]
	var b strings.Builder
	b.WriteString(c.Source + "\n")
	switch o := w.Output(c.ID); {
	case c.Kind != notebook.Code:
	case o == nil || o.Unsaved:
		b.WriteString("\n(not run)\n")
	case o.Failed():
		b.WriteString("\nfailed: " + o.Err + "\n")
	default:
		b.WriteString("\n" + string(o.NUON) + "\n")
	}
	return b.String(), "text/plain", nil
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return line
}

// escape percent-encodes all but URI's unreserved characters, so a
// sheet's name fits a template's simple variable.
func escape(s string) string {
	var b strings.Builder
	for _, c := range []byte(s) {
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.IndexByte("-._~", c) >= 0 {
			b.WriteByte(c)
		} else {
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}

func unescape(s string) (string, error) {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '%' {
			b.WriteByte(s[i])
			continue
		}
		if i+2 >= len(s) {
			return "", fmt.Errorf("%q: a %% needs two hex digits", s)
		}
		v, err := strconv.ParseUint(s[i+1:i+3], 16, 8)
		if err != nil {
			return "", fmt.Errorf("%q: a %% needs two hex digits", s)
		}
		b.WriteByte(byte(v))
		i += 2
	}
	return b.String(), nil
}
