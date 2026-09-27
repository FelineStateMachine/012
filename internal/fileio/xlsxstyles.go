package fileio

import (
	"encoding/xml"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/FelineStateMachine/012/internal/locale"
	"github.com/FelineStateMachine/012/internal/sheet"
)

// xlsxStyles is what 012 reads of the styles part: custom number
// formats, fonts, borders, and the cell formats (cellXfs) that cells
// refer to by index, resolved to 012 formats and styles on first use.
type xlsxStyles struct {
	numFmts map[int]string
	fonts   []xlsxFont
	borders []sheet.Borders
	xfs     []xlsxXf
	cache   map[int]xlsxStyle
	dxfs    []sheet.RuleStyle // conditional formats' styles, see xlsxrulesread.go
	// loc is the importing workbook's locale, whose Currency and Date
	// codes read as those formats (see formatIn).
	loc *locale.Locale
}

type xlsxFont struct{ bold, italic, strike, underline bool }

// xlsxXf is a cell format; numFmt, font and border are -1 when not
// given.
type xlsxXf struct {
	numFmt, font, border               int
	applyFont, applyAlign, applyBorder bool
	align                              sheet.Align
	wrap                               bool
}

// style is the cell format with index id, as 012 keeps it; an unknown
// id is the default.
func (s *xlsxStyles) style(id int) xlsxStyle {
	if id <= 0 || id >= len(s.xfs) {
		return xlsxStyle{}
	}
	if st, ok := s.cache[id]; ok {
		return st
	}
	xf := s.xfs[id]
	var out xlsxStyle
	out.format = s.format(xf.numFmt)
	if xf.applyFont && xf.font >= 0 && xf.font < len(s.fonts) {
		f := s.fonts[xf.font]
		out.style.Bold, out.style.Italic, out.style.Strikethrough, out.style.Underline = f.bold, f.italic, f.strike, f.underline
	}
	if xf.applyAlign {
		out.style.Align = xf.align
		if xf.wrap {
			out.style.Wrap = sheet.WrapOn
		}
	}
	if xf.applyBorder && xf.border >= 0 && xf.border < len(s.borders) {
		out.style.Borders = s.borders[xf.border]
	}
	if s.cache == nil {
		s.cache = map[int]xlsxStyle{}
	}
	s.cache[id] = out
	return out
}

// format maps a number format id to a 012 format. Built-in ids (and the
// locale-dependent ones) mean their built-in code even when the file
// defines them again; others are looked up among the custom codes.
func (s *xlsxStyles) format(id int) sheet.Format {
	if id < 0 {
		return sheet.Format{}
	}
	if builtinNumFmt(id) {
		return formatOf(id, "")
	}
	return formatIn(0, s.numFmts[id], s.loc)
}

// builtinNumFmt reports whether id is one of Excel's built-in number
// formats: the ones every locale shares, and the ranges each locale
// defines for itself.
func builtinNumFmt(id int) bool {
	switch {
	case id >= 0 && id <= 4, id >= 9 && id <= 22, id >= 37 && id <= 49:
		return true
	case id >= 27 && id <= 36, id >= 50 && id <= 62, id >= 67 && id <= 81:
		return true
	}
	return false
}

// read streams the styles part.
func (s *xlsxStyles) read(p *xlsxPackage, part string) error {
	x, err := p.open(part)
	if x == nil || err != nil {
		return err
	}
	defer x.close()
	s.numFmts = map[int]string{}
	section := ""   // numFmts, fonts or cellXfs, at depth 2
	var cur *xlsxXf // the <xf> being read
	for {
		t, err := x.next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		switch t := t.(type) {
		case xml.StartElement:
			if x.depth == 2 {
				section = t.Name.Local
				continue
			}
			if cur, err = s.element(x, t, section, cur); err != nil {
				return err
			}
		case xml.EndElement:
			if x.depth < 2 {
				section = ""
			}
		}
	}
}

// styleLists name what each list of the styles part holds, by section
// and element, for the error when one is too long.
var styleLists = map[[2]string]string{
	{"dxfs", "dxf"}: "styles", {"numFmts", "numFmt"}: "styles", {"fonts", "font"}: "fonts",
	{"borders", "border"}: "borders", {"cellXfs", "xf"}: "cell formats",
}

