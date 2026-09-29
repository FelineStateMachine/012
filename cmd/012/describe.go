package main

import (
	"fmt"

	"github.com/FelineStateMachine/012/internal/headless"
)

// 012 describe: what a workbook holds, for agents and people deciding
// what to read (see docs/agents/README.md).

const describeUsage = "usage: 012 describe file.012 [--format text|json|nuon] [--notebooks] [--jev] [--trust]"

// runDescribe is 012 describe: its sheets, used ranges, guessed header
// rows, names, tables, regions, notebooks and charts.
func runDescribe(args []string, e env) error {
	a, err := parseArgs(args, []string{"format"}, append([]string{"help"}, evalFlags...))
	switch {
	case err != nil:
		return usageError(err.Error(), describeUsage)
	case a.has("help"):
		fmt.Fprintln(e.stdout, describeUsage)
		return nil
	case len(a.pos) != 1:
		return usageError("", describeUsage)
	}
	format, err := resultFormat(a)
	if err != nil {
		return usageError(err.Error(), describeUsage)
	}
	f, err := headless.Open(a.pos[0], false)
	if err != nil {
		return err
	}
	failed := evaluate(f, a, e)
	if err := failed.fatal(); err != nil {
		return err
	}
	d := headless.Describe(f.Book)
	if format == "text" {
		err = headless.WriteDescription(e.stdout, d)
	} else {
		err = headless.Encode(e.stdout, format, d)
	}
	if err != nil {
		return err
	}
	return failed.err()
}
