package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/FelineStateMachine/012/internal/fileio"
	"github.com/FelineStateMachine/012/internal/headless"
	"github.com/FelineStateMachine/012/internal/sheet"
)

const exportUsage = "usage: 012 export file.012 out.csv|tsv|xlsx|json|nuon|sqlite [ref] [--format kind] [--table name] [--regions] [--jev] [--trust]"

// runExport is 012 export: a sheet, a range or (to formats holding
// several sheets) the whole workbook written as File > Download writes
// it.
func runExport(args []string, e env) error {
	a, err := parseArgs(args, []string{"format", "table"}, append([]string{"help"}, evalFlags...))
	switch {
	case err != nil:
		return usageError(err.Error(), exportUsage)
	case a.has("help"):
		fmt.Fprintln(e.stdout, exportUsage)
		return nil
	case len(a.pos) < 2 || len(a.pos) > 3:
		return usageError("", exportUsage)
	}
	src, out := a.pos[0], a.pos[1]
	k, err := exportKind(out, a.flags["format"])
	if err != nil {
		return usageError(err.Error(), exportUsage)
	}
	if same(src, out) {
		return fmt.Errorf("%s is the workbook being exported: name another file", out)
	}
	f, err := headless.Open(src, false)
	if err != nil {
		return err
	}
	ref := ""
	if len(a.pos) == 3 {
		ref = a.pos[2]
	}
	t, err := headless.Resolve(f.Book, ref)
	if err != nil {
		return err
	}
	failed := evaluate(f, a, e)
	if err := failed.fatal(); err != nil {
		return err
	}
	snap := exportSnap(t, k, ref == "")
	res, err := fileio.Export(context.Background(), out, k, snap, fileio.ExportOptions{Table: a.flags["table"]})
	if err != nil {
		return fmt.Errorf("%s: %w", out, err)
	}
	for _, n := range res.Notes {
		fmt.Fprintln(e.stderr, "012: note: "+n)
	}
	return failed.err()
}

// exportKind is the format to write: --format's, or out's extension's.
func exportKind(out, named string) (fileio.Kind, error) {
	k, ok := fileio.KindOf(out)
	if named != "" {
		k, ok = fileio.KindNamed(named)
		if !ok {
			return 0, fmt.Errorf("--format %s: %s", named, exportable())
		}
	}
	switch {
	case !ok && strings.EqualFold(filepath.Ext(out), sheet.FileExt):
		return 0, fmt.Errorf("%s is a workbook: copy the file instead, or %s", out, exportable())
	case !ok:
		return 0, fmt.Errorf("%s: name the format with --format, or give the file one of %s's extensions", out, exportable())
	case !k.CanExport():
		return 0, fmt.Errorf("012 reads %s files but doesn't write them; %s", k, exportable())
	}
	return k, nil
}

// exportable lists the formats 012 export writes.
func exportable() string {
	var names []string
	for _, k := range fileio.Kinds() {
		if k.CanExport() {
			names = append(names, strings.ToLower(k.String()))
		}
	}
	return "012 export writes " + strings.Join(names, ", ")
}

// exportSnap is what's written, as File > Download writes it: every
// sheet for formats that hold several when no reference narrows it,
// else the target, with the rows a filter hides left out of formats
// that can't hide them.
func exportSnap(t headless.Target, k fileio.Kind, whole bool) *fileio.Snapshot {
	if whole && k.HoldsSheets() {
		return fileio.SnapBook(t.Sheet)
	}
	r := t.Range
	if t.Whole {
		r = sheet.Rect{}
	}
	return fileio.Snap(t.Sheet, r, t.Sheet.Name())
}

// same reports whether two paths name one file.
func same(a, b string) bool {
	sa, err1 := os.Stat(a)
	sb, err2 := os.Stat(b)
	return err1 == nil && err2 == nil && os.SameFile(sa, sb)
}
