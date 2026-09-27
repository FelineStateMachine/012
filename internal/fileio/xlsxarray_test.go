package fileio

import (
	"archive/zip"
	"context"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestExcelArrayFormulas(t *testing.T) {
	for _, tc := range []struct {
		in, want string
		arrays   bool
		ok       bool
	}{
		{"=SEQUENCE(3)", "_xlfn.SEQUENCE(3)", true, true},
		{"=FILTER(A1:A9, B1:B9>2)", "_xlfn._xlws.FILTER(A1:A9,B1:B9>2)", true, true},
		{"=sort(unique(A:A))", "_xlfn._xlws.SORT(_xlfn.UNIQUE(A:A))", true, true},
		{"=ARRAYFORMULA(A1:A3*2)", "A1:A3*2", true, true},
		{"=SUM(ARRAYFORMULA(A1:A3*2))", "", false, false},
		{"=LET(x, 2, x*3)", "_xlfn.LET(_xlpm.x,2,_xlpm.x*3)", true, true},
		{"=LET(f, LAMBDA(v, v+1), f(2))", "_xlfn.LET(_xlpm.f,_xlfn.LAMBDA(_xlpm.v,_xlpm.v+1),_xlpm.f(2))", true, true},
		{"=MAP(A1:A3, LAMBDA(v, v*2))", "_xlfn.MAP(A1:A3,_xlfn.LAMBDA(_xlpm.v,_xlpm.v*2))", true, true},
		{"={1,-2;\"a\",TRUE}", `{1,-2;"a",TRUE}`, true, true},
		{"={A1,B1}", "", false, false},
		{"=SUM({1,2})", "SUM({1,2})", true, true},
		{"=SORTN(A1:A9)", "", false, false},
		{`=SPLIT(A1, ",")`, "", false, false},
		{`=REGEXREPLACE(A1, "a+", "b")`, `_xlfn.REGEXREPLACE(A1, "a+", "b")`, false, true},
		{`=REGEXMATCH(A1, "a")`, "", false, false},
		{"=SUM(A1:A3*2)", "SUM(A1:A3*2)", false, true},
	} {
		got, arrays, ok := excelFormula(tc.in, nil)
		if ok != tc.ok || ok && (got != tc.want || arrays != tc.arrays) {
			t.Errorf("excelFormula(%q) = %q, %v, %v, want %q, %v, %v", tc.in, got, arrays, ok, tc.want, tc.arrays, tc.ok)
		}
	}
	for in, want := range map[string]string{
		"_xlfn._xlws.FILTER(A1:A9,B1:B9>2)":        "=FILTER(A1:A9,B1:B9>2)",
		"_xlfn.LET(_xlpm.x,2,_xlpm.x*3)":           "=LET(x,2,x*3)",
		"_xlfn.MAP(A1:A3,_xlfn.LAMBDA(_xlpm.v,1))": "=MAP(A1:A3,LAMBDA(v,1))",
	} {
		if got := fromExcelFormula(in); got != want {
			t.Errorf("fromExcelFormula(%q) = %q, want %q", in, got, want)
		}
	}
}

// A spill goes out as Excel's dynamic array formula over the cells it
// spills into, which hold its values, and comes back as the formula,
// spilling again rather than blocked by its own values.
func TestXLSXSpill(t *testing.T) {
	src := build(t, map[string]string{"A1": "3", "A2": "1", "A3": "2", "C1": "=SORT(A1:A3)", "D1": "=SUM(A1:A3)", "E1": "=SPLIT(\"x,y\", \",\")"})
	name := filepath.Join(t.TempDir(), "spill.xlsx")
	res, err := Export(context.Background(), name, XLSX, SnapBook(src), ExportOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Notes) != 1 || !strings.Contains(res.Notes[0], "e.g. E1") {
		t.Errorf("notes %q", res.Notes)
	}
	parts := zipText(t, name)
	ws := parts["xl/worksheets/sheet1.xml"]
	for _, want := range []string{
		`<c r="C1" cm="1"><f t="array" ref="C1:C3">_xlfn._xlws.SORT(A1:A3)</f><v>1</v></c>`,
		`<c r="C2"><v>2</v></c>`, `<c r="C3"><v>3</v></c>`,
		`<c r="D1"><f>SUM(A1:A3)</f><v>6</v></c>`,
		`<c r="E1" t="inlineStr"><is><t xml:space="preserve">x</t></is></c>`,
	} {
		if !strings.Contains(ws, want) {
			t.Errorf("sheet1.xml lacks %s:\n%s", want, ws)
		}
	}
	if !strings.Contains(parts["xl/metadata.xml"], `fDynamic="1"`) ||
		!strings.Contains(parts["[Content_Types].xml"], `/xl/metadata.xml`) ||
		!strings.Contains(parts["xl/_rels/workbook.xml.rels"], `Target="metadata.xml"`) {
		t.Error("the dynamic array metadata part is missing or not linked")
	}
	x, err := excelize.OpenFile(name)
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := x.GetCellValue("Sheet1", "C3"); v != "3" {
		t.Errorf("excelize reads C3 as %q", v)
	}
	x.Close()

	got, err := Import(context.Background(), name, Options{})
	if err != nil {
		t.Fatal(err)
	}
	s := got.Sheet
	if in := input(s, addr(t, "C1")); in != "=SORT(A1:A3)" {
		t.Errorf("C1 read back as %q", in)
	}
	for cell, want := range map[string]string{"C1": "1", "C2": "2", "C3": "3"} {
		if v := shown(s, addr(t, cell)); v != want {
			t.Errorf("%s shows %q, want %q", cell, v, want)
		}
	}
	if !s.Cell(addr(t, "C2")).Spilled() {
		t.Error("C2 came back as a value, not spilled")
	}
}

// A workbook without arrays has no metadata part.
func TestXLSXNoArrays(t *testing.T) {
	name := filepath.Join(t.TempDir(), "plain.xlsx")
	if _, err := Export(context.Background(), name, XLSX, SnapBook(build(t, map[string]string{"A1": "=1+1"})), ExportOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, ok := zipText(t, name)["xl/metadata.xml"]; ok {
		t.Error("metadata.xml written without dynamic arrays")
	}
}

// zipText reads every part of a zip file.
func zipText(t *testing.T, name string) map[string]string {
	t.Helper()
	zr, err := zip.OpenReader(name)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	out := map[string]string{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		b, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		out[f.Name] = string(b)
	}
	return out
}
