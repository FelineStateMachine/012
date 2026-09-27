package fileio

import (
	"cmp"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/FelineStateMachine/012/internal/numfmt"
	"github.com/FelineStateMachine/012/internal/sheet"
)

// ruleSkips counts the rules an import left out, by why.
type ruleSkips struct {
	formats, validations map[string]int
}

func (k *ruleSkips) skip(format bool, why string) {
	m := &k.validations
	if format {
		m = &k.formats
	}
	if *m == nil {
		*m = map[string]int{}
	}
	(*m)[why]++
}

// notes says what was left out, e.g. "3 conditional formats left out:
// data bars (2), icon sets (1)".
func (k *ruleSkips) notes() []string {
	var out []string
	for _, part := range []struct {
		m         map[string]int
		one, many string
	}{{k.formats, "conditional format", "conditional formats"}, {k.validations, "data validation rule", "data validation rules"}} {
		n := 0
		var whys []string
		for _, why := range slices.Sorted(maps.Keys(part.m)) {
			n += part.m[why]
			whys = append(whys, fmt.Sprintf("%s (%d)", why, part.m[why]))
		}
		if n > 0 {
			out = append(out, count(n, part.one, part.many)+" left out: "+strings.Join(whys, ", "))
		}
	}
	return out
}

// loadRules puts the rules read from a sheet on s, counting those left
// out.
func (bk *xlsxBook) loadRules(s *sheet.Sheet, r *xlsxSheetReader) {
	cfs := slices.Clone(r.cfs)
	slices.SortStableFunc(cfs, func(a, b xlsxCF) int { return cmp.Compare(a.priority, b.priority) })
	var fs []sheet.CondFormat
	for _, c := range cfs {
		f, why := bk.condFormat(c)
		if why != "" {
			bk.skips.skip(true, why)
			continue
		}
		fs = append(fs, f)
	}
	for range s.LoadCondFormats(fs) {
		bk.skips.skip(true, "without a text color, fill or text style 012 draws")
	}
	var vs []sheet.Validation
	for _, d := range r.dvs {
		v, why := validationOf(d)
		switch {
		case why == "any":
		case why != "":
			bk.skips.skip(false, why)
		default:
			vs = append(vs, v)
		}
	}
	for range s.LoadValidations(vs) {
		bk.skips.skip(false, "that 012 can't check")
	}
	for range r.extRules {
		bk.skips.skip(true, "in Excel 2010's extension (data bars, icon sets)")
	}
}

// sqrefRanges reads an sqref, "A1:A9 C1".
func sqrefRanges(ref string) ([]sheet.Rect, bool) {
	var out []sheet.Rect
	for _, f := range strings.Fields(ref) {
		r, ok := sheet.ParseRange(strings.ReplaceAll(f, "$", ""))
		if !ok {
			return nil, false
		}
		out = append(out, r)
	}
	return out, len(out) > 0
}

// cfOps are Excel's comparisons by name.
var cfOps = func() map[string]sheet.RuleOp {
	m := map[string]sheet.RuleOp{}
	for op, name := range cellIsOps {
		m[name] = op
	}
	return m
}()

// unsupported names the conditional formats 012 has no rule for.
var unsupported = map[string]string{"dataBar": "data bars", "iconSet": "icon sets", "top10": "top or bottom values",
	"aboveAverage": "above or below average", "duplicateValues": "duplicate values", "uniqueValues": "unique values",
	"containsErrors": "errors", "notContainsErrors": "errors"}

