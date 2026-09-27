package fileio

import (
	"archive/zip"
	"bufio"
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/FelineStateMachine/012/internal/formula"
	"github.com/FelineStateMachine/012/internal/sheet"
)

// XLSX files are read by xlsximport.go (on xlsxpkg.go, xlsxbook.go,
// xlsxstyles.go and xlsxsheet.go) and written here and in xlsxwrite.go,
// all with archive/zip and encoding/xml's escaping.

// Column widths: Excel measures in characters of its default font, 012
// in terminal columns including one of padding.
const (
	excelDefaultWidth = 8.43
	excelPadding      = 1
)

func clamp(v, lo, hi int) int { return max(lo, min(v, hi)) }

// xlsxStyle is what 012 keeps of an Excel cell style.
type xlsxStyle struct {
	format sheet.Format
	style  sheet.Style
}

// exportXLSX writes a workbook: the snapshot's sheets (or just the
// snapshot), each with values, formulas in Excel's syntax with their
// results cached, number formats, text styles, alignment and column
// widths, and the named ranges. Excel recalculates it on opening.
func exportXLSX(_ context.Context, name string, snap *Snapshot, _ ExportOptions) (*ExportResult, error) {
	sheets := snap.Sheets
	if len(sheets) == 0 {
		sheets = []*Snapshot{snap}
	}
	w := &xlsxWriter{styles: newXLSXStyleTable(), multi: len(sheets) > 1, known: map[string]bool{}, renamed: map[string]string{}}
	res := &ExportResult{}
	names, hidden, active := make([]string, len(sheets)), make([]bool, len(sheets)), 0
	used := map[string]bool{}
	for i, sn := range sheets {
		names[i] = uniqueSheetName(sheetName(sn.Name), used)
		if sn == snap {
			active = i
		}
		hidden[i] = sn.Hidden
		// A sheet renamed to suit Excel is named so in formulas too.
		key := formula.SheetKey(sn.Name)
		w.known[key] = true
		if names[i] != sn.Name {
			w.renamed[key] = names[i]
		}
	}
	hidden[active] = false
	err := writeFile(name, func(out io.Writer) error {
		zw := zip.NewWriter(out)
		var withNotes []int
		for i, sn := range sheets {
			rows, err := w.writeSheet(zw, i, names[i], sn, i == active)
			if err != nil {
				return err
			}
			res.Rows += rows
			if len(sn.Notes) > 0 {
				withNotes = append(withNotes, i+1)
				if err := writeNotes(zw, i+1, sn.Notes); err != nil {
					return err
				}
			}
		}
		if err := writePart(zw, "xl/styles.xml", w.styles.xml()); err != nil {
			return err
		}
		if err := writePackage(zw, names, hidden, active, w.definedNames(snap.Names), filterRanges(sheets, names), notesTypes(withNotes)); err != nil {
			return err
		}
		return zw.Close()
	})
	if err != nil {
		return nil, err
	}
	if w.values.n > 0 {
		res.Notes = append(res.Notes, fmt.Sprintf("%s with no Excel equivalent saved as values, e.g. %s",
			count(w.values.n, "formula", "formulas"), w.values.example))
	}
	if w.missing.n > 0 {
		res.Notes = append(res.Notes, fmt.Sprintf("%s naming a sheet that doesn't exist saved as values, e.g. %s (%s)",
			count(w.missing.n, "formula", "formulas"), w.missing.example, sheet.QuoteSheet(w.missingSheet)))
	}
	return res, nil
}

// definedNames are the named ranges as Excel's defined names hold them,
// with the names their sheets are written under: Q3!$B$2:$B$9.
func (w *xlsxWriter) definedNames(names []SnapName) [][2]string {
	out := make([][2]string, len(names))
	for i, n := range names {
		ws := n.Sheet
		if to, ok := w.renamed[formula.SheetKey(ws)]; ok {
			ws = to
		}
		out[i] = [2]string{n.Name, excelRange(ws, n.Range)}
	}
	return out
}

// uniqueSheetName is name, or name with a number when another sheet
// already took it (Excel's names ignore case).
func uniqueSheetName(name string, used map[string]bool) string {
	out := name
	for n := 2; used[strings.ToLower(out)]; n++ {
		suffix := " (" + strconv.Itoa(n) + ")"
		r := []rune(name)
		out = string(r[:min(len(r), 31-len(suffix))]) + suffix
	}
	used[strings.ToLower(out)] = true
	return out
}

const (
	xmlHead   = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n"
	relsNS    = `http://schemas.openxmlformats.org/package/2006/relationships`
	officeRel = `http://schemas.openxmlformats.org/officeDocument/2006/relationships`
	sheetMain = `http://schemas.openxmlformats.org/spreadsheetml/2006/main`
	mlType    = `application/vnd.openxmlformats-officedocument.spreadsheetml.`
)

