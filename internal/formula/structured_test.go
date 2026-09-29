package formula

import (
	"strings"
	"testing"
)

// Structured references parse into what they name and print back in
// 012's spelling; Excel's file form spells the formula's row #This Row.
func TestTableRefParse(t *testing.T) {
	cases := []struct {
		in     string
		want   TableRef
		prints string
		excel  string
	}{
		{"Sales[Amount]", TableRef{Table: "Sales", From: "Amount"}, "Sales[Amount]", "Sales[Amount]"},
		{"sales[ Amount ]", TableRef{Table: "sales", From: "Amount"}, "sales[Amount]", "sales[Amount]"},
		{"Sales[]", TableRef{Table: "Sales"}, "Sales[]", "Sales[]"},
		{"Sales[#All]", TableRef{Table: "Sales", Items: ItemAll}, "Sales[#All]", "Sales[#All]"},
		{"Sales[#headers]", TableRef{Table: "Sales", Items: ItemHeaders}, "Sales[#Headers]", "Sales[#Headers]"},
		{"Sales[#This Row]", TableRef{Table: "Sales", Items: ItemThisRow}, "Sales[@]", "Sales[#This Row]"},
		{"Sales[@Amount]", TableRef{Table: "Sales", Items: ItemThisRow, From: "Amount"}, "Sales[@Amount]", "Sales[[#This Row],[Amount]]"},
		{"Sales[@[Unit Price]]", TableRef{Table: "Sales", Items: ItemThisRow, From: "Unit Price"}, "Sales[@[Unit Price]]", "Sales[[#This Row],[Unit Price]]"},
		{"Sales[[#This Row],[Amount]]", TableRef{Table: "Sales", Items: ItemThisRow, From: "Amount"}, "Sales[@Amount]", "Sales[[#This Row],[Amount]]"},
		{"Sales[[#Headers],[Amount]]", TableRef{Table: "Sales", Items: ItemHeaders, From: "Amount"}, "Sales[[#Headers],[Amount]]", "Sales[[#Headers],[Amount]]"},
		{"Sales[[#Headers];[#Data];[Amount]:[Units]]", TableRef{Table: "Sales", Items: ItemHeaders | ItemData, From: "Amount", To: "Units"}, "Sales[[#Headers],[#Data],[Amount]:[Units]]", "Sales[[#Headers],[#Data],[Amount]:[Units]]"},
		{"Sales[[Amount]:[Units]]", TableRef{Table: "Sales", From: "Amount", To: "Units"}, "Sales[[Amount]:[Units]]", "Sales[[Amount]:[Units]]"},
		{"Sales[[Total, EUR]]", TableRef{Table: "Sales", From: "Total, EUR"}, "Sales[[Total, EUR]]", "Sales[[Total, EUR]]"},
		{"Sales['#Items]", TableRef{Table: "Sales", From: "#Items"}, "Sales[['#Items]]", "Sales[['#Items]]"},
		{"Sales[[Q'[1']]]", TableRef{Table: "Sales", From: "Q[1]"}, "Sales[[Q'[1']]]", "Sales[[Q'[1']]]"},
		{"Sales[Größe]", TableRef{Table: "Sales", From: "Größe"}, "Sales[Größe]", "Sales[Größe]"},
		{"app[status]", TableRef{Table: "app", From: "status"}, "app[status]", "app[status]"},
	}
	for _, c := range cases {
		n, err := Parse("="+c.in, testFuncs)
		if err != nil {
			t.Errorf("%s: %v", c.in, err)
			continue
		}
		got, ok := n.(TableRef)
		if !ok || got != c.want {
			t.Errorf("%s = %#v, want %#v", c.in, n, c.want)
			continue
		}
		if p := Text(n); p != "="+c.prints {
			t.Errorf("%s prints %s, want %s", c.in, p, c.prints)
		}
		if back, err := Parse(Text(n), testFuncs); err != nil || back != n {
			t.Errorf("%s: printed %s parses as %#v, %v", c.in, Text(n), back, err)
		}
		if x := got.Excel(); x != c.excel {
			t.Errorf("%s in Excel's form is %s, want %s", c.in, x, c.excel)
		}
	}
}

// Structured references take part in expressions like any reference.
func TestTableRefInFormulas(t *testing.T) {
	n, err := Parse("=SUM(Sales[Amount])*Sales[@Qty]+nu.app", testFuncs)
	if err != nil {
		t.Fatal(err)
	}
	var tables []string
	WalkTables(n, func(r TableRef) { tables = append(tables, r.String()) })
	if strings.Join(tables, " ") != "Sales[Amount] Sales[@Qty]" {
		t.Errorf("walked %v", tables)
	}
	out, changed := Rewrite(n, Rewriter{Table: func(r TableRef) Node {
		r.Table = "Revenue"
		return r
	}})
	if !changed || Text(out) != "=SUM(Revenue[Amount])*Revenue[@Qty]+nu.app" {
		t.Errorf("renamed to %s", Text(out))
	}
}

func TestTableRefErrors(t *testing.T) {
	for in, msg := range map[string]string{
		"=Sales[Amount":                 "Missing ]",
		"=Sales[#Everything]":           "Unknown table item #Everything",
		"=Sales[[#Nope],[A]]":           "Unknown table item #Nope",
		"=Sales[[#This Row],[#Data]]":   "#This Row can't be combined",
		"=Sales[[#Headers],[#Totals]]":  "#Headers and #Totals need #Data",
		"=Sales[[A],[B]]":               "Name one column",
		"=Sales[[A] [B]]":               "Expected , between",
		"=Sales[[A]:B]":                 "Expected [column] after :",
		"=Sales[@[#Headers]]":           "Only columns can follow @",
		"=Sales[[A]]x":                  "Unexpected",
		"=Sales[[A]]]":                  "Unexpected",
		"=SUM(Sales[[#Data],[A]:[B]]":   "Expected , or )",
		"=Sales[Amount]:Sales[Units]+1": "Unexpected",
	} {
		_, err := Parse(in, testFuncs)
		if err == nil || !strings.Contains(err.Error(), msg) {
			t.Errorf("%s: %v, want %q", in, err, msg)
		}
	}
}

// Separators between a reference's items follow the locale; names in
// brackets are left as they are.
func TestTableRefLocale(t *testing.T) {
	de := mustLocale(t, "de-DE")
	typed := "=SUMME(Sales[[#Headers];[Total, EUR]];1,5)"
	stored := Delocalize(typed, de)
	if stored != "=SUMME(Sales[[#Headers],[Total, EUR]],1.5)" {
		t.Errorf("stored %s", stored)
	}
	if back := Localize(stored, de); back != typed {
		t.Errorf("shown %s", back)
	}
}
