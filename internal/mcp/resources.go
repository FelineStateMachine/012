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

// Resources are workbooks' data by address, for hosts that let people
// attach them to a conversation:
//
//	o12://budget.012/Sheet1!A1:D40         a range, as read_range returns it (JSON)
//	o12://budget.012/table/Sales           a table, header row included (JSON)
//	o12://budget.012/notebook/Notes/2      a notebook cell: its source and output
//
// The workbook is named as tools name it, relative to the folder open
// to the server (or absolute, o12:///Users/me/budget.012/...), each
// part of it and the sheet's name percent-encoded. The scheme is o12
// because a URI's scheme starts with a letter. Each sheet's used range,
// table and code cell of the workbooks used lately is listed; the
// templates reach any range of any workbook open to the server.
const (
	scheme      = "o12://"
	maxListed   = 200
	maxRecent   = 5 // workbooks whose resources are listed
	tableSeg    = "table"
	notebookSeg = "notebook"
)

func (s *Server) addResources() {
	s.AddResourceTemplate(&sdk.ResourceTemplate{URITemplate: scheme + "{+book}/{sheet}!{+range}", Name: "range", MIMEType: "application/json",
		Description: "A range of a sheet as rows of values, as read_range returns it; the workbook's path and the sheet's name percent-encoded"}, s.readResource)
	s.AddResourceTemplate(&sdk.ResourceTemplate{URITemplate: scheme + "{+book}/table/{name}", Name: "table", MIMEType: "application/json",
		Description: "A named table, header row included"}, s.readResource)
	s.AddResourceTemplate(&sdk.ResourceTemplate{URITemplate: scheme + "{+book}/notebook/{sheet}/{cell}", Name: "notebook cell", MIMEType: "text/plain",
		Description: "A notebook cell's source, and its output as NUON or why it failed"}, s.readResource)
}

// used notes that a tool used b, listing its resources from now on.
func (s *Server) used(ctx context.Context, b *book) {
	s.mu.Lock()
	known := false
	for i, r := range s.recent {
		if r.path == b.path {
			s.recent = append(s.recent[:i], s.recent[i+1:]...)
			known = true
			break
		}
	}
	s.recent = append(s.recent, b)
	if len(s.recent) > maxRecent {
		s.recent = s.recent[len(s.recent)-maxRecent:]
	}
	s.mu.Unlock()
	if !known {
		s.refreshResources(ctx)
	}
}

