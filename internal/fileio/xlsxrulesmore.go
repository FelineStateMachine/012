package fileio

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// The rules Excel 2007 has a type of its own for, both ways:
//
//	012                           Excel
//	top, bottom (percent) values  top10 with rank, percent and bottom
//	above, below average          aboveAverage (aboveAverage="0" for below)
//	duplicate, unique values      duplicateValues, uniqueValues
//	date is in a period           timePeriod where Excel has the period
//	                              (weeks, months, the past week), else an
//	                              expression on INT(cell)
//	data bar                      dataBar: two cfvo and a color
//	icon set                      iconSet: 3Arrows, 5Quarters, 4Rating ...
//
// Coming in, traffic lights, signs and quarters read as circles, flags
// and the other symbols as symbols, and gray arrows as arrows. Rules
// only Excel 2010's extension holds (its own icon sets, data bars'
// extra settings) are left out, counted in the import's note, but for
// the extension's copy of a data bar the main part also holds.

// rankRule writes a top, bottom, average or duplicates rule.
func rankRule(head string, op sheet.RuleOp, arg string) string {
	switch op {
	case sheet.RuleAboveAverage:
		return fmt.Sprintf(head, "aboveAverage") + "/>"
	case sheet.RuleBelowAverage:
		return fmt.Sprintf(head, "aboveAverage") + ` aboveAverage="0"/>`
	case sheet.RuleDuplicate:
		return fmt.Sprintf(head, "duplicateValues") + "/>"
	case sheet.RuleUnique:
		return fmt.Sprintf(head, "uniqueValues") + "/>"
	}
	attrs := fmt.Sprintf(` rank="%s"`, strings.TrimSpace(arg))
	if op == sheet.RuleTopPercent || op == sheet.RuleBottomPercent {
		attrs += ` percent="1"`
	}
	if op == sheet.RuleBottom || op == sheet.RuleBottomPercent {
		attrs += ` bottom="1"`
	}
	return fmt.Sprintf(head, "top10") + attrs + "/>"
}

// excelPeriods are the periods Excel's timePeriod names, with 012's
// words for them.
var excelPeriods = map[string]string{"today": "today", "yesterday": "yesterday", "tomorrow": "tomorrow",
	"past week": "last7Days", "this week": "thisWeek", "last week": "lastWeek", "next week": "nextWeek",
	"this month": "thisMonth", "last month": "lastMonth", "next month": "nextMonth"}

// periodBounds are Excel formulas for the first and last days of a
// period, and false for a word that isn't one.
func periodBounds(word string) (lo, hi string, ok bool) {
	word = strings.Join(strings.Fields(strings.ToLower(word)), " ")
	word = strings.TrimPrefix(strings.TrimPrefix(word, "in the "), "the ")
	switch word {
	case "today":
		return "TODAY()", "TODAY()", true
	case "tomorrow":
		return "TODAY()+1", "TODAY()+1", true
	case "yesterday":
		return "TODAY()-1", "TODAY()-1", true
	case "past week", "last 7 days":
		return "TODAY()-6", "TODAY()", true
	case "past month":
		return "EDATE(TODAY(),-1)", "TODAY()", true
	case "past year":
		return "EDATE(TODAY(),-12)", "TODAY()", true
	}
	which, unit, _ := strings.Cut(word, " ")
	step, known := map[string]int{"this": 0, "last": -1, "next": 1}[which]
	if !known {
		return "", "", false
	}
	k := func(n int) string {
		if n == 0 {
			return ""
		}
		return fmt.Sprintf("%+d", n)
	}
	switch unit {
	case "week":
		lo = "TODAY()-WEEKDAY(TODAY())+1" + k(7*step)
		return lo, lo + "+6", true
	case "month":
		return "DATE(YEAR(TODAY()),MONTH(TODAY())" + k(step) + ",1)", "DATE(YEAR(TODAY()),MONTH(TODAY())" + k(step+1) + ",0)", true
	case "year":
		return "DATE(YEAR(TODAY())" + k(step) + ",1,1)", "DATE(YEAR(TODAY())" + k(step) + ",12,31)", true
	}
	return "", "", false
}

// periodRule writes a date rule on a period: a timePeriod where Excel
// has one for "Date is", else an expression.
func periodRule(head, cell string, op sheet.RuleOp, word string) (string, bool) {
	lo, hi, ok := periodBounds(word)
	if !ok {
		return "", false
	}
	day := "INT(" + cell + ")"
	var fx string
	switch op {
	case sheet.RuleDateBefore:
		fx = day + "<" + lo
	case sheet.RuleDateAfter:
		fx = day + ">" + hi
	default:
		fx = "AND(" + day + ">=" + lo + "," + day + "<=" + hi + ")"
		if p := excelPeriods[strings.Join(strings.Fields(strings.ToLower(word)), " ")]; p != "" {
			return fmt.Sprintf(head, "timePeriod") + fmt.Sprintf(` timePeriod="%s"><formula>%s</formula></cfRule>`, p, escapeXML(fx, false)), true
		}
	}
	return fmt.Sprintf(head, "expression") + "><formula>" + escapeXML(fx, false) + "</formula></cfRule>", true
}

// cfvoXML writes a point of a data bar or icon set.
func cfvoXML(p sheet.ScalePoint) string {
	s := fmt.Sprintf(`<cfvo type="%s"`, p.Kind)
	if p.Kind.TakesValue() {
		s += fmt.Sprintf(` val="%s"`, escapeXML(strings.TrimSpace(p.Value), true))
	}
	return s + "/>"
}

