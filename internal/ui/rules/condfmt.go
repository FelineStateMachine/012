package rules

import (
	"strings"

	"github.com/FelineStateMachine/012/internal/locale"
	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/ui/theme"
)

// cfKind lists conditional formats.
type cfKind struct{}

func (cfKind) title() string                { return "Conditional format rules" }
func (cfKind) count(s *sheet.Sheet) int     { return len(s.CondFormats()) }
func (cfKind) remove(s *sheet.Sheet, i int) { s.DeleteCondFormat(i) }

func (cfKind) ordered() bool    { return true }
func (cfKind) listHint() string { return "The first rule that matches a cell wins" }

func (cfKind) move(s *sheet.Sheet, i, to int) bool {
	s.MoveCondFormat(i, to)
	return true
}

// item is a rule's sample, its ranges and what it tests.
func (cfKind) item(th *theme.Theme, h Host, i, w int, sel bool) string {
	f := h.Sheet().CondFormats()[i]
	base := th.MenuBar
	if sel {
		base = th.MenuSelected
	}
	text := base.Render(" ") + sample(th, h, f, 4) + base.Render(" "+sheet.RangesText(f.Ranges)+"  ")
	return spread(base, text+base.Render(f.Summary()), "", w)
}

// sample draws w columns of a rule's look: its style on "123", its color
// scale, its data bar or its icons.
func sample(th *theme.Theme, h Host, f sheet.CondFormat, w int) string {
	switch {
	case f.IsBar():
		var b strings.Builder
		for _, n := range theme.BarCover(0.6, w) {
			b.WriteString(theme.Block(n))
		}
		return th.RuleText[f.Bar].Render(b.String())
	case f.IsIcons():
		n := len(f.Scale) + 1
		gs := f.Icons.Glyphs(n)
		var b strings.Builder
		for k := range min(len(gs), w) {
			b.WriteString(th.RuleText[f.Icons.IconColor(k, n)].Render(gs[k]))
		}
		return b.String() + strings.Repeat(" ", max(w-len(gs), 0))
	case f.IsScale():
		return scaleBar(th, h, f.Scale, w)
	}
	// The fill spans the sample; text styles only its text, as in a cell.
	base := th.Rule(f.Style)
	s := th.Text(base, sheet.Style{Bold: f.Style.Bold, Italic: f.Style.Italic,
		Underline: f.Style.Underline, Strikethrough: f.Style.Strikethrough})
	text := theme.Center("123", w)
	lead := len(text) - len(strings.TrimLeft(text, " "))
	body := strings.TrimSpace(text)
	return base.Render(text[:lead]) + s.Render(body) + base.Render(text[lead+len(body):])
}

// scaleBar draws a color scale's shades across w columns.
func scaleBar(th *theme.Theme, h Host, ps []sheet.ScalePoint, w int) string {
	var b strings.Builder
	for x := range w {
		pos := float64(x) / float64(max(w-1, 1))
		from, to := ps[0].Color, ps[len(ps)-1].Color
		if len(ps) == 3 {
			if pos <= 0.5 {
				from, to, pos = ps[0].Color, ps[1].Color, pos*2
			} else {
				from, to, pos = ps[1].Color, ps[2].Color, (pos-0.5)*2
			}
		}
		b.WriteString(th.ScaleFill(from, to, pos, h.Slot).Wrap(" "))
	}
	return b.String()
}

// cfForm is a conditional format rule being edited.
type cfForm struct {
	i      int
	ranges string
	scale  int // formSingle, formScale, formBar or formIcons
	bar    barForm
	op     int // index in sheet.CondFormatOps
	args   [2]string
	text   int // index in sheet.Colors
	fill   int
	bold   bool
	italic bool
	under  bool
	strike bool
	pts    [3]cfPoint
	loc    *locale.Locale // what args are typed in
}

// cfPoint is a color scale point being edited: kind is an index in its
// row's kinds, where the midpoint's first is none, and color a named
// color less one, as points have no None.
type cfPoint struct {
	kind  int
	value string
	color int
}

// pointKinds are the kinds each point offers: the minimum, the
// midpoint (none first) and the maximum.
var pointKinds = [3][]sheet.PointKind{
	{sheet.PointMin, sheet.PointNumber, sheet.PointPercent, sheet.PointPercentile},
	{sheet.PointMin, sheet.PointNumber, sheet.PointPercent, sheet.PointPercentile}, // PointMin stands for none
	{sheet.PointMax, sheet.PointNumber, sheet.PointPercent, sheet.PointPercentile},
}

