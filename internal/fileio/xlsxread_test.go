package fileio

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// readParts imports a workbook of parts with the given limits.
func readParts(t testing.TB, parts map[string]string, lim xlsxLimits) (*Result, error) {
	t.Helper()
	data := zipParts(t, parts)
	bk, err := openXLSX(bytes.NewReader(data), int64(len(data)), lim)
	if err != nil {
		return nil, err
	}
	return bk.importBook(context.Background(), Options{})
}

func TestXLSXReader(t *testing.T) {
	res, err := readParts(t, kitchenParts(), defaultXLSXLimits)
	if err != nil {
		t.Fatal(err)
	}
	book := res.Sheet.Book()
	if book.Len() != 3 || res.Sheet.Name() != "Refs" || res.Rows != 12 {
		t.Fatalf("%d sheets, showing %s, %d rows", book.Len(), res.Sheet.Name(), res.Rows)
	}
	data := book.Sheet(0)
	for cell, want := range map[string]string{
		"A1": "Region", "B1": "Rich text", "C1": "inline", "D1": "TRUE", "E1": "#N/A", "F1": `=A1&"x"`,
		"A3": "=SUM(A2:B2)",
		"B3": `=A3*2+$A$2+A$2+$A2+SUM(A1:A2)+"A1"`,
		"C3": `=B3*2+$A$2+B$2+$A2+SUM(B1:B2)+"A1"`,
		"D3": `=C3*2+$A$2+C$2+$A2+SUM(C1:C2)+"A1"`,
		"B4": `=A4*2+$A$2+A$2+$A3+SUM(A2:A3)+"A1"`,
		"C4": "9", "E5": "'=not a formula", "B6": "'123", "C6": "12",
		"A7": "7", "B7": `=CONCAT(A1,"a")`, "C7": "FALSE", "D7": "#REF!", "E7": "Region",
		"A8": "1E+300", "B8": "' 12", "C8": "abc", "D8": "", "E8": "5",
		"B9": "inA", "C9": "TRUE", "E9": "8",
	} {
		if g := input(data, addr(t, cell)); g != want {
			t.Errorf("Data!%s input %q, want %q", cell, g, want)
		}
	}
	if g := input(data, addr(t, "A1")); g != "Region" {
		t.Errorf("A1 %q", g)
	}
	for _, cell := range []string{"C2", "D8", "A6"} {
		if data.Cell(addr(t, cell)) == nil {
			t.Errorf("formatted blank %s left out", cell)
		}
	}
	for _, cell := range []string{"F5", "F8"} {
		if c := data.Cell(addr(t, cell)); c != nil {
			t.Errorf("%s kept: %+v", cell, c)
		}
	}
	checkKitchenStyles(t, data)
	for col, want := range map[int]int{0: 21, 1: 11, 2: 6, 3: 6, 4: 31, 5: 11, 200: 11} {
		if w := data.ColWidth(col); w != want {
			t.Errorf("Data column %d width %d, want %d", col, w, want)
		}
	}
	if w := book.Sheet(2).ColWidth(7); w != 13 {
		t.Errorf("Refs width %d", w)
	}
	if !book.Sheet(1).Hidden() || res.Sheet.Hidden() {
		t.Errorf("Hidden hidden %v, Refs hidden %v", book.Sheet(1).Hidden(), res.Sheet.Hidden())
	}
	if g := input(book.Sheet(1), addr(t, "B2")); g != "Region" {
		t.Errorf("Hidden!B2 %q", g)
	}
	if g := shown(book.Sheet(2), addr(t, "B1")); g != "24.50" {
		t.Errorf("Refs!B1 shows %q", g) // Data!A5*2 + SUM(Total), A2 a date: 2.5
	}
	notes := strings.Join(res.Notes, "; ")
	for _, want := range []string{
		"2 named ranges left out, e.g. Local",
		"4 formulas kept as values, e.g. A7 =CUBEVALUE(1)",
	} {
		if !strings.Contains(notes, want) {
			t.Errorf("notes %q, want %q", notes, want)
		}
	}
}

