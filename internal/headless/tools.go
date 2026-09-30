package headless

import (
	"fmt"
	"strings"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// FilterColumn is one column's criteria for Filter: a condition (as
// Data > Create a filter's "Filter by condition" names them) with its
// value, and values to hide ("Filter by values").
type FilterColumn struct {
	Column    string   `json:"column" jsonschema:"a column letter (B) or the header's text"`
	Condition string   `json:"condition,omitempty" jsonschema:"empty, not_empty, contains, not_contains, starts_with, ends_with, exactly, gt, ge, lt, le, eq or ne"`
	Value     string   `json:"value,omitempty" jsonschema:"what the condition compares with, as typed: 100, 2026-01-31, North"`
	Hide      []string `json:"hide,omitempty" jsonschema:"values to hide, as the cells show them"`
}

// Filter puts a filter on the range ref names (its first row the
// header) with the columns' criteria, replacing the sheet's filter, as
// Data > Create a filter does; remove takes the sheet's filter off.
func Filter(w *sheet.Workbook, ref string, cols []FilterColumn, remove bool) error {
	t, err := Resolve(w, ref)
	if err != nil {
		return err
	}
	s := t.Sheet
	if remove {
		s.RemoveFilter()
		return nil
	}
	r := targetRect(t)
	if f := s.Filter(); f == nil || f.Range != r {
		s.CreateFilter(r)
	}
	for _, c := range cols {
		col, err := columnOf(s, r, c.Column, true)
		if err != nil {
			return err
		}
		cr := sheet.Criteria{Hidden: c.Hide}
		if c.Condition != "" {
			op, ok := sheet.ParseCondOp(c.Condition)
			if !ok {
				return fmt.Errorf("no condition %q: empty, not_empty, contains, not_contains, starts_with, ends_with, exactly, gt, ge, lt, le, eq or ne", c.Condition)
			}
			cr.Cond = sheet.Condition{Op: op, Arg: sheet.CondArg(op, c.Value, s.Locale())}
		}
		s.FilterColumn(col, cr)
	}
	return nil
}

// FindOptions tune Find.
type FindOptions struct {
	Ref        string `json:"ref,omitempty" jsonschema:"a sheet or range to search; every sheet when empty"`
	MatchCase  bool   `json:"match_case,omitempty"`
	WholeCell  bool   `json:"whole_cell,omitempty" jsonschema:"the whole cell must match"`
	Regex      bool   `json:"regex,omitempty" jsonschema:"the query is a regular expression"`
	InFormulas bool   `json:"in_formulas,omitempty" jsonschema:"search formulas' text rather than their values"`
	Limit      int    `json:"limit,omitempty" jsonschema:"the most matches to return; 100 when 0"`
}

// Match is a cell Find found: where, what it shows and what was typed.
type Match struct {
	Cell  string `json:"cell"`
	Text  string `json:"text"`
	Input string `json:"input"`
}

// Found is what Find returns: the first matches, and how many there
// are in all.
type Found struct {
	Matches []Match `json:"matches"`
	Total   int     `json:"total"`
}

// Find searches the workbook's sheets, or the sheet or range o.Ref
// names, as Edit > Find and replace does.
func Find(w *sheet.Workbook, query string, o FindOptions) (Found, error) {
	if o.Limit <= 0 {
		o.Limit = 100
	}
	sheets, within := w.Sheets(), (*sheet.Rect)(nil)
	if o.Ref != "" {
		t, err := Resolve(w, o.Ref)
		if err != nil {
			return Found{}, err
		}
		sheets = []*sheet.Sheet{t.Sheet}
		if !t.Whole {
			r := t.Range
			within = &r
		}
	}
	out := Found{Matches: []Match{}}
	for _, s := range sheets {
		if s.IsNotebook() {
			continue
		}
		addrs, err := s.Find(query, sheet.FindOptions{MatchCase: o.MatchCase, WholeCell: o.WholeCell, Regex: o.Regex,
			InFormulas: o.InFormulas, Within: within})
		if err != nil {
			return Found{}, err
		}
		out.Total += len(addrs)
		for _, a := range addrs {
			if len(out.Matches) == o.Limit {
				break
			}
			m := Match{Cell: sheet.Qualified(s.Name(), sheet.Rect{From: a, To: a}), Text: s.LocalText(a)}
			if c := s.Cell(a); c != nil {
				m.Input = c.Input
			}
			out.Matches = append(out.Matches, m)
		}
	}
	return out, nil
}

// Evaluated is a formula's result, computed without keeping it.
type Evaluated struct {
	Cell  string `json:"cell"`  // where it was computed
	Value Value  `json:"value"` // typed as ReadRange types values
	Text  string `json:"text"`  // as the cell would show it
	Error string `json:"error"` // why, when Value is an error
	Spill *Range `json:"spill,omitempty"`
}

// Evaluate computes formula as if typed in the cell at names (a cell
// below the shown sheet's data when ""), and returns what it shows.
// It changes w: call it on a workbook to throw away.
func Evaluate(w *sheet.Workbook, formula, at string) (Evaluated, error) {
	if !strings.HasPrefix(formula, "=") {
		formula = "=" + formula
	}
	s, a, err := evalCell(w, at)
	if err != nil {
		return Evaluated{}, err
	}
	out := Evaluated{Cell: sheet.Qualified(s.Name(), sheet.Rect{From: a, To: a})}
	if err := w.Batch(sheet.Change{Label: "evaluate", Sheet: s}, func() error { return setFormula(s, a, formula) }); err != nil {
		return out, err
	}
	res := ReadRange(Target{Sheet: s, Range: sheet.Rect{From: a, To: a}}, ReadOptions{})
	out.Value, out.Text = res.Values[0][0], s.LocalText(a)
	if v := s.Value(a); v.Kind == sheet.Error {
		out.Error = s.ExplainError(a)
	}
	if area, ok := s.SpillArea(a); ok && area != (sheet.Rect{From: a, To: a}) {
		spill := ReadRange(Target{Sheet: s, Range: area}, ReadOptions{MaxCells: 1000})
		out.Spill = &spill
	}
	return out, nil
}

// evalCell is where Evaluate computes: the cell at names, or the first
// cell of the column A two rows below the shown sheet's data.
func evalCell(w *sheet.Workbook, at string) (*sheet.Sheet, sheet.Addr, error) {
	if at != "" {
		return ResolveCell(w, at)
	}
	s := w.Sheet(w.Active())
	row := 0
	if used, ok := s.UsedRange(); ok {
		row = min(used.To.Row+2, sheet.MaxRows-1)
	}
	return s, sheet.Addr{Row: row}, nil
}

// setFormula types formula in a, with a parse error said as Set says it.
func setFormula(s *sheet.Sheet, a sheet.Addr, formula string) error {
	_, err := setOne(s.Book(), Entry{Ref: sheet.Qualified(s.Name(), sheet.Rect{From: a, To: a}), Input: formula}, SetOptions{Force: true})
	return err
}