// condFormat is an Excel rule as 012's, or why it was left out.
func (bk *xlsxBook) condFormat(c xlsxCF) (sheet.CondFormat, string) {
	rs, ok := sqrefRanges(c.sqref)
	if !ok {
		return sheet.CondFormat{}, "ranges 012 can't read"
	}
	f := sheet.CondFormat{Ranges: rs}
	if c.dxf >= 0 && c.dxf < len(bk.styles.dxfs) {
		f.Style = bk.styles.dxfs[c.dxf]
	}
	first := func() string {
		if len(c.formulas) == 0 {
			return ""
		}
		return c.formulas[0]
	}
	switch c.typ {
	case "cellIs":
		return cellIsRule(f, c)
	case "containsText", "notContainsText", "beginsWith", "endsWith":
		f.Op = map[string]sheet.RuleOp{"containsText": sheet.RuleContains, "notContainsText": sheet.RuleNotContains,
			"beginsWith": sheet.RuleStartsWith, "endsWith": sheet.RuleEndsWith}[c.typ]
		f.Args[0] = c.text
	case "containsBlanks":
		f.Op = sheet.RuleEmpty
	case "notContainsBlanks":
		f.Op = sheet.RuleNotEmpty
	case "timePeriod":
		if c.period != "today" && c.period != "tomorrow" && c.period != "yesterday" {
			return f, "dates in periods other than today, tomorrow and yesterday"
		}
		f.Op, f.Args[0] = sheet.RuleDateIs, c.period
	case "expression":
		if op, day, ok := dateExpression(first()); ok {
			f.Op, f.Args[0] = op, day
			break
		}
		f.Op, f.Args[0] = sheet.RuleFormula, fromExcelFormula(first())
	case "colorScale":
		return colorScaleRule(f, c)
	default:
		if why, ok := unsupported[c.typ]; ok {
			return f, why
		}
		return f, c.typ + " rules"
	}
	return f, ""
}

// cellIsRule reads a comparison: equal to text is "Text is exactly".
func cellIsRule(f sheet.CondFormat, c xlsxCF) (sheet.CondFormat, string) {
	op, ok := cfOps[c.op]
	if !ok || len(c.formulas) < op.Args() {
		return f, "comparisons 012 can't read"
	}
	f.Op = op
	for k := range op.Args() {
		f.Args[k] = fromExcelArg(c.formulas[k], false)
	}
	if text, quoted := unquoteExcel(c.formulas[0]); quoted && op == sheet.RuleEqual {
		f.Op, f.Args[0] = sheet.RuleExactly, text
	}
	return f, ""
}

// colorScaleRule reads a color scale's points and colors.
func colorScaleRule(f sheet.CondFormat, c xlsxCF) (sheet.CondFormat, string) {
	if len(c.cfvo) < 2 || len(c.cfvo) != len(c.colors) {
		return f, "color scales 012 can't read"
	}
	for k, v := range c.cfvo {
		kind, ok := sheet.ParsePointKind(map[string]string{"min": "min", "max": "max", "num": "num",
			"percent": "percent", "percentile": "percentile"}[v[0]])
		if !ok {
			return f, "color scale points set by formulas"
		}
		f.Scale = append(f.Scale, sheet.ScalePoint{Kind: kind, Value: v[1], Color: c.colors[k].scaleColor()})
	}
	return f, ""
}

// dateRE matches the expressions dates are written with: INT(A1)<DATE(2026,9,30)
// or INT(A1)=TODAY()+1.
var dateRE = regexp.MustCompile(`^INT\(\$?[A-Za-z]{1,3}\$?\d+\)([=<>])(?:DATE\((\d+),(\d+),(\d+)\)|TODAY\(\)([+-]1)?)$`)

// dateExpression reads a date test written as xlsxrules.go writes one.
func dateExpression(fx string) (sheet.RuleOp, string, bool) {
	m := dateRE.FindStringSubmatch(strings.ReplaceAll(fx, " ", ""))
	if m == nil {
		return 0, "", false
	}
	op := map[string]sheet.RuleOp{"=": sheet.RuleDateIs, "<": sheet.RuleDateBefore, ">": sheet.RuleDateAfter}[m[1]]
	if m[2] != "" {
		y, _ := strconv.Atoi(m[2])
		mo, _ := strconv.Atoi(m[3])
		d, _ := strconv.Atoi(m[4])
		return op, fmt.Sprintf("%d-%02d-%02d", y, mo, d), true
	}
	return op, map[string]string{"": "today", "+1": "tomorrow", "-1": "yesterday"}[m[5]], true
}

