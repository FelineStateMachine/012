package fileio

import (
	"reflect"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

// Top and bottom values, averages, duplicates, periods Excel names,
// data bars and icon sets go out as Excel's own rules and come back the
// same; periods Excel has no name for, and date rules before or after a
// period, come back as custom formulas that test the same days.
func TestXLSXNewRulesRoundTrip(t *testing.T) {
	src := months(t)
	formats := []string{
		`{"ranges":"B2:B5","condition":"top","values":["2"],"fill":"green"}`,
		`{"ranges":"B2:B5","condition":"bottom_percent","values":["25"],"fill":"red"}`,
		`{"ranges":"B2:B5","condition":"top_percent","values":["10"],"bold":true}`,
		`{"ranges":"B2:B5","condition":"bottom","values":["1"],"italic":true}`,
		`{"ranges":"B2:B5","condition":"above_average","text":"blue"}`,
		`{"ranges":"B2:B5","condition":"below_average","text":"red"}`,
		`{"ranges":"C2:C5","condition":"duplicate","fill":"yellow"}`,
		`{"ranges":"C2:C5","condition":"unique","underline":true}`,
		`{"ranges":"A2:A5","condition":"date_is","values":["this week"],"fill":"cyan"}`,
		`{"ranges":"A2:A5","condition":"date_is","values":["past week"],"fill":"cyan"}`,
		`{"ranges":"A2:A5","condition":"date_is","values":["next month"],"fill":"cyan"}`,
		`{"ranges":"B2:B5","dataBar":{"color":"blue","min":{"type":"min"},"max":{"type":"max"}}}`,
		`{"ranges":"B2:B5","dataBar":{"color":"green","min":{"type":"num","value":"0"},"max":{"type":"percentile","value":"90"},"barOnly":true}}`,
		`{"ranges":"B2:B5","iconSet":{"icons":"arrows","points":[{"type":"percent","value":"33"},{"type":"percent","value":"67"}]}}`,
		`{"ranges":"B2:B5","iconSet":{"icons":"arrows","points":[{"type":"percent","value":"25"},{"type":"percent","value":"50"},{"type":"percent","value":"75"}],"reverse":true}}`,
		`{"ranges":"B2:B5","iconSet":{"icons":"circles","points":[{"type":"percent","value":"20"},{"type":"percent","value":"40"},{"type":"percent","value":"60"},{"type":"percent","value":"80"}],"iconOnly":true}}`,
		`{"ranges":"B2:B5","iconSet":{"icons":"symbols","points":[{"type":"num","value":"100"},{"type":"num","value":"1000"}]}}`,
		`{"ranges":"B2:B5","iconSet":{"icons":"rating","points":[{"type":"percentile","value":"25"},{"type":"percentile","value":"50"},{"type":"percentile","value":"75"}]}}`,
	}
	mustLoad(t, src, formats, nil)
	formats = formatLines(src)
	name, got := exportImport(t, src)
	if len(got.Notes) > 0 {
		t.Errorf("notes %q", got.Notes)
	}
	if f := formatLines(got.Sheet.Book().Sheet(0)); !reflect.DeepEqual(f, formats) {
		t.Errorf("conditional formats:\n%s\nwant\n%s", strings.Join(f, "\n"), strings.Join(formats, "\n"))
	}
	x, err := excelize.OpenFile(name)
	if err != nil {
		t.Fatal(err)
	}
	defer x.Close()
	if _, err := x.GetConditionalFormats("Sheet1"); err != nil {
		t.Errorf("excelize: %v", err)
	}

	// Periods Excel has no name for.
	other := months(t)
	mustLoad(t, other, []string{
		`{"ranges":"A2:A5","condition":"date_is","values":["last year"],"fill":"cyan"}`,
		`{"ranges":"A2:A5","condition":"date_before","values":["this month"],"fill":"cyan"}`,
	}, nil)
	_, got = exportImport(t, other)
	want := []string{
		"=AND(INT(A2)>=DATE(YEAR(TODAY())-1,1,1),INT(A2)<=DATE(YEAR(TODAY())-1,12,31))",
		"=INT(A2)<DATE(YEAR(TODAY()),MONTH(TODAY()),1)",
	}
	var args []string
	for _, f := range got.Sheet.CondFormats() {
		args = append(args, f.Op.String()+" "+f.Args[0])
	}
	if !reflect.DeepEqual(args, []string{"formula " + want[0], "formula " + want[1]}) {
		t.Errorf("periods as formulas:\n%s\nwant\n%s", strings.Join(args, "\n"), strings.Join(want, "\n"))
	}
}

// A dropdown shown as plain text goes out with Excel's arrow hidden; a
// checkbox of its own values goes out as a list of them, which comes
// back as a dropdown, as Excel has no such checkbox.
func TestXLSXDropdownDisplayAndCheckboxValues(t *testing.T) {
	src := months(t)
	mustLoad(t, src, nil, []string{
		`{"ranges":"D2:D5","criteria":"list","items":["a","b"],"display":"plain"}`,
		`{"ranges":"E2:E5","criteria":"list","items":["a","b"],"display":"chip"}`,
		`{"ranges":"F2:F5","criteria":"checkbox","items":["Yes","No"]}`,
	})
	_, got := exportImport(t, src)
	want := []string{
		`{"ranges":"D2:D5","criteria":"list","items":["a","b"],"display":"plain"}`,
		`{"ranges":"E2:E5","criteria":"list","items":["a","b"]}`,
		`{"ranges":"F2:F5","criteria":"list","items":["Yes","No"]}`,
	}
	if v := validationLines(got.Sheet); !reflect.DeepEqual(v, want) {
		t.Errorf("validations:\n%s\nwant\n%s", strings.Join(v, "\n"), strings.Join(want, "\n"))
	}
}