// refreshResources lists the resources of the workbooks used lately as
// they stand, telling clients when the list changes.
func (s *Server) refreshResources(ctx context.Context) {
	s.mu.Lock()
	recent := append([]*book(nil), s.recent...)
	s.mu.Unlock()
	var want []sdk.Resource
	for i := len(recent) - 1; i >= 0; i-- {
		b := recent[i]
		_ = b.View(ctx, func(w *sheet.Workbook) error {
			want = append(want, listResources(w, b.name)...)
			return nil
		})
	}
	want = want[:min(len(want), maxListed)]
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

// listResources are a workbook's resources: each sheet's used range,
// table and notebook code cell.
func listResources(w *sheet.Workbook, name string) []sdk.Resource {
	var out []sdk.Resource
	base := bookURI(name)
	for _, sh := range w.Sheets() {
		if sh.IsNotebook() {
			for i, c := range sh.NotebookCells() {
				if c.Kind == notebook.Code {
					out = append(out, sdk.Resource{URI: fmt.Sprintf("%s/%s/%s/%d", base, notebookSeg, escape(sh.Name()), i+1),
						Name: fmt.Sprintf("%s %s cell %d", name, sh.Name(), i+1), MIMEType: "text/plain", Description: firstLine(c.Source)})
				}
			}
			continue
		}
		if used, ok := sh.UsedRange(); ok {
			out = append(out, sdk.Resource{URI: rangeURI(name, sh.Name(), used), Name: name + " " + sh.Name(), Title: sh.Name() + " " + used.String(),
				MIMEType: "application/json", Description: "The used range of " + sh.Name() + " in " + name})
		}
		for _, t := range sh.Tables() {
			out = append(out, sdk.Resource{URI: base + "/" + tableSeg + "/" + escape(t.Name), Name: name + " table " + t.Name, MIMEType: "application/json",
				Description: "Table " + t.Name + " on " + sh.Name() + ", " + t.Range.String() + ": " + strings.Join(t.Cols, ", ")})
		}
	}
	return out
}

// bookURI is where a workbook's resources start: the scheme and its
// path, each part percent-encoded.
func bookURI(name string) string {
	parts := strings.Split(name, "/")
	for i, p := range parts {
		parts[i] = escape(p)
	}
	return scheme + strings.Join(parts, "/")
}

// rangeURI is the resource of a range of a sheet of a workbook.
func rangeURI(book, sheetName string, r sheet.Rect) string {
	return bookURI(book) + "/" + escape(sheetName) + "!" + r.String()
}

func (s *Server) readResource(ctx context.Context, req *sdk.ReadResourceRequest) (*sdk.ReadResourceResult, error) {
	uri := req.Params.URI
	name, rest, err := splitURI(uri)
	if err != nil {
		return nil, err
	}
	b, err := s.book(ctx, req.Session, name)
	if err != nil {
		return nil, err
	}
	var text, mime string
	err = b.View(ctx, func(w *sheet.Workbook) error {
		var err error
		text, mime, err = resourceText(w, rest)
		return err
	})
	if err != nil {
		return nil, err
	}
	return &sdk.ReadResourceResult{Contents: []*sdk.ResourceContents{{URI: uri, MIMEType: mime, Text: text}}}, nil
}

// splitURI is the workbook a resource's URI names, and the rest of it
// (what's in the workbook) as its last parts: Sheet1!A1:D40,
// table/Sales or notebook/Notes/2.
func splitURI(uri string) (book string, rest []string, err error) {
	parts := strings.Split(strings.TrimPrefix(uri, scheme), "/")
	n := len(parts)
	switch {
	case !strings.HasPrefix(uri, scheme) || n < 2:
		return "", nil, sdk.ResourceNotFoundError(uri)
	case strings.Contains(parts[n-1], "!"):
		rest = parts[n-1:]
	case n >= 4 && parts[n-3] == notebookSeg:
		rest = parts[n-3:]
	case n >= 3 && parts[n-2] == tableSeg:
		rest = parts[n-2:]
	default:
		return "", nil, sdk.ResourceNotFoundError(uri)
	}
	path := parts[:n-len(rest)]
	for i, p := range path {
		if path[i], err = unescape(p); err != nil {
			return "", nil, err
		}
	}
	return strings.Join(path, "/"), rest, nil
}

// resourceText is what the rest of a resource's URI names in a
// workbook, and its type.
func resourceText(w *sheet.Workbook, rest []string) (string, string, error) {
	switch {
	case rest[0] == notebookSeg:
		return notebookCellText(w, rest[1], rest[2])
	case rest[0] == tableSeg:
		name, err := unescape(rest[1])
		if err != nil {
			return "", "", err
		}
		return rangeJSON(w, name)
	}
	sh, rng, _ := strings.Cut(rest[0], "!")
	name, err := unescape(sh)
	if err != nil {
		return "", "", err
	}
	return rangeJSON(w, sheet.QuoteSheet(name)+"!"+rng)
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
// failed.
func notebookCellText(w *sheet.Workbook, sheetPart, cell string) (string, string, error) {
	name, err := unescape(sheetPart)
	if err != nil {
		return "", "", err
	}
	n, err := strconv.Atoi(cell)
	s := w.Lookup(name)
	if s == nil || !s.IsNotebook() || err != nil || n < 1 || n > len(s.NotebookCells()) {
		return "", "", fmt.Errorf("no cell %s of a notebook %q", cell, name)
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
// sheet's name or a part of a path fits a template's simple variable.
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
