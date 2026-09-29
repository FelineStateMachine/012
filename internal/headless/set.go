package headless

import (
	"errors"
	"fmt"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// Entry is one cell to set: a reference to one cell, and what to type
// in it, as the file stores entries (numbers and dates in en-US's form,
// formulas with commas, whatever the workbook's locale); "" clears it.
type Entry struct {
	Ref, Input string
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

func setOne(w *sheet.Workbook, e Entry, o SetOptions) (string, error) {
	s, a, err := ResolveCell(w, e.Ref)
	if err != nil {
		return "", err
	}
	at := sheet.Qualified(s.Name(), sheet.Rect{From: a, To: a})
	if p, ok := s.Protecting(sheet.Rect{From: a, To: a}); ok && !o.Force {
		return "", fmt.Errorf("%s is protected (%s): --force sets it anyway", at, p.Label())
	}
	warn := ""
	if bad := s.CheckEntry(a, e.Input); bad != nil {
		if bad.Reject {
			return "", fmt.Errorf("%s: %s", at, bad.Help)
		}
		warn = at + ": " + bad.Help
	}
	if err := s.Set(a, e.Input); err != nil {
		var pe *sheet.ParseError
		if errors.As(err, &pe) {
			return "", fmt.Errorf("%s: %s, at character %d of %s", at, pe.Msg, pe.Pos+1, e.Input)
		}
		return "", fmt.Errorf("%s: %w", at, err)
	}
	return warn, nil
}
