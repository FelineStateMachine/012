package sheet

import (
	"strings"
	"testing"
)

func TestLinesRoundTrip(t *testing.T) {
	c := Chart{Type: ChartLine, Data: Rect{To: Addr{Col: 2, Row: 6}}, At: Addr{Col: 4, Row: 1}, W: 44, H: 14, Header: true, Title: "Sales"}
	c.Legend = LegendNone
	got, err := ParseChart(c.JSON())
	if err != nil || got.JSON() != c.JSON() {
		t.Fatalf("chart %s came back %s, %v", c.JSON(), got.JSON(), err)
	}
	if _, err := ParseChart(`{"type":"blimp","data":"A1:B2","at":"C1"}`); err == nil {
		t.Error("a chart of an unknown type was read")
	}

	p := Pivot{Source: "Q3 plan", Range: Rect{To: Addr{Col: 3, Row: 9}}, Rows: []PivotGroup{{Col: 1, Desc: true}},
		Values: []PivotValue{{Col: 3, Summarize: SumBy}}, Filters: []PivotFilter{{Col: 0, Criteria: Criteria{Hidden: []string{"North"}}}}, RowTotals: true}
	back, err := ParsePivot(p.JSON())
	if err != nil || back.JSON() != p.JSON() || !strings.HasPrefix(p.JSON(), `{"source":"'Q3 plan'!A1:D10"`) {
		t.Fatalf("pivot %s came back %s, %v", p.JSON(), back.JSON(), err)
	}

	cr := Criteria{Hidden: []string{"", "x"}, Cond: Condition{Op: CondGreater, Arg: "3"}}
	if cr.JSON() != `{"hidden":["","x"],"condition":"gt","value":"3"}` {
		t.Fatalf("criteria %s", cr.JSON())
	}
	crBack, err := ParseCriteria(cr.JSON())
	if err != nil || crBack.JSON() != cr.JSON() {
		t.Fatalf("criteria came back %s, %v", crBack.JSON(), err)
	}
	if empty, _ := ParseCriteria(`{"condition":"empty","value":"ignored"}`); empty.Cond.Arg != "" {
		t.Errorf("a condition without a value kept %q", empty.Cond.Arg)
	}
}
