package fileio

import (
	"encoding/xml"
	"math"
	"strconv"
	"strings"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// Reading a sheet's conditional formatting and data validation (see
// xlsxrules.go for how they map): the elements after the rows, and the
// same rules in Excel 2010's extension (x14:dataValidation, used for
// lists from other sheets). Colors come in as the nearest named color.

// xlsxColor is a color as SpreadsheetML gives it: ARGB, a theme color,
// or an index into the legacy palette.
type xlsxColor struct {
	rgb     string
	theme   int
	indexed int
}

func readColor(se xml.StartElement) xlsxColor {
	c := xlsxColor{theme: intAttr(se, "theme", -1), indexed: intAttr(se, "indexed", -1)}
	c.rgb, _ = attr(se, "rgb")
	return c
}

// officeTheme is the default Office theme's colors by index, and
// legacyPalette the first entries of the indexed palette.
var (
	officeTheme   = []uint32{0xFFFFFF, 0x000000, 0xE7E6E6, 0x44546A, 0x4472C4, 0xED7D31, 0xA5A5A5, 0xFFC000, 0x5B9BD5, 0x70AD47}
	legacyPalette = []uint32{0x000000, 0xFFFFFF, 0xFF0000, 0x00FF00, 0x0000FF, 0xFFFF00, 0xFF00FF, 0x00FFFF,
		0x000000, 0xFFFFFF, 0xFF0000, 0x00FF00, 0x0000FF, 0xFFFF00, 0xFF00FF, 0x00FFFF,
		0x800000, 0x008000, 0x000080, 0x808000, 0x800080, 0x008080, 0xC0C0C0, 0x808080}
)

// value is the color's RGB, if it has one 012 knows.
func (c xlsxColor) value() (uint32, bool) {
	switch {
	case len(c.rgb) >= 6:
		v, err := strconv.ParseUint(c.rgb[len(c.rgb)-6:], 16, 32)
		return uint32(v), err == nil
	case c.theme >= 0 && c.theme < len(officeTheme):
		return officeTheme[c.theme], true
	case c.indexed >= 0 && c.indexed < len(legacyPalette):
		return legacyPalette[c.indexed], true
	}
	return 0, false
}

// named is the named color nearest c: one of 012's own colors exactly,
// else by hue, where grays (and a color 012 can't read) are none.
func (c xlsxColor) named() sheet.Color {
	v, ok := c.value()
	if !ok {
		return sheet.ColorNone
	}
	hex := strings.ToUpper(strconv.FormatUint(uint64(v)|0xFF000000, 16))
	for i, rgb := range ruleRGB {
		for _, own := range rgb {
			if own == hex {
				return sheet.Color(i)
			}
		}
	}
	return hueColor(v)
}

// hueColor is the named color of v's hue, or none for grays.
func hueColor(v uint32) sheet.Color {
	r, g, b := float64(v>>16&0xFF), float64(v>>8&0xFF), float64(v&0xFF)
	hi, lo := max(r, g, b), min(r, g, b)
	if hi == 0 || (hi-lo)/hi < 0.05 {
		return sheet.ColorNone
	}
	var h float64
	switch d := hi - lo; hi {
	case r:
		h = math.Mod((g-b)/d, 6)
	case g:
		h = (b-r)/d + 2
	default:
		h = (r-g)/d + 4
	}
	h = math.Mod(h*60+360, 360)
	switch {
	case h < 20 || h >= 330:
		return sheet.ColorRed
	case h < 70:
		return sheet.ColorYellow
	case h < 165:
		return sheet.ColorGreen
	case h < 200:
		return sheet.ColorCyan
	case h < 260:
		return sheet.ColorBlue
	}
	return sheet.ColorMagenta
}

// scaleColor is the named color nearest c for a color scale, whose
// points need one: grays, white included, take the scale color nearest
// them.
func (c xlsxColor) scaleColor() sheet.Color {
	if n := c.named(); n != sheet.ColorNone {
		return n
	}
	v, _ := c.value()
	best, dist := sheet.ColorYellow, math.Inf(1)
	for i := 1; i < sheet.NumColors; i++ {
		s, _ := strconv.ParseUint(ruleRGB[i][2][2:], 16, 32)
		dr, dg, db := float64(v>>16&0xFF)-float64(s>>16&0xFF), float64(v>>8&0xFF)-float64(s>>8&0xFF), float64(v&0xFF)-float64(s&0xFF)
		if d := dr*dr + dg*dg + db*db; d < dist {
			best, dist = sheet.Color(i), d
		}
	}
	return best
}

// readDxf reads the rest of a <dxf>: its font's styles and color, and
// its fill's color (a dxf's solid fill is its pattern's background).
func readDxf(x *xmlStream) (sheet.RuleStyle, error) {
	var st sheet.RuleStyle
	var fg, bg xlsxColor
	in := ""
	for depth := x.depth; ; {
		t, err := x.next()
		if err != nil {
			return st, eofAsUnexpected(err)
		}
		switch t := t.(type) {
		case xml.StartElement:
			if x.depth == depth+1 {
				in = t.Name.Local
			}
			readDxfPart(t, in, &st, &fg, &bg)
		case xml.EndElement:
			if x.depth < depth {
				st.Fill = bg.named()
				if st.Fill == sheet.ColorNone {
					st.Fill = fg.named()
				}
				return st, nil
			}
		}
	}
}

// readDxfPart reads an element of a dxf's font or fill.
func readDxfPart(t xml.StartElement, in string, st *sheet.RuleStyle, fg, bg *xlsxColor) {
	switch {
	case in == "font" && t.Name.Local == "b":
		st.Bold = boolAttr(t, "val", true)
	case in == "font" && t.Name.Local == "i":
		st.Italic = boolAttr(t, "val", true)
	case in == "font" && t.Name.Local == "strike":
		st.Strikethrough = boolAttr(t, "val", true)
	case in == "font" && t.Name.Local == "u":
		st.Underline = attrOr(t, "val", "single") != "none"
	case in == "font" && t.Name.Local == "color":
		st.Text = readColor(t).named()
	case in == "fill" && t.Name.Local == "bgColor":
		*bg = readColor(t)
	case in == "fill" && t.Name.Local == "fgColor":
		*fg = readColor(t)
	}
}

// xlsxCF is a <cfRule>, with the sqref of its <conditionalFormatting>.
type xlsxCF struct {
	sqref                 string
	typ, op, text, period string
	dxf, priority         int
	formulas              []string
	cfvo                  [][2]string // type and val
	colors                []xlsxColor
	// top10's, aboveAverage's, dataBar's and iconSet's settings; see
	// xlsxrulesmore.go.
	rank, stdDev               int
	percent, bottom            bool
	aboveAverage, equalAverage bool
	iconSet                    string
	reverse, hideValue         bool
}

// xlsxDV is a <dataValidation>, of the main part or the extension.
type xlsxDV struct {
	typ, op, style, prompt, err, sqref string
	showErr, hideArrow                 bool
	formulas                           [2]string
}

// maxRules caps the rules read from one sheet.
const maxRules = 10000

// readRuleElement reads a <conditionalFormatting> or <dataValidation>
// (of the main part or the extension) just started, at any depth, into
// the reader's rules; it reports whether se was one.
func (r *xlsxSheetReader) readRuleElement(se xml.StartElement) (bool, error) {
	switch {
	case len(r.cfs)+len(r.dvs) >= maxRules:
		return false, nil
	case se.Name.Local == "conditionalFormatting" && r.x.depth == 2:
		return true, r.readCondFormatting(se)
	case se.Name.Local == "dataValidation":
		return true, r.readValidation(se)
	case se.Name.Local == "conditionalFormatting":
		return true, r.readExtRules()
	}
	return false, nil
}

// readExtRules counts the rules of a <conditionalFormatting> of Excel
// 2010's extension, which 012 leaves out: its own icon sets and the
// like. A data bar there is the extension's copy of one the main part
// holds too, with settings 012 doesn't draw, so it isn't counted.
func (r *xlsxSheetReader) readExtRules() error {
	for depth := r.x.depth; ; {
		t, err := r.x.next()
		if err != nil {
			return eofAsUnexpected(err)
		}
		switch t := t.(type) {
		case xml.EndElement:
			if r.x.depth < depth {
				return nil
			}
		case xml.StartElement:
			if t.Name.Local == "cfRule" && attrOr(t, "type", "") != "dataBar" {
				r.extRules++
			}
		}
	}
}

func (r *xlsxSheetReader) readCondFormatting(se xml.StartElement) error {
	ref, _ := attr(se, "sqref")
	var cur *xlsxCF
	for depth := r.x.depth; ; {
		t, err := r.x.next()
		if err != nil {
			return eofAsUnexpected(err)
		}
		switch t := t.(type) {
		case xml.EndElement:
			if r.x.depth < depth {
				return nil
			}
		case xml.StartElement:
			if t.Name.Local == "cfRule" && len(r.cfs) < maxRules {
				r.cfs = append(r.cfs, xlsxCF{sqref: ref, typ: attrOr(t, "type", ""), op: attrOr(t, "operator", ""),
					text: attrOr(t, "text", ""), period: attrOr(t, "timePeriod", ""), dxf: intAttr(t, "dxfId", -1),
					priority: intAttr(t, "priority", len(r.cfs)+1), rank: intAttr(t, "rank", 10), stdDev: intAttr(t, "stdDev", 0),
					percent: boolAttr(t, "percent", false), bottom: boolAttr(t, "bottom", false),
					aboveAverage: boolAttr(t, "aboveAverage", true), equalAverage: boolAttr(t, "equalAverage", false)})
				cur = &r.cfs[len(r.cfs)-1]
			} else if err := r.cfPart(t, cur); err != nil {
				return err
			}
		}
	}
}

// cfPart reads an element inside a <cfRule>: a formula, a color scale's,
// data bar's or icon set's point or color, or the bar or set itself.
// Each rule keeps at most three formulas and colors and five points.
func (r *xlsxSheetReader) cfPart(t xml.StartElement, cur *xlsxCF) error {
	switch t.Name.Local {
	case "dataBar":
		if cur != nil {
			cur.hideValue = !boolAttr(t, "showValue", true)
		}
	case "iconSet":
		if cur != nil {
			cur.iconSet, cur.reverse, cur.hideValue = attrOr(t, "iconSet", ""), boolAttr(t, "reverse", false), !boolAttr(t, "showValue", true)
		}
	case "formula":
		var err error
		if r.buf, err = r.x.text(r.buf[:0]); err != nil {
			return err
		}
		if cur != nil && len(cur.formulas) < 3 {
			cur.formulas = append(cur.formulas, string(r.buf))
		}
	case "cfvo":
		if cur != nil && len(cur.cfvo) < 5 {
			cur.cfvo = append(cur.cfvo, [2]string{attrOr(t, "type", ""), attrOr(t, "val", "")})
		}
	case "color":
		if cur != nil && len(cur.colors) < 3 {
			cur.colors = append(cur.colors, readColor(t))
		}
	}
	return nil
}

func (r *xlsxSheetReader) readValidation(se xml.StartElement) error {
	dv := xlsxDV{typ: attrOr(se, "type", "none"), op: attrOr(se, "operator", "between"), style: attrOr(se, "errorStyle", "stop"),
		prompt: attrOr(se, "prompt", ""), err: attrOr(se, "error", ""), sqref: attrOr(se, "sqref", ""),
		showErr: boolAttr(se, "showErrorMessage", false), hideArrow: boolAttr(se, "showDropDown", false)}
	for depth := r.x.depth; ; {
		t, err := r.x.next()
		if err != nil {
			return eofAsUnexpected(err)
		}
		switch t := t.(type) {
		case xml.EndElement:
			if r.x.depth < depth {
				r.dvs = append(r.dvs, dv)
				return nil
			}
		case xml.StartElement:
			k := -1
			switch t.Name.Local {
			case "formula1":
				k = 0
			case "formula2":
				k = 1
			case "sqref":
				k = 2
			default:
				continue
			}
			if r.buf, err = r.x.text(r.buf[:0]); err != nil {
				return err
			}
			if k == 2 {
				dv.sqref = string(r.buf)
			} else {
				dv.formulas[k] = string(r.buf)
			}
		}
	}
}
