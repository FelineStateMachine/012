package fileio

import (
	"bufio"
	"fmt"
	"strconv"
	"strings"

	"github.com/FelineStateMachine/012/internal/formula"
	"github.com/FelineStateMachine/012/internal/numfmt"
	"github.com/FelineStateMachine/012/internal/sheet"
)

// A sheet's rules go out as Excel's conditional formatting (single-color
// rules with a differential style, dxf, and color scales) and data
// validation, after the autoFilter as the schema orders them. The rules
// are tried in order and the first that applies wins, so each single-
// color rule stops the ones after it (stopIfTrue) and its priority is its
// place. Named colors are written in the colors of Sheets' palette.
//
//	012                         Excel
//	is empty, is not empty      containsBlanks, notContainsBlanks
//	text contains, starts ...   containsText, notContainsText, beginsWith, endsWith
//	text is exactly             cellIs equal to the text
//	date is, before, after      timePeriod (today, tomorrow, yesterday) or an expression on INT(cell)
//	comparisons, between        cellIs
//	custom formula              expression
//	color scale                 colorScale
//	checkbox                    list validation of TRUE,FALSE
//	dropdown, from a range      list validation of the items, or of the range
//	number, date, text length   decimal, date, textLength validation
//	custom formula              custom validation

// ruleRGB are the colors named rule colors are written in, for text, a
// fill and a color scale's points.
var ruleRGB = [sheet.NumColors][3]string{
	{},
	{"FFCC0000", "FFF4CCCC", "FFF8696B"}, // red
	{"FFBF9000", "FFFFF2CC", "FFFFEB84"}, // yellow
	{"FF38761D", "FFD9EAD3", "FF63BE7B"}, // green
	{"FF134F5C", "FFD0E0E3", "FF67C9D3"}, // cyan
	{"FF1155CC", "FFCFE2F3", "FF5A8AC6"}, // blue
	{"FF741B47", "FFEAD1DC", "FFC27BC0"}, // magenta
}

// ruleNotes counts rules Excel has no equivalent for, keeping the first
// one's range as an example.
type ruleNotes struct {
	formats, validations valueCount
}

// writeRules writes a sheet's conditional formats and data validation.
func (w *xlsxWriter) writeRules(bw *bufio.Writer, ws string, snap *Snapshot) {
	for i, f := range snap.CondFormats {
		rule, ok := w.cfRule(f, i+1)
		if !ok {
			w.rules.formats.add(w.multi, ws, f.Ranges[0].From)
			continue
		}
		fmt.Fprintf(bw, `<conditionalFormatting sqref="%s">%s</conditionalFormatting>`, sqref(f.Ranges), rule)
	}
	var dv strings.Builder
	n := 0
	for _, v := range snap.Validations {
		x, ok := w.validation(v)
		if !ok {
			w.rules.validations.add(w.multi, ws, v.Ranges[0].From)
			continue
		}
		dv.WriteString(x)
		n++
	}
	if n > 0 {
		fmt.Fprintf(bw, `<dataValidations count="%d">%s</dataValidations>`, n, dv.String())
	}
}

// sqref writes ranges as Excel's sqref: "A1:A9 C1:C9".
func sqref(rs []sheet.Rect) string {
	parts := make([]string, len(rs))
	for i, r := range rs {
		parts[i] = excelRect(r)
	}
	return strings.Join(parts, " ")
}

// excelText quotes text as a formula's string.
func excelText(s string) string { return `"` + strings.ReplaceAll(s, `"`, `""`) + `"` }

func excelNum(n float64) string { return strconv.FormatFloat(n, 'f', -1, 64) }

// excelArg writes a rule's value as an Excel formula: a formula
// translated, a number (a date as Excel's serial), or text.
func (w *xlsxWriter) excelArg(arg string) (string, bool) {
	arg = strings.TrimSpace(arg)
	if sheet.IsFormulaEntry(arg) {
		return toExcelFormula(arg, w.renamed)
	}
	if n, f, ok := sheet.ParseValue(arg); ok {
		return excelNum(toExcelSerial(n, f)), true
	}
	return excelText(arg), true
}

// cellIsOps are Excel's names of the comparisons.
var cellIsOps = map[sheet.RuleOp]string{sheet.RuleGreater: "greaterThan", sheet.RuleGreaterEq: "greaterThanOrEqual",
	sheet.RuleLess: "lessThan", sheet.RuleLessEq: "lessThanOrEqual", sheet.RuleEqual: "equal",
	sheet.RuleNotEqual: "notEqual", sheet.RuleBetween: "between", sheet.RuleNotBetween: "notBetween"}

