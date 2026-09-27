package fileio

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"

	"012/internal/sheet"
)

func TestXLSXRoundTrip(t *testing.T) {
	src := build(t, map[string]string{
		"A1": "Bills", "B1": "Due", "C1": "Amount", "D1": "Share",
		"A2": "Rent", "B2": "10/1/2026", "C2": "$1,450.00", "D2": "=C2/C$4",
		"A3": "Food", "B3": "2026-09-28", "C3": "612.4", "D3": "=@ROUND(C3/C$4..C$4, 2)",
		"A4": "Total", "C4": "=SUM(C2:C3)", "D4": "=IFNA(XLOOKUP(\"Rent\",A2:A3,C2:C3),0)",
		"A5": "'123", "B5": "14:30", "C5": "TRUE", "D5": "1.5E+30",
		"A6": "old", "B6": "2/1/1900", "C6": "=C2>C3 #AND# C3>0", "E6": "=A1&\" x\"",
	})
	r := func(s string) sheet.Rect { return sheet.NewRect(addr(t, s), addr(t, s)) }
	src.SetFormat(r("D2"), sheet.Preset(sheet.FmtPercent))
	src.SetFormat(r("C3"), sheet.Format{Kind: sheet.FmtAccounting, Decimals: 1})
	src.SetFormat(r("B5"), sheet.Format{Kind: sheet.FmtTime, Pattern: "h:mm AM/PM"})
	src.SetFormat(r("A6"), sheet.Format{Kind: sheet.FmtCustom, Pattern: "0.000"})
	src.SetStyle(r("A1"), func(st *sheet.Style) { st.Bold, st.Italic = true, true })
	src.SetStyle(r("A2"), func(st *sheet.Style) { st.Underline, st.Strikethrough = true, true })
	src.SetStyle(r("B1"), func(st *sheet.Style) { st.Align = sheet.AlignRight })
	src.SetColWidth(0, 14)
	src.SetColWidth(3, 7)

	name := filepath.Join(t.TempDir(), "bills.xlsx")
	res, err := Export(context.Background(), name, XLSX, Snap(src, sheet.Rect{}, "Bills"), ExportOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Notes) != 1 || !strings.Contains(res.Notes[0], "1 formula with no Excel equivalent saved as values, e.g. C6") {
		t.Errorf("notes %q", res.Notes)
	}

	// Excel sees Excel formulas, with _xlfn. for newer functions.
	x, err := excelize.OpenFile(name)
	if err != nil {
		t.Fatal(err)
	}
	for cell, want := range map[string]string{
		"D3": "ROUND(C3/C$4:C$4, 2)", "D4": `_xlfn.IFNA(_xlfn.XLOOKUP("Rent",A2:A3,C2:C3),0)`, "C6": "",
	} {
		if got, _ := x.GetCellFormula("Bills", cell); got != want {
			t.Errorf("Excel formula in %s = %q, want %q", cell, got, want)
		}
	}
	if v, _ := x.GetCellValue("Bills", "C4"); v != "$2,062.40" {
		t.Errorf("cached C4 = %q", v)
	}
	x.Close()

	got, err := Import(context.Background(), name, Options{})
	if err != nil {
		t.Fatal(err)
	}
	s := got.Sheet
	for _, a := range src.Addrs() {
		if want, g := shown(src, a), shown(s, a); g != want {
			t.Errorf("%s shows %q after the round trip, want %q", a, g, want)
		}
		if want, g := src.Cell(a).Style, s.Cell(a).Style; g != want {
			t.Errorf("%s style %+v, want %+v", a, g, want)
		}
	}
	for cell, want := range map[string]string{
		"D2": "=C2/C$4", "D3": "=ROUND(C3/C$4:C$4, 2)", "D4": `=IFNA(XLOOKUP("Rent",A2:A3,C2:C3),0)`,
		"A5": "'123", "C5": "TRUE", "C6": "TRUE", "E6": `=A1&" x"`,
	} {
		if g := input(s, addr(t, cell)); g != want {
			t.Errorf("%s input = %q, want %q", cell, g, want)
		}
	}
	for cell, want := range map[string]sheet.Format{
		"B2": {Kind: sheet.FmtDate}, "C3": {Kind: sheet.FmtAccounting, Decimals: 1},
		"B5": {Kind: sheet.FmtTime, Pattern: "h:mm AM/PM"}, "A6": {Kind: sheet.FmtCustom, Pattern: "0.000"},
	} {
		if g := s.Cell(addr(t, cell)).Format; g != want {
			t.Errorf("%s format %+v, want %+v", cell, g, want)
		}
	}
	if w := s.ColWidth(0); w != 14 {
		t.Errorf("A width = %d", w)
	}
	if w := s.ColWidth(3); w != 7 {
		t.Errorf("D width = %d", w)
	}
	if w := s.ColWidth(1); w != sheet.DefaultWidth {
		t.Errorf("B width = %d", w)
	}
}