func (cfKind) form(h Host, i int, sel sheet.Rect) form {
	f := &cfForm{i: i, ranges: sel.String(), fill: int(sheet.ColorGreen), loc: h.Sheet().Locale(), bar: newBarForm(),
		op:  indexOf(sheet.CondFormatOps(), sheet.RuleNotEmpty),
		pts: [3]cfPoint{{color: int(sheet.ColorRed) - 1}, {value: "50", color: int(sheet.ColorYellow) - 1}, {color: int(sheet.ColorGreen) - 1}}}
	if i < 0 {
		return f
	}
	r := h.Sheet().CondFormats()[i]
	f.ranges = sheet.RangesText(r.Ranges)
	switch {
	case r.IsBar():
		f.scale = formBar
		f.bar.load(r)
		return f
	case r.IsIcons():
		f.scale = formIcons
		f.bar.load(r)
		return f
	}
	if r.IsScale() {
		f.scale = formScale
		for k, p := range r.Scale {
			slot := k
			if k == len(r.Scale)-1 {
				slot = 2
			}
			f.pts[slot] = cfPoint{kind: max(indexOf(pointKinds[slot], p.Kind), 0), value: sheet.LocalArg(p.Value, f.loc), color: int(p.Color) - 1}
		}
		return f
	}
	f.op, f.args = max(indexOf(sheet.CondFormatOps(), r.Op), 0), r.Args
	if !r.Op.OnText() {
		f.args = localArgs(r.Args, f.loc)
	}
	st := r.Style
	f.text, f.fill = int(st.Text), int(st.Fill)
	f.bold, f.italic, f.under, f.strike = st.Bold, st.Italic, st.Underline, st.Strikethrough
	return f
}

func indexOf[T comparable](xs []T, x T) int {
	for i, y := range xs {
		if y == x {
			return i
		}
	}
	return -1
}

func (f *cfForm) title() string {
	if f.i < 0 {
		return "Add conditional format"
	}
	return "Edit conditional format"
}

// rule is the rule the form describes.
func (f *cfForm) rule() (sheet.CondFormat, error) {
	rs, ok := sheet.ParseRanges(f.ranges)
	if !ok && strings.TrimSpace(f.ranges) != "" {
		return sheet.CondFormat{}, errRange
	}
	r := sheet.CondFormat{Ranges: rs}
	loc := func(s string) string { return sheet.CanonicalArg(s, f.loc) }
	switch f.scale {
	case formBar:
		f.bar.barRule(&r, loc)
		return r, nil
	case formIcons:
		f.bar.iconRule(&r, loc)
		return r, nil
	}
	if f.scale == formScale {
		for k, p := range f.pts {
			if k == 1 && p.kind == 0 {
				continue // no midpoint
			}
			r.Scale = append(r.Scale, sheet.ScalePoint{Kind: pointKinds[k][p.kind], Value: sheet.CanonicalArg(strings.TrimSpace(p.value), f.loc), Color: sheet.Color(p.color + 1)})
		}
		return r, nil
	}
	r.Op = sheet.CondFormatOps()[f.op]
	r.Args = canonicalArgs(f.args, f.loc)
	if r.Op.OnText() {
		r.Args = [2]string{strings.TrimSpace(f.args[0]), strings.TrimSpace(f.args[1])}
	}
	r.Style = sheet.RuleStyle{Text: sheet.Color(f.text), Fill: sheet.Color(f.fill), Bold: f.bold,
		Italic: f.italic, Underline: f.under, Strikethrough: f.strike}
	return r, nil
}

func (f *cfForm) save(h Host) error {
	r, err := f.rule()
	if err != nil {
		return err
	}
	return h.SaveFormat(f.i, r)
}

