package headless

import (
	"strings"
	"testing"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// Each type written as a Value is the cell typing it makes, shown as a
// person would type it, and reads back as the same Value.
func TestValuesBothWays(t *testing.T) {
	for _, c := range []struct {
		value, shown string
		format       sheet.Format
		back         string // what reads back, when not value
	}{
		{value: `3.5`, shown: "3.5"},
		{value: `"00123"`, shown: "00123"},
		{value: `"=1+1"`, shown: "=1+1"},
		{value: `true`, shown: "TRUE"},
		{value: `{"currency":3.5}`, shown: "$3.50", format: sheet.Format{Kind: sheet.FmtCurrency, Decimals: 2}},
		{value: `{"currency":-1200,"decimals":0}`, shown: "-$1,200", format: sheet.Format{Kind: sheet.FmtCurrency}},
		{value: `{"currency":3.5,"symbol":"€"}`, shown: "€3.50", format: sheet.Format{Kind: sheet.FmtCustom, Pattern: `"€"#,##0.00`}},
		{value: `{"percent":0.12}`, shown: "12%", format: sheet.Format{Kind: sheet.FmtPercent}},
		{value: `{"percent":0.125}`, shown: "12.50%", format: sheet.Format{Kind: sheet.FmtPercent, Decimals: 2}},
		{value: `{"date":"2026-09-29"}`, shown: "9/29/2026", format: sheet.Format{Kind: sheet.FmtDate}},
		{value: `{"date":"2026-09-29T14:30:00"}`, shown: "9/29/2026 14:30:00", format: sheet.Format{Kind: sheet.FmtDateTime}},
		{value: `{"date":"2026-09-29","format":"yyyy-mm-dd"}`, shown: "2026-09-29", format: sheet.Format{Kind: sheet.FmtDate, Pattern: "yyyy-mm-dd"}},
		{value: `{"time":"14:30"}`, shown: "2:30:00 PM", format: sheet.Format{Kind: sheet.FmtTime}, back: `{"time":"14:30:00"}`},
		{value: `{"duration":"90min"}`, shown: "1:30:00", format: sheet.Format{Kind: sheet.FmtDuration}},
		{value: `{"duration":5400}`, shown: "1:30:00", format: sheet.Format{Kind: sheet.FmtDuration}, back: `{"duration":"90min"}`},
		{value: `{"size":1500}`, shown: "1.5 kB", format: sheet.Format{Kind: sheet.FmtSize, Decimals: 1}},
		{value: `{"size":"1.5kb"}`, shown: "1.5 kB", format: sheet.Format{Kind: sheet.FmtSize, Decimals: 1}, back: `{"size":1500}`},
		{value: `{"number":1234.5,"decimals":2}`, shown: "1,234.50", format: sheet.Format{Kind: sheet.FmtNumber, Decimals: 2}},
		{value: `{"number":0.5,"format":"0.000"}`, shown: "0.500", format: sheet.Format{Kind: sheet.FmtCustom, Pattern: "0.000"}},
		{value: `{"text":"12%"}`, shown: "12%", back: `"12%"`},
	} {
		w := sheet.NewBook()
		s := w.Sheet(0)
		if _, err := Set(w, []Entry{{Ref: "B2", Value: Value(c.value)}}, SetOptions{}); err != nil {
			t.Errorf("%s: %v", c.value, err)
			continue
		}
		a := addr("B2")
		if got := s.LocalText(a); got != c.shown {
			t.Errorf("%s shows %q, want %q", c.value, got, c.shown)
		}
		if got := s.DisplayFormat(a); got != c.format {
			t.Errorf("%s is formatted %+v, want %+v", c.value, got, c.format)
		}
		back := c.back
		if back == "" {
			back = c.value
		}
		if got := string(CellValue(s, a)); got != back {
			t.Errorf("%s reads back as %s, want %s", c.value, got, back)
		}
	}
}

// Cells typed in the grid read as Values of their types.
func TestTypedEntriesRead(t *testing.T) {
	w := sheet.NewBook()
	s := w.Sheet(0)
	for in, want := range map[string]string{
		"$3.50":      `{"currency":3.5}`,
		"12%":        `{"percent":0.12}`,
		"2026-09-29": `{"date":"2026-09-29","format":"yyyy-mm-dd"}`,
		"1,234":      `{"number":1234,"decimals":0}`,
		"'00123":     `"00123"`,
		"25:30:00":   `{"duration":"1530min"}`,
	} {
		if _, err := Set(w, []Entry{{Ref: "A1", Input: in}}, SetOptions{}); err != nil {
			t.Fatal(err)
		}
		if got := string(CellValue(s, addr("A1"))); got != want {
			t.Errorf("%s reads as %s, want %s", in, got, want)
		}
	}
}

func TestValueErrors(t *testing.T) {
	for value, want := range map[string]string{
		`{"price":3}`:                  `"price" isn't a field of a value`,
		`{"currency":"3"}`:             `currency: "3" is a number`,
		`{"date":"29/9/2026"}`:         `date: "29/9/2026" isn't a date`,
		`{"size":"3 apples"}`:          `size: "3 apples" isn't nushell's size`,
		`{"duration":"1kb"}`:           `duration: 1000b isn't a duration`,
		`[1, 2]`:                       `isn't a cell's value`,
		`{"decimals":2}`:               `names its type`,
		`{"currency":1,"percent":0.5}`: `"percent" isn't a field`,
	} {
		_, err := Set(sheet.NewBook(), []Entry{{Ref: "A1", Value: Value(value)}}, SetOptions{})
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: %v, want %q", value, err, want)
		}
	}
	if _, err := Set(sheet.NewBook(), []Entry{{Ref: "A1", Input: "1", Value: Value("1")}}, SetOptions{}); err == nil {
		t.Error("input and value both taken")
	}
}

