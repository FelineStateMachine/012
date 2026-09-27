package sheet

import (
	"errors"
	"math"
	"slices"
	"strings"
)

// Data bars and icon sets, as Excel's conditional formatting draws them:
// a bar across each number's cell as long as the number is far along
// from the rule's shortest point to its longest, or an icon by which of
// its thresholds the number reaches. Both keep their points in the
// rule's Scale, as a color scale does: a bar its shortest and longest,
// an icon set the threshold of each icon after the first.

// IconSet is a set of icons, drawn in a terminal's own glyphs so they
// read without color: arrows, circles filling up, symbols and bars
// rising. Each set comes in the sizes Excel has it in.
type IconSet uint8

const (
	IconsNone IconSet = iota
	IconsArrows
	IconsCircles
	IconsSymbols
	IconsRating
	numIconSets
)

var iconSetNames = [numIconSets]string{"", "arrows", "circles", "symbols", "rating"}
var iconSetTitles = [numIconSets]string{"None", "Arrows", "Circles", "Symbols", "Rating"}

// iconGlyphs are each set's icons by size, lowest first.
var iconGlyphs = [numIconSets]map[int][]string{
	IconsArrows:  {3: {"↓", "→", "↑"}, 4: {"↓", "↘", "↗", "↑"}, 5: {"↓", "↘", "→", "↗", "↑"}},
	IconsCircles: {3: {"○", "◑", "●"}, 4: {"○", "◔", "◕", "●"}, 5: {"○", "◔", "◑", "◕", "●"}},
	IconsSymbols: {3: {"✗", "!", "✓"}},
	IconsRating:  {4: {"▂", "▄", "▆", "█"}, 5: {"▁", "▂", "▄", "▆", "█"}},
}

func (i IconSet) String() string {
	if i >= numIconSets {
		return ""
	}
	return iconSetNames[i]
}

// Title names the set for people, e.g. "Arrows".
func (i IconSet) Title() string {
	if i >= numIconSets {
		return ""
	}
	return iconSetTitles[i]
}

// ParseIconSet is the inverse of IconSet.String.
func ParseIconSet(s string) (IconSet, bool) {
	i := slices.Index(iconSetNames[:], s)
	return IconSet(max(i, 0)), i >= 0
}

// IconSets lists the sets in the order the rules editor offers them.
func IconSets() []IconSet { return []IconSet{IconsArrows, IconsCircles, IconsSymbols, IconsRating} }

// Sizes are how many icons the set comes with.
func (i IconSet) Sizes() []int {
	if i == IconsNone || i >= numIconSets {
		return nil
	}
	var out []int
	for n := 3; n <= 5; n++ {
		if _, ok := iconGlyphs[i][n]; ok {
			out = append(out, n)
		}
	}
	return out
}

// Glyphs are the set's n icons, lowest first.
func (i IconSet) Glyphs(n int) []string {
	if i >= numIconSets {
		return nil
	}
	return iconGlyphs[i][n]
}

// IconColor is the color icon k of n draws in: the lowest red, the
// highest green and those between yellow, as Excel colors arrows and
// symbols, or blue for every bar of a rating.
func (i IconSet) IconColor(k, n int) Color {
	switch {
	case i == IconsRating:
		return ColorBlue
	case k == 0:
		return ColorRed
	case k == n-1:
		return ColorGreen
	}
	return ColorYellow
}

// IsBar reports whether the rule is a data bar.
func (f CondFormat) IsBar() bool { return f.Bar != ColorNone }

// IsIcons reports whether the rule is an icon set.
func (f CondFormat) IsIcons() bool { return f.Icons != IconsNone }

// checkBar checks a data bar: its two points and color.
func checkBar(f CondFormat) error {
	if len(f.Scale) != 2 {
		return errors.New("A data bar has a shortest and a longest point")
	}
	if f.Bar >= numColors {
		return errors.New("Choose the bar's color")
	}
	return checkPoints(f.Scale)
}

// checkIcons checks an icon set: a size it comes in, and a threshold
// for each icon after the first, rising.
func checkIcons(f CondFormat) error {
	n := len(f.Scale) + 1
	if !slices.Contains(f.Icons.Sizes(), n) {
		return errors.New("The " + strings.ToLower(f.Icons.Title()) + " come in " + sizesText(f.Icons.Sizes()))
	}
	return checkPoints(f.Scale)
}

func sizesText(ns []int) string {
	parts := make([]string, len(ns))
	for i, n := range ns {
		parts[i] = string(rune('0' + n))
	}
	return strings.Join(parts, " or ") + " icons"
}

// checkPoints checks points that take values: a number, or a percent or
// percentile from 0 to 100.
func checkPoints(ps []ScalePoint) error {
	for _, p := range ps {
		if p.Kind >= numPointKinds {
			return errors.New("Unknown kind of point")
		}
		if !p.Kind.TakesValue() {
			continue
		}
		n, _, ok := ParseValue(strings.TrimSpace(p.Value))
		switch {
		case !ok:
			return errors.New("Enter a number for the " + strings.ToLower(p.Kind.Title()))
		case p.Kind != PointNumber && (n < 0 || n > 100):
			return errors.New(p.Kind.Title() + " goes from 0 to 100")
		}
	}
	return nil
}

// barStats is a data bar's or icon set's points among the values of its
// ranges.
type barStats struct {
	ok bool
	at []float64
}

func (c *looksCache) bar(s *Sheet, i int) *barStats {
	if st, ok := c.bars[i]; ok {
		return st
	}
	f := s.rules.formats[i]
	keep := slices.ContainsFunc(f.Scale, func(p ScalePoint) bool { return p.Kind == PointPercentile })
	lo, hi, n := math.Inf(1), math.Inf(-1), 0
	var vals []float64
	for _, r := range f.Ranges {
		for _, v := range s.cells.valuesIn(r) {
			if v.Kind != Number {
				continue
			}
			lo, hi, n = min(lo, v.Num), max(hi, v.Num), n+1
			if keep {
				vals = append(vals, v.Num)
			}
		}
	}
	st := &barStats{ok: n > 0}
	if st.ok {
		for k, p := range f.Scale {
			at := p.at(vals, lo, hi)
			// A bar's automatic shortest starts at zero for positive
			// numbers, as Excel's does, so a bar's length reads as the
			// number's size.
			if f.IsBar() && k == 0 && p.Kind == PointMin {
				at = min(lo, 0)
			}
			if f.IsBar() && k == 1 && p.Kind == PointMax {
				at = max(hi, 0)
			}
			st.at = append(st.at, at)
		}
	}
	c.bars[i] = st
	return st
}

// barLook is how far along a data bar's cell of value v is drawn, from
// 0 to 1, and whether it has a bar.
func (st *barStats) barLook(v Value) (float64, bool) {
	if !st.ok || v.Kind != Number || len(st.at) != 2 {
		return 0, false
	}
	lo, hi := st.at[0], st.at[1]
	if hi <= lo {
		return 1, true
	}
	return min(max((v.Num-lo)/(hi-lo), 0), 1), true
}

// icon is which of n icons the value v gets: the last threshold it
// reaches, reversed when the set is.
func (st *barStats) icon(v Value, reverse bool) (int, bool) {
	if !st.ok || v.Kind != Number {
		return 0, false
	}
	k := 0
	for i, at := range st.at {
		if v.Num >= at {
			k = i + 1
		}
	}
	if reverse {
		k = len(st.at) - k
	}
	return k, true
}
