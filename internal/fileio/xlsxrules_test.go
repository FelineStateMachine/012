package fileio

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// lines are rules as the file writes them, for comparing.
func formatLines(s *sheet.Sheet) []string {
	var out []string
	for _, f := range s.CondFormats() {
		out = append(out, f.JSON())
	}
	return out
}

func validationLines(s *sheet.Sheet) []string {
	var out []string
	for _, v := range s.Validations() {
		out = append(out, v.JSON())
	}
	return out
}

func mustLoad(t *testing.T, s *sheet.Sheet, formats, validations []string) {
	t.Helper()
	var fs []sheet.CondFormat
	for _, l := range formats {
		f, err := sheet.ParseCondFormat(l)
		if err != nil {
			t.Fatalf("%s: %v", l, err)
		}
		fs = append(fs, f)
	}
	var vs []sheet.Validation
	for _, l := range validations {
		v, err := sheet.ParseValidation(l)
		if err != nil {
			t.Fatalf("%s: %v", l, err)
		}
		vs = append(vs, v)
	}
	if s.LoadCondFormats(fs)+s.LoadValidations(vs) > 0 {
		t.Fatal("rules left out")
	}
}

// Every rule 012 has goes out as Excel's and comes back the same.
func TestXLSXRulesRoundTrip(t *testing.T) {
	src := months(t)
	lists, err := src.Book().AddSheet("Lists", 1)
	if err != nil {
		t.Fatal(err)
	}
	lists.Set(addr(t, "A1"), "North")
	formats := []string{
		`{"ranges":"B2:B5","condition":"gt","values":["100"],"fill":"green","bold":true}`,
		`{"ranges":"B2:B5","condition":"between","values":["1","10"],"text":"red","fill":"yellow"}`,
		`{"ranges":"C2:C5","condition":"contains","values":["o"],"italic":true}`,
		`{"ranges":"C2:C5","condition":"not_contains","values":["x"],"text":"blue"}`,
		`{"ranges":"C2:C5","condition":"starts_with","values":["re"],"underline":true}`,
		`{"ranges":"C2:C5","condition":"ends_with","values":["s"],"strikethrough":true}`,
		`{"ranges":"C2:C5","condition":"exactly","values":["Fees"],"fill":"magenta"}`,
		`{"ranges":"A2:A5,C2:C5","condition":"empty","fill":"cyan"}`,
		`{"ranges":"A2:A5","condition":"not_empty","text":"magenta"}`,
		`{"ranges":"A2:A5","condition":"date_is","values":["today"],"fill":"red"}`,
		`{"ranges":"A2:A5","condition":"date_before","values":["2026-09-30"],"fill":"red"}`,
		`{"ranges":"A2:A5","condition":"date_after","values":["yesterday"],"fill":"blue"}`,
		`{"ranges":"B2:B5","condition":"eq","values":["=B$2"],"fill":"blue"}`,
		`{"ranges":"A2:C5","condition":"formula","values":["=$B2>=Lists!$B$1"],"text":"green"}`,
		`{"ranges":"B2:B5","scale":[{"type":"min","color":"red"},{"type":"percentile","value":"50","color":"yellow"},{"type":"max","color":"green"}]}`,
		`{"ranges":"B2:B5","scale":[{"type":"num","value":"0","color":"cyan"},{"type":"percent","value":"90","color":"blue"}]}`,
	}
	validations := []string{
		`{"ranges":"D2:D5","criteria":"list","items":["Yes","No"],"reject":true,"help":"Pick one"}`,
		`{"ranges":"E2:E5","criteria":"range","source":"Lists!A1:A5"}`,
		`{"ranges":"F2:F5","criteria":"range","source":"A2:A5","reject":true}`,
		`{"ranges":"G2:G5","criteria":"checkbox"}`,
		`{"ranges":"H2:H5","criteria":"number","condition":"between","values":["1","10"],"reject":true}`,
		`{"ranges":"I2:I5","criteria":"date","condition":"gt","values":["2026-01-31"]}`,
		`{"ranges":"J2:J5","criteria":"date"}`,
		`{"ranges":"K2:K5","criteria":"length","condition":"le","values":["=$B$2"]}`,
		`{"ranges":"L2:L5","criteria":"formula","values":["=L2<>A2"]}`,
	}
	mustLoad(t, src, formats, validations)
	formats, validations = formatLines(src), validationLines(src)
	name, got := exportImport(t, src)
	if len(got.Notes) > 0 {
		t.Errorf("notes %q", got.Notes)
	}
	back := got.Sheet.Book().Sheet(0)
	if f := formatLines(back); !reflect.DeepEqual(f, formats) {
		t.Errorf("conditional formats:\n%s\nwant\n%s", strings.Join(f, "\n"), strings.Join(formats, "\n"))
	}
	if v := validationLines(back); !reflect.DeepEqual(v, validations) {
		t.Errorf("validations:\n%s\nwant\n%s", strings.Join(v, "\n"), strings.Join(validations, "\n"))
	}
	// excelize reads what Excel would.
	x, err := excelize.OpenFile(name)
	if err != nil {
		t.Fatal(err)
	}
	defer x.Close()
	// excelize keeps one rule per range, the last written there.
	cfs, err := x.GetConditionalFormats("Sheet1")
	if err != nil || len(cfs) != 5 { // by sqref: B2:B5, C2:C5, A2:A5 C2:C5, A2:A5, A2:C5
		t.Errorf("excelize: %d ranges %v", len(cfs), err)
	}
	if r := cfs["B2:B5"]; len(r) != 1 || r[0].Type != "2_color_scale" || r[0].MinType != "num" || r[0].MaxValue != "90" {
		t.Errorf("excelize B2:B5: %+v", r)
	}
	if r := cfs["C2:C5"]; len(r) != 1 || r[0].Type != "cell" || r[0].Criteria != "equal to" || r[0].Value != `"Fees"` {
		t.Errorf("excelize C2:C5: %+v", r)
	}
	dvs, err := x.GetDataValidations("Sheet1")
	if err != nil || len(dvs) != len(validations) {
		t.Fatalf("excelize: %d validations %v", len(dvs), err)
	}
	if dv := dvs[0]; dv.Type != "list" || dv.Formula1 != `"Yes,No"` || dv.ErrorStyle == nil || *dv.ErrorStyle != "stop" || dv.Prompt == nil || *dv.Prompt != "Pick one" {
		t.Errorf("excelize list: %+v", dv)
	}
	if dv := dvs[1]; dv.Formula1 != "Lists!$A$1:$A$5" {
		t.Errorf("excelize range: %+v", dv)
	}
	// excelize reports colors only for files with a theme part.
	style, err := x.GetConditionalStyle(0)
	if err != nil || style.Font == nil || !style.Font.Bold || style.Fill.Pattern != 1 {
		t.Errorf("excelize dxf 0: %+v %v", style, err)
	}
}