// textRules are the text tests Excel has a rule type for, with the
// formula each is written with, on the cell and the text.
var textRules = map[sheet.RuleOp]struct{ typ, op, formula string }{
	sheet.RuleContains:    {"containsText", "containsText", `NOT(ISERROR(SEARCH(%[2]s,%[1]s)))`},
	sheet.RuleNotContains: {"notContainsText", "notContains", `ISERROR(SEARCH(%[2]s,%[1]s))`},
	sheet.RuleStartsWith:  {"beginsWith", "beginsWith", `LEFT(%[1]s,LEN(%[2]s))=%[2]s`},
	sheet.RuleEndsWith:    {"endsWith", "endsWith", `RIGHT(%[1]s,LEN(%[2]s))=%[2]s`},
}

// cfRule writes rule f as a <cfRule> of priority p, or reports false
// when Excel can't take it.
func (w *xlsxWriter) cfRule(f sheet.CondFormat, p int) (string, bool) {
	if f.IsScale() {
		return colorScaleXML(f.Scale, p), true
	}
	cell := excelRect(sheet.Rect{From: f.Ranges[0].From, To: f.Ranges[0].From})
	head := fmt.Sprintf(`<cfRule type="%%s" dxfId="%d" priority="%d" stopIfTrue="1"`, w.styles.dxf(f.Style), p)
	formulas := func(fs ...string) string {
		var b strings.Builder
		for _, fx := range fs {
			b.WriteString("<formula>" + escapeXML(fx, false) + "</formula>")
		}
		return b.String()
	}
	arg := strings.TrimSpace(f.Args[0])
	switch op := f.Op; {
	case op == sheet.RuleEmpty:
		return fmt.Sprintf(head, "containsBlanks") + ">" + formulas("LEN(TRIM("+cell+"))=0") + "</cfRule>", true
	case op == sheet.RuleNotEmpty:
		return fmt.Sprintf(head, "notContainsBlanks") + ">" + formulas("LEN(TRIM("+cell+"))>0") + "</cfRule>", true
	case textRules[op].typ != "" && !sheet.IsFormulaEntry(arg):
		t := textRules[op]
		return fmt.Sprintf(head, t.typ) + fmt.Sprintf(` operator="%s" text="%s">`, t.op, escapeXML(arg, true)) +
			formulas(fmt.Sprintf(t.formula, cell, excelText(arg))) + "</cfRule>", true
	case op == sheet.RuleExactly && !sheet.IsFormulaEntry(arg):
		return fmt.Sprintf(head, "cellIs") + ` operator="equal">` + formulas(excelText(arg)) + "</cfRule>", true
	case op == sheet.RuleDateIs || op == sheet.RuleDateBefore || op == sheet.RuleDateAfter:
		return w.dateRule(head, cell, op, arg)
	case op == sheet.RuleFormula:
		fx, ok := toExcelFormula(arg, w.renamed)
		return fmt.Sprintf(head, "expression") + ">" + formulas(fx) + "</cfRule>", ok
	case cellIsOps[op] != "":
		lo, ok1 := w.excelArg(f.Args[0])
		fs := []string{lo}
		ok2 := true
		if op.Args() == 2 {
			var hi string
			hi, ok2 = w.excelArg(f.Args[1])
			fs = append(fs, hi)
		}
		return fmt.Sprintf(head, "cellIs") + fmt.Sprintf(` operator="%s">`, cellIsOps[op]) + formulas(fs...) + "</cfRule>", ok1 && ok2
	}
	return "", false // a text test on a formula's value
}

// dateRule writes a date test: a timePeriod for "Date is today" and the
// like, else an expression comparing the cell's day.
func (w *xlsxWriter) dateRule(head, cell string, op sheet.RuleOp, arg string) (string, bool) {
	days := map[string]string{"today": "TODAY()", "tomorrow": "TODAY()+1", "yesterday": "TODAY()-1"}
	word := strings.ToLower(arg)
	if op == sheet.RuleDateIs && days[word] != "" {
		return fmt.Sprintf(head, "timePeriod") + fmt.Sprintf(` timePeriod="%s"><formula>FLOOR(%s,1)=%s</formula></cfRule>`, word, cell, days[word]), true
	}
	day := days[word]
	if day == "" {
		n, _, ok := sheet.ParseValue(arg)
		if !ok {
			return "", false
		}
		y, m, d := numfmt.Civil(int64(n))
		day = fmt.Sprintf("DATE(%d,%d,%d)", y, m, d)
	}
	cmp := map[sheet.RuleOp]string{sheet.RuleDateIs: "=", sheet.RuleDateBefore: "<", sheet.RuleDateAfter: ">"}[op]
	return fmt.Sprintf(head, "expression") + ">" + "<formula>" + escapeXML("INT("+cell+")"+cmp+day, false) + "</formula></cfRule>", true
}