// TestXLSXImport reads a workbook written by excelize the way Excel
// writes one: several sheets, shared strings, built-in formats and
// formulas 012 can't read.
func TestXLSXImport(t *testing.T) {
	x := excelize.NewFile()
	x.SetSheetName("Sheet1", "Q1")
	x.NewSheet("Q2")
	x.NewSheet("Q3")
	x.SetCellValue("Q1", "A1", "Region")
	x.SetCellValue("Q1", "B1", 1234.5)
	x.SetCellValue("Q1", "C1", "=not a formula")
	x.SetCellValue("Q1", "D1", 45000)
	x.SetCellValue("Q1", "E1", true)
	x.SetCellFormula("Q1", "A2", "Q2!A1*2")
	x.SetCellValue("Q1", "A2", 84) // the cached value
	x.SetCellFormula("Q1", "A2", "Q2!A1*2")
	x.SetCellFormula("Q1", "B2", "SUM(B1,1)")
	x.SetCellValue("Q2", "A1", 42)
	id, _ := x.NewStyle(&excelize.Style{NumFmt: 4, Font: &excelize.Font{Bold: true}})
	x.SetCellStyle("Q1", "B1", "B1", id)
	date, _ := x.NewStyle(&excelize.Style{NumFmt: 15})
	x.SetCellStyle("Q1", "D1", "D1", date)
	pct, _ := x.NewStyle(&excelize.Style{NumFmt: 10, Alignment: &excelize.Alignment{Horizontal: "center"}})
	x.SetCellStyle("Q1", "B2", "B2", pct)
	name := filepath.Join(t.TempDir(), "q.xlsx")
	if err := x.SaveAs(name); err != nil {
		t.Fatal(err)
	}

	prog := NewProgress()
	res, err := Import(context.Background(), name, Options{Progress: prog})
	if err != nil {
		t.Fatal(err)
	}
	s := res.Sheet
	for cell, want := range map[string]string{
		"A1": "Region", "B1": "1,234.50", "C1": "=not a formula", "D1": "15-Mar-23", "E1": "TRUE",
		"A2": "84", "B2": "123550.00%",
	} {
		if g := shown(s, addr(t, cell)); g != want {
			t.Errorf("%s shows %q, want %q", cell, g, want)
		}
	}
	if g := input(s, addr(t, "B2")); g != "=SUM(B1,1)" {
		t.Errorf("B2 input %q", g)
	}
	if st := s.Cell(addr(t, "B1")).Style; !st.Bold {
		t.Errorf("B1 not bold")
	}
	if st := s.Cell(addr(t, "B2")).Style; st.Align != sheet.AlignCenter {
		t.Errorf("B2 align %v", st.Align)
	}
	notes := strings.Join(res.Notes, "; ")
	for _, want := range []string{
		"first sheet only, Q1; not imported: Q2, Q3",
		"1 formula kept as values, e.g. A2 =Q2!A1*2",
	} {
		if !strings.Contains(notes, want) {
			t.Errorf("notes %q, want %q", notes, want)
		}
	}
	if rows, frac := prog.Get(); rows != 2 {
		t.Errorf("progress %d rows, %v", rows, frac)
	}
}

