package fileio

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"path"
	"strconv"
)

// xlsxBook is an open workbook: its sheets, names, shared strings and
// cell styles. Worksheets are streamed one at a time by readSheet.
type xlsxBook struct {
	pkg      *xlsxPackage
	sheets   []xlsxSheetInfo
	active   int // index of the sheet shown in Excel
	names    []xlsxName
	date1904 bool
	sst      sharedStrings
	styles   xlsxStyles
}

// xlsxSheetInfo is a sheet as the workbook lists it.
type xlsxSheetInfo struct {
	name   string
	id     int    // sheetId
	part   string // the worksheet part, "" when missing
	kind   string // the relationship type: worksheet, chartsheet...
	hidden bool   // state hidden or veryHidden
}

// xlsxName is a defined name: sheet is the index of the sheet it is
// scoped to, or -1 for the workbook.
type xlsxName struct {
	name, refersTo string
	sheet          int
}

// openXLSX opens a workbook and reads everything but its worksheets.
func openXLSX(r io.ReaderAt, size int64, lim xlsxLimits) (*xlsxBook, error) {
	p, err := openXLSXPackage(r, size, lim)
	if err != nil {
		return nil, err
	}
	bk := &xlsxBook{pkg: p}
	wb := "xl/workbook.xml"
	root, err := p.rels("")
	if err != nil {
		return nil, err
	}
	for _, rel := range root {
		if rel.typ == "officeDocument" && p.has(rel.target) {
			wb = rel.target
		}
	}
	rels, err := p.rels(wb)
	if err != nil {
		return nil, err
	}
	ids, err := bk.readWorkbook(wb)
	if err != nil {
		return nil, err
	}
	dir := path.Dir(wb) + "/"
	sst, styles := dir+"sharedStrings.xml", dir+"styles.xml"
	for _, rel := range rels {
		switch rel.typ {
		case "sharedStrings":
			sst = rel.target
		case "styles":
			styles = rel.target
		}
	}
	for i, id := range ids {
		if rel, ok := rels[id]; ok && p.has(rel.target) {
			bk.sheets[i].part, bk.sheets[i].kind = rel.target, rel.typ
		}
	}
	if err := bk.sst.read(p, sst); err != nil {
		return nil, err
	}
	if err := bk.styles.read(p, styles); err != nil {
		return nil, err
	}
	return bk, nil
}

// readWorkbook reads the workbook part: its sheets (returning their
// relationship ids), the active tab, defined names and date system.
func (bk *xlsxBook) readWorkbook(part string) (ids []string, err error) {
	x, err := bk.pkg.open(part)
	if err != nil {
		return nil, err
	}
	if x == nil {
		return nil, fmt.Errorf("not an Excel workbook: no %s", part)
	}
	defer x.close()
	activeTab, views := 0, 0
	for {
		t, err := x.next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		se, ok := t.(xml.StartElement)
		if !ok {
			continue
		}
		switch se.Name.Local {
		case "sheet":
			if len(bk.sheets) >= bk.pkg.lim.sheets {
				return nil, fmt.Errorf("the workbook has more than %d sheets: %w", bk.pkg.lim.sheets, errXLSXLimit)
			}
			name, _ := attr(se, "name")
			id, _ := attr(se, "sheetId")
			state, _ := attr(se, "state")
			n, _ := strconv.Atoi(id)
			bk.sheets = append(bk.sheets, xlsxSheetInfo{name: name, id: n, hidden: state == "hidden" || state == "veryHidden"})
			ids = append(ids, relAttr(se))
		case "workbookView":
			if views++; views == 1 {
				v, _ := attr(se, "activeTab")
				activeTab, _ = strconv.Atoi(v)
			}
		case "workbookPr":
			v, _ := attr(se, "date1904")
			bk.date1904 = v == "1" || v == "true"
		case "definedName":
			if err := bk.readName(x, se); err != nil {
				return nil, err
			}
		}
	}
	bk.active = bk.activeIndex(activeTab)
	return ids, nil
}

// activeIndex is the index of the sheet Excel showed, as excelize finds
// it: the active tab's sheet by its sheetId, else the first.
func (bk *xlsxBook) activeIndex(tab int) int {
	if len(bk.sheets) == 0 {
		return 0
	}
	id := bk.sheets[0].id
	if tab >= 0 && tab < len(bk.sheets) && bk.sheets[tab].id != 0 {
		id = bk.sheets[tab].id
	}
	for i, s := range bk.sheets {
		if s.id == id {
			return i
		}
	}
	return 0
}