func checkKitchenStyles(t *testing.T, data *sheet.Sheet) {
	t.Helper()
	cell := func(s string) sheet.Cell {
		if c := data.Cell(addr(t, s)); c != nil {
			return *c
		}
		return sheet.Cell{}
	}
	if f := cell("A2").Format; f.Kind != sheet.FmtDate { // the row's style
		t.Errorf("A2 format %+v", f)
	}
	if f := cell("B2").Format; f != (sheet.Format{Kind: sheet.FmtCustom, Pattern: "0.000"}) {
		t.Errorf("B2 format %+v", f)
	}
	if c := cell("E2"); c.Format.Kind != sheet.FmtDate || c.Style.Bold || c.Style.Align != sheet.AlignLeft {
		t.Errorf("E2 %+v %+v (applyFont=0 isn't bold)", c.Format, c.Style)
	}
	if c := cell("A5"); !c.Style.Bold || c.Format != (sheet.Format{Kind: sheet.FmtNumber, Decimals: 2}) {
		t.Errorf("A5 %+v %+v", c.Format, c.Style)
	}
	if st := cell("E5").Style; !st.Strikethrough || st.Underline || st.Align != sheet.AlignAuto {
		t.Errorf("E5 style %+v", st)
	}
	if st := cell("A6").Style; st.Bold || st.Align != sheet.AlignCenter {
		t.Errorf("A6 style %+v", st)
	}
	if f := cell("C6").Format; !f.IsZero() {
		t.Errorf("C6 format %+v (id 5 is no built-in to excelize)", f)
	}
	if c := cell("D9"); c.Format.Kind != sheet.FmtDate || shown(data, addr(t, "D9")) != "1/30/1900" {
		t.Errorf("D9 %+v shows %s", c.Format, shown(data, addr(t, "D9")))
	}
}

func TestShiftFormula(t *testing.T) {
	for _, tc := range []struct {
		in         string
		dCol, dRow int
		want       string
	}{
		{"A1+B2", 1, 1, "B2+C3"},
		{"$A$1+$A1+A$1", 2, 3, "$A$1+$A4+C$1"},
		{"SUM(A1:B2)*LOG10(A1)", 0, 1, "SUM(A2:B3)*LOG10(A2)"},
		{"Sheet2!A1+'My sheet'!B$2", 1, 1, "Sheet2!B2+'My sheet'!C$2"},
		{"SUM(A:A,1:1,$B:$B,$2:$2)", 1, 1, "SUM(B:B,2:2,$B:$B,$2:$2)"},
		{`"A1"&A1&Total&TRUE&1E+5&0.5`, 1, 0, `"A1"&B1&Total&TRUE&1E+5&0.5`},
		{"A1-1", -1, 0, "#REF!-1"},
		{"XFD1+A1048576", 1, 1, "#REF!+#REF!"},
		{"Table1[[#This Row],[A1]]+A1", 1, 0, "Table1[[#This Row],[A1]]+B1"},
		{"_xlfn.XLOOKUP(A1,B:B,C:C)", 0, 5, "_xlfn.XLOOKUP(A6,B:B,C:C)"},
		{"#REF!+A1", 0, 1, "#REF!+A2"},
		{"A1 B1", 25, 0, "Z1 AA1"},
		{"ZZ1", 1, 0, "AAA1"},
	} {
		if got := shiftFormula(tc.in, tc.dCol, tc.dRow); got != tc.want {
			t.Errorf("shiftFormula(%q, %d, %d) = %q, want %q", tc.in, tc.dCol, tc.dRow, got, tc.want)
		}
	}
}

