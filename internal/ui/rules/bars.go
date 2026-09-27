package rules

import (
	"strconv"
	"strings"

	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/ui/theme"
)

// The data bar's and icon set's parts of the conditional format form.

// Kinds of the form's rule, the choices of its Format row.
const (
	formSingle = iota
	formScale
	formBar
	formIcons
)

var formatTitles = []string{"Single color", "Color scale", "Data bar", "Icon set"}

// barKinds are the kinds a bar's shortest and longest points offer, and
// iconKinds an icon's threshold's.
var (
	barKinds = [2][]sheet.PointKind{
		{sheet.PointMin, sheet.PointNumber, sheet.PointPercent, sheet.PointPercentile},
		{sheet.PointMax, sheet.PointNumber, sheet.PointPercent, sheet.PointPercentile},
	}
	iconKinds = []sheet.PointKind{sheet.PointPercent, sheet.PointNumber, sheet.PointPercentile}
)

// barForm is a data bar or icon set being edited.
type barForm struct {
	color   int        // index in sheet.Colors()[1:]
	pts     [2]cfPoint // the bar's shortest and longest; color unused
	barOnly bool

	set      int        // index in sheet.IconSets()
	size     int        // index in the set's sizes
	thr      [4]cfPoint // the icons' thresholds, the second icon's first
	sized    int        // the size thr holds values for
	reverse  bool
	iconOnly bool
}

func newBarForm() barForm {
	b := barForm{color: int(sheet.ColorBlue) - 1}
	b.fitThresholds(3)
	return b
}

// fitThresholds gives n icons thresholds evenly apart in percent, as
// Excel does, unless they already have values for n.
func (b *barForm) fitThresholds(n int) {
	if b.sized == n {
		return
	}
	for k := range b.thr {
		b.thr[k] = cfPoint{value: strconv.Itoa(100 * (k + 1) / n)}
	}
	b.sized = n
}

// sizes are the current set's sizes.
func (b *barForm) sizes() []int { return sheet.IconSets()[b.set].Sizes() }

// icons is how many icons the form's set has.
func (b *barForm) icons() int {
	ss := b.sizes()
	b.size = min(b.size, len(ss)-1)
	return ss[b.size]
}

// load reads rule r into the form.
func (b *barForm) load(r sheet.CondFormat) {
	switch {
	case r.IsBar():
		b.color, b.barOnly = int(r.Bar)-1, r.BarOnly
		for k, p := range r.Scale {
			b.pts[k] = cfPoint{kind: max(indexOf(barKinds[k], p.Kind), 0), value: p.Value}
		}
	case r.IsIcons():
		b.set = max(indexOf(sheet.IconSets(), r.Icons), 0)
		n := len(r.Scale) + 1
		b.size = max(indexOf(b.sizes(), n), 0)
		for k, p := range r.Scale {
			b.thr[k] = cfPoint{kind: max(indexOf(iconKinds, p.Kind), 0), value: p.Value}
		}
		b.sized, b.reverse, b.iconOnly = n, r.Reverse, r.BarOnly
	}
}

// barRule fills r's data bar from the form.
func (b *barForm) barRule(r *sheet.CondFormat, loc func(string) string) {
	r.Bar, r.BarOnly = sheet.Color(b.color+1), b.barOnly
	for k, p := range b.pts {
		r.Scale = append(r.Scale, sheet.ScalePoint{Kind: barKinds[k][p.kind], Value: loc(strings.TrimSpace(p.value))})
	}
}

// iconRule fills r's icon set from the form.
func (b *barForm) iconRule(r *sheet.CondFormat, loc func(string) string) {
	n := b.icons()
	r.Icons, r.Reverse, r.BarOnly = sheet.IconSets()[b.set], b.reverse, b.iconOnly
	for _, p := range b.thr[:n-1] {
		r.Scale = append(r.Scale, sheet.ScalePoint{Kind: iconKinds[p.kind], Value: loc(strings.TrimSpace(p.value))})
	}
}