// filterRanges is each sheet's filter range as Excel names it, with its
// sheet (_xlnm._FilterDatabase), or "" for a sheet without a filter.
func filterRanges(sheets []*Snapshot, names []string) []string {
	out := make([]string, len(sheets))
	for i, sn := range sheets {
		if sn.Filter != nil {
			out[i] = excelRange(names[i], sn.Filter.Range)
		}
	}
	return out
}

// writePackage writes the parts around the worksheets and styles: the
// workbook with its sheets, active tab, names (with each filter's range,
// filters[i] for sheet i) and calculation settings, the relationships
// and the content types, with extra ones for the sheets' parts.
func writePackage(zw *zip.Writer, names []string, hidden []bool, active int, defined [][2]string, filters []string, extraTypes string) error {
	var types, rels, book strings.Builder
	types.WriteString(xmlHead + `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">` +
		`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>` +
		`<Default Extension="xml" ContentType="application/xml"/>` +
		`<Override PartName="/xl/workbook.xml" ContentType="` + mlType + `sheet.main+xml"/>` +
		`<Override PartName="/xl/styles.xml" ContentType="` + mlType + `styles+xml"/>` + extraTypes)
	rels.WriteString(xmlHead + `<Relationships xmlns="` + relsNS + `">`)
	book.WriteString(xmlHead + `<workbook xmlns="` + sheetMain + `" xmlns:r="` + officeRel + `">` +
		`<bookViews><workbookView activeTab="` + strconv.Itoa(active) + `"/></bookViews><sheets>`)
	for i, ws := range names {
		n := strconv.Itoa(i + 1)
		fmt.Fprintf(&types, `<Override PartName="/xl/worksheets/sheet%s.xml" ContentType="%sworksheet+xml"/>`, n, mlType)
		fmt.Fprintf(&rels, `<Relationship Id="rId%s" Type="%s/worksheet" Target="worksheets/sheet%s.xml"/>`, n, officeRel, n)
		state := ""
		if hidden[i] {
			state = ` state="hidden"`
		}
		fmt.Fprintf(&book, `<sheet name="%s" sheetId="%s"%s r:id="rId%s"/>`, escapeXML(ws, true), n, state, n)
	}
	fmt.Fprintf(&rels, `<Relationship Id="rId%d" Type="%s/styles" Target="styles.xml"/></Relationships>`, len(names)+1, officeRel)
	types.WriteString(`</Types>`)
	book.WriteString(`</sheets>`)
	var dn strings.Builder
	for i, r := range filters {
		if r != "" {
			fmt.Fprintf(&dn, `<definedName name="_xlnm._FilterDatabase" localSheetId="%d" hidden="1">%s</definedName>`, i, escapeXML(r, false))
		}
	}
	for _, n := range defined {
		fmt.Fprintf(&dn, `<definedName name="%s">%s</definedName>`, escapeXML(n[0], true), escapeXML(n[1], false))
	}
	if dn.Len() > 0 {
		book.WriteString(`<definedNames>` + dn.String() + `</definedNames>`)
	}
	book.WriteString(`<calcPr calcId="191029" fullCalcOnLoad="1"/></workbook>`)
	for _, p := range []struct{ name, body string }{
		{"[Content_Types].xml", types.String()},
		{"_rels/.rels", xmlHead + `<Relationships xmlns="` + relsNS + `"><Relationship Id="rId1" Type="` + officeRel + `/officeDocument" Target="xl/workbook.xml"/></Relationships>`},
		{"xl/workbook.xml", book.String()},
		{"xl/_rels/workbook.xml.rels", rels.String()},
	} {
		if err := writePart(zw, p.name, p.body); err != nil {
			return err
		}
	}
	return nil
}

func writePart(zw *zip.Writer, name, body string) error {
	w, err := zw.Create(name)
	if err != nil {
		return err
	}
	_, err = io.WriteString(w, body)
	return err
}

// writeSheet writes snapshot sn as worksheet part i and returns the rows
// written.
func (w *xlsxWriter) writeSheet(zw *zip.Writer, i int, ws string, sn *Snapshot, active bool) (int, error) {
	part, err := zw.Create("xl/worksheets/sheet" + strconv.Itoa(i+1) + ".xml")
	if err != nil {
		return 0, err
	}
	bw := bufio.NewWriterSize(part, 64<<10)
	rows := w.sheet(bw, ws, sn, active)
	return rows, bw.Flush()
}

// sheetName makes a valid Excel sheet name: at most 31 characters, none
// of : \ / ? * [ ].
func sheetName(s string) string {
	s = strings.Map(func(r rune) rune {
		if strings.ContainsRune(`:\/?*[]`, r) {
			return '_'
		}
		return r
	}, s)
	s = strings.Trim(strings.TrimSpace(s), "'")
	if r := []rune(s); len(r) > 31 {
		s = string(r[:31])
	}
	if s == "" {
		return "Sheet1"
	}
	return s
}
