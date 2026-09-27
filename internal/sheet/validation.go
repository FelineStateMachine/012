package sheet

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/FelineStateMachine/012/internal/formula"
	"github.com/FelineStateMachine/012/internal/functions"
)

// Data validation follows Sheets' Data > Data validation: a rule on
// ranges of the sheet says what their cells may hold (an item of a
// list, TRUE or FALSE for a checkbox, a number or date in bounds, text
// of a length, or whatever meets a formula) and whether an entry that
// doesn't is rejected or kept with a warning. Each cell has at most one
// rule: adding one over cells that had another takes them from it.

// ValidKind is what a validation rule checks.
type ValidKind uint8

const (
	ValidList     ValidKind = iota // one of Items, picked from a dropdown
	ValidRange                     // one of the values of Source, picked from a dropdown
	ValidCheckbox                  // TRUE or FALSE, drawn as a checkbox
	ValidNumber                    // a number meeting Op
	ValidDate                      // a date meeting Op; any date with RuleNone
	ValidLength                    // text whose length meets Op
	ValidFormula                   // anything for which the formula Args[0] is TRUE
	numValidKinds
)

var validNames = [numValidKinds]string{"list", "range", "checkbox", "number", "date", "length", "formula"}
var validTitles = [numValidKinds]string{"Dropdown", "Dropdown (from a range)", "Checkbox", "Number",
	"Date", "Text length", "Custom formula is"}

func (k ValidKind) String() string {
	if k >= numValidKinds {
		return ""
	}
	return validNames[k]
}

// Title names the kind for people, e.g. "Dropdown (from a range)".
func (k ValidKind) Title() string {
	if k >= numValidKinds {
		return ""
	}
	return validTitles[k]
}

// ParseValidKind is the inverse of ValidKind.String.
func ParseValidKind(s string) (ValidKind, bool) {
	i := slices.Index(validNames[:], s)
	return ValidKind(max(i, 0)), i >= 0
}

// ValidKinds lists the kinds in the order the rules editor offers them.
func ValidKinds() []ValidKind {
	out := make([]ValidKind, numValidKinds)
	for i := range out {
		out[i] = ValidKind(i)
	}
	return out
}

// Compares reports whether the kind compares with Op and Args.
func (k ValidKind) Compares() bool { return k == ValidNumber || k == ValidDate || k == ValidLength }

// Dropdown reports whether cells of the kind offer a list to pick from.
func (k ValidKind) Dropdown() bool { return k == ValidList || k == ValidRange }

// Validation is a data validation rule.
type Validation struct {
	Ranges []Rect
	Kind   ValidKind
	// Op compares numbers, dates or lengths with Args, as typed; RuleNone
	// with ValidDate accepts any date. With ValidFormula, Args[0] is the
	// formula, relative to the first range's top-left cell.
	Op    RuleOp
	Args  [2]string
	Items []string // ValidList's items
	// Source is ValidRange's range, e.g. "A2:A9" or "Lists!A1:A20".
	Source string
	// Reject refuses an invalid entry; otherwise it's kept and marked.
	Reject bool
	// Help replaces the rule's own help text, shown on the context line.
	Help string
}

func (v Validation) clone() Validation {
	v.Ranges, v.Items = slices.Clone(v.Ranges), slices.Clone(v.Items)
	return v
}

// Check reports why a rule can't be used, in words for the rules editor.
func (v Validation) Check() error {
	switch {
	case len(v.Ranges) == 0:
		return errors.New("Enter the range the rule applies to, e.g. B2:B100")
	case v.Kind >= numValidKinds:
		return errors.New("Choose the criteria")
	case v.Kind == ValidList && len(v.Items) == 0:
		return errors.New("Enter the items, separated by commas")
	case v.Kind == ValidRange:
		if _, _, ok := v.sourceRange(); !ok {
			return errors.New("Enter the range the items are in, e.g. Lists!A1:A20")
		}
	case v.Kind == ValidFormula:
		return checkArg(RuleFormula, strings.TrimSpace(v.Args[0]))
	case v.Kind.Compares():
		return v.checkBounds()
	}
	return nil
}

func (v Validation) checkBounds() error {
	if v.Op == RuleNone && v.Kind == ValidDate {
		return nil
	}
	if !slices.Contains(CompareOps(), v.Op) {
		return errors.New("Choose a comparison")
	}
	for i := range v.Op.Args() {
		arg := strings.TrimSpace(v.Args[i])
		if IsFormulaEntry(arg) {
			if err := checkArg(RuleFormula, arg); err != nil {
				return err
			}
			continue
		}
		if _, _, ok := ParseValue(arg); !ok {
			what := "a number"
			if v.Kind == ValidDate {
				what = "a date, e.g. 2026-09-30"
			}
			return errors.New("Enter " + what)
		}
	}
	return nil
}

