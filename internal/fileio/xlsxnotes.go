package fileio

import (
	"archive/zip"
	"encoding/xml"
	"fmt"
	"io"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// Cell notes travel as Excel's notes (legacy comments, which Sheets
// imports as notes and writes its notes as): a comments part per sheet
// with the text, and the VML drawing Excel needs to show each note's
// box, both related to the worksheet, which names the drawing in
// <legacyDrawing>.

const (
	commentsType = mlType + "comments+xml"
	vmlType      = "application/vnd.openxmlformats-officedocument.vmlDrawing"
	notesRelID   = "rIdNotes"
	vmlRelID     = "rIdVML"
)

// writeNotes writes worksheet part n's notes: the comments part and the
// VML drawing, which notesRels relate to it.
func writeNotes(zw *zip.Writer, n int, notes map[sheet.Addr]string) error {
	num := strconv.Itoa(n)
	addrs := slices.SortedFunc(maps.Keys(notes), func(a, b sheet.Addr) int {
		if a.Row != b.Row {
			return a.Row - b.Row
		}
		return a.Col - b.Col
	})
	var comments, vml strings.Builder
	comments.WriteString(xmlHead + `<comments xmlns="` + sheetMain + `"><authors><author></author></authors><commentList>`)
	fmt.Fprintf(&vml, `<xml xmlns:v="urn:schemas-microsoft-com:vml" xmlns:o="urn:schemas-microsoft-com:office:office" xmlns:x="urn:schemas-microsoft-com:office:excel">`+
		`<o:shapelayout v:ext="edit"><o:idmap v:ext="edit" data="%d"/></o:shapelayout>`+
		`<v:shapetype id="_x0000_t202" coordsize="21600,21600" o:spt="202" path="m,l,21600r21600,l21600,xe">`+
		`<v:stroke joinstyle="miter"/><v:path gradientshapeok="t" o:connecttype="rect"/></v:shapetype>`, n)
	for k, a := range addrs {
		comments.WriteString(`<comment ref="` + a.String() + `" authorId="0"><text><t xml:space="preserve">`)
		comments.Write(appendEscaped(nil, notes[a], true))
		comments.WriteString(`</t></text></comment>`)
		fmt.Fprintf(&vml, `<v:shape id="_x0000_s%d" type="#_x0000_t202" style="position:absolute;margin-left:60pt;margin-top:2pt;width:108pt;height:60pt;z-index:%d;visibility:hidden" fillcolor="#ffffe1" o:insetmode="auto">`+
			`<v:fill color2="#ffffe1"/><v:shadow on="t" color="black" obscured="t"/><v:path o:connecttype="none"/>`+
			`<v:textbox style="mso-direction-alt:auto"><div style="text-align:left"></div></v:textbox>`+
			`<x:ClientData ObjectType="Note"><x:MoveWithCells/><x:SizeWithCells/><x:Anchor>%d, 15, %d, 2, %d, 15, %d, 4</x:Anchor>`+
			`<x:AutoFill>False</x:AutoFill><x:Row>%d</x:Row><x:Column>%d</x:Column></x:ClientData></v:shape>`,
			n*1024+k+1, k+1, a.Col+1, a.Row, a.Col+3, a.Row+4, a.Row, a.Col)
	}
	comments.WriteString(`</commentList></comments>`)
	vml.WriteString(`</xml>`)
	for _, p := range []struct{ name, body string }{
		{"xl/comments" + num + ".xml", comments.String()},
		{"xl/drawings/vmlDrawing" + num + ".vml", vml.String()},
	} {
		if err := writePart(zw, p.name, p.body); err != nil {
			return err
		}
	}
	return nil
}

// notesRels are the relationships from worksheet part n to its notes'
// parts.
func notesRels(n int) []string {
	num := strconv.Itoa(n)
	return []string{
		`<Relationship Id="` + notesRelID + `" Type="` + officeRel + `/comments" Target="../comments` + num + `.xml"/>`,
		`<Relationship Id="` + vmlRelID + `" Type="` + officeRel + `/vmlDrawing" Target="../drawings/vmlDrawing` + num + `.vml"/>`,
	}
}

// notesTypes are the content types of the notes of the worksheet parts
// numbered in withNotes.
func notesTypes(withNotes []int) string {
	if len(withNotes) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(`<Default Extension="vml" ContentType="` + vmlType + `"/>`)
	for _, n := range withNotes {
		fmt.Fprintf(&b, `<Override PartName="/xl/comments%d.xml" ContentType="%s"/>`, n, commentsType)
	}
	return b.String()
}

// readNotes reads the notes of worksheet part, from the comments part
// its relationships name, into s.
func (bk *xlsxBook) readNotes(s *sheet.Sheet, part string) error {
	rels, err := bk.pkg.rels(part)
	if err != nil {
		return err
	}
	for _, rel := range rels {
		if rel.typ == "comments" && bk.pkg.has(rel.target) {
			if err := bk.readComments(s, rel.target); err != nil {
				return fmt.Errorf("notes: %w", err)
			}
		}
	}
	return nil
}

// readComments reads a comments part: each <comment ref="B2"> with the
// text of its runs.
func (bk *xlsxBook) readComments(s *sheet.Sheet, part string) error {
	x, err := bk.pkg.open(part)
	if x == nil || err != nil {
		return err
	}
	defer x.close()
	var buf []byte
	for {
		t, err := x.next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		se, ok := t.(xml.StartElement)
		if !ok || se.Name.Local != "comment" {
			continue
		}
		ref, _ := attr(se, "ref")
		if buf, err = inlineText(x, buf[:0]); err != nil {
			return err
		}
		if a, ok := sheet.ParseAddr(ref); ok {
			s.LoadNote(a, string(unescapeOOXML(buf)))
		}
	}
}
