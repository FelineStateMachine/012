package fileio

import (
	"bufio"
	"encoding/xml"
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// Wrapping, borders, row heights and merged cells in XLSX, both ways.
// Wrapped text is a cell format's alignment wrapText; clipping has no
// XLSX form, so clipped text overflows in Excel. Borders are the styles
// part's <borders>, a cell format pointing at one: Excel's hair, thin,
// dotted and dashed lines read as thin, medium and thick ones as thick,
// double as double, and 012's thick is written as medium, the weight
// Sheets' middle line has. A row's height is in points, 15 to a line,
// written with customHeight as a height set by hand; only those are
// read, as Excel sizes the others to fit. Merged cells are <mergeCells>
// after the cells.

// pointsPerLine is the height of a line of text in points: Excel's
// default row height.
const pointsPerLine = 15

// borderLine maps an Excel border style to a line.
func borderLine(style string) sheet.Line {
	switch style {
	case "", "none":
		return sheet.LineNone
	case "double":
		return sheet.LineDouble
	case "medium", "thick", "mediumDashed", "mediumDashDot", "mediumDashDotDot", "slantDashDot":
		return sheet.LineThick
	}
	return sheet.LineThin
}

// excelBorderStyle is the Excel border style that draws l.
func excelBorderStyle(l sheet.Line) string {
	switch l {
	case sheet.LineThick:
		return "medium"
	case sheet.LineDouble:
		return "double"
	}
	return "thin"
}

// xlsxEdges are a <border>'s elements for each edge; start and end are
// left and right in left-to-right sheets.
var xlsxEdges = map[string]sheet.Edge{
	"top": sheet.EdgeTop, "bottom": sheet.EdgeBottom, "left": sheet.EdgeLeft, "start": sheet.EdgeLeft, "right": sheet.EdgeRight, "end": sheet.EdgeRight,
}

// excelVAligns are Excel's vertical alignments; justify and distributed
// have no match and sit where 012's text sits by default.
var excelVAligns = map[string]sheet.VAlign{"top": sheet.VAlignTop, "center": sheet.VAlignMiddle, "bottom": sheet.VAlignBottom}

// excelVAlign is the vertical alignment Excel writes for v.
func excelVAlign(v sheet.VAlign) string {
	for name, va := range excelVAligns {
		if va == v {
			return name
		}
	}
	return ""
}

// readBorder reads the rest of a <border>: its four edges, each with
// its line's color as the nearest named color (see xlsxColor.named).
func readBorder(x *xmlStream) (sheet.Borders, error) {
	var b sheet.Borders
	depth := x.depth
	edge, inEdge := sheet.EdgeTop, false
	for {
		t, err := x.next()
		if err != nil {
			return b, eofAsUnexpected(err)
		}
		switch t := t.(type) {
		case xml.StartElement:
			switch {
			case x.depth == depth+1:
				edge, inEdge = xlsxEdges[t.Name.Local]
				if inEdge {
					b = b.With(edge, borderLine(attrOr(t, "style", "")))
				}
			case x.depth == depth+2 && inEdge && t.Name.Local == "color" && b.Line(edge) != sheet.LineNone:
				b = b.WithColor(edge, readColor(t).named())
			}
		case xml.EndElement:
			if x.depth < depth {
				return b, nil
			}
		}
	}
}

// borderID is the index of the <border> drawing b, 0 for none.
func (t *xlsxStyleTable) borderID(b sheet.Borders) int {
	if i := slices.Index(t.borders, b); i >= 0 {
		return i
	}
	t.borders = append(t.borders, b)
	return len(t.borders) - 1
}

// bordersXML writes the <borders>, the first drawing none.
func (t *xlsxStyleTable) bordersXML() string {
	var b strings.Builder
	fmt.Fprintf(&b, `<borders count="%d">`, len(t.borders))
	for _, br := range t.borders {
		b.WriteString(`<border>`)
		for _, e := range [...]struct {
			tag  string
			l    sheet.Line
			edge sheet.Edge
		}{{"left", br.Left(), sheet.EdgeLeft}, {"right", br.Right(), sheet.EdgeRight}, {"top", br.Top(), sheet.EdgeTop}, {"bottom", br.Bottom(), sheet.EdgeBottom}} {
			if e.l == sheet.LineNone {
				fmt.Fprintf(&b, `<%s/>`, e.tag)
				continue
			}
			fmt.Fprintf(&b, `<%s style="%s">%s</%s>`, e.tag, excelBorderStyle(e.l), borderColorXML(br.Color(e.edge)), e.tag)
		}
		b.WriteString(`<diagonal/></border>`)
	}
	return b.String() + `</borders>`
}

// borderColorXML is the <color> of a border line: a named color's, as
// rules write it, or the automatic one.
func borderColorXML(c sheet.Color) string {
	if c == sheet.ColorNone {
		return `<color auto="1"/>`
	}
	return fmt.Sprintf(`<color rgb="%s"/>`, ruleRGB[c][0])
}

// alignmentXML is a cell format's <alignment>, or "" when it has none.
func alignmentXML(st sheet.Style) string {
	if st.Align == sheet.AlignAuto && st.Wrap != sheet.WrapOn && st.VAlign == sheet.VAlignAuto {
		return ""
	}
	s := `<alignment`
	if st.Align != sheet.AlignAuto {
		s += fmt.Sprintf(` horizontal="%s"`, st.Align)
	}
	if v := excelVAlign(st.VAlign); v != "" {
		s += fmt.Sprintf(` vertical="%s"`, v)
	}
	if st.Wrap == sheet.WrapOn {
		s += ` wrapText="1"`
	}
	return s + `/>`
}

// rowHeight is the height in lines of a <row> set by hand to more than
// a line, and 0 for one that fits its contents.
func rowHeight(se xml.StartElement) int {
	if !boolAttr(se, "customHeight", false) {
		return 0
	}
	var ht float64
	if _, err := fmt.Sscan(attrOr(se, "ht", ""), &ht); err != nil || !(ht > 0) {
		return 0
	}
	if lines := clamp(int(math.Round(ht/pointsPerLine)), 1, sheet.MaxRowHeight); lines > 1 {
		return lines
	}
	return 0
}

// maxMergesRead caps the merged ranges read from a worksheet.
const maxMergesRead = 1 << 16

// writeMerges writes the sheet's <mergeCells>.
func writeMerges(bw *bufio.Writer, snap *Snapshot) {
	if len(snap.Merges) == 0 {
		return
	}
	fmt.Fprintf(bw, `<mergeCells count="%d">`, len(snap.Merges))
	for _, m := range snap.Merges {
		fmt.Fprintf(bw, `<mergeCell ref="%s"/>`, excelRect(m))
	}
	bw.WriteString(`</mergeCells>`)
}

// readMerge reads a <mergeCell>'s range, ignoring one that isn't a
// range of the sheet.
func (r *xlsxSheetReader) readMerge(se xml.StartElement) {
	ref, _ := attr(se, "ref")
	if m, ok := sheet.ParseRange(strings.ReplaceAll(ref, "$", "")); ok && len(r.merges) < maxMergesRead {
		r.merges = append(r.merges, m)
	}
}