func TestFormatCodes(t *testing.T) {
	for _, tc := range []struct {
		id   int
		code string
		want sheet.Format
	}{
		{0, "", sheet.Format{}},
		{0, "General", sheet.Format{}},
		{3, "", sheet.Format{Kind: sheet.FmtNumber}},
		{4, "", sheet.Format{Kind: sheet.FmtNumber, Decimals: 2}},
		{7, "", sheet.Format{Kind: sheet.FmtCurrency, Decimals: 2}},
		{9, "", sheet.Format{Kind: sheet.FmtPercent}},
		{11, "", sheet.Format{Kind: sheet.FmtScientific, Decimals: 2}},
		{14, "", sheet.Format{Kind: sheet.FmtDate}},
		{22, "", sheet.Format{Kind: sheet.FmtDateTime, Pattern: "m/d/yyyy h:mm"}},
		{44, "", sheet.Format{Kind: sheet.FmtAccounting, Decimals: 2}},
		{46, "", sheet.Format{Kind: sheet.FmtDuration}},
		{49, "", sheet.Format{Kind: sheet.FmtText}},
		{164, `\$#,##0.00`, sheet.Format{Kind: sheet.FmtCurrency, Decimals: 2}},
		{164, `[$$-409]#,##0`, sheet.Format{Kind: sheet.FmtCurrency}},
		{164, "yyyy-mm-dd", sheet.Format{Kind: sheet.FmtDate, Pattern: "yyyy-mm-dd"}},
		{164, "hh:mm", sheet.Format{Kind: sheet.FmtTime, Pattern: "hh:mm"}},
		{164, "0.0", sheet.Format{Kind: sheet.FmtCustom, Pattern: "0.0"}},
		{164, "[Red]0.00", sheet.Format{Kind: sheet.FmtCustom, Pattern: "[Red]0.00"}},
	} {
		if got := formatOf(tc.id, tc.code); got != tc.want {
			t.Errorf("formatOf(%d, %q) = %+v, want %+v", tc.id, tc.code, got, tc.want)
		}
	}
	// Every format 012 writes reads back as itself.
	for k := sheet.FmtText; k <= sheet.FmtDuration; k++ {
		for d := range 4 {
			f := sheet.Format{Kind: k}
			if sheet.Preset(k).Decimals > 0 {
				f.Decimals = d
			}
			if got := formatOf(164, excelCode(f)); got != f {
				t.Errorf("%+v -> %q -> %+v", f, excelCode(f), got)
			}
		}
	}
}

func TestExcelFormulas(t *testing.T) {
	for _, tc := range []struct {
		in, want string
		ok       bool
	}{
		{"=SUM(A1:B2)", "SUM(A1:B2)", true},
		{"=@SUM(A1..B2)", "SUM(A1:B2)", true},
		{`=IF(A1="@x..y",1,2)`, `IF(A1="@x..y",1,2)`, true},
		{"=@PI*2", "PI()*2", true},
		{"=$A$1*1.5E3", "$A$1*1.5E3", true},
		{"=xlookup(1,A:A,B:B)", "_xlfn.XLOOKUP(1,A:A,B:B)", true},
		{`=JEV.TEST(A2,"Is it?")`, "", false},
		{"=A1>1 #AND# B1<2", "", false},
		{"=#REF!+1", "#REF!+1", true},
	} {
		got, ok := toExcelFormula(tc.in)
		if ok != tc.ok || ok && got != tc.want {
			t.Errorf("toExcelFormula(%q) = %q, %v, want %q, %v", tc.in, got, ok, tc.want, tc.ok)
		}
	}
	for in, want := range map[string]string{
		"SUM(A1:B2)":                             "=SUM(A1:B2)",
		"_xlfn.IFNA(_xlfn.XLOOKUP(1,A:A,B:B),0)": "=IFNA(XLOOKUP(1,A:A,B:B),0)",
		`"_xlfn.kept"&A1`:                        `="_xlfn.kept"&A1`,
	} {
		if got := fromExcelFormula(in); got != want {
			t.Errorf("fromExcelFormula(%q) = %q, want %q", in, got, want)
		}
	}
}
