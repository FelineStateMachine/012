package sheet

import (
	"regexp"
	"strings"
)

// FindOptions mirror Google Sheets' Find and replace dialog.
type FindOptions struct {
	MatchCase  bool
	WholeCell  bool  // the whole cell must match, not just part of it
	Regex      bool  // the query is a regular expression; replacements may use $1
	InFormulas bool  // search formula text instead of formula results
	Within     *Rect // limit the search to a range; nil searches the sheet
}

// finder is a compiled query.
type finder struct {
	re *regexp.Regexp
	o  FindOptions
}

func newFinder(query string, o FindOptions) (*finder, error) {
	pat := query
	if !o.Regex {
		pat = regexp.QuoteMeta(query)
	}
	if o.WholeCell {
		pat = "^(?:" + pat + ")$"
	}
	if !o.MatchCase {
		pat = "(?i)" + pat
	}
	re, err := regexp.Compile(pat)
	if err != nil {
		return nil, err
	}
	return &finder{re: re, o: o}, nil
}

// text is what the search looks at in a cell: formula text when searching
// within formulas, otherwise what the cell shows.
func (f *finder) text(c *Cell) string {
	if c.IsFormula() && f.o.InFormulas {
		return c.Input
	}
	if c.IsFormula() {
		return c.Value.String()
	}
	return strings.TrimPrefix(c.Input, "'")
}

// Find returns the cells matching query in reading order: row by row,
// left to right.
func (s *Sheet) Find(query string, o FindOptions) ([]Addr, error) {
	if query == "" {
		return nil, nil
	}
	f, err := newFinder(query, o)
	if err != nil {
		return nil, err
	}
	var out []Addr
	for _, a := range s.Addrs() {
		if o.Within != nil && !o.Within.Contains(a) {
			continue
		}
		if f.re.MatchString(f.text(s.cells[a])) {
			out = append(out, a)
		}
	}
	return out, nil
}

// Replace replaces matches of query in the cell at a and reports whether
// the cell changed. Formula cells are only changed when searching within
// formulas, as in Sheets. A replacement that turns a formula invalid is
// returned as an error and leaves the cell unchanged.
func (s *Sheet) Replace(a Addr, query, repl string, o FindOptions) (bool, error) {
	f, err := newFinder(query, o)
	if err != nil {
		return false, err
	}
	return s.replace(a, f, repl)
}

func (s *Sheet) replace(a Addr, f *finder, repl string) (bool, error) {
	c := s.cells[a]
	if c == nil || (c.IsFormula() && !f.o.InFormulas) {
		return false, nil
	}
	old := f.text(c)
	if !f.re.MatchString(old) {
		return false, nil
	}
	if !f.o.Regex {
		repl = strings.ReplaceAll(repl, "$", "$$")
	}
	next := f.re.ReplaceAllString(old, repl)
	if strings.HasPrefix(c.Input, "'") && !c.IsFormula() {
		next = "'" + next // keep text forced as text
	}
	if next == c.Input {
		return false, nil
	}
	return true, s.Set(a, next)
}

// ReplaceAll replaces every match and returns how many cells changed. It
// stops at the first cell whose replacement is an invalid formula.
func (s *Sheet) ReplaceAll(query, repl string, o FindOptions) (int, error) {
	cells, err := s.Find(query, o)
	if err != nil {
		return 0, err
	}
	f, err := newFinder(query, o)
	if err != nil {
		return 0, err
	}
	n := 0
	var focus Rect
	if len(cells) > 0 {
		focus = NewRect(cells[0], cells[len(cells)-1])
	}
	// One undo step for the whole replacement.
	err = s.Batch(Change{Label: "replace all", Focus: focus}, func() error {
		for _, a := range cells {
			changed, err := s.replace(a, f, repl)
			if err != nil {
				return err
			}
			if changed {
				n++
			}
		}
		return nil
	})
	return n, err
}
