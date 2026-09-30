package mcp

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/FelineStateMachine/012/internal/confine"
	"github.com/FelineStateMachine/012/internal/fileio"
	"github.com/FelineStateMachine/012/internal/sheet"
)

// Workbooks are named by path in every tool. A path is confined to the
// folders the client shares as its roots (the MCP roots capability) or,
// when it shares none, to Options.Roots, each a confine.Root: a
// relative path is looked for in each folder in turn, an absolute one
// must lie inside one, and hidden files and links leading out are
// refused as confine refuses them. Options.Default, the file named on
// the command line, is open to tools without a path wherever it is.

// bookIn is the argument naming a tool's workbook.
type bookIn struct {
	Path string `json:"path,omitempty" jsonschema:"the workbook: a .012 file, or a file 012 imports (CSV, XLSX, JSON, SQLite...) to read it; relative to the folder open to the server, or absolute inside it"`
}

// book is a workbook a tool works on: its backend and the path that
// names it to the client.
type book struct {
	Backend
	path string // absolute, as resolved
	name string // as tools and resources name it: relative to its root, or absolute
}

// NewRoots confines paths to dirs, the working directory when there
// are none.
func NewRoots(dirs []string) ([]confine.Root, error) {
	if len(dirs) == 0 {
		dirs = []string{"."}
	}
	out := make([]confine.Root, 0, len(dirs))
	for _, d := range dirs {
		r, err := confine.New(d)
		if err != nil {
			return nil, fmt.Errorf("--root %s: %w", d, err)
		}
		out = append(out, r)
	}
	return out, nil
}

// resolve is the file on disk a tool's path names, within roots.
func resolve(roots []confine.Root, name string) (string, error) {
	if u, err := url.Parse(name); err == nil && u.Scheme == "file" {
		name = filepath.FromSlash(u.Path)
	}
	if len(roots) == 0 {
		return "", errors.New("no folder is open to this server")
	}
	if !filepath.IsAbs(name) {
		found := ""
		for _, r := range roots {
			p, err := r.Resolve(name)
			if err != nil {
				return "", outside(roots, name, err)
			}
			if _, err := os.Stat(p); err == nil {
				return p, nil
			}
			if found == "" {
				found = p
			}
		}
		return found, nil
	}
	real := realPath(name)
	for _, r := range roots {
		if rel, err := filepath.Rel(r.Dir(), real); err == nil && filepath.IsLocal(rel) {
			p, err := r.Resolve(rel)
			if err != nil {
				return "", outside(roots, name, err)
			}
			return p, nil
		}
	}
	return "", outside(roots, name, confine.ErrOutside)
}

// outside is why name was refused, err, naming the folders open.
func outside(roots []confine.Root, name string, err error) error {
	return fmt.Errorf("%s: %w; the folders open to this server are %s, without their hidden files", name, err, strings.Join(rootDirs(roots), ", "))
}

// realPath is p with the symbolic links along the part of it that
// exists followed, so it compares with a root's resolved folder.
func realPath(p string) string {
	p = filepath.Clean(p)
	var rest []string
	for cur := p; ; cur = filepath.Dir(cur) {
		if real, err := filepath.EvalSymlinks(cur); err == nil {
			return filepath.Join(append([]string{real}, rest...)...)
		}
		if filepath.Dir(cur) == cur {
			return p
		}
		rest = append([]string{filepath.Base(cur)}, rest...)
	}
}

// displayName is how tools and resources name the file at path: relative
// to the first root when it's in it, else absolute.
func displayName(roots []confine.Root, path string) string {
	if len(roots) > 0 {
		if rel, err := filepath.Rel(roots[0].Dir(), path); err == nil && filepath.IsLocal(rel) {
			return filepath.ToSlash(rel)
		}
	}
	return path
}

// isWorkbook reports whether a file's name is a .012 workbook's, and
// readable whether it's one 012 at least imports.
func isWorkbook(name string) (own, readable bool) {
	if strings.EqualFold(filepath.Ext(name), sheet.FileExt) {
		return true, true
	}
	k, ok := fileio.KindOf(name)
	return false, ok && k.CanImport()
}

// book is the workbook a tool's path names for the client of req, or
// the default one when the path is empty.
func (s *Server) book(ctx context.Context, ss *sdk.ServerSession, name string) (*book, error) {
	roots := s.roots(ctx, ss)
	var path string
	switch {
	case name == "" && s.opts.Default == "":
		return nil, errors.New("name the workbook with path; describe without one lists the workbooks open to this server")
	case name == "" || s.opts.Default != "" && filepath.IsAbs(name) && realPath(name) == s.opts.Default:
		path = s.opts.Default
	default:
		p, err := resolve(roots, name)
		if err != nil {
			return nil, err
		}
		path = p
	}
	if path != s.opts.Default {
		if _, ok := isWorkbook(path); !ok {
			return nil, fmt.Errorf("%s isn't a workbook: a %s file, or one 012 imports", name, sheet.FileExt)
		}
		if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("%s doesn't exist: create_workbook makes a workbook, and describe without a path lists them", name)
		}
	}
	return &book{Backend: s.backend(path), path: path, name: displayName(roots, path)}, nil
}

// backend is the one backend of the file at path, so calls on the same
// file take turns.
func (s *Server) backend(path string) Backend {
	s.mu.Lock()
	defer s.mu.Unlock()
	if b, ok := s.backends[path]; ok {
		return b
	}
	var b Backend
	if own, _ := isWorkbook(path); own || path == s.opts.Default {
		b = &FileBackend{Path: path, Prepare: s.opts.Prepare, Save: s.opts.Save}
	} else {
		b = &ImportBackend{Path: path, Prepare: s.opts.Prepare}
	}
	s.backends[path] = b
	return b
}

// rootDirs are the roots' folders, for describe.
func rootDirs(roots []confine.Root) []string {
	dirs := make([]string, len(roots))
	for i, r := range roots {
		dirs[i] = r.Dir()
	}
	return dirs
}
