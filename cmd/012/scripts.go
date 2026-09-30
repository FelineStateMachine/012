package main

import (
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/FelineStateMachine/012/internal/diff"
	"github.com/FelineStateMachine/012/internal/headless"
	"github.com/FelineStateMachine/012/internal/notebook"
	"github.com/FelineStateMachine/012/internal/nuon"
)

// 012 get, set and recalc: workbook files read and changed without the
// screen, for scripts (see docs/files/scripts.md). 012 export is in
// export.go, and what --notebooks, --jev and --trust run in evaluate.go.

const (
	getUsage    = "usage: 012 get file.012 [ref] [--format text|csv|tsv|json|nuon] [--input] [--no-header] [--notebooks] [--jev] [--trust]"
	setUsage    = "usage: 012 set file.012 [ref input ...] [--value] [--table ref] [--force] [--dry-run] [--format text|json|nuon]"
	recalcUsage = "usage: 012 recalc file.012 [--format text|json|nuon] [--notebooks] [--jev] [--trust]"
)

// evalFlags are the flags that let a command run a workbook's notebook
// cells or ask JEV; nothing runs without them.
var evalFlags = []string{"notebooks", "jev", "trust"}

// runGet is 012 get: a cell's value or input, or a range's as a table.
func runGet(args []string, e env) error {
	a, err := parseArgs(args, []string{"format"}, append([]string{"input", "no-header", "help"}, evalFlags...))
	switch {
	case err != nil:
		return usageError(err.Error(), getUsage)
	case a.has("help"):
		fmt.Fprintln(e.stdout, getUsage)
		return nil
	case len(a.pos) < 1 || len(a.pos) > 2:
		return usageError("", getUsage)
	}
	o := headless.GetOptions{Format: strings.ToLower(a.flags["format"]), Input: a.has("input"), NoHeader: a.has("no-header")}
	if o.Format != "" && !slices.Contains(headless.Formats, o.Format) {
		return usageError("--format "+o.Format+": 012 get writes "+strings.Join(headless.Formats, ", "), getUsage)
	}
	f, err := headless.Open(a.pos[0], false)
	if err != nil {
		return err
	}
	t, err := headless.Resolve(f.Book, strings.Join(a.pos[1:], ""))
	if err != nil {
		return err
	}
	failed := evaluate(f, a, e)
	if err := failed.fatal(); err != nil {
		return err
	}
	if err := headless.Get(e.stdout, t, o); err != nil {
		return err
	}
	if missing := headless.MissingOutputs(t.Sheet); !a.has("notebooks") && len(missing) > 0 {
		fmt.Fprintf(e.stderr, "012: note: %s shows no rows for %s, whose output the file doesn't hold; --notebooks runs the notebook\n",
			t.Sheet.Name(), strings.Join(missing, ", "))
	}
	return failed.err()
}

// runSet is 012 set: type entries into cells and save, or with
// --dry-run print what would change, as 012 diff would, and not save.
func runSet(args []string, e env) error {
	a, err := parseArgs(args, []string{"format", "table"}, []string{"force", "dry-run", "value", "help"})
	switch {
	case err != nil:
		return usageError(err.Error(), setUsage)
	case a.has("help"):
		fmt.Fprintln(e.stdout, setUsage)
		return nil
	case len(a.pos) < 1 || len(a.pos) < 3 && !a.has("table") || len(a.pos)%2 == 0:
		return usageError("", setUsage)
	}
	format, err := resultFormat(a)
	if err != nil {
		return usageError(err.Error(), setUsage)
	}
	f, err := headless.Open(a.pos[0], true)
	if err != nil {
		return err
	}
	warnings, err := setEntries(f, a, e.stdin)
	if err != nil {
		return fmt.Errorf("%w; %s is unchanged", err, f.Path)
	}
	res := headless.SetResult{File: f.Path, Warnings: warnings, Changes: []headless.Change{}}
	if format == "text" {
		for _, w := range warnings {
			fmt.Fprintln(e.stderr, "012: warning: "+w)
		}
	}
	if format != "text" || a.has("dry-run") {
		changes, err := f.Changes()
		if err != nil {
			return err
		}
		if format == "text" {
			color, _ := useColor("", e)
			return diff.WriteChanges(e.stdout, changes, "text", color)
		}
		res.Changes = headless.Changes(changes)
	}
	if !a.has("dry-run") {
		if err := save(f, e); err != nil {
			return err
		}
		res.Saved = true
	}
	if format == "text" {
		return nil
	}
	if res.Warnings == nil {
		res.Warnings = []string{}
	}
	return headless.Encode(e.stdout, format, res)
}