// sourceRange is ValidRange's source: its sheet as written ("" for the
// rule's own) and range.
func (v Validation) sourceRange() (string, Rect, bool) {
	sheet, rest := SplitSheet(strings.TrimPrefix(strings.TrimSpace(v.Source), "="))
	r, ok := ParseRange(strings.ReplaceAll(rest, "$", ""))
	return sheet, r, ok
}

// Summary describes the rule in a few words, e.g. "Number between 1 and 10".
func (v Validation) Summary() string {
	switch {
	case v.Kind == ValidList:
		return "Dropdown: " + strings.Join(v.Items, ", ")
	case v.Kind == ValidRange:
		return "Dropdown from " + v.Source
	case v.Kind == ValidFormula:
		return "Custom formula " + v.Args[0]
	case v.Kind.Compares():
		return v.Kind.Title() + " " + compareText(v.Kind, v.Op, v.Args)
	}
	return v.Kind.Title()
}

// compareText is a comparison in words: "between 1 and 10", "after 1/1/2026".
func compareText(k ValidKind, op RuleOp, args [2]string) string {
	if k == ValidDate && op == RuleNone {
		return "is a valid date"
	}
	words := map[RuleOp]string{RuleBetween: "between", RuleNotBetween: "not between",
		RuleEqual: "equal to", RuleNotEqual: "not equal to", RuleGreater: "greater than",
		RuleGreaterEq: "greater than or equal to", RuleLess: "less than", RuleLessEq: "less than or equal to"}
	if k == ValidDate {
		words[RuleEqual], words[RuleNotEqual] = "on", "not on"
		words[RuleGreater], words[RuleGreaterEq] = "after", "on or after"
		words[RuleLess], words[RuleLessEq] = "before", "on or before"
	}
	w := words[op]
	if op.Args() == 2 {
		return w + " " + args[0] + " and " + args[1]
	}
	return w + " " + args[0]
}

// HelpText is what the context line says about a cell under the rule:
// the rule's own help text, or Sheets' message for its criteria.
func (v Validation) HelpText() string {
	if h := strings.TrimSpace(v.Help); h != "" {
		return h
	}
	switch v.Kind {
	case ValidList:
		return "Input must be an item on the specified list"
	case ValidRange:
		return "Input must fall within specified range"
	case ValidCheckbox:
		return "Input must be TRUE or FALSE"
	case ValidNumber:
		return "Input must be a number " + compareText(v.Kind, v.Op, v.Args)
	case ValidDate:
		if v.Op == RuleNone {
			return "Input must be a valid date"
		}
		return "Input must be a date " + compareText(v.Kind, v.Op, v.Args)
	case ValidLength:
		return "Input must be text whose length is " + compareText(v.Kind, v.Op, v.Args)
	}
	return "Input must satisfy the formula " + v.Args[0]
}

// Validations returns the sheet's validation rules.
func (s *Sheet) Validations() []Validation {
	out := make([]Validation, len(s.rules.validations))
	for i, v := range s.rules.validations {
		out[i] = v.clone()
	}
	return out
}

// Validation returns the rule of the cell at a, if it has one.
func (s *Sheet) Validation(a Addr) (Validation, bool) {
	if i := s.validationIndex(a); i >= 0 {
		return s.rules.validations[i].clone(), true
	}
	return Validation{}, false
}

func (s *Sheet) validationIndex(a Addr) int {
	for i, v := range s.rules.validations {
		if inRanges(v.Ranges, a) {
			return i
		}
	}
	return -1
}

// AddValidation adds a rule, taking its cells from the rules they had,
// as one undo step.
func (s *Sheet) AddValidation(v Validation) error {
	return s.putValidation(-1, v, "add data validation "+rangesText(v.Ranges))
}

// SetValidation replaces rule i, as one undo step.
func (s *Sheet) SetValidation(i int, v Validation) error {
	if i < 0 || i >= len(s.rules.validations) {
		return errors.New("That rule was removed")
	}
	return s.putValidation(i, v, "edit data validation "+rangesText(v.Ranges))
}

// putValidation stores v in place of rule i (-1 adds it), taking its
// cells from the others.
func (s *Sheet) putValidation(i int, v Validation, label string) error {
	if err := v.Check(); err != nil {
		return err
	}
	v = v.clone()
	r := s.rules
	r.validations = nil
	for j, o := range s.rules.validations {
		if j == i {
			r.validations = append(r.validations, v)
			continue
		}
		for _, cut := range v.Ranges {
			o.Ranges = subtractAll(o.Ranges, cut)
		}
		if len(o.Ranges) > 0 {
			r.validations = append(r.validations, o)
		}
	}
	if i < 0 {
		r.validations = append(r.validations, v)
	}
	s.setRules(label, v.Ranges[0], r)
	return nil
}