// colorScaleXML writes a color scale of priority p.
func colorScaleXML(ps []sheet.ScalePoint, p int) string {
	var cfvo, colors strings.Builder
	for _, pt := range ps {
		fmt.Fprintf(&cfvo, `<cfvo type="%s"`, pt.Kind)
		if pt.Kind.TakesValue() {
			fmt.Fprintf(&cfvo, ` val="%s"`, escapeXML(strings.TrimSpace(pt.Value), true))
		}
		cfvo.WriteString("/>")
		fmt.Fprintf(&colors, `<color rgb="%s"/>`, ruleRGB[pt.Color][2])
	}
	return fmt.Sprintf(`<cfRule type="colorScale" priority="%d"><colorScale>%s%s</colorScale></cfRule>`, p, cfvo.String(), colors.String())
}

// validTypes are Excel's names of the kinds that compare.
var validTypes = map[sheet.ValidKind]string{sheet.ValidNumber: "decimal", sheet.ValidDate: "date", sheet.ValidLength: "textLength"}

// validation writes rule v as a <dataValidation>, or reports false when
// Excel can't take it: list items with commas or quotes, or longer than
// Excel's 255 characters.
func (w *xlsxWriter) validation(v sheet.Validation) (string, bool) {
	typ, op := "list", ""
	var fs []string
	ok := true
	switch v.Kind {
	case sheet.ValidList:
		list := strings.Join(v.Items, ",")
		ok = len(list) <= 255 && !strings.ContainsAny(strings.Join(v.Items, ""), `,"`)
		fs = []string{excelText(list)}
	case sheet.ValidCheckbox:
		fs = []string{`"TRUE,FALSE"`}
	case sheet.ValidRange:
		name, r, rok := v.SourceRange()
		ref, _ := strings.CutPrefix(excelRange("x", r), sheet.QuoteSheet("x")+"!")
		if name != "" {
			if to, renamed := w.renamed[formula.SheetKey(name)]; renamed {
				name = to
			}
			ref = excelRange(name, r)
		}
		fs, ok = []string{ref}, rok
	case sheet.ValidFormula:
		typ = "custom"
		fx, fok := toExcelFormula(v.Args[0], w.renamed)
		fs, ok = []string{fx}, fok
	default:
		typ, op = validTypes[v.Kind], cellIsOps[v.Op]
		if v.Kind == sheet.ValidDate && v.Op == sheet.RuleNone {
			op, fs = "greaterThan", []string{"0"} // any date
			break
		}
		for k := range v.Op.Args() {
			fx, aok := w.excelArg(v.Args[k])
			fs, ok = append(fs, fx), ok && aok
		}
	}
	if !ok {
		return "", false
	}
	var b strings.Builder
	fmt.Fprintf(&b, `<dataValidation type="%s"`, typ)
	if op != "" {
		fmt.Fprintf(&b, ` operator="%s"`, op)
	}
	style := "stop"
	if !v.Reject {
		style = "warning"
	}
	fmt.Fprintf(&b, ` allowBlank="1" showErrorMessage="1" errorStyle="%s"`, style)
	if h := strings.TrimSpace(v.Help); h != "" {
		fmt.Fprintf(&b, ` showInputMessage="1" prompt="%s" error="%s"`, escapeXML(h, true), escapeXML(h, true))
	}
	fmt.Fprintf(&b, ` sqref="%s">`, sqref(v.Ranges))
	for k, fx := range fs {
		fmt.Fprintf(&b, "<formula%d>%s</formula%d>", k+1, escapeXML(fx, false), k+1)
	}
	return b.String() + "</dataValidation>", true
}

// rulesNotes says which rules were left out.
func (w *xlsxWriter) rulesNotes() []string {
	var out []string
	if n := w.rules.formats; n.n > 0 {
		out = append(out, fmt.Sprintf("%s with no Excel equivalent left out, e.g. %s",
			count(n.n, "conditional format", "conditional formats"), n.example))
	}
	if n := w.rules.validations; n.n > 0 {
		out = append(out, fmt.Sprintf("%s Excel can't hold left out, e.g. %s (list items with commas or quotes, or over 255 characters, or a formula with no Excel equivalent)",
			count(n.n, "data validation rule", "data validation rules"), n.example))
	}
	return out
}
