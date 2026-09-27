package sheet

import (
	"strings"
	"testing"
)

// The hooks pair up like parentheses, a recalculation holding the pivot
// refreshes it causes, and get the workbook's trace back.
func TestTelemetryHooksNest(t *testing.T) {
	w := sales(t)
	newPivot(t, w, func(p *Pivot) {
		p.Rows = []PivotGroup{{Col: 0}}
		p.Values = []PivotValue{{Col: 3, Summarize: SumBy}}
	})
	trace := new(int)
	w.SetTrace(trace)
	var got []string
	var traces []any
	OnBegin = func(tr any, op string) { got, traces = append(got, "("+op), append(traces, tr) }
	OnRecalc = func(tr any, _ RecalcInfo) { got, traces = append(got, "recalc)"), append(traces, tr) }
	OnPivot = func(tr any, _ PivotInfo) { got, traces = append(got, "pivot)"), append(traces, tr) }
	defer func() { OnBegin, OnRecalc, OnPivot = nil, nil, nil }()
	w.Lookup("Sales").Set(at("D2"), "100")
	want := "(recalc (pivot pivot) (recalc recalc) recalc)"
	if s := strings.Join(got, " "); s != want {
		t.Errorf("hooks ran %s, want %s", s, want)
	}
	for _, tr := range traces {
		if tr != trace {
			t.Errorf("hook got trace %v, want the workbook's", tr)
		}
	}
}