// resultFormat is --format's form for a command's result: text, json
// or nuon.
// setEntries makes 012 set's change: the table --table reads from in,
// then the entries, typed or with --value each a value with its type.
func setEntries(f *headless.File, a cliArgs, in io.Reader) ([]string, error) {
	o := headless.SetOptions{Force: a.has("force")}
	var warnings []string
	if a.has("table") {
		data, err := io.ReadAll(in)
		if err != nil {
			return nil, err
		}
		rows, err := tableRows(data)
		if err != nil {
			return nil, fmt.Errorf("--table: %w", err)
		}
		if warnings, err = headless.WriteTable(f.Book, headless.TableSpec{At: a.flags["table"], Rows: rows}, o); err != nil {
			return nil, err
		}
	}
	var entries []headless.Entry
	for i := 1; i < len(a.pos); i += 2 {
		e := headless.Entry{Ref: a.pos[i], Input: a.pos[i+1]}
		if a.has("value") {
			e.Input, e.Value = "", headless.Value(a.pos[i+1])
		}
		entries = append(entries, e)
	}
	more, err := headless.Set(f.Book, entries, o)
	return append(warnings, more...), err
}

// tableRows are the records of a NUON or JSON table, each as NUON.
func tableRows(data []byte) ([]headless.Row, error) {
	v, err := nuon.Parse(data)
	if err != nil {
		return nil, err
	}
	if v.Kind == nuon.Record {
		v = nuon.ListValue(v)
	}
	if v.Kind != nuon.List {
		return nil, errors.New("standard input isn't a table: a list of records, as NUON or JSON")
	}
	rows := make([]headless.Row, len(v.List))
	for i, r := range v.List {
		rows[i] = headless.Row(nuon.Append(nil, r))
	}
	return rows, nil
}

func resultFormat(a cliArgs) (string, error) {
	switch f := strings.ToLower(a.flags["format"]); f {
	case "", "text":
		return "text", nil
	case "json", "nuon":
		return f, nil
	default:
		return "", errors.New("--format " + f + ": text, json or nuon")
	}
}

// runRecalc is 012 recalc: recalculate, save, and list the cells whose
// formulas show errors, exiting 1 when there are any.
func runRecalc(args []string, e env) error {
	a, err := parseArgs(args, []string{"format"}, append([]string{"help"}, evalFlags...))
	switch {
	case err != nil:
		return usageError(err.Error(), recalcUsage)
	case a.has("help"):
		fmt.Fprintln(e.stdout, recalcUsage)
		return nil
	case len(a.pos) != 1:
		return usageError("", recalcUsage)
	}
	format, err := resultFormat(a)
	if err != nil {
		return usageError(err.Error(), recalcUsage)
	}
	f, err := headless.Open(a.pos[0], false)
	if err != nil {
		return err
	}
	failed := evaluate(f, a, e)
	if err := failed.fatal(); err != nil {
		return err
	}
	problems, circular := headless.Recalc(f.Book)
	if err := save(f, e); err != nil {
		return err
	}
	if format != "text" {
		res := headless.RecalcResult{File: f.Path, Errors: headless.Problems(problems), Circular: circular,
			NotebookFailures: append([]string{}, failed.cells...)}
		if err := headless.Encode(e.stdout, format, res); err != nil {
			return err
		}
		return problemStatus(problems, circular, failed)
	}
	return reportProblems(e.stdout, problems, circular, failed)
}

// save saves a workbook, keeping as much of its notebook outputs as the
// config's nu-save-cell-kb and nu-save-notebook-kb allow, as the screen
// saves it.
func save(f *headless.File, e env) error {
	cfg, err := loadConfig(e, nil)
	if err != nil {
		return err
	}
	f.Book.SetOutputCaps(notebook.Caps{Cell: cfg.Int("nu-save-cell-kb") << 10, Total: cfg.Int("nu-save-notebook-kb") << 10})
	return f.Save()
}

// reportProblems lists the cells showing errors, one a line, and says
// how many; the status is 1 when there are any or a region failed.
func reportProblems(w io.Writer, problems []headless.Problem, circular bool, failed evalFailures) error {
	for _, p := range problems {
		line := p.At() + "  " + p.Value
		switch {
		case p.JEV:
			line += "  JEV functions need --jev and an API key to be answered"
		case p.Why != "":
			line += "  " + p.Why
		}
		fmt.Fprintln(w, line)
	}
	if circular {
		fmt.Fprintln(w, "circular reference: a formula reads its own cell, directly or through others")
	}
	return problemStatus(problems, circular, failed)
}

// problemStatus is recalc's exit status: 1 when cells show errors or a
// notebook cell failed.
func problemStatus(problems []headless.Problem, circular bool, failed evalFailures) error {
	if len(problems) > 0 || circular {
		return &exitError{code: 1, err: fmt.Errorf("%d %s showing errors", len(problems), plural(len(problems), "cell", "cells"))}
	}
	return failed.err()
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// evalFailures are what --notebooks and --jev couldn't do: a reason
// nothing ran at all (fatal), or notebook cells that failed.
type evalFailures struct {
	stop  error
	cells []string
}

func (f evalFailures) fatal() error { return f.stop }

// err is the status once the command's output is written: 1 when a
// notebook cell failed, each already reported.
func (f evalFailures) err() error {
	if len(f.cells) == 0 {
		return nil
	}
	return &exitError{code: 1, err: errors.New(fmt.Sprint(len(f.cells)) + " notebook " + plural(len(f.cells), "cell", "cells") + " failed")}
}
