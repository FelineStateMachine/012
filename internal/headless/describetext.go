package headless

import (
	"fmt"
	"io"
	"strings"
)

// WriteDescription writes d as text: a line for each sheet saying what
// it holds, with what's on it indented below, then the named ranges.
func WriteDescription(w io.Writer, d Description) error {
	var b strings.Builder
	for _, s := range d.Sheets {
		describeSheetText(&b, s)
	}
	if len(d.Names) > 0 {
		b.WriteString("names\n")
		for _, n := range d.Names {
			fmt.Fprintf(&b, "  %s  %s\n", n.Name, n.Range)
		}
	}
	_, err := io.WriteString(w, b.String())
	return err
}

func describeSheetText(b *strings.Builder, s SheetDescription) {
	var tags []string
	if s.Shown {
		tags = append(tags, "shown")
	}
	if s.Hidden {
		tags = append(tags, "hidden")
	}
	if s.Kind != "sheet" {
		tags = append(tags, s.Kind)
	}
	b.WriteString(s.Name)
	if len(tags) > 0 {
		b.WriteString(" (" + strings.Join(tags, ", ") + ")")
	}
	switch {
	case s.Kind == "notebook":
		fmt.Fprintf(b, "  %d %s\n", len(s.Notebook), plural(len(s.Notebook), "cell", "cells"))
	case s.Used == "":
		b.WriteString("  empty\n")
	default:
		fmt.Fprintf(b, "  %s, %d %s x %d %s, %d %s%s\n", s.Used, s.Rows, plural(s.Rows, "row", "rows"), s.Cols,
			plural(s.Cols, "column", "columns"), s.Cells, plural(s.Cells, "cell", "cells"), formulaCount(s))
	}
	describeParts(b, s)
}

// formulaCount says how many formulas there are and how many show
// errors.
func formulaCount(s SheetDescription) string {
	if s.Formulas <= 0 {
		return ""
	}
	out := fmt.Sprintf(", %d %s", s.Formulas, plural(s.Formulas, "formula", "formulas"))
	if s.Errors > 0 {
		out += fmt.Sprintf(" (%d showing errors)", s.Errors)
	}
	return out
}

// describeParts writes what's on a sheet, a line each.
func describeParts(b *strings.Builder, s SheetDescription) {
	if s.Header > 0 {
		fmt.Fprintf(b, "  header row %d: %s\n", s.Header, strings.Join(s.Columns, ", "))
	}
	if s.Frozen != [2]int{} {
		fmt.Fprintf(b, "  frozen %d %s, %d %s\n", s.Frozen[0], plural(s.Frozen[0], "row", "rows"), s.Frozen[1], plural(s.Frozen[1], "column", "columns"))
	}
	if s.Filter != "" {
		fmt.Fprintf(b, "  filter %s\n", s.Filter)
	}
	if s.Pivot != nil {
		fmt.Fprintf(b, "  pivot table of %s\n", s.Pivot.Source)
	}
	for _, t := range s.Tables {
		fmt.Fprintf(b, "  table %s  %s  %s\n", t.Name, t.Range, strings.Join(t.Columns, ", "))
	}
	for _, r := range s.Regions {
		switch r.Kind {
		case "linked":
			fmt.Fprintf(b, "  linked %s  %s  following %s\n", r.Name, r.Range, r.File)
		case "source":
			fmt.Fprintf(b, "  source %s  %s  reading %s in place\n", r.Name, r.Range, r.File)
		default:
			fmt.Fprintf(b, "  output %s  %s\n", r.Name, r.Range)
		}
	}
	for _, c := range s.Charts {
		fmt.Fprintf(b, "  chart %d  %s  %s of %s at %s\n", c.Number, c.Title, c.Type, c.Data, c.At)
	}
	for _, c := range s.Notebook {
		line := []rune(strings.SplitN(c.Source, "\n", 2)[0])
		if len(line) > 60 {
			line = append(line[:57], []rune("...")...)
		}
		state := ""
		switch {
		case c.Error != "":
			state = "  (failed: " + c.Error + ")"
		case c.State != "":
			state = "  (" + c.State + ")"
		}
		fmt.Fprintf(b, "  %d %s  %s%s\n", c.Number, c.Kind, string(line), state)
	}
}