// barRows are the rows of a data bar: its points, color, whether the
// values show, and a preview.
func (b *barForm) barRows(th *theme.Theme, preview func(w int) string) []row {
	names := [2]string{"Shortest", "Longest"}
	var rs []row
	for k := range b.pts {
		p := &b.pts[k]
		kinds := make([]string, len(barKinds[k]))
		for j, pk := range barKinds[k] {
			kinds[j] = pk.Title()
		}
		kinds[0] = "Automatic"
		rs = append(rs, row{kind: rowChoice, label: names[k], choices: kinds, at: &p.kind,
			hint: "Where the bar is shortest or longest: automatic runs from zero, or the lowest below it, to the highest"})
		if barKinds[k][p.kind].TakesValue() {
			rs = append(rs, row{kind: rowText, label: "  Value", text: &p.value, placeholder: "0", hint: "The point's number, percent or percentile"})
		}
	}
	colors := colorTitles()[1:]
	return append(rs,
		row{kind: rowChoice, label: "Bar color", choices: colors, at: &b.color, swatch: func(i int) string {
			return th.RuleText[i+1].Render("██")
		}, hint: "The bar's color, from the terminal's palette"},
		row{kind: rowCheck, label: "Show bar only", on: &b.barOnly, hint: "Hide the values, showing only their bars"},
		row{kind: rowSep},
		row{kind: rowPreview, label: "Preview", preview: preview})
}

// iconRows are the rows of an icon set: the set and its size, each
// threshold, the order, whether the values show, and a preview.
func (b *barForm) iconRows(preview func(w int) string) []row {
	sets := sheet.IconSets()
	titles := make([]string, len(sets))
	for i, s := range sets {
		titles[i] = s.Title()
	}
	n := b.icons()
	b.fitThresholds(n)
	sizes := make([]string, len(b.sizes()))
	for i, s := range b.sizes() {
		sizes[i] = strconv.Itoa(s) + " icons"
	}
	glyphs := sets[b.set].Glyphs(n)
	rs := []row{
		{kind: rowChoice, label: "Icons", choices: titles, at: &b.set, hint: "The set: arrows, circles filling up, symbols or rising bars"},
		{kind: rowChoice, label: "How many", choices: sizes, at: &b.size, hint: "How many icons the values are sorted into"},
	}
	kinds := make([]string, len(iconKinds))
	for j, k := range iconKinds {
		kinds[j] = k.Title()
	}
	for k := range n - 1 {
		p := &b.thr[k]
		rs = append(rs,
			row{kind: rowChoice, label: glyphs[k+1] + " from", choices: kinds, at: &p.kind, hint: "What the icon's threshold is: a percent of the way from lowest to highest, a number or a percentile"},
			row{kind: rowText, label: "  Value", text: &p.value, placeholder: "50", hint: "The lowest value the icon goes to"})
	}
	return append(rs,
		row{kind: rowCheck, label: "Reverse icons", on: &b.reverse, hint: "Give the lowest values the highest icon"},
		row{kind: rowCheck, label: "Show icon only", on: &b.iconOnly, hint: "Hide the values, showing only their icons"},
		row{kind: rowSep},
		row{kind: rowPreview, label: "Preview", preview: preview})
}

// barSample draws a data bar's look across w columns: bars a quarter,
// half and all of the way, as a sheet's cells would show them.
func barSample(th *theme.Theme, c sheet.Color, w int) string {
	var b strings.Builder
	each := max(w/3, 2)
	for _, frac := range []float64{0.25, 0.6, 1} {
		for _, n := range theme.BarCover(frac, each-1) {
			b.WriteString(th.RuleText[c].Render(theme.Block(n)))
		}
		b.WriteString(" ")
	}
	return b.String()
}

// iconSample draws an icon set's icons, lowest first, in their colors.
func iconSample(th *theme.Theme, set sheet.IconSet, n int, reverse bool) string {
	var b strings.Builder
	gs := set.Glyphs(n)
	for k := range gs {
		i := k
		if reverse {
			i = len(gs) - 1 - k
		}
		b.WriteString(th.RuleText[set.IconColor(i, n)].Render(gs[i]) + " ")
	}
	return b.String()
}