// Rules only Excel has, or ones 012 can't write, are left out with a
// note.
func TestXLSXRulesLimits(t *testing.T) {
	s := months(t)
	mustLoad(t, s, []string{
		`{"ranges":"A2:A5","condition":"formula","values":["=B2>1#AND#B2<9"],"fill":"red"}`,
	}, []string{
		`{"ranges":"D2:D5","criteria":"list","items":["a,b","c"]}`,
	})
	_, got := exportImport(t, s)
	want := []string{
		"1 conditional format with no Excel equivalent left out, e.g. A2",
		"1 data validation rule Excel can't hold left out, e.g. D2 (list items with commas or quotes, or over 255 characters, or a formula with no Excel equivalent)",
	}
	if res, err := Export(context.Background(), t.TempDir()+"/x.xlsx", XLSX, SnapBook(s), ExportOptions{}); err != nil || !reflect.DeepEqual(res.Notes, want) {
		t.Errorf("export notes %q %v", res.Notes, err)
	}
	if len(got.Sheet.CondFormats())+len(got.Sheet.Validations()) != 0 {
		t.Error("rules came back")
	}
}

// What Excel writes: theme and indexed colors, rules 012 doesn't have,
// validation in the 2010 extension.
func TestXLSXRulesImport(t *testing.T) {
	p := oneSheetParts("", `<row r="1"><c r="A1"><v>5</v></c></row>`)
	p["xl/styles.xml"] = `<styleSheet ` + mainNS + `><cellXfs count="1"><xf numFmtId="0" fontId="0"/></cellXfs><dxfs count="3">` +
		`<dxf><font><color rgb="FF9C0006"/></font><fill><patternFill><bgColor rgb="FFFFC7CE"/></patternFill></fill></dxf>` +
		`<dxf><font><b/><color theme="4"/></font></dxf>` +
		`<dxf><numFmt numFmtId="164" formatCode="0.0"/><border><left style="thin"/></border></dxf></dxfs></styleSheet>`
	p["xl/worksheets/sheet1.xml"] = `<worksheet ` + mainNS + ` xmlns:x14="http://schemas.microsoft.com/office/spreadsheetml/2009/9/main" xmlns:xm="http://schemas.microsoft.com/office/excel/2006/main">` +
		`<sheetData><row r="1"><c r="A1"><v>5</v></c></row></sheetData>` +
		`<conditionalFormatting sqref="A1:A10"><cfRule type="dataBar" priority="4"><dataBar><cfvo type="min"/><cfvo type="max"/><color rgb="FF638EC6"/></dataBar></cfRule>` +
		`<cfRule type="cellIs" dxfId="0" priority="2" operator="lessThan"><formula>10</formula></cfRule>` +
		`<cfRule type="iconSet" priority="5"><iconSet iconSet="3Arrows"><cfvo type="percent" val="0"/><cfvo type="percent" val="33"/><cfvo type="percent" val="67"/></iconSet></cfRule></conditionalFormatting>` +
		`<conditionalFormatting sqref="B1:B10 D1"><cfRule type="expression" dxfId="1" priority="1"><formula>INT(B1)&lt;DATE(2026,1,5)</formula></cfRule>` +
		`<cfRule type="expression" dxfId="2" priority="3"><formula>B1&gt;0</formula></cfRule>` +
		`<cfRule type="colorScale" priority="6"><colorScale><cfvo type="min"/><cfvo type="max"/><color theme="0"/><color rgb="FF57BB8A"/></colorScale></cfRule>` +
		`<cfRule type="timePeriod" dxfId="0" priority="7" timePeriod="lastWeek"><formula>x</formula></cfRule></conditionalFormatting>` +
		`<dataValidations count="4"><dataValidation type="list" allowBlank="1" showErrorMessage="1" sqref="C1:C9"><formula1>"Yes,No"</formula1></dataValidation>` +
		`<dataValidation type="whole" operator="greaterThanOrEqual" errorStyle="warning" showErrorMessage="1" error="Positive please" sqref="E1:E9"><formula1>0</formula1></dataValidation>` +
		`<dataValidation type="time" sqref="F1"><formula1>0.5</formula1></dataValidation>` +
		`<dataValidation type="list" sqref="G1"><formula1>Choices</formula1></dataValidation></dataValidations>` +
		`<extLst><ext uri="{CCE6A557-97BC-4b89-ADB6-D9C93CAAB3DF}"><x14:dataValidations count="1"><x14:dataValidation type="list" allowBlank="1" showErrorMessage="1">` +
		`<x14:formula1><xm:f>Lists!$A$1:$A$5</xm:f></x14:formula1><xm:sqref>H1:H9</xm:sqref></x14:dataValidation></x14:dataValidations></ext>` +
		`<ext uri="{78C0D931-6437-407d-A8EE-F0AAD7539E65}"><x14:conditionalFormattings><x14:conditionalFormatting><x14:cfRule type="dataBar" id="{1}"/><xm:sqref>A1:A10</xm:sqref></x14:conditionalFormatting>` +
		`<x14:conditionalFormatting><x14:cfRule type="iconSet" id="{2}"><x14:iconSet iconSet="3Stars"/></x14:cfRule><xm:sqref>C1:C5</xm:sqref></x14:conditionalFormatting></x14:conditionalFormattings></ext></extLst></worksheet>`
	path := writeParts(t, t.TempDir(), "excel.xlsx", p)
	got, err := Import(context.Background(), path, Options{})
	if err != nil {
		t.Fatal(err)
	}
	s := got.Sheet
	wantF := []string{
		`{"ranges":"B1:B10,D1","condition":"date_before","values":["2026-01-05"],"text":"blue","bold":true}`,
		`{"ranges":"A1:A10","condition":"lt","values":["10"],"text":"red","fill":"red"}`,
		`{"ranges":"A1:A10","dataBar":{"color":"blue","min":{"type":"min"},"max":{"type":"max"}}}`,
		`{"ranges":"A1:A10","iconSet":{"icons":"arrows","points":[{"type":"percent","value":"33"},{"type":"percent","value":"67"}]}}`,
		`{"ranges":"B1:B10,D1","scale":[{"type":"min","color":"yellow"},{"type":"max","color":"green"}]}`,
		`{"ranges":"B1:B10,D1","condition":"date_is","values":["last week"],"text":"red","fill":"red"}`,
	}
	if f := formatLines(s); !reflect.DeepEqual(f, wantF) {
		t.Errorf("conditional formats:\n%s\nwant\n%s", strings.Join(f, "\n"), strings.Join(wantF, "\n"))
	}
	wantV := []string{
		`{"ranges":"C1:C9","criteria":"list","items":["Yes","No"],"reject":true}`,
		`{"ranges":"E1:E9","criteria":"number","condition":"ge","values":["0"],"help":"Positive please"}`,
		`{"ranges":"H1:H9","criteria":"range","source":"Lists!A1:A5","reject":true}`,
	}
	if v := validationLines(s); !reflect.DeepEqual(v, wantV) {
		t.Errorf("validations:\n%s\nwant\n%s", strings.Join(v, "\n"), strings.Join(wantV, "\n"))
	}
	wantNotes := []string{
		"2 conditional formats left out: in Excel 2010's extension (its own icon sets and the like) (1), without a text color, fill or text style 012 draws (1)",
		"2 data validation rules left out: lists from named ranges or formulas (1), time rules (1)",
	}
	if !reflect.DeepEqual(got.Notes, wantNotes) {
		t.Errorf("notes:\n%q\nwant\n%q", got.Notes, wantNotes)
	}
}