// element reads one element inside a section of the styles part.
func (s *xlsxStyles) element(x *xmlStream, se xml.StartElement, section string, cur *xlsxXf) (*xlsxXf, error) {
	full := len(s.numFmts) >= x.g.p.lim.styles || len(s.fonts) >= x.g.p.lim.styles || len(s.xfs) >= x.g.p.lim.styles ||
		len(s.dxfs) >= x.g.p.lim.styles || len(s.borders) >= x.g.p.lim.styles
	if what, listed := styleLists[[2]string{section, se.Name.Local}]; listed && full && x.depth == 3 {
		return nil, fmt.Errorf("the workbook has more than %d %s: %w", x.g.p.lim.styles, what, errXLSXLimit)
	}
	switch {
	case section == "dxfs" && se.Name.Local == "dxf" && x.depth == 3:
		st, err := readDxf(x)
		if err != nil {
			return nil, err
		}
		s.dxfs = append(s.dxfs, st)
	case section == "numFmts" && se.Name.Local == "numFmt" && x.depth == 3:
		id, err := strconv.Atoi(attrOr(se, "numFmtId", ""))
		if err == nil {
			s.numFmts[id], _ = attr(se, "formatCode")
		}
	case section == "fonts" && se.Name.Local == "font" && x.depth == 3:
		f, err := readFont(x)
		if err != nil {
			return nil, err
		}
		s.fonts = append(s.fonts, f)
	case section == "borders" && se.Name.Local == "border" && x.depth == 3:
		b, err := readBorder(x)
		if err != nil {
			return nil, err
		}
		s.borders = append(s.borders, b)
	case section == "cellXfs" && se.Name.Local == "xf" && x.depth == 3:
		s.xfs = append(s.xfs, xlsxXf{
			numFmt: intAttr(se, "numFmtId", -1), font: intAttr(se, "fontId", -1), border: intAttr(se, "borderId", -1),
			applyFont: boolAttr(se, "applyFont", true), applyAlign: boolAttr(se, "applyAlignment", true),
			applyBorder: boolAttr(se, "applyBorder", true),
		})
		return &s.xfs[len(s.xfs)-1], nil
	case section == "cellXfs" && se.Name.Local == "alignment" && x.depth == 4 && cur != nil:
		cur.wrap = boolAttr(se, "wrapText", false)
		switch h, _ := attr(se, "horizontal"); h {
		case "left":
			cur.align = sheet.AlignLeft
		case "center", "centerContinuous":
			cur.align = sheet.AlignCenter
		case "right":
			cur.align = sheet.AlignRight
		}
	}
	return cur, nil
}

// readFont reads the rest of a <font>: bold, italic, strikethrough and
// underline (any but u val="none").
func readFont(x *xmlStream) (xlsxFont, error) {
	var f xlsxFont
	depth := x.depth
	for {
		t, err := x.next()
		if err != nil {
			return f, eofAsUnexpected(err)
		}
		switch t := t.(type) {
		case xml.StartElement:
			if x.depth != depth+1 {
				continue
			}
			switch t.Name.Local {
			case "b":
				f.bold = boolAttr(t, "val", true)
			case "i":
				f.italic = boolAttr(t, "val", true)
			case "strike":
				f.strike = boolAttr(t, "val", true)
			case "u":
				f.underline = attrOr(t, "val", "single") != "none"
			}
		case xml.EndElement:
			if x.depth < depth {
				return f, nil
			}
		}
	}
}

func attrOr(se xml.StartElement, name, def string) string {
	if v, ok := attr(se, name); ok {
		return v
	}
	return def
}

func intAttr(se xml.StartElement, name string, def int) int {
	if v, ok := attr(se, name); ok {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			return n
		}
	}
	return def
}

// boolAttr reads an xsd:boolean attribute; an empty value is true, and
// one that isn't a boolean is false.
func boolAttr(se xml.StartElement, name string, def bool) bool {
	v, ok := attr(se, name)
	if !ok {
		return def
	}
	if v = strings.TrimSpace(v); v == "" {
		return true
	}
	b, _ := strconv.ParseBool(v)
	return b
}