// A format alone formats a range; with an input it formats the cell.
func TestEntryFormats(t *testing.T) {
	w := sheet.NewBook()
	s := w.Sheet(0)
	_, err := Set(w, []Entry{{Ref: "A1", Input: "3.5"}, {Ref: "A2", Input: "4"}, {Ref: "A1:A2", Format: `"$"#,##0.00`}, {Ref: "B1", Input: "0.25", Format: "0.0%"}}, SetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for a, want := range map[string]string{"A1": "$3.50", "A2": "$4.00", "B1": "25.0%"} {
		if got := s.LocalText(addr(a)); got != want {
			t.Errorf("%s shows %q, want %q", a, got, want)
		}
	}
}

// A table written as records gets a header, typed cells and column
// formats, blanks and plain numbers taking their column's.
func TestWriteTable(t *testing.T) {
	w := sheet.NewBook()
	s := w.Sheet(0)
	rows := []Row{
		Row(`{"Item": "Tea", "Price": {"currency": 3.5}, "Bought": {"date": "2026-09-29"}, "Code": "00123"}`),
		Row(`{"Item": "Cake", "Price": 3, "Code": "7", "Share": 0.5}`),
		Row(`{Item: Jam, Price: null, Bought: 2026-09-28T10:00:00Z, Size: 1.5kb}`),
	}
	warn, err := WriteTable(w, TableSpec{At: "B2", Rows: rows, Formats: map[string]string{"Share": "0%"}}, SetOptions{})
	if err != nil || len(warn) != 0 {
		t.Fatal(err, warn)
	}
	for a, want := range map[string]string{"B2": "Item", "C2": "Price", "G2": "Size", "C3": "$3.50", "C4": "$3.00", "D3": "9/29/2026",
		"E3": "00123", "E4": "7", "F4": "50%", "G5": "1.5 kB"} {
		if got := s.LocalText(addr(a)); got != want {
			t.Errorf("%s shows %q, want %q", a, got, want)
		}
	}
	if f := s.DisplayFormat(addr("C5")); f.Kind != sheet.FmtCurrency {
		t.Errorf("the blank price is formatted %+v", f)
	}
	if f := s.DisplayFormat(addr("D5")); f.Kind != sheet.FmtDateTime {
		t.Errorf("a date and time in a column of dates is formatted %+v", f)
	}
	if v := s.Value(addr("E4")); v.Kind != sheet.Text {
		t.Errorf("text that looks like a number is %+v", v)
	}
	if _, err := WriteTable(w, TableSpec{At: "A1", Rows: []Row{Row(`[1]`)}}, SetOptions{}); err == nil || !strings.Contains(err.Error(), "row 1 isn't a record") {
		t.Errorf("a row that isn't a record: %v", err)
	}
}