// Things excelize reads wrongly or not at all: shared formulas referring
// to another sheet move, the 1904 date system, t="d" ISO dates, and
// columns past 1-2-3's IV.
func TestXLSXBeyondExcelize(t *testing.T) {
	parts := oneSheetParts(`<workbookPr date1904="1"/>`,
		`<row r="1"><c r="A1"><v>1</v></c><c r="B1"><f t="shared" ref="B1:B2" si="0">Other!A1+A1</f></c><c r="C1" s="8"><v>0</v></c><c r="D1" t="d"><v>2026-09-26T14:30:00</v></c><c r="E1" t="d"><v>2026-09-26</v></c><c r="F1" t="d"><v>soon</v></c><c r="IZ1"><v>1</v></c></row>`+
			`<row r="2"><c r="B2"><f t="shared" si="0"/></c></row>`)
	res, err := readParts(t, parts, defaultXLSXLimits)
	if err != nil {
		t.Fatal(err)
	}
	s := res.Sheet
	if g := input(s, addr(t, "B2")); g != "=Other!A2+A2" {
		t.Errorf("B2 %q", g)
	}
	for cell, want := range map[string]string{"C1": "1/1/1904", "D1": "9/26/2026 14:30:00", "E1": "9/26/2026", "F1": "soon"} {
		if g := shown(s, addr(t, cell)); g != want {
			t.Errorf("%s shows %q, want %q", cell, g, want)
		}
	}
	if g := input(s, addr(t, "IZ1")); g != "1" || len(res.Notes) != 0 { // past 1-2-3's IV, inside the grid
		t.Errorf("IZ1 %q, notes %q", g, res.Notes)
	}
}

func TestXLSXLimits(t *testing.T) {
	small := defaultXLSXLimits
	small.part, small.total, small.slack, small.token, small.ratio, small.entries, small.shared = 1<<20, 2<<20, 64<<10, 64<<10, 100, 50, 1<<20
	bomb := oneSheetParts("", strings.Repeat(`<row><c><v>0</v></c></row>`, 100_000))
	var rows, text strings.Builder
	for i := range 60_000 {
		rows.WriteString(`<row><c><v>` + itoa(i*7919%100_003) + `</v></c></row>`)
		text.WriteString(itoa(i * 7919 % 100_003))
	}
	big := oneSheetParts("", rows.String())
	long := oneSheetParts("", `<row><c t="inlineStr"><is><t>`+text.String()+`</t></is></c></row>`)
	deep := oneSheetParts("", `<row><c><v>1</v><x>`+strings.Repeat("<y>", 300)+strings.Repeat("</y>", 300)+`</x></c></row>`)
	many := kitchenParts()
	for i := range small.entries {
		many["xl/media/image"+itoa(i)+".png"] = ""
	}
	for name, tc := range map[string]struct {
		parts map[string]string
		want  string
		ratio int64 // instead of small.ratio
	}{
		"zip bomb":       {bomb, "compressed over 100 to 1, like a zip bomb", 0},
		"big part":       {big, "unpacks to more than 1 MB", 0},
		"long text":      {long, "a tag or text over", 0},
		"deep":           {deep, "nests elements more than 256 deep", 0},
		"many files":     {many, "files, more than 50", 0},
		"row past Excel": {oneSheetParts("", `<row r="1048577"><c><v>1</v></c></row>`), "past Excel's last row", 0},
		"col past Excel": {oneSheetParts("", `<row><c r="XFE1"><v>1</v></c></row>`), `"XFE1" isn't a cell reference`, 0},
		"bad reference":  {oneSheetParts("", `<row><c r="A0"><v>1</v></c></row>`), `"A0" isn't a cell reference`, 0},
		"too many cells": {oneSheetParts("", `<row>`+strings.Repeat(`<c r="A1"/>`, excelCols+1)+`</row>`), "more than 16,384 cells", 1 << 30},
		"unsafe name":    {mergeParts(kitchenParts(), "../evil.xml", "x"), "unsafe name", 0},
		"absolute name":  {mergeParts(kitchenParts(), "/etc/passwd", "x"), "unsafe name", 0},
		"duplicate":      {mergeParts(kitchenParts(), "XL/Workbook.xml", "x"), "twice", 0},
		"entity":         {oneSheetParts("", `<row><c t="inlineStr"><is><t>&bomb;</t></is></c></row>`), "invalid character entity &bomb;", 0},
		"shared formulas": {oneSheetParts("", `<row r="1"><c r="A1"><f t="shared" ref="A1:A9" si="0">`+strings.Repeat("B1+", 1000)+`1</f></c></row>`+
			strings.Repeat(`<row><c r="A2"><f t="shared" si="0"/></c></row>`, 400)), "shared formulas expand to more than 1 MB", 0},
		"not xml": {oneSheetParts("", `<row><c>`), "closed by </sheetData>", 0},
	} {
		t.Run(name, func(t *testing.T) {
			lim := small
			if tc.ratio > 0 {
				lim.ratio = tc.ratio
			}
			_, err := readParts(t, tc.parts, lim)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %v, want %q", err, tc.want)
			}
		})
	}
	// The limits error wraps errXLSXLimit.
	if _, err := readParts(t, bomb, small); !errors.Is(err, errXLSXLimit) {
		t.Errorf("%v isn't a limits error", err)
	}
	// A DTD declaring entities is refused, not expanded.
	dtd := oneSheetParts("", "")
	dtd["xl/sharedStrings.xml"] = `<?xml version="1.0"?><!DOCTYPE sst [<!ENTITY a "aaaaaaaaaa"><!ENTITY b "&a;&a;&a;&a;">]><sst ` + mainNS + `><si><t>&b;</t></si></sst>`
	if _, err := readParts(t, dtd, small); err == nil || !strings.Contains(err.Error(), "invalid character entity &b;") {
		t.Errorf("entity: %v", err)
	}
}

