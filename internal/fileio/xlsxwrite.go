package fileio

import (
	"bufio"
	"fmt"
	"maps"
	"math"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/FelineStateMachine/012/internal/formula"
	"github.com/FelineStateMachine/012/internal/sheet"
)

// excelMaxText is the most characters an Excel cell holds; longer text
// is cut, or Excel reports the file as damaged.
const excelMaxText = 32767

// xlsxWriter writes worksheets, sharing cell formats among them.
type xlsxWriter struct {
	styles *xlsxStyleTable
	multi  bool   // examples name their sheet
	buf    []byte // a row's cells

	// Formulas are written as their values when Excel has no equivalent,
	// or when they name a sheet the file doesn't have, which Excel would
	// refuse.
	values       valueCount
	missing      valueCount
	missingSheet string            // the sheet missing's example names
	known        map[string]bool   // keys of the sheets written
	renamed      map[string]string // the names of sheets written under another, by key
	rules        ruleNotes         // rules left out, see xlsxrules.go
}

// valueCount counts formulas written as values, keeping the first one's
// address as an example.
type valueCount struct {
	n       int
	example string
}

func (v *valueCount) add(multi bool, ws string, a sheet.Addr) {
	v.n++
	if v.example == "" {
		v.example = a.String()
		if multi {
			v.example = sheet.QuoteSheet(ws) + "!" + a.String()
		}
	}
}

// unknownSheet returns a sheet c's formula names that isn't written.
func (w *xlsxWriter) unknownSheet(c SnapCell) (string, bool) {
	for _, name := range c.Sheets {
		if !w.known[formula.SheetKey(name)] {
			return name, true
		}
	}
	return "", false
}

// sheet writes a snapshot as a worksheet and returns the rows written.
// Errors stick in bw.
func (w *xlsxWriter) sheet(bw *bufio.Writer, ws string, snap *Snapshot, active bool) int {
	r := snap.Range
	bw.WriteString(xmlHead + `<worksheet xmlns="` + sheetMain + `" xmlns:r="` + officeRel + `">`)
	if len(snap.HiddenRows) > 0 {
		bw.WriteString(`<sheetPr filterMode="1"/>`) // a filter is hiding rows
	}
	fmt.Fprintf(bw, `<dimension ref="%s%d:%s%d"/>`, excelColName(r.From.Col+1), r.From.Row+1, excelColName(r.To.Col+1), r.To.Row+1)
	writeSheetView(bw, snap, active)
	bw.WriteString(`<sheetFormatPr defaultRowHeight="15"/>`)
	w.writeCols(bw, snap)
	bw.WriteString(`<sheetData>`)
	styled := slices.Sorted(maps.Keys(snap.RowFormats)) // rows with a style of their own, written even without cells
	for row := r.From.Row; row <= r.To.Row; row++ {
		styled = w.styledRows(bw, snap, styled, row)
		_, rowStyled := snap.RowFormats[row]
		if rowStyled {
			styled = styled[1:]
		}
		b := w.buf[:0]
		for col := r.From.Col; col <= r.To.Col; col++ {
			a := sheet.Addr{Col: col, Row: row}
			if c, ok := snap.Cells[a]; ok {
				b = w.cell(b, ws, a, c)
			}
		}
		if len(b) > 0 || rowStyled || snap.HiddenRows[row] {
			w.rowStart(bw, snap, row)
			bw.Write(b)
			bw.WriteString(`</row>`)
		}
		w.buf = b
	}
	w.styledRows(bw, snap, styled, sheet.MaxRows)
	bw.WriteString(`</sheetData>`)
	writeAutoFilter(bw, snap)
	w.writeRules(bw, ws, snap)
	bw.WriteString(`</worksheet>`)
	return r.To.Row - r.From.Row + 1
}

// cell appends snapshot cell c at a as a <c> element. A formula's value
// is written as its cached result; a formula with no Excel equivalent,
// or naming a sheet that isn't written, is written as its value alone,
// and counted.
func (w *xlsxWriter) cell(b []byte, ws string, a sheet.Addr, c SnapCell) []byte {
	fx := w.formula(ws, a, c)
	b = append(b, `<c r="`...)
	b = append(b, excelColName(a.Col+1)...)
	b = strconv.AppendInt(b, int64(a.Row+1), 10)
	b = append(b, '"')
	// A formula's inferred format is written too, so its result shows
	// the same in Excel.
	if !c.Format.IsZero() || !c.Style.IsZero() {
		b = append(b, ` s="`...)
		b = strconv.AppendInt(b, int64(w.styles.id(c.Format, c.Style)), 10)
		b = append(b, '"')
	}
	typ, v, inline := cellValue(c, fx != "")
	if typ != "" {
		b = append(b, ` t="`...)
		b = append(b, typ...)
		b = append(b, '"')
	}
	if fx == "" && v == "" {
		return append(b, "/>"...)
	}
	b = append(b, '>')
	if fx != "" {
		b = append(b, "<f>"...)
		b = appendEscaped(b, fx, false)
		b = append(b, "</f>"...)
	}
	switch {
	case inline && fx == "":
		b = append(b, `<is><t xml:space="preserve">`...)
		b = appendEscaped(b, v, true)
		b = append(b, "</t></is>"...)
	case v != "":
		b = append(b, "<v>"...)
		b = appendEscaped(b, v, typ == "str")
		b = append(b, "</v>"...)
	}
	return append(b, "</c>"...)
}