func (bk *xlsxBook) readName(x *xmlStream, se xml.StartElement) error {
	if len(bk.names) >= bk.pkg.lim.styles {
		return fmt.Errorf("the workbook has more than %d names: %w", bk.pkg.lim.styles, errXLSXLimit)
	}
	n := xlsxName{sheet: -1}
	n.name, _ = attr(se, "name")
	if v, ok := attr(se, "localSheetId"); ok {
		if id, err := strconv.Atoi(v); err == nil && id >= 0 {
			n.sheet = id
		}
	}
	text, err := x.text(nil)
	n.refersTo = string(text)
	bk.names = append(bk.names, n)
	return err
}

// sharedStrings holds the workbook's shared strings end to end in one
// buffer, so a table of many short strings costs its text and four
// bytes a string rather than a string header and allocation each.
type sharedStrings struct {
	buf  []byte
	ends []uint32
}

func (s *sharedStrings) get(i int) (string, bool) {
	if i < 0 || i >= len(s.ends) {
		return "", false
	}
	from := uint32(0)
	if i > 0 {
		from = s.ends[i-1]
	}
	return string(s.buf[from:s.ends[i]]), true
}

// read streams the shared strings part: each <si> is its <t> text, or
// the text of its rich-text runs (<r><t>), leaving out phonetic hints.
func (s *sharedStrings) read(p *xlsxPackage, part string) error {
	x, err := p.open(part)
	if x == nil || err != nil {
		return err
	}
	defer x.close()
	var item []byte
	for {
		t, err := x.next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		se, ok := t.(xml.StartElement)
		if !ok || se.Name.Local != "si" {
			continue
		}
		if item, err = inlineText(x, item[:0]); err != nil {
			return err
		}
		if len(s.ends) >= p.lim.strings || len(s.buf)+len(item) > 1<<32-1 {
			return fmt.Errorf("the workbook has more than %d shared strings: %w", p.lim.strings, errXLSXLimit)
		}
		s.buf = append(s.buf, unescapeOOXML(item)...)
		s.ends = append(s.ends, uint32(len(s.buf)))
	}
}

// inlineText reads the rest of an <si> or <is> element: the text of its
// <t> children and of the <t> in its <r> runs, not in <rPh> hints.
func inlineText(x *xmlStream, buf []byte) ([]byte, error) {
	depth := x.depth
	inT, skipDepth := false, 0
	for {
		t, err := x.next()
		if err != nil {
			return buf, eofAsUnexpected(err)
		}
		switch t := t.(type) {
		case xml.StartElement:
			switch {
			case skipDepth > 0:
			case t.Name.Local == "rPh" || t.Name.Local == "phoneticPr":
				skipDepth = x.depth
			case t.Name.Local == "t":
				inT = true
			}
		case xml.EndElement:
			if x.depth < depth {
				return buf, nil
			}
			if skipDepth > 0 && x.depth < skipDepth {
				skipDepth = 0
			}
			inT = false
		case xml.CharData:
			if inT && skipDepth == 0 {
				buf = append(buf, t...)
			}
		}
	}
}

// unescapeOOXML decodes _xHHHH_, how OOXML writes characters XML can't
// hold (_x000D_ for a carriage return), and _x005F_, an escaped _.
func unescapeOOXML(b []byte) []byte {
	if !bytes.Contains(b, []byte("_x")) {
		return b
	}
	out := make([]byte, 0, len(b))
	for i := 0; i < len(b); {
		if r, ok := ooxmlEscape(b[i:]); ok {
			if r == '_' {
				out = append(out, '_')
			} else {
				out = append(out, string(r)...)
			}
			i += 7
			continue
		}
		out = append(out, b[i])
		i++
	}
	return out
}

// ooxmlEscape decodes the _xHHHH_ at the start of b.
func ooxmlEscape(b []byte) (rune, bool) {
	if len(b) < 7 || b[0] != '_' || b[1] != 'x' || b[6] != '_' {
		return 0, false
	}
	v, err := strconv.ParseUint(string(b[2:6]), 16, 16)
	if err != nil {
		return 0, false
	}
	return rune(v), true
}
