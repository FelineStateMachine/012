package headless

import (
	"errors"
	"fmt"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// Entry is one cell to set: a reference to one cell, and what to type
// in it, as the file stores entries (numbers and dates in en-US's form,
// formulas with commas, whatever the workbook's locale), or a typed
// Value; "" clears it. Format, a number format code, formats the cell,
// or with neither input nor value every cell of a range.
type Entry struct {
	Ref    string `json:"ref" jsonschema:"one cell, as formulas write it: B7, Q3!B7, 'Q3 plan'!B7; a range when only format is given"`
	Input  string `json:"input,omitempty" jsonschema:"what to type, as a person types it in en-US form: =SUM(A1:A6), $1,200.50, 12%, 2026-09-29, '00123 for text; empty clears the cell"`
	Value  Value  `json:"value,omitempty" jsonschema:"instead of input, the value with its type: 3.5, \"00123\" (text), true, null (clears), {\"currency\": 3.5}, {\"percent\": 0.12}, {\"date\": \"2026-09-29\"}, {\"time\": \"14:30\"}, {\"duration\": \"90min\"}, {\"size\": \"1.5kb\"}, {\"number\": 1234.5, \"decimals\": 2}"`
	Format string `json:"format,omitempty" jsonschema:"a number format code for the cell, or alone for every cell of ref: $#,##0.00, 0.0%, yyyy-mm-dd, #,##0"`
}

// SetOptions tune Set.
type SetOptions struct {
	// Force sets cells in protected ranges, which are refused otherwise,
	// as the screen asks before editing them.
	Force bool
}

// Set types each entry into its cell, in order, as one change. An entry
// a cell can't take (a formula that doesn't parse, a validation rule
// that rejects it, part of an array, a pivot table or a region, a
// protected range without Force) stops it with an error naming the
// cell, and the workbook should then be thrown away rather than saved.
// Entries a rule would only mark invalid are set, and returned as
// warnings.
func Set(w *sheet.Workbook, entries []Entry, o SetOptions) (warnings []string, err error) {
	err = w.Batch(sheet.Change{Label: "set"}, func() error {
		for _, e := range entries {
			warn, err := setOne(w, e, o)
			if err != nil {
				return err
			}
			if warn != "" {
				warnings = append(warnings, warn)
			}
		}
		return nil
	})
	return warnings, err
}

// typedEntry is what e types: its input, or its value's, and the
// format it sets.
func typedEntry(e Entry) (typed, error) {
	t := typed{input: e.Input}
	if e.Value != nil {
		if e.Input != "" {
			return t, errors.New("give input or value, not both")
		}
		var err error
		if t, err = parseValue(e.Value); err != nil {
			return t, err
		}
	}
	if e.Format != "" {
		t.format, t.formatted = FormatOfCode(e.Format), true
	}
	return t, nil
}

func setOne(w *sheet.Workbook, e Entry, o SetOptions) (string, error) {
	if e.Format != "" && e.Input == "" && e.Value == nil {
		return "", formatRange(w, e.Ref, FormatOfCode(e.Format), o)
	}
	s, a, err := ResolveCell(w, e.Ref)
	if err != nil {
		return "", err
	}
	t, err := typedEntry(e)
	if err != nil {
		return "", fmt.Errorf("%s: %w", sheet.Qualified(s.Name(), sheet.Rect{From: a, To: a}), err)
	}
	return putTyped(s, a, t, o)
}

// putTyped types t into the cell at a, checked as typing it is.
func putTyped(s *sheet.Sheet, a sheet.Addr, t typed, o SetOptions) (string, error) {
	cell := sheet.Rect{From: a, To: a}
	at := sheet.Qualified(s.Name(), cell)
	if p, ok := s.Protecting(cell); ok && !o.Force {
		return "", fmt.Errorf("%s is protected (%s): --force sets it anyway", at, p.Label())
	}
	warn := ""
	if bad := s.CheckEntry(a, t.input); bad != nil {
		if bad.Reject {
			return "", fmt.Errorf("%s: %s", at, bad.Help)
		}
		warn = at + ": " + bad.Help
	}
	if t.formatted && s.DisplayFormat(a).Kind == sheet.FmtText {
		s.SetFormat(cell, t.format) // a number typed into Plain text would stay text
	}
	if err := s.Set(a, t.input); err != nil {
		var pe *sheet.ParseError
		if errors.As(err, &pe) {
			return "", fmt.Errorf("%s: %s, at character %d of %s", at, pe.Msg, pe.Pos+1, t.input)
		}
		return "", fmt.Errorf("%s: %w", at, err)
	}
	if t.formatted {
		s.SetFormat(cell, t.format)
	}
	return warn, nil
}

// formatRange gives every cell of the range ref names format f.
func formatRange(w *sheet.Workbook, ref string, f sheet.Format, o SetOptions) error {
	t, err := Resolve(w, ref)
	if err != nil {
		return err
	}
	r := targetRect(t)
	if p, ok := t.Sheet.Protecting(r); ok && !o.Force {
		return fmt.Errorf("%s is protected (%s): --force formats it anyway", t, p.Label())
	}
	t.Sheet.SetFormat(r, f)
	return nil
}