func mergeParts(p map[string]string, name, content string) map[string]string {
	p[name] = content
	return p
}

// tableParts is a workbook of one sheet with a table over A1:B3 and a
// formula reading it.
func tableParts() map[string]string {
	p := oneSheetParts("", `<row r="1"><c r="A1" t="inlineStr"><is><t>Item</t></is></c><c r="B1" t="inlineStr"><is><t>Cost</t></is></c></row>`+
		`<row r="2"><c r="B2"><v>5</v></c><c r="C2"><f>SUM(T[Cost])+T[[#This Row],[Cost]]</f></c></row>`)
	p["xl/worksheets/sheet1.xml"] = strings.Replace(p["xl/worksheets/sheet1.xml"], `</worksheet>`, `<tableParts count="1"><tablePart r:id="rT"/></tableParts></worksheet>`, 1)
	p["xl/worksheets/_rels/sheet1.xml.rels"] = `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rT" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/table" Target="../tables/table1.xml"/></Relationships>`
	p["xl/tables/table1.xml"] = `<table ` + mainNS + ` id="1" name="T" displayName="T" ref="A1:B3" totalsRowCount="1"><autoFilter ref="A1:B2"/><tableColumns count="2"><tableColumn id="1" name="Item"/><tableColumn id="2" name="Cost"/></tableColumns><tableStyleInfo name="TableStyleLight1" showRowStripes="1"/></table>`
	return p
}

// FuzzReadXLSX feeds the reader arbitrary files: it must fail cleanly,
// never panic or run past its limits.
func FuzzReadXLSX(f *testing.F) {
	f.Add(zipParts(f, kitchenParts()))
	f.Add(zipParts(f, oneSheetParts(`<workbookPr date1904="1"/>`, `<row><c r="B2" t="d"><v>2026-01-01</v></c></row>`)))
	f.Add(zipParts(f, tableParts()))
	f.Fuzz(func(t *testing.T, data []byte) {
		lim := defaultXLSXLimits
		lim.part, lim.total = 4<<20, 8<<20
		bk, err := openXLSX(bytes.NewReader(data), int64(len(data)), lim)
		if err != nil {
			return
		}
		bk.importBook(context.Background(), Options{})
	})
}

// FuzzReadXLSXParts fuzzes the XML of a worksheet, the shared strings
// and the styles, zipped into a workbook, where the zip's checksums
// would stop most mutations of a whole file.
func FuzzReadXLSXParts(f *testing.F) {
	k := kitchenParts()
	f.Add(k["xl/worksheets/sheet1.xml"], k["xl/sharedStrings.xml"], k["xl/styles.xml"])
	f.Add(`<worksheet><sheetData><row r="3"><c r="C3"><f t="shared" ref="C3:C9" si="0">A1:B$2+'x y'!A1</f></c></row><row><c r="C4"><f t="shared" si="0"/></c></row></sheetData></worksheet>`, "", "")
	f.Fuzz(func(t *testing.T, ws, sst, styles string) {
		p := kitchenParts()
		p["xl/worksheets/sheet1.xml"], p["xl/sharedStrings.xml"], p["xl/styles.xml"] = ws, sst, styles
		lim := defaultXLSXLimits
		lim.part, lim.total = 4<<20, 8<<20
		readParts(t, p, lim)
	})
}
