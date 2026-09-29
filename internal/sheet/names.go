package sheet

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/FelineStateMachine/012/internal/formula"
)

// Named ranges, as in Sheets' Data > Named ranges: a name such as Sales
// stands for a range in formulas, =SUM(Sales). Formulas keep the name and
// resolve it when they're evaluated, so redefining or deleting a name
// recalculates every formula that uses it. Inserting, deleting and moving
// cells moves a name's range like any other range reference.

// Name is a named range. Names belong to the workbook, as in Sheets, and
// each points at a range on one of its sheets.
type Name struct {
	Name  string // as the user spelled it; formulas match it in any case
	Sheet *Sheet // the sheet the range is on
	Range Rect
	// Lost is set when every cell of the range was deleted: formulas using
	// the name show #REF!, as in Sheets. Range is then zero.
	Lost bool
}

// Gone reports whether the name no longer points at cells: its cells or
// its sheet were deleted.
func (n Name) Gone() bool { return n.Lost || n.Sheet == nil || !n.Sheet.live }

// Ref is what the name stands for, e.g. "B2:B20", or "#REF!" when gone.
// In a workbook of several sheets it names the sheet: "Sales!B2:B20".
func (n Name) Ref() string {
	switch {
	case n.Gone():
		return "#REF!"
	case n.Sheet.wb.Len() > 1:
		return Qualified(n.Sheet.name, n.Range)
	}
	return n.Range.String()
}

// maxNameLen is Sheets' limit on the length of a name.
const maxNameLen = 250

func nameKey(name string) string { return strings.ToUpper(name) }

// ValidName checks that name can be a named range, following Sheets'
// rules: letters, digits, _ and ., starting with a letter or _, and not
// something a formula would read as a cell or a boolean.
func ValidName(name string) error { return checkName(name, formula.LooksLikeRef) }

// checkName checks name against the rules of names, isRef telling
// which read as a cell reference.
func checkName(name string, isRef func(string) bool) error {
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
	case isRef(k):
		return errors.New("A name can't look like a cell reference, e.g. A1 or R1C1")
	}
	return nil
}

// Names returns the named ranges, sorted by name.
func (w *Workbook) Names() []Name {
	// The keys are already upper case: sorting names by nameKey upper-cased
	// both sides of every comparison, 20,000 allocations a frame for 1000
	// names, since the formula bar looks for the selection's name.
	keys := slices.Sorted(maps.Keys(w.names))
	out := make([]Name, len(keys))
	for i, k := range keys {
		out[i] = w.names[k]
	}
	return out
}

// LookupName finds a named range, ignoring case.
func (w *Workbook) LookupName(name string) (Name, bool) {
	n, ok := w.names[nameKey(name)]
	return n, ok
}

// NameUsers returns how many formulas mention the name.
func (w *Workbook) NameUsers(name string) int { return len(w.nameUsers[nameKey(name)]) }

// DefineName names the range r on sheet s, as one undo step.
func (w *Workbook) DefineName(name string, s *Sheet, r Rect) error {
	if err := ValidName(name); err != nil {
		return err
	}
	if old, ok := w.LookupName(name); ok {
		return fmt.Errorf("%s already names %s", old.Name, old.Ref())
	}
	if _, _, ok := w.regionName(nameKey(name)); ok {
		return fmt.Errorf("%s names a shell region", name)
	}
	if _, t, ok := w.Table(name); ok {
		return fmt.Errorf("%s names a table", t.Name)
	}
	w.change(s, "name "+r.String()+" "+name, r, func() { w.putName(nameKey(name), &Name{Name: name, Sheet: s, Range: r}) })
	return nil
}

// EditName renames the named range old and points it at r on sheet s, as
// one undo step. Formulas that use it are rewritten to the new name, as
// in Sheets.
func (w *Workbook) EditName(old, name string, s *Sheet, r Rect) error {
	cur, ok := w.LookupName(old)
	if !ok {
		return fmt.Errorf("There's no range named %s", old)
	}
	if err := ValidName(name); err != nil {
		return err
	}
	if other, ok := w.LookupName(name); ok && nameKey(name) != nameKey(old) {
		return fmt.Errorf("%s already names %s", other.Name, other.Ref())
	}
	w.change(s, "edit "+name, r, func() {
		from, to := nameKey(cur.Name), nameKey(name)
		if cur.Name != name {
			w.renameInFormulas(from, name)
		}
		if from != to {
			w.putName(from, nil)
		}
		w.putName(to, &Name{Name: name, Sheet: s, Range: r})
	})
	return nil
}

// DeleteName removes a named range, as one undo step. Formulas that use
// it show #NAME? until it is defined again.
func (w *Workbook) DeleteName(name string) error {
	cur, ok := w.LookupName(name)
	if !ok {
		return fmt.Errorf("There's no range named %s", name)
	}
	w.change(cur.Sheet, "delete "+cur.Name, cur.Range, func() { w.putName(nameKey(name), nil) })
	return nil
}

// The sheet's name methods are the workbook's, with ranges on this sheet.

func (s *Sheet) Names() []Name                        { return s.wb.Names() }
func (s *Sheet) LookupName(name string) (Name, bool)  { return s.wb.LookupName(name) }
func (s *Sheet) NameUsers(name string) int            { return s.wb.NameUsers(name) }
func (s *Sheet) DefineName(name string, r Rect) error { return s.wb.DefineName(name, s, r) }
func (s *Sheet) DeleteName(name string) error         { return s.wb.DeleteName(name) }
func (s *Sheet) EditName(old, name string, r Rect) error {
	return s.wb.EditName(old, name, s, r)
}

// renameInFormulas rewrites the formulas that use the name with key from
// to spell it to instead.
func (w *Workbook) renameInFormulas(from, to string) {
	rw := formula.Rewriter{Name: func(n formula.Name) Node {
		if nameKey(n.Name) == from {
			return formula.Name{Name: to}
		}
		return n
	}}
	for _, l := range slices.Collect(maps.Keys(w.nameUsers[from])) {
		l.s.place(l.a, l.s.cells.get(l.a).rewritten(rw))
	}
}

// namePtr returns a copy of the named range with key k, or nil.
func (w *Workbook) namePtr(k string) *Name {
	if n, ok := w.names[k]; ok {
		return &n
	}
	return nil
}

// putName stores (or with nil removes) the named range with key k,
// recording it for undo, and returns the formulas that use it, which need
// recalculating. Every change to names goes through here.
func (w *Workbook) putName(k string, n *Name) []loc {
	w.recordName(k)
	if n == nil {
		delete(w.names, k)
	} else {
		w.names[k] = *n
	}
	users := slices.Collect(maps.Keys(w.nameUsers[k]))
	if w.hist.open != nil {
		w.hist.dirty = append(w.hist.dirty, users...)
	}
	return users
}

// remapNames moves the named ranges on this sheet along with the cells
// they cover, when rows or columns are inserted or deleted or cells are
// moved. A range whose cells are all gone is lost.
func (s *Sheet) remapNames(rng func(Rect) (Rect, bool)) {
	w := s.wb
	for k, n := range w.names {
		if n.Lost || n.Sheet != s {
			continue
		}
		next := n
		if r, ok := rng(n.Range); ok {
			next.Range = r
		} else {
			next.Range, next.Lost = Rect{}, true
		}
		if next != n {
			w.putName(k, &next)
		}
	}
}