// dataBarXML writes a data bar of priority p.
func dataBarXML(f sheet.CondFormat, p int) string {
	var b strings.Builder
	fmt.Fprintf(&b, `<cfRule type="dataBar" priority="%d"><dataBar`, p)
	if f.BarOnly {
		b.WriteString(` showValue="0"`)
	}
	b.WriteString(">")
	for _, pt := range f.Scale {
		b.WriteString(cfvoXML(pt))
	}
	fmt.Fprintf(&b, `<color rgb="%s"/></dataBar></cfRule>`, ruleRGB[f.Bar][2])
	return b.String()
}

// excelIconSets are Excel's icon sets for 012's, by size.
var excelIconSets = map[sheet.IconSet]map[int]string{
	sheet.IconsArrows:  {3: "3Arrows", 4: "4Arrows", 5: "5Arrows"},
	sheet.IconsCircles: {3: "3TrafficLights1", 4: "4TrafficLights", 5: "5Quarters"},
	sheet.IconsSymbols: {3: "3Symbols"},
	sheet.IconsRating:  {4: "4Rating", 5: "5Rating"},
}

// iconSetOf is 012's set for an Excel icon set, by its name.
func iconSetOf(name string) (sheet.IconSet, int, bool) {
	if name == "" {
		name = "3TrafficLights1" // Excel's default
	}
	n, err := strconv.Atoi(name[:1])
	if err != nil {
		return 0, 0, false
	}
	rest := name[1:]
	switch {
	case strings.HasPrefix(rest, "Arrows"):
		return sheet.IconsArrows, n, true
	case strings.HasPrefix(rest, "TrafficLights"), rest == "Signs", rest == "Quarters", rest == "RedToBlack":
		return sheet.IconsCircles, n, true
	case strings.HasPrefix(rest, "Symbols"), rest == "Flags":
		return sheet.IconsSymbols, n, n == 3
	case rest == "Rating":
		return sheet.IconsRating, n, true
	}
	return 0, 0, false
}

// iconSetXML writes an icon set of priority p: a first point at the
// lowest value, then the rule's thresholds.
func iconSetXML(f sheet.CondFormat, p int) (string, bool) {
	name := excelIconSets[f.Icons][len(f.Scale)+1]
	if name == "" {
		return "", false
	}
	var b strings.Builder
	fmt.Fprintf(&b, `<cfRule type="iconSet" priority="%d"><iconSet iconSet="%s"`, p, name)
	if f.Reverse {
		b.WriteString(` reverse="1"`)
	}
	if f.BarOnly {
		b.WriteString(` showValue="0"`)
	}
	b.WriteString(`><cfvo type="percent" val="0"/>`)
	for _, pt := range f.Scale {
		b.WriteString(cfvoXML(pt))
	}
	return b.String() + "</iconSet></cfRule>", true
}

// pointOf reads a data bar's or icon set's cfvo.
func pointOf(v [2]string) (sheet.ScalePoint, bool) {
	kind, ok := sheet.ParsePointKind(map[string]string{"min": "min", "max": "max", "num": "num",
		"percent": "percent", "percentile": "percentile", "autoMin": "min", "autoMax": "max"}[v[0]])
	return sheet.ScalePoint{Kind: kind, Value: v[1]}, ok
}

// barRule reads a data bar.
func barRule(f sheet.CondFormat, c xlsxCF) (sheet.CondFormat, string) {
	if len(c.cfvo) != 2 || len(c.colors) == 0 {
		return f, "data bars 012 can't read"
	}
	for _, v := range c.cfvo {
		p, ok := pointOf(v)
		if !ok {
			return f, "data bars set by formulas"
		}
		f.Scale = append(f.Scale, p)
	}
	f.Bar, f.BarOnly, f.Style = c.colors[0].scaleColor(), c.hideValue, sheet.RuleStyle{}
	return f, ""
}

// iconRule reads an icon set; its first point is the lowest value's.
func iconRule(f sheet.CondFormat, c xlsxCF) (sheet.CondFormat, string) {
	set, n, ok := iconSetOf(c.iconSet)
	if !ok || n != len(c.cfvo) || !contains(set.Sizes(), n) {
		return f, "icon sets 012 has no icons for"
	}
	for _, v := range c.cfvo[1:] {
		p, ok := pointOf(v)
		if !ok {
			return f, "icon sets set by formulas"
		}
		f.Scale = append(f.Scale, p)
	}
	f.Icons, f.Reverse, f.BarOnly, f.Style = set, c.reverse, c.hideValue, sheet.RuleStyle{}
	return f, ""
}

func contains(ns []int, n int) bool {
	for _, m := range ns {
		if m == n {
			return true
		}
	}
	return false
}

// rankRuleOf reads a top10, aboveAverage, duplicateValues or
// uniqueValues rule.
func rankRuleOf(f sheet.CondFormat, c xlsxCF) (sheet.CondFormat, string) {
	switch c.typ {
	case "duplicateValues":
		f.Op = sheet.RuleDuplicate
	case "uniqueValues":
		f.Op = sheet.RuleUnique
	case "aboveAverage":
		if c.stdDev != 0 || c.equalAverage {
			return f, "above or below average by deviations, or equal to it"
		}
		f.Op = sheet.RuleAboveAverage
		if !c.aboveAverage {
			f.Op = sheet.RuleBelowAverage
		}
	default: // top10
		f.Op = map[[2]bool]sheet.RuleOp{{false, false}: sheet.RuleTop, {false, true}: sheet.RuleTopPercent,
			{true, false}: sheet.RuleBottom, {true, true}: sheet.RuleBottomPercent}[[2]bool{c.bottom, c.percent}]
		f.Args[0] = strconv.Itoa(c.rank)
	}
	return f, ""
}

// periodOf reads a timePeriod as 012's word for it.
func periodWord(period string) (string, bool) {
	for word, p := range excelPeriods {
		if p == period {
			return word, true
		}
	}
	return "", false
}
