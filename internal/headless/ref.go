package headless

import (
	"fmt"
	"strings"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// Target is what a reference names: a range of one sheet, or the whole
// sheet (from A1 to its last cell with contents).
type Target struct {
	Sheet *sheet.Sheet
	Range sheet.Rect
	Whole bool
}

// Cell reports whether the target is one cell.
func (t Target) Cell() bool { return !t.Whole && t.Range.From == t.Range.To }

// String names the target as a formula would, with its sheet.
func (t Target) String() string {
	if t.Whole {
		return sheet.QuoteSheet(t.Sheet.Name())
	}
	return sheet.Qualified(t.Sheet.Name(), t.Range)
}

// Resolve finds what ref names in w: a cell or range (B7, A1:C9, A:A),
// on a sheet (Q3!B7, 'Q3 plan'!A1:C9) or else the sheet shown when the
// file was saved; a named range (Sales); a table by name, all of it
// with its header row, or by a structured reference as formulas read it
// (Sales[Amount]); or a sheet by name (Q3, or Q3!), meaning all of it.
// "" is the whole sheet shown.
func Resolve(w *sheet.Workbook, ref string) (Target, error) {
	ref = strings.TrimSpace(ref)
	shown := w.Sheet(w.Active())
	if ref == "" {
		return Target{Sheet: shown, Whole: true}, nil
	}
	name, rest := sheet.SplitSheet(ref)
	s := shown
	if name != "" {
		if s = w.Lookup(name); s == nil {
			return Target{}, noSheet(w, name)
		}
		if rest == "" {
			return Target{Sheet: s, Whole: true}, nil
		}
	}
	if r, ok := sheet.ParseRange(strings.ReplaceAll(rest, "$", "")); ok {
		return Target{Sheet: s, Range: r}, nil
	}
	if name == "" {
		if n, ok := w.LookupName(rest); ok {
			if n.Gone() {
				return Target{}, fmt.Errorf("the named range %s lost its cells (#REF!)", n.Name)
			}
			return Target{Sheet: n.Sheet, Range: n.Range}, nil
		}
		if s, r, ok, err := w.TableRange(rest); ok {
			return Target{Sheet: s, Range: r}, err
		}
		if s := w.Lookup(rest); s != nil {
			return Target{Sheet: s, Whole: true}, nil
		}
	}
	return Target{}, fmt.Errorf("%q isn't a cell, a range, a named range, a table or a sheet (sheets: %s)", ref, sheetList(w))
}

// ResolveCell is Resolve for a reference that must be one cell.
func ResolveCell(w *sheet.Workbook, ref string) (*sheet.Sheet, sheet.Addr, error) {
	t, err := Resolve(w, ref)
	if err != nil {
		return nil, sheet.Addr{}, err
	}
	if !t.Cell() {
		return nil, sheet.Addr{}, fmt.Errorf("%s is %s: name one cell", ref, describe(t))
	}
	return t.Sheet, t.Range.From, nil
}

func describe(t Target) string {
	if t.Whole {
		return "a whole sheet"
	}
	return "the range " + t.Range.String()
}

func noSheet(w *sheet.Workbook, name string) error {
	return fmt.Errorf("no sheet named %q (sheets: %s)", name, sheetList(w))
}

func sheetList(w *sheet.Workbook) string {
	names := make([]string, 0, w.Len())
	for _, s := range w.Sheets() {
		names = append(names, s.Name())
	}
	return strings.Join(names, ", ")
}