// DeleteValidation removes rule i.
func (s *Sheet) DeleteValidation(i int) {
	if i < 0 || i >= len(s.rules.validations) {
		return
	}
	v := s.rules.validations[i]
	r := s.rules
	r.validations = slices.Delete(slices.Clone(r.validations), i, i+1)
	s.setRules("remove data validation "+rangesText(v.Ranges), v.Ranges[0], r)
}

// ClearValidations takes the cells of cr out of every rule, as Sheets'
// "Remove rule" does for a selection.
func (s *Sheet) ClearValidations(cr Rect) {
	r := s.rules
	r.validations = nil
	for _, v := range s.rules.validations {
		v.Ranges = subtractAll(v.Ranges, cr)
		if len(v.Ranges) > 0 {
			r.validations = append(r.validations, v)
		}
	}
	s.setRules("remove data validation from "+cr.String(), cr, r)
}

// LoadValidations adds rules as a loader does: without recording undo.
// Rules that don't check are left out and counted.
func (s *Sheet) LoadValidations(vs []Validation) (skipped int) {
	for _, v := range vs {
		if v.Check() != nil {
			skipped++
			continue
		}
		s.rules.validations = append(s.rules.validations, v.clone())
	}
	s.looks.reset()
	return skipped
}

// InvalidEntry is an entry a cell's validation doesn't accept.
type InvalidEntry struct {
	Addr   Addr
	Help   string // what the rule wants, e.g. "Input must be a number between 1 and 10"
	Reject bool   // the rule refuses it; otherwise it may be kept, marked invalid
}

func (e *InvalidEntry) Error() string {
	return fmt.Sprintf("The data you entered in %s violates the data validation rules set on this cell. %s", e.Addr, e.Help)
}

// CheckEntry reports whether the cell at a would accept input: nil when
// it has no rule or input meets it (blanks always do), or an
// *InvalidEntry. A formula is checked by the value it computes.
func (s *Sheet) CheckEntry(a Addr, input string) *InvalidEntry {
	i := s.validationIndex(a)
	if i < 0 || input == "" {
		return nil
	}
	v := s.rules.validations[i]
	val, f := s.entryValue(a, input)
	if f.IsZero() {
		f = s.DisplayFormat(a)
	}
	if v.Kind == ValidFormula {
		// The formula reads the entry in its own cell.
		s.looks.fresh(s)
		rd := s.wb.lookupOn(s, func(t *Sheet, b Addr) Value {
			if t == s && b == a {
				return val
			}
			return t.Value(b)
		})
		n, err := Parse(strings.TrimSpace(v.Args[0]))
		if err == nil {
			from := anchor(v.Ranges)
			n, _ = formula.Rewrite(n, formula.Shift(a.Col-from.Col, a.Row-from.Row))
			if truthy(functions.EvalAt(n, rd.lib, a)) {
				return nil
			}
		}
	} else if s.looks.check(s, i, a, val, f) {
		return nil
	}
	return &InvalidEntry{Addr: a, Help: v.HelpText(), Reject: v.Reject}
}

// entryValue is what input would compute to at a, and the format it
// implies, without storing it.
func (s *Sheet) entryValue(a Addr, input string) (Value, Format) {
	n, f, err := classify(input)
	switch {
	case err != nil:
		return ErrValue, f
	case n == nil:
		return Value{Kind: Text, Str: strings.TrimPrefix(input, "'")}, f
	}
	return s.looks.eval(s, n, a), f
}

// DropdownItems lists what a dropdown cell offers: the rule's items, or
// the distinct values shown in its range, in order, blanks left out.
func (s *Sheet) DropdownItems(a Addr) []string {
	v, ok := s.Validation(a)
	switch {
	case !ok:
		return nil
	case v.Kind == ValidList:
		return v.Items
	case v.Kind == ValidRange:
		return s.sourceItems(v)
	}
	return nil
}

// maxDropdown caps the items a dropdown from a range lists.
const maxDropdown = 1000

// sourceItems reads the distinct values shown in a rule's source range.
func (s *Sheet) sourceItems(v Validation) []string {
	name, r, ok := v.sourceRange()
	t := s
	if name != "" {
		t = s.wb.resolve(s, name)
	}
	if !ok || t == nil {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for a := range t.cells.inRange(r) {
		text := strings.TrimSpace(t.ShownText(a))
		key := strings.ToLower(text)
		if text == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, text)
		if len(out) == maxDropdown {
			break
		}
	}
	return out
}