// formula is c's formula in Excel's syntax, or "" when it has none or
// is written as its value, which it counts.
func (w *xlsxWriter) formula(ws string, a sheet.Addr, c SnapCell) string {
	if !c.Formula {
		return ""
	}
	if name, ok := w.unknownSheet(c); ok {
		if w.missing.n == 0 {
			w.missingSheet = name
		}
		w.missing.add(w.multi, ws, a)
		return ""
	}
	fx, ok := toExcelFormula(c.Input, w.renamed)
	if !ok {
		w.values.add(w.multi, ws, a)
	}
	return fx
}

// cellValue is a cell's value as Excel stores it: the type attribute,
// the text, and whether text goes inline (<is>) rather than in <v>,
// which only a formula's cached text does (t="str"). Errors are written
// as text, as Excel recalculates on opening.
func cellValue(c SnapCell, hasFormula bool) (typ, v string, inline bool) {
	switch c.Value.Kind {
	case sheet.Number:
		n := toExcelSerial(c.Value.Num, c.Format)
		if math.IsNaN(n) || math.IsInf(n, 0) {
			return "e", "#NUM!", false
		}
		return "", numInput(n), false
	case sheet.Bool:
		if c.Value.Num != 0 {
			return "b", "1", false
		}
		return "b", "0", false
	case sheet.Text, sheet.Error:
		s := c.Value.Str
		if utf8.RuneCountInString(s) > excelMaxText {
			s = string([]rune(s)[:excelMaxText])
		}
		if hasFormula {
			return "str", s, false
		}
		return "inlineStr", s, true
	}
	return "", "", false
}

// escapeXML escapes s for XML text or, with attr, an attribute value.
func escapeXML(s string, attr bool) string {
	return string(appendXML(nil, s, attr))
}

func appendXML(b []byte, s string, attr bool) []byte {
	for _, r := range s {
		switch {
		case r == '&':
			b = append(b, "&amp;"...)
		case r == '<':
			b = append(b, "&lt;"...)
		case r == '>':
			b = append(b, "&gt;"...)
		case r == '"' && attr:
			b = append(b, "&quot;"...)
		case r == '\r' || attr && (r == '\n' || r == '\t'):
			b = append(b, "&#x"...)
			b = strconv.AppendInt(b, int64(r), 16)
			b = append(b, ';')
		case !xmlChar(r):
			b = utf8.AppendRune(b, utf8.RuneError)
		default:
			b = utf8.AppendRune(b, r)
		}
	}
	return b
}

// appendEscaped appends s as XML text; with ooxml, as SpreadsheetML cell
// text, where characters XML can't hold are written _xHHHH_ and a _x
// that would read as such an escape is itself escaped (_x005F_).
func appendEscaped(b []byte, s string, ooxml bool) []byte {
	if !ooxml {
		return appendXML(b, s, false)
	}
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		switch {
		case r == '_' && isOOXMLEscape(s[i:]):
			b = append(b, "_x005F"...)
		case !xmlChar(r) && r != utf8.RuneError && r <= 0xFFFF:
			b = fmt.Appendf(b, "_x%04X_", r)
			i += size
			continue
		}
		b = appendXML(b, s[i:i+size], false)
		i += size
	}
	return b
}

func isOOXMLEscape(s string) bool {
	_, ok := ooxmlEscape([]byte(s[:min(len(s), 7)]))
	return ok
}

// xmlChar reports whether XML 1.0 allows r in a document.
func xmlChar(r rune) bool {
	return r == '\t' || r == '\n' || r == '\r' ||
		r >= 0x20 && r <= 0xD7FF || r >= 0xE000 && r <= 0xFFFD || r >= 0x10000 && r <= 0x10FFFF
}

// xlsxStyleTable collects the cell formats a workbook uses: each
// distinct number format and text style is a cellXfs entry, with its
// number format code and font.
type xlsxStyleTable struct {
	ids   map[xlsxStyle]int
	xfs   []xlsxStyle // index 0 is the default
	codes []string    // custom number formats, from id 164
	fonts []sheet.Style
	dxfs  []sheet.RuleStyle // conditional formats' styles, see xlsxrules.go
}

// dxf is the index of the differential style for a rule's style.
func (t *xlsxStyleTable) dxf(st sheet.RuleStyle) int {
	if i := slices.Index(t.dxfs, st); i >= 0 {
		return i
	}
	t.dxfs = append(t.dxfs, st)
	return len(t.dxfs) - 1
}

