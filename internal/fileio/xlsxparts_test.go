package fileio

import (
	"archive/zip"
	"bytes"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// Hand-written SpreadsheetML for tests of the XLSX reader: what Excel
// writes and excelize doesn't (shared formulas, inline and rich text,
// row and column styles, cells without references).

const (
	contentTypes = `<?xml version="1.0" encoding="UTF-8"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/><Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/><Override PartName="/xl/worksheets/sheet2.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/><Override PartName="/xl/worksheets/sheet3.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/><Override PartName="/xl/sharedStrings.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sharedStrings+xml"/><Override PartName="/xl/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.styles+xml"/></Types>`
	rootRels     = `<?xml version="1.0" encoding="UTF-8"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/></Relationships>`
	bookRels     = `<?xml version="1.0" encoding="UTF-8"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/><Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="/xl/worksheets/sheet2.xml"/><Relationship Id="rId3" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet3.xml"/><Relationship Id="rId4" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/sharedStrings" Target="sharedStrings.xml"/><Relationship Id="rId5" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/></Relationships>`
	mainNS       = `xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"`

	kitchenBook = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<workbook ` + mainNS + `><workbookPr defaultThemeVersion="124226"/><bookViews><workbookView activeTab="2"/></bookViews><sheets><sheet name="Data" sheetId="1" r:id="rId1"/><sheet name="Hidden" sheetId="2" state="hidden" r:id="rId2"/><sheet name="Refs" sheetId="5" r:id="rId3"/></sheets><definedNames><definedName name="_xlnm.Print_Area" localSheetId="0">Data!$A$1:$F$8</definedName><definedName name="Total">Data!$A$2:$B$2</definedName><definedName name="Local" localSheetId="0">Data!$A$1</definedName><definedName name="Const">42</definedName></definedNames><calcPr calcId="152511"/></workbook>`

	kitchenStrings = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<sst ` + mainNS + ` count="6" uniqueCount="6"><si><t>Region</t></si><si><r><rPr><b/></rPr><t xml:space="preserve">Rich </t></r><r><t>text</t></r><rPh sb="0" eb="1"><t>hint</t></rPh></si><si><t>line_x000D_break _x005F_x0041_ tab&#9;end</t></si><si><t>=not a formula</t></si><si><t>123</t></si><si><t/></si></sst>`

	kitchenStyles = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<styleSheet ` + mainNS + `><numFmts count="2"><numFmt numFmtId="164" formatCode="0.000"/><numFmt numFmtId="165" formatCode="yyyy\-mm\-dd"/></numFmts><fonts count="5"><font><sz val="11"/></font><font><b/><sz val="11"/></font><font><i/><u/></font><font><strike/><u val="none"/></font><font><b val="0"/></font></fonts><cellStyleXfs count="1"><xf numFmtId="0" fontId="1"/></cellStyleXfs><cellXfs count="9"><xf numFmtId="0" fontId="0"/><xf numFmtId="4" fontId="1" applyFont="1" applyNumberFormat="1"/><xf numFmtId="164" fontId="0"/><xf numFmtId="165" fontId="0"/><xf numFmtId="0" fontId="2"><alignment horizontal="center"/></xf><xf numFmtId="10" fontId="3" applyAlignment="0"><alignment horizontal="right"/></xf><xf numFmtId="5" fontId="0"/><xf numFmtId="0" fontId="4"><alignment horizontal="centerContinuous"/></xf><xf numFmtId="14" fontId="1" applyFont="0"><alignment horizontal="left"/></xf></cellXfs></styleSheet>`

	kitchenData = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<worksheet ` + mainNS + `><dimension ref="A1:F9"/><sheetFormatPr defaultColWidth="10" defaultRowHeight="15"/><cols><col min="1" max="1" width="20" customWidth="1"/><col min="3" max="5" width="5" style="2" customWidth="1"/><col min="6" max="6" style="4"/><col min="5" max="5" width="30"/></cols><sheetData>` +
		`<row r="1"><c r="A1" t="s"><v>0</v></c><c r="B1" t="s"><v>1</v></c><c r="C1" t="inlineStr"><is><t>inline</t></is></c><c r="D1" t="b"><v>1</v></c><c r="E1" t="e"><v>#N/A</v></c><c r="F1" t="str"><f>A1&amp;"x"</f><v>Regionx</v></c></row>` +
		`<row r="2" s="3" customFormat="1"><c><v>1.5</v></c><c s="2"><v>2</v></c><c r="E2" s="8"><v>45000</v></c></row>` +
		`<row r="3"><c r="A3"><f>SUM(A2:B2)</f><v>3.5</v></c><c r="B3"><f t="shared" ref="B3:D4" si="0">A3*2+$A$2+A$2+$A2+SUM(A1:A2)+"A1"</f><v>1</v></c><c r="C3"><f t="shared" si="0"/><v>2</v></c><c r="D3"><f t="shared" si="0"/></c></row>` +
		`<row r="4"><c r="B4"><f t="shared" si="0"/><v>4</v></c><c r="C4"><f t="shared" si="7"/><v>9</v></c></row>` +
		`<row r="5"><c r="A5" s="1"><v>10</v></c><c r="E5" s="5" t="s"><v>3</v></c><c r="F5" s="4"/></row>` +
		`<row r="6"><c r="A6" s="7"/><c r="B6" t="s"><v> 4 </v></c><c r="C6" s="6"><v>12</v></c></row>` +
		`<row r="7"><c r="A7"><f>CUBEVALUE(1)</f><v>7</v></c><c r="B7" t="str"><f>_xlfn.CONCAT(A1,"a")</f><v>Regiona</v></c><c r="C7" t="b"><f>CUBESET(1)</f><v>0</v></c><c r="D7" t="e"><f>CUBEMEMBER(1)</f><v>#REF!</v></c><c r="E7" t="s"><f>CUBEKPI(1)</f><v>0</v></c></row>` +
		`<row r="8"><c r="A8" t="n"><v>1E+300</v></c><c r="B8"><v> 12</v></c><c r="C8"><v>abc</v></c><c r="D8" t="s"><v>99</v></c><c r="E8"><v>5</v></c><c r="F8" t="s"><v>5</v></c></row>` +
		`<row r="9"><c r="B9" t="inlineStr"><is><r><t>in</t></r><r><t>_x0041_</t></r></is></c><c r="C9" t="b"><v>TRUE</v></c><c r="D9" s="8"><v>30</v></c><c r="E9" t="x"><v>8</v></c></row>` +
		`</sheetData><mergeCells count="1"><mergeCell ref="E3:F3"/></mergeCells></worksheet>`

	kitchenHidden = `<?xml version="1.0" encoding="UTF-8"?><worksheet ` + mainNS + `><sheetData><row><c><v>1</v></c></row><row><c/><c t="s"><v>0</v></c></row></sheetData></worksheet>`

	kitchenRefs = `<?xml version="1.0" encoding="UTF-8"?><worksheet ` + mainNS + `><dimension ref="A1"/><sheetFormatPr baseColWidth="12"/><sheetData><row r="1"><c r="A1"><v>42</v></c><c r="B1"><f>Data!A5*2+SUM(Total)</f><v>84</v></c><c r="C1"><f>SUM(Hidden!A1:A2)</f></c></row></sheetData></worksheet>`
)

// kitchenParts is a workbook of three sheets using most of what the
// reader reads.
func kitchenParts() map[string]string {
	return map[string]string{
		"[Content_Types].xml":        contentTypes,
		"_rels/.rels":                rootRels,
		"xl/workbook.xml":            kitchenBook,
		"xl/_rels/workbook.xml.rels": bookRels,
		"xl/sharedStrings.xml":       kitchenStrings,
		"xl/styles.xml":              kitchenStyles,
		"xl/worksheets/sheet1.xml":   kitchenData,
		"xl/worksheets/sheet2.xml":   kitchenHidden,
		"xl/worksheets/sheet3.xml":   kitchenRefs,
	}
}

// oneSheetParts is a workbook of one sheet with the given sheetData
// content, and the workbook's properties.
func oneSheetParts(workbookPr, sheetData string) map[string]string {
	p := kitchenParts()
	p["xl/workbook.xml"] = `<workbook ` + mainNS + `>` + workbookPr + `<sheets><sheet name="S" sheetId="1" r:id="rId1"/></sheets></workbook>`
	p["xl/worksheets/sheet1.xml"] = `<worksheet ` + mainNS + `><sheetData>` + sheetData + `</sheetData></worksheet>`
	delete(p, "xl/worksheets/sheet2.xml")
	delete(p, "xl/worksheets/sheet3.xml")
	return p
}

// zipParts packs parts into an XLSX file's bytes, in name order.
func zipParts(t testing.TB, parts map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, name := range slices.Sorted(maps.Keys(parts)) {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(parts[name])); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// writeParts writes parts as an XLSX file in dir and returns its path.
func writeParts(t testing.TB, dir, name string, parts map[string]string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, zipParts(t, parts), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
