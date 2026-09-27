package rules

import (
	"errors"
	"strings"

	"github.com/FelineStateMachine/012/internal/locale"
	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/ui/theme"
)

var errRange = errors.New("Enter the range the rule applies to, e.g. B2:B100")

// dvKind lists data validation rules.
type dvKind struct{}

func (dvKind) title() string                    { return "Data validation rules" }
func (dvKind) count(s *sheet.Sheet) int         { return len(s.Validations()) }
func (dvKind) remove(s *sheet.Sheet, i int)     { s.DeleteValidation(i) }
func (dvKind) move(*sheet.Sheet, int, int) bool { return false }
func (dvKind) ordered() bool                    { return false }
func (dvKind) listHint() string {
	return "A cell has one rule: a new rule takes its cells from the others"
}

// marks are how the list shows each kind of rule, as the cells do.
var marks = map[sheet.ValidKind]string{sheet.ValidList: " ▾ ", sheet.ValidRange: " ▾ ", sheet.ValidCheckbox: "[✓]"}

func (dvKind) item(th *theme.Theme, h Host, i, w int, sel bool) string {
	v := h.Sheet().Validations()[i]
	base := th.MenuBar
	if sel {
		base = th.MenuSelected
	}
	mark := marks[v.Kind]
	if mark == "" {
		mark = "   "
	}
	text := base.Render(" "+mark+" "+sheet.RangesText(v.Ranges)+"  ") + base.Render(v.Summary())
	right := ""
	if v.Reject {
		right = th.Muted.Inherit(base).Render("rejects ")
	}
	return spread(base, text, right, w)
}

// dvForm is a data validation rule being edited.
type dvForm struct {
	i         int
	ranges    string
	kind      int // a sheet.ValidKind
	items     string
	source    string
	op        int // index in the comparisons of the kind; see ops
	args      [2]string
	formula   string
	reject    int // 0 warns, 1 rejects
	help      string
	display   int    // index in sheet.DropDisplays
	checked   string // a checkbox's own values, "" for TRUE and FALSE
	unchecked string
	loc       *locale.Locale // what args are typed in
}

func (dvKind) form(h Host, i int, sel sheet.Rect) form {
	f := &dvForm{i: i, ranges: sel.String(), kind: int(sheet.ValidList), loc: h.Sheet().Locale()}
	if i < 0 {
		return f
	}
	v := h.Sheet().Validations()[i]
	f.ranges, f.kind, f.source, f.help = sheet.RangesText(v.Ranges), int(v.Kind), v.Source, v.Help
	f.display = max(indexOf(sheet.DropDisplays(), v.Display), 0)
	if on, off, custom := v.CheckboxValues(); custom {
		f.checked, f.unchecked = on, off
	} else {
		f.items = strings.Join(v.Items, ", ")
	}
	if v.Reject {
		f.reject = 1
	}
	switch {
	case v.Kind == sheet.ValidFormula:
		f.formula = sheet.LocalArg(v.Args[0], f.loc)
	case v.Kind.Compares():
		f.op, f.args = max(indexOf(f.ops(), v.Op), 0), localArgs(v.Args, f.loc)
	}
	return f
}

func (f *dvForm) title() string {
	if f.i < 0 {
		return "Add data validation"
	}
	return "Edit data validation"
}

// ops are the comparisons the kind offers: dates may be any date.
func (f *dvForm) ops() []sheet.RuleOp {
	if sheet.ValidKind(f.kind) == sheet.ValidDate {
		return append([]sheet.RuleOp{sheet.RuleNone}, sheet.CompareOps()...)
	}
	return sheet.CompareOps()
}

func displayTitles() []string {
	ds := sheet.DropDisplays()
	out := make([]string, len(ds))
	for i, d := range ds {
		out[i] = d.Title()
	}
	return out
}

// opTitle names a comparison as the kind reads it.
func opTitle(k sheet.ValidKind, op sheet.RuleOp) string {
	titles := map[sheet.RuleOp]string{sheet.RuleNone: "Is a valid date", sheet.RuleBetween: "Between",
		sheet.RuleNotBetween: "Not between", sheet.RuleEqual: "Equal to", sheet.RuleNotEqual: "Not equal to",
		sheet.RuleGreater: "Greater than", sheet.RuleGreaterEq: "Greater than or equal to",
		sheet.RuleLess: "Less than", sheet.RuleLessEq: "Less than or equal to"}
	if k == sheet.ValidDate {
		titles[sheet.RuleEqual], titles[sheet.RuleNotEqual] = "On", "Not on"
		titles[sheet.RuleGreater], titles[sheet.RuleGreaterEq] = "After", "On or after"
		titles[sheet.RuleLess], titles[sheet.RuleLessEq] = "Before", "On or before"
	}
	return titles[op]
}

