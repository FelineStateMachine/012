package main

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"slices"
	"strings"

	"github.com/FelineStateMachine/012/internal/diff"
	"github.com/FelineStateMachine/012/internal/headless"
)

// 012 diff and 012 merge-driver: workbooks compared and merged cell by
// cell, alone or as git's diff and merge drivers (see docs/files/git.md).

const (
	diffUsage = "usage: 012 diff a.012 b.012 [--format text|json|nuon] [--color auto|always|never]\n" +
		"       012 diff --textconv file.012"
	mergeUsage = "usage: 012 merge-driver base.012 ours.012 theirs.012 [path]"
)

// runDiff is 012 diff: exit status 0 when the workbooks are the same, 1
// when they differ and 2 on trouble, as diff(1). Called by git as
// diff.<driver>.command, with git's seven or nine arguments, it writes
// the changes under a header naming the file and exits 0, as git
// expects of an external diff; with --textconv it writes one workbook
// as lines for git's own diff.
func runDiff(args []string, e env) error {
	a, err := parseArgs(args, []string{"format", "color"}, []string{"textconv", "help"})
	switch {
	case err != nil:
		return usageError(err.Error(), diffUsage)
	case a.has("help"):
		fmt.Fprintln(e.stdout, diffUsage)
		return nil
	case a.has("textconv"):
		if len(a.pos) != 1 {
			return usageError("", diffUsage)
		}
		return textconv(a.pos[0], e.stdout)
	}
	format := strings.ToLower(a.flags["format"])
	if format != "" && !slices.Contains(diff.Formats, format) {
		return usageError("--format "+format+": 012 diff writes "+strings.Join(diff.Formats, ", "), diffUsage)
	}
	color, err := useColor(a.flags["color"], e)
	if err != nil {
		return usageError(err.Error(), diffUsage)
	}
	var oldPath, newPath, gitPath string
	switch len(a.pos) {
	case 2:
		oldPath, newPath = a.pos[0], a.pos[1]
	case 7, 9: // path old-file old-hex old-mode new-file new-hex new-mode [new-path xfrm]
		gitPath, oldPath, newPath = a.pos[0], a.pos[1], a.pos[4]
	default:
		return usageError("", diffUsage)
	}
	changes, err := compare(oldPath, newPath)
	if err != nil {
		return &exitError{code: 2, err: err}
	}
	if gitPath != "" {
		return gitDiff(e.stdout, gitPath, changes, format, color)
	}
	if err := diff.WriteChanges(e.stdout, changes, format, color && format != "json" && format != "nuon"); err != nil {
		return &exitError{code: 2, err: err}
	}
	if len(changes) > 0 {
		return &exitError{code: 1}
	}
	return nil
}

// gitDiff writes the changes to one file for git: a header naming it,
// as git's own diff has, and exit status 0.
func gitDiff(w io.Writer, path string, changes []diff.Change, format string, color bool) error {
	head := "diff --012 a/" + path + " b/" + path
	if color {
		head = "\x1b[1m" + head + "\x1b[0m"
	}
	if format == "" || format == "text" {
		fmt.Fprintln(w, head)
	}
	if err := diff.WriteChanges(w, changes, format, color); err != nil {
		return &exitError{code: 2, err: err}
	}
	return nil
}

// useColor decides whether the text output is colored: --color always
// or never, or by default when standard output is a terminal or git's
// pager, unless NO_COLOR is set.
func useColor(flag string, e env) (bool, error) {
	switch flag {
	case "always":
		return true, nil
	case "never":
		return false, nil
	case "", "auto":
		return e.getenv("NO_COLOR") == "" && (e.stdoutTTY || e.getenv("GIT_PAGER_IN_USE") == "true"), nil
	}
	return false, fmt.Errorf("--color %s: auto, always or never", flag)
}

// compare reads two workbooks and lists what changed.
func compare(oldPath, newPath string) ([]diff.Change, error) {
	a, err := readBook(oldPath)
	if err != nil {
		return nil, err
	}
	b, err := readBook(newPath)
	if err != nil {
		return nil, err
	}
	return diff.Compare(a, b), nil
}

// readBook reads a workbook to compare; /dev/null, which git names for
// a file added or deleted, is a workbook without sheets.
func readBook(path string) (*diff.Book, error) {
	data, err := readSide(path)
	if err != nil {
		return nil, err
	}
	b, err := diff.Read(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return b, nil
}

func readSide(path string) ([]byte, error) {
	if path == os.DevNull {
		return nil, nil
	}
	return os.ReadFile(path)
}

// textconv is 012 diff --textconv: a workbook as lines of text.
func textconv(path string, w io.Writer) error {
	b, err := readBook(path)
	if err != nil {
		return &exitError{code: 2, err: err}
	}
	return diff.Dump(w, b)
}

// runMergeDriver is 012 merge-driver, git's merge driver for .012
// files: it merges theirs into ours cell by cell, from their common
// base, and writes the result over ours. With conflicts it keeps ours
// for each, lists them on standard error and exits 1, which git takes
// as a conflict to resolve; on trouble it leaves ours as it was and
// exits 2.
func runMergeDriver(args []string, e env) error {
	a, err := parseArgs(args, nil, []string{"help"})
	switch {
	case err != nil:
		return usageError(err.Error(), mergeUsage)
	case a.has("help"):
		fmt.Fprintln(e.stdout, mergeUsage)
		return nil
	case len(a.pos) != 3 && len(a.pos) != 4:
		return usageError("", mergeUsage)
	}
	ours, label := a.pos[1], a.pos[1]
	if len(a.pos) == 4 {
		label = a.pos[3]
	}
	var sides [3][]byte
	for i, path := range a.pos[:3] {
		if sides[i], err = readSide(path); err != nil {
			return &exitError{code: 2, err: err}
		}
	}
	merged, conflicts, err := diff.Merge(sides[0], sides[1], sides[2])
	if err != nil {
		return &exitError{code: 2, err: fmt.Errorf("merging %s: %w; ours is left as it was", label, err)}
	}
	if merged != nil {
		if err := headless.WriteAtomic(ours, fileMode(ours), func(w io.Writer) error {
			_, err := io.Copy(w, bytes.NewReader(merged))
			return err
		}); err != nil {
			return &exitError{code: 2, err: err}
		}
	}
	if len(conflicts) == 0 {
		return nil
	}
	fmt.Fprintf(e.stderr, "012 merge-driver: %d %s in %s, where ours is kept; a cell's note says what theirs had:\n",
		len(conflicts), plural(len(conflicts), "conflict", "conflicts"), label)
	for _, c := range conflicts {
		fmt.Fprintln(e.stderr, "  "+c.String())
	}
	return &exitError{code: 1}
}

// fileMode is path's permissions, or 0644 when it has none to keep.
func fileMode(path string) fs.FileMode {
	st, err := os.Stat(path)
	if err != nil {
		return 0o644
	}
	return st.Mode().Perm()
}
