// Package notebook is a notebook's document: its cells (code cells
// holding a nushell pipeline, note cells holding Markdown), the names
// cells give their outputs, what each cell reads, the order cells run
// in, and what a cell's last run left (its output). It knows nothing of
// sheets, terminals or nu itself: the engine (internal/sheet) keeps a
// notebook tab's cells in its undo history and its file, and the UI runs
// them and draws them.
package notebook

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

// Kind is what a cell holds.
type Kind uint8

const (
	// Code cells hold a nushell pipeline, several lines if need be.
	Code Kind = iota
	// Note cells hold Markdown.
	Note
)

// String is how the file names the kind.
func (k Kind) String() string {
	if k == Note {
		return "note"
	}
	return "code"
}

// ParseKind reads a kind as the file names it.
func ParseKind(s string) (Kind, bool) {
	switch s {
	case "code", "":
		return Code, true
	case "note":
		return Note, true
	}
	return Code, false
}

// Cell is one cell of a notebook. Cells are values: a change makes a new
// one, so the undo history can keep the list it replaced.
type Cell struct {
	// ID identifies the cell for as long as the workbook is open: its
	// output is kept by it. It isn't saved.
	ID     int
	Kind   Kind
	Source string
}

// Name is the name the cell gives its output, from a source of the form
// `name = pipeline`, or "".
func (c Cell) Name() string {
	if c.Kind != Code {
		return ""
	}
	name, _ := SplitName(c.Source)
	return name
}

// Pipeline is what runs: the source without its name.
func (c Cell) Pipeline() string {
	_, p := SplitName(c.Source)
	return p
}

// SplitName splits `name = pipeline` into the name and the pipeline; a
// source that doesn't start so has no name. `==` isn't a name's `=`.
func SplitName(src string) (name, pipeline string) {
	head, rest, ok := strings.Cut(src, "=")
	head = strings.TrimSpace(head)
	if !ok || strings.HasPrefix(rest, "=") || ValidName(head) != nil {
		return "", src
	}
	return head, strings.TrimLeft(rest, " \t")
}

// WithName is src given the name name, replacing any name it has; ""
// takes its name away.
func WithName(src, name string) string {
	_, p := SplitName(src)
	if name == "" {
		return p
	}
	return name + " = " + p
}

// Reserved are nushell's own variables and the notebook's: $selection
// is the selection on a sheet, $sheet.A1:C9 a range of one.
var Reserved = []string{"in", "env", "nu", "it", "selection", "sheet"}

// ValidName checks that name can name a cell's output: a nushell
// variable's name of letters, digits and _, not starting with a digit,
// not one of Reserved, and not starting with __, which the notebook's
// own variables do.
func ValidName(name string) error {
	switch {
	case name == "":
		return errors.New("Enter a name")
	case len(name) > 64:
		return errors.New("A name can be at most 64 characters")
	case isDigit(name[0]):
		return errors.New("A name can't start with a digit")
	case strings.HasPrefix(name, "__"):
		return errors.New("Names starting with __ are the notebook's own")
	case slices.Contains(Reserved, strings.ToLower(name)):
		return fmt.Errorf("$%s is nushell's own", name)
	}
	for i := range len(name) {
		if c := name[i]; !isLetter(c) && !isDigit(c) && c != '_' {
			return errors.New("A name can only have letters, digits and _")
		}
	}
	return nil
}

func isLetter(c byte) bool { return c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' }
func isDigit(c byte) bool  { return c >= '0' && c <= '9' }
func isWord(c byte) bool   { return isLetter(c) || isDigit(c) || c == '_' }

// Output is what a code cell's last run left: what it printed, as NUON,
// or why it failed. Outputs are values too, replaced whole.
type Output struct {
	// NUON is what the pipeline printed, as NUON; nil when it failed or
	// its output wasn't saved.
	NUON []byte
	// Err is nu's message when the run failed, and Detail the rest of
	// what it said (its help line).
	Err, Detail string
	// Note says what was left out.
	Note string
	// Count is the run's number in this session, [3]; 0 for an output
	// read from the file.
	Count int
	// Seq identifies the output among the workbook's, for telling
	// whether what a cell read has changed since.
	Seq int
	// Took is how long the run took.
	Took time.Duration
	// Source is the cell's source when it ran.
	Source string
	// Reads are the Seqs of the outputs the run read, by name.
	Reads map[string]int
	// Selection is the range the run read as $selection, "Sheet1!A1:C9".
	Selection string
	// Unsaved is set on an output the file didn't keep, being larger
	// than the caps allowed: there's nothing to show until it runs.
	Unsaved bool
}

// Failed reports whether the run failed.
func (o *Output) Failed() bool { return o != nil && o.Err != "" }