// rule is the rule the form describes.
func (f *dvForm) rule() (sheet.Validation, error) {
	rs, ok := sheet.ParseRanges(f.ranges)
	if !ok && strings.TrimSpace(f.ranges) != "" {
		return sheet.Validation{}, errRange
	}
	k := sheet.ValidKind(f.kind)
	v := sheet.Validation{Ranges: rs, Kind: k, Reject: f.reject == 1, Help: strings.TrimSpace(f.help)}
	if k.Dropdown() {
		v.Display = sheet.DropDisplays()[f.display]
	}
	switch {
	case k == sheet.ValidCheckbox && strings.TrimSpace(f.checked+f.unchecked) != "":
		v.Items = []string{strings.TrimSpace(f.checked)}
		if off := strings.TrimSpace(f.unchecked); off != "" {
			v.Items = append(v.Items, off)
		}
	case k == sheet.ValidList:
		for _, it := range strings.Split(f.items, ",") {
			if it = strings.TrimSpace(it); it != "" {
				v.Items = append(v.Items, it)
			}
		}
	case k == sheet.ValidRange:
		v.Source = strings.TrimSpace(f.source)
	case k == sheet.ValidFormula:
		v.Args[0] = sheet.CanonicalArg(strings.TrimSpace(f.formula), f.loc)
	case k.Compares():
		ops := f.ops()
		v.Op = ops[min(f.op, len(ops)-1)]
		v.Args = canonicalArgs(f.args, f.loc)
	}
	return v, nil
}

func (f *dvForm) save(h Host) error {
	v, err := f.rule()
	if err != nil {
		return err
	}
	return h.SaveValidation(f.i, v)
}

func (f *dvForm) rows(*theme.Theme, Host) []row {
	kinds := sheet.ValidKinds()
	titles := make([]string, len(kinds))
	for i, k := range kinds {
		titles[i] = k.Title()
	}
	rs := []row{
		{kind: rowText, label: "Apply to", text: &f.ranges, placeholder: "B2:B100", hint: "The ranges the rule applies to, e.g. B2:B100 or B2:B9,D2:D9"},
		{kind: rowChoice, label: "Criteria", choices: titles, at: &f.kind, hint: "What the cells may hold"},
	}
	rs = append(rs, f.criteriaRows()...)
	return append(rs, row{kind: rowSep},
		row{kind: rowChoice, label: "If invalid", choices: []string{"Show a warning", "Reject the input"}, at: &f.reject,
			hint: "Keep an invalid entry and mark it, or refuse it"},
		row{kind: rowText, label: "Help text", text: &f.help, placeholder: "the rule's own", hint: "What the context line says about the cells; empty for the rule's own words"})
}

// criteriaRows are the rows of what the criteria need.
func (f *dvForm) criteriaRows() []row {
	k := sheet.ValidKind(f.kind)
	display := row{kind: rowChoice, label: "Display", choices: displayTitles(), at: &f.display,
		hint: "How the cells show their dropdown: the value as a chip, an arrow at the right, or plain text"}
	switch {
	case k == sheet.ValidList:
		return []row{{kind: rowText, label: "Items", text: &f.items, placeholder: "Yes, No, Maybe", hint: "The dropdown's items, separated by commas"}, display}
	case k == sheet.ValidRange:
		return []row{{kind: rowText, label: "From range", text: &f.source, placeholder: "Lists!A1:A20", hint: "The range whose values the dropdown lists"}, display}
	case k == sheet.ValidCheckbox:
		return []row{
			{kind: rowText, label: "Checked", text: &f.checked, placeholder: "TRUE", hint: "What a checked box holds; empty for TRUE"},
			{kind: rowText, label: "Unchecked", text: &f.unchecked, placeholder: "FALSE", hint: "What an unchecked box holds; empty for FALSE, or a blank with a value of its own for checked"},
		}
	case k == sheet.ValidFormula:
		return []row{{kind: rowText, label: "Formula", text: &f.formula, placeholder: "=B2<=C2", hint: "TRUE for valid entries, written for the first cell"}}
	case !k.Compares():
		return nil
	}
	ops := f.ops()
	f.op = min(f.op, len(ops)-1)
	titles := make([]string, len(ops))
	for i, op := range ops {
		titles[i] = opTitle(k, op)
	}
	rs := []row{{kind: rowChoice, label: "Condition", choices: titles, at: &f.op, hint: "How entries compare"}}
	holder := "number"
	if k == sheet.ValidDate {
		holder = "2026-09-30"
	}
	switch ops[f.op].Args() {
	case 1:
		rs = append(rs, row{kind: rowText, label: "Value", text: &f.args[0], placeholder: holder, hint: "The value to compare with, or a formula starting with ="})
	case 2:
		rs = append(rs, row{kind: rowText, label: "Between", text: &f.args[0], placeholder: holder, hint: "The lowest value"},
			row{kind: rowText, label: "And", text: &f.args[1], placeholder: holder, hint: "The highest value"})
	}
	return rs
}