// unquoteExcel reads a formula that is only a string, "text".
func unquoteExcel(fx string) (string, bool) {
	fx = strings.TrimSpace(fx)
	if len(fx) < 2 || fx[0] != '"' || fx[len(fx)-1] != '"' || strings.Contains(strings.ReplaceAll(fx[1:len(fx)-1], `""`, ""), `"`) {
		return "", false
	}
	return strings.ReplaceAll(fx[1:len(fx)-1], `""`, `"`), true
}

// fromExcelArg reads a rule's value: text, a number (a date as a date,
// for date rules) or a formula.
func fromExcelArg(fx string, date bool) string {
	if text, ok := unquoteExcel(fx); ok {
		return text
	}
	n, err := strconv.ParseFloat(strings.TrimSpace(fx), 64)
	switch {
	case err != nil:
		return fromExcelFormula(fx)
	case date:
		y, m, d := numfmt.Civil(int64(fromExcelSerial(n, sheet.Format{Kind: sheet.FmtDate})))
		return fmt.Sprintf("%d-%02d-%02d", y, m, d)
	}
	return strings.TrimSpace(fx)
}

// validKinds are Excel's validation types as 012's kinds.
var validKinds = map[string]sheet.ValidKind{"whole": sheet.ValidNumber, "decimal": sheet.ValidNumber,
	"date": sheet.ValidDate, "textLength": sheet.ValidLength, "custom": sheet.ValidFormula}

// validationOf is an Excel rule as 012's, or why it was left out ("any"
// for a rule that allows anything, which has nothing to keep).
func validationOf(d xlsxDV) (sheet.Validation, string) {
	rs, ok := sqrefRanges(d.sqref)
	if !ok {
		return sheet.Validation{}, "ranges 012 can't read"
	}
	v := sheet.Validation{Ranges: rs, Reject: d.showErr && d.style == "stop", Help: cmp.Or(d.prompt, d.err)}
	switch k, ok := validKinds[d.typ]; {
	case d.typ == "list":
		return listRule(v, d.formulas[0])
	case d.typ == "none" || d.typ == "":
		return v, "any"
	case !ok:
		return v, d.typ + " rules"
	case k == sheet.ValidFormula:
		v.Kind, v.Args[0] = k, fromExcelFormula(d.formulas[0])
	default:
		op, ok := cfOps[d.op]
		if !ok {
			return v, "comparisons 012 can't read"
		}
		v.Kind, v.Op = k, op
		if k == sheet.ValidDate && op == sheet.RuleGreater && strings.TrimSpace(d.formulas[0]) == "0" {
			v.Op = sheet.RuleNone // any date
			break
		}
		for i := range op.Args() {
			v.Args[i] = fromExcelArg(d.formulas[i], k == sheet.ValidDate)
		}
	}
	return v, ""
}

// listRule reads a list: items in quotes, or a range; TRUE and FALSE
// alone are a checkbox.
func listRule(v sheet.Validation, fx string) (sheet.Validation, string) {
	if text, ok := unquoteExcel(fx); ok {
		for _, it := range strings.Split(text, ",") {
			if it = strings.TrimSpace(it); it != "" {
				v.Items = append(v.Items, it)
			}
		}
		v.Kind = sheet.ValidList
		if len(v.Items) == 2 && strings.EqualFold(v.Items[0], "TRUE") && strings.EqualFold(v.Items[1], "FALSE") {
			v.Kind, v.Items = sheet.ValidCheckbox, nil
		}
		return v, ""
	}
	v.Kind, v.Source = sheet.ValidRange, strings.ReplaceAll(strings.TrimPrefix(strings.TrimSpace(fx), "="), "$", "")
	if _, _, ok := v.SourceRange(); !ok {
		return v, "lists from named ranges or formulas"
	}
	return v, ""
}
