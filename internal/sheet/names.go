package sheet

import (
	"cmp"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
)

// Named ranges, as in Sheets' Data > Named ranges: a name such as Sales
// stands for a range in formulas, =SUM(Sales). Formulas keep the name and
// resolve it when they're evaluated, so redefining or deleting a name
// recalculates every formula that uses it. Inserting, deleting and moving
// cells moves a name's range like any other range reference.

// Name is a named range.
type Name struct {
	Name  string // as the user spelled it; formulas match it in any case
	Range Rect
	// Lost is set when every cell of the range was deleted: formulas using
	// the name show #REF!, as in Sheets. Range is then zero.
	Lost bool
}

// Ref is what the name stands for, e.g. "B2:B20", or "#REF!" when lost.
func (n Name) Ref() string {
	if n.Lost {
		return "#REF!"
	}
	return n.Range.String()
}

// maxNameLen is Sheets' limit on the length of a name.
const maxNameLen = 250

func nameKey(name string) string { return strings.ToUpper(name) }

// ValidName checks that name can be a named range, following Sheets'
// rules: letters, digits, _ and ., starting with a letter or _, and not
// something a formula would read as a cell or a boolean.
func ValidName(name string) error {
	switch {
	case name == "":
		return errors.New("Enter a name")
	case len(name) > maxNameLen:
		return fmt.Errorf("A name can be at most %d characters", maxNameLen)
	case !isLetter(name[0]) && name[0] != '_':
		return errors.New("A name must start with a letter or _")
	}
	for i := range len(name) {
		if c := name[i]; !isLetter(c) && !isDigit(c) && c != '_' && c != '.' {
			return errors.New("A name can only have letters, digits, _ and .")
		}
	}
	switch k := nameKey(name); {
	case k == "TRUE" || k == "FALSE":
		return errors.New("TRUE and FALSE can't be names")
	case looksLikeRef(k):
		return errors.New("A name can't look like a cell reference, e.g. A1 or R1C1")
	}
	return nil
}

// looksLikeRef reports whether an upper-case name reads as a cell in A1 or
// R1C1 style, even beyond this sheet's edges, so names stay unambiguous in
// other spreadsheets too.
func looksLikeRef(k string) bool {
	letters := strings.TrimLeft(k, "ABCDEFGHIJKLMNOPQRSTUVWXYZ")
	if letters != k && letters != "" && strings.Trim(letters, "0123456789") == "" {
		return true
	}
	rest, ok := strings.CutPrefix(k, "R")
	if !ok {
		rest, ok = k, strings.HasPrefix(k, "C")
	}
	if !ok {
		return false
	}
	rest = strings.TrimLeft(rest, "0123456789")
	rest, _ = strings.CutPrefix(rest, "C")
	return strings.Trim(rest, "0123456789") == ""
}

// Names returns the named ranges, sorted by name.
func (s *Sheet) Names() []Name {
	out := slices.Collect(maps.Values(s.names))
	slices.SortFunc(out, func(a, b Name) int { return cmp.Compare(nameKey(a.Name), nameKey(b.Name)) })
	return out
}

// LookupName finds a named range, ignoring case.
func (s *Sheet) LookupName(name string) (Name, bool) {
	n, ok := s.names[nameKey(name)]
	return n, ok
}

// NameUsers returns how many formulas mention the name.
func (s *Sheet) NameUsers(name string) int { return len(s.nameUsers[nameKey(name)]) }

// DefineName names the range r, as one undo step.
func (s *Sheet) DefineName(name string, r Rect) error {
	if err := ValidName(name); err != nil {
		return err
	}
	if old, ok := s.LookupName(name); ok {
		return fmt.Errorf("%s already names %s", old.Name, old.Ref())
	}
	s.change("name "+r.String()+" "+name, r, func() { s.putName(nameKey(name), &Name{Name: name, Range: r}) })
	return nil
}

// EditName renames the named range old and points it at r, as one undo
// step. Formulas that use it are rewritten to the new name, as in Sheets.
func (s *Sheet) EditName(old, name string, r Rect) error {
	cur, ok := s.LookupName(old)
	if !ok {
		return fmt.Errorf("There's no range named %s", old)
	}
	if err := ValidName(name); err != nil {
		return err
	}
	if other, ok := s.LookupName(name); ok && nameKey(name) != nameKey(old) {
		return fmt.Errorf("%s already names %s", other.Name, other.Ref())
	}
	s.change("edit "+name, r, func() {
		from, to := nameKey(cur.Name), nameKey(name)
		if cur.Name != name {
			s.renameInFormulas(from, name)
		}
		if from != to {
			s.putName(from, nil)
		}
		s.putName(to, &Name{Name: name, Range: r})
	})
	return nil
}

// DeleteName removes a named range, as one undo step. Formulas that use
// it show #NAME? until it is defined again.
func (s *Sheet) DeleteName(name string) error {
	cur, ok := s.LookupName(name)
	if !ok {
		return fmt.Errorf("There's no range named %s", name)
	}
	s.change("delete "+cur.Name, cur.Range, func() { s.putName(nameKey(name), nil) })
	return nil
}

// renameInFormulas rewrites the formulas that use the name with key from
// to spell it to instead.
func (s *Sheet) renameInFormulas(from, to string) {
	rw := refRewrite{name: func(n nameNode) Node {
		if nameKey(n.name) == from {
			return nameNode{to}
		}
		return n
	}}
	for _, a := range slices.Collect(maps.Keys(s.nameUsers[from])) {
		s.place(a, s.cells[a].rewritten(rw))
	}
}

// namePtr returns a copy of the named range with key k, or nil.
func (s *Sheet) namePtr(k string) *Name {
	if n, ok := s.names[k]; ok {
		return &n
	}
	return nil
}

// putName stores (or with nil removes) the named range with key k,
// recording it for undo, and returns the formulas that use it, which need
// recalculating. Every change to names goes through here.
func (s *Sheet) putName(k string, n *Name) []Addr {
	s.recordName(k)
	if n == nil {
		delete(s.names, k)
	} else {
		s.names[k] = *n
	}
	users := slices.Collect(maps.Keys(s.nameUsers[k]))
	if s.hist.open != nil {
		s.hist.dirty = append(s.hist.dirty, users...)
	}
	return users
}

// remapNames moves named ranges along with the cells they cover, when
// rows or columns are inserted or deleted or cells are moved. A range
// whose cells are all gone is lost.
func (s *Sheet) remapNames(rng func(Rect) (Rect, bool)) {
	for k, n := range s.names {
		if n.Lost {
			continue
		}
		next := n
		if r, ok := rng(n.Range); ok {
			next.Range = r
		} else {
			next.Range, next.Lost = Rect{}, true
		}
		if next != n {
			s.putName(k, &next)
		}
	}
}

// bound returns the cell's formula with its names replaced by the ranges
// they stand for: what is evaluated and what dependencies are traced
// through. Undefined names stay, and evaluate to #NAME?.
func (s *Sheet) bound(c *Cell) Node {
	if len(c.names) == 0 {
		return c.expr
	}
	const fixed = absCol | absRow
	n, _ := rewrite(c.expr, refRewrite{name: func(nn nameNode) Node {
		nm, ok := s.names[nameKey(nn.name)]
		switch {
		case !ok:
			return nn
		case nm.Lost:
			return refErrNode{}
		case nm.Range.From == nm.Range.To:
			return refNode{nm.Range.From, fixed}
		}
		return rangeNode{nm.Range, [2]absFlags{fixed, fixed}}
	}})
	return n
}