// dxfsXML writes the differential styles: a font with the text styles
// and color, and a solid fill. Excel reads a dxf's fill from the
// pattern's background color and some readers from its foreground, so
// both are the fill.
func (t *xlsxStyleTable) dxfsXML() string {
	if len(t.dxfs) == 0 {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, `<dxfs count="%d">`, len(t.dxfs))
	for _, st := range t.dxfs {
		b.WriteString(`<dxf><font>`)
		for _, on := range []struct {
			set bool
			tag string
		}{{st.Bold, `<b/>`}, {st.Italic, `<i/>`}, {st.Strikethrough, `<strike/>`}, {st.Underline, `<u/>`}} {
			if on.set {
				b.WriteString(on.tag)
			}
		}
		if st.Text != sheet.ColorNone {
			fmt.Fprintf(&b, `<color rgb="%s"/>`, ruleRGB[st.Text][0])
		}
		b.WriteString(`</font>`)
		if st.Fill != sheet.ColorNone {
			c := ruleRGB[st.Fill][1]
			fmt.Fprintf(&b, `<fill><patternFill patternType="solid"><fgColor rgb="%s"/><bgColor rgb="%s"/></patternFill></fill>`, c, c)
		}
		b.WriteString(`</dxf>`)
	}
	return b.String() + `</dxfs>`
}

func newXLSXStyleTable() *xlsxStyleTable {
	return &xlsxStyleTable{ids: map[xlsxStyle]int{{}: 0}, xfs: []xlsxStyle{{}}, fonts: []sheet.Style{{}}}
}

// id is the index of the cell format for f and st.
func (t *xlsxStyleTable) id(f sheet.Format, st sheet.Style) int {
	key := xlsxStyle{f, st}
	if id, ok := t.ids[key]; ok {
		return id
	}
	id := len(t.xfs)
	t.ids[key] = id
	t.xfs = append(t.xfs, key)
	return id
}

// xml is the styles part.
func (t *xlsxStyleTable) xml() string {
	var xfs strings.Builder
	for _, x := range t.xfs {
		numFmt := 0
		if code := excelCode(x.format); code != "" {
			i := slices.Index(t.codes, code)
			if i < 0 {
				i = len(t.codes)
				t.codes = append(t.codes, code)
			}
			numFmt = 164 + i
		}
		font := fontOf(x.style)
		i := slices.Index(t.fonts, font)
		if i < 0 {
			i = len(t.fonts)
			t.fonts = append(t.fonts, font)
		}
		fmt.Fprintf(&xfs, `<xf numFmtId="%d" fontId="%d" fillId="0" borderId="0" xfId="0"`, numFmt, i)
		if numFmt != 0 {
			xfs.WriteString(` applyNumberFormat="1"`)
		}
		if i != 0 {
			xfs.WriteString(` applyFont="1"`)
		}
		if x.style.Align == sheet.AlignAuto {
			xfs.WriteString(`/>`)
			continue
		}
		fmt.Fprintf(&xfs, ` applyAlignment="1"><alignment horizontal="%s"/></xf>`, x.style.Align)
	}
	var b strings.Builder
	b.WriteString(xmlHead + `<styleSheet xmlns="` + sheetMain + `">`)
	if len(t.codes) > 0 {
		fmt.Fprintf(&b, `<numFmts count="%d">`, len(t.codes))
		for i, code := range t.codes {
			fmt.Fprintf(&b, `<numFmt numFmtId="%d" formatCode="%s"/>`, 164+i, escapeXML(code, true))
		}
		b.WriteString(`</numFmts>`)
	}
	fmt.Fprintf(&b, `<fonts count="%d">`, len(t.fonts))
	for _, f := range t.fonts {
		b.WriteString(`<font>`)
		for _, on := range []struct {
			set bool
			tag string
		}{{f.Bold, `<b/>`}, {f.Italic, `<i/>`}, {f.Strikethrough, `<strike/>`}, {f.Underline, `<u/>`}} {
			if on.set {
				b.WriteString(on.tag)
			}
		}
		b.WriteString(`<sz val="11"/><name val="Calibri"/><family val="2"/></font>`)
	}
	b.WriteString(`</fonts><fills count="2"><fill><patternFill patternType="none"/></fill><fill><patternFill patternType="gray125"/></fill></fills>` +
		`<borders count="1"><border><left/><right/><top/><bottom/><diagonal/></border></borders>` +
		`<cellStyleXfs count="1"><xf numFmtId="0" fontId="0" fillId="0" borderId="0"/></cellStyleXfs>`)
	fmt.Fprintf(&b, `<cellXfs count="%d">%s</cellXfs>`, len(t.xfs), xfs.String())
	b.WriteString(`<cellStyles count="1"><cellStyle name="Normal" xfId="0" builtinId="0"/></cellStyles>` + t.dxfsXML() + `</styleSheet>`)
	return b.String()
}

// fontOf is the part of a style a font holds.
func fontOf(st sheet.Style) sheet.Style {
	return sheet.Style{Bold: st.Bold, Italic: st.Italic, Underline: st.Underline, Strikethrough: st.Strikethrough}
}