func (f *cfForm) rows(th *theme.Theme, h Host) []row {
	rs := []row{
		{kind: rowText, label: "Apply to", text: &f.ranges, placeholder: "A2:A100", hint: "The ranges the rule applies to, e.g. A2:A100 or A2:A9,C2:C9"},
		{kind: rowChoice, label: "Format", choices: formatTitles, at: &f.scale, hint: "Color cells that meet a condition, every number along a scale, or draw bars or icons"},
		{kind: rowSep},
	}
	switch f.scale {
	case formScale:
		return append(rs, f.scaleRows(th, h)...)
	case formBar:
		return append(rs, f.bar.barRows(th, func(w int) string {
			return barSample(th, sheet.Color(f.bar.color+1), min(w, 24))
		})...)
	case formIcons:
		return append(rs, f.bar.iconRows(func(int) string {
			return iconSample(th, sheet.IconSets()[f.bar.set], f.bar.icons(), f.bar.reverse)
		})...)
	}
	op := sheet.CondFormatOps()[f.op]
	rs = append(rs, row{kind: rowChoice, label: "Format if", choices: opTitles(), at: &f.op, hint: "The condition cells must meet"})
	switch op.Args() {
	case 1:
		hint, holder := argHint(op)
		rs = append(rs, row{kind: rowText, label: "Value", text: &f.args[0], placeholder: holder, hint: hint})
	case 2:
		rs = append(rs, row{kind: rowText, label: "Between", text: &f.args[0], placeholder: "lowest", hint: "The lowest value"},
			row{kind: rowText, label: "And", text: &f.args[1], placeholder: "highest", hint: "The highest value"})
	}
	swatch := func(fill bool) func(int) string {
		return func(i int) string {
			if i == 0 {
				return th.MenuBar.Render("  ")
			}
			if fill {
				return th.RuleFill[i].Render("  ")
			}
			return th.RuleText[i].Render("Aa")
		}
	}
	rs = append(rs, row{kind: rowSep},
		row{kind: rowChoice, label: "Text color", choices: colorTitles(), at: &f.text, swatch: swatch(false), hint: "The text's color, from the terminal's palette"},
		row{kind: rowChoice, label: "Fill", choices: colorTitles(), at: &f.fill, swatch: swatch(true), hint: "The cell's background, from the terminal's palette"},
		row{kind: rowCheck, label: "Bold", on: &f.bold, hint: "Bold text"},
		row{kind: rowCheck, label: "Italic", on: &f.italic, hint: "Italic text"},
		row{kind: rowCheck, label: "Underline", on: &f.under, hint: "Underlined text"},
		row{kind: rowCheck, label: "Strikethrough", on: &f.strike, hint: "Struck through text"},
		row{kind: rowSep},
		row{kind: rowPreview, label: "Preview", preview: func(int) string {
			r, _ := f.rule()
			return sample(th, h, r, 12)
		}})
	return rs
}

// scaleRows are the rows of a color scale: each point's kind, value and
// color, and a preview.
func (f *cfForm) scaleRows(th *theme.Theme, h Host) []row {
	var rs []row
	names := [3]string{"Minpoint", "Midpoint", "Maxpoint"}
	for k := range f.pts {
		p := &f.pts[k]
		kinds := make([]string, len(pointKinds[k]))
		for j, pk := range pointKinds[k] {
			kinds[j] = pk.Title()
		}
		if k == 1 {
			kinds[0] = "None"
		}
		rs = append(rs, row{kind: rowChoice, label: names[k], choices: kinds, at: &p.kind, hint: "Where the point's color goes among the values"})
		if k == 1 && p.kind == 0 {
			continue
		}
		if pointKinds[k][p.kind].TakesValue() {
			rs = append(rs, row{kind: rowText, label: "  Value", text: &p.value, placeholder: "50", hint: "The point's number, percent or percentile"})
		}
		rs = append(rs, row{kind: rowChoice, label: "  Color", choices: colorTitles()[1:], at: &p.color, swatch: func(i int) string {
			return th.RuleFill[i+1].Render("  ")
		}, hint: "The point's color, from the terminal's palette"})
	}
	return append(rs, row{kind: rowSep}, row{kind: rowPreview, label: "Preview", preview: func(w int) string {
		r, _ := f.rule()
		return scaleBar(th, h, r.Scale, min(w, 24))
	}})
}

// argHint is the hint and placeholder of a condition's value.
func argHint(op sheet.RuleOp) (hint, holder string) {
	switch op {
	case sheet.RuleFormula:
		return "TRUE for cells to format, written for the first cell: =$C2>100", "=$C2>100"
	case sheet.RuleDateIs, sheet.RuleDateBefore, sheet.RuleDateAfter:
		return "A date, or a period: " + strings.Join(sheet.Periods(), ", "), "2026-09-30 or this week"
	case sheet.RuleTop, sheet.RuleBottom:
		return "How many of the highest or lowest values, 1 to 1000", "10"
	case sheet.RuleTopPercent, sheet.RuleBottomPercent:
		return "What percent of the highest or lowest values, 1 to 100", "10"
	}
	return "The value to compare with, or a formula starting with =", "value or =formula"
}

func opTitles() []string {
	ops := sheet.CondFormatOps()
	out := make([]string, len(ops))
	for i, op := range ops {
		out[i] = op.Title()
	}
	return out
}

func colorTitles() []string {
	cs := sheet.Colors()
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.Title()
	}
	return out
}
