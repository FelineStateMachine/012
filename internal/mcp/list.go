package mcp

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/FelineStateMachine/012/internal/confine"
	"github.com/FelineStateMachine/012/internal/fileio"
)

// describe without a path lists the workbooks open to the server: the
// default one, then the .012 files and files 012 imports in each root,
// not looking in hidden folders or node_modules, nor deeper than
// listDepth, and stopping at maxListedFiles files or maxListedDirs
// folders looked in, so a root as wide as a home folder answers
// quickly.
const (
	maxListedFiles = 500
	maxListedDirs  = 2000
	listDepth      = 4
)

// listedFile is a workbook describe lists.
type listedFile struct {
	Path     string `json:"path" jsonschema:"what to pass as path"`
	Format   string `json:"format" jsonschema:"012, or the format it's imported from (read only)"`
	Size     int64  `json:"size"`
	Modified string `json:"modified" jsonschema:"when it last changed, RFC 3339"`
}

// lister gathers the files of a listing.
type lister struct {
	roots   []confine.Root
	skip    string // the default workbook, listed first
	files   []listedFile
	dirs    int
	stopped bool
}

// listFiles lists the workbooks in roots, and the default one; more is
// set when there were more than it lists.
func (s *Server) listFiles(roots []confine.Root) (files []listedFile, more bool) {
	l := &lister{roots: roots, skip: s.opts.Default}
	if d := s.opts.Default; d != "" {
		if info, err := os.Stat(d); err == nil {
			l.add(d, info)
		}
	}
	for _, r := range roots {
		if l.stopped {
			break
		}
		_ = filepath.WalkDir(r.Dir(), func(p string, e fs.DirEntry, err error) error { return l.visit(r, p, e, err) })
	}
	return l.files, l.stopped
}

func (l *lister) add(path string, info fs.FileInfo) {
	format := "012"
	if own, _ := isWorkbook(path); !own {
		k, _ := fileio.KindOf(path)
		format = k.String()
	}
	l.files = append(l.files, listedFile{Path: displayName(l.roots, path), Format: format, Size: info.Size(),
		Modified: info.ModTime().UTC().Format("2006-01-02T15:04:05Z")})
}

// visit is the walk's step: a folder to look in or not, or a file to
// list or not.
func (l *lister) visit(r confine.Root, p string, e fs.DirEntry, err error) error {
	if err != nil || p == r.Dir() {
		return nil
	}
	if e.IsDir() {
		rel, _ := filepath.Rel(r.Dir(), p)
		l.dirs++
		switch {
		case l.dirs > maxListedDirs:
			l.stopped = true
			return filepath.SkipAll
		case strings.HasPrefix(e.Name(), ".") || e.Name() == "node_modules" || strings.Count(rel, string(filepath.Separator)) >= listDepth-1:
			return filepath.SkipDir
		}
		return nil
	}
	if _, ok := isWorkbook(p); !ok || !e.Type().IsRegular() || strings.HasPrefix(e.Name(), ".") || p == l.skip {
		return nil
	}
	if len(l.files) == maxListedFiles {
		l.stopped = true
		return filepath.SkipAll
	}
	if info, err := e.Info(); err == nil {
		l.add(p, info)
	}
	return nil
}
