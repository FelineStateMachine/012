package sheet

import (
	"encoding/json"
	"fmt"
)

// A chart, a pivot table and a filter column's criteria as the lines of
// the file that hold them, to be written and read one at a time: macros
// answer the dialogs that make them with these (see
// docs/reference/macro-api.md), as they answer the rules panel with a
// rule's line.

// JSON is the chart as a line of the file.
func (c Chart) JSON() string {
	raw, _ := encodeChart(c)
	return string(raw)
}

// ParseChart reads a chart written by JSON.
func ParseChart(line string) (Chart, error) {
	var fc fileChart
	if err := json.Unmarshal([]byte(line), &fc); err != nil {
		return Chart{}, err
	}
	return decodeChart(fc)
}

// JSON is the pivot table's definition as a line of the file.
func (p Pivot) JSON() string {
	raw, _ := encodePivot(&p)
	return string(raw)
}

// ParsePivot reads a pivot table's definition written by JSON.
func ParsePivot(line string) (Pivot, error) {
	var fp filePivot
	if err := json.Unmarshal([]byte(line), &fp); err != nil {
		return Pivot{}, err
	}
	p, err := decodePivot(fp)
	if err != nil {
		return Pivot{}, err
	}
	return *p, nil
}

// JSON is the criteria as a filter column's are written in the file:
// the values hidden, and a condition and its value.
func (cr Criteria) JSON() string {
	raw, _ := json.Marshal(fileCriteria{Hidden: cr.Hidden, Condition: cr.Cond.Op.String(), Value: cr.Cond.Arg})
	return string(raw)
}

// ParseCriteria reads criteria written by JSON.
func ParseCriteria(line string) (Criteria, error) {
	var fc fileCriteria
	if err := json.Unmarshal([]byte(line), &fc); err != nil {
		return Criteria{}, err
	}
	op, ok := ParseCondOp(fc.Condition)
	if !ok {
		return Criteria{}, fmt.Errorf("unknown filter condition %q", fc.Condition)
	}
	cr := Criteria{Hidden: fc.Hidden, Cond: Condition{Op: op, Arg: fc.Value}}
	if !op.TakesArg() {
		cr.Cond.Arg = ""
	}
	return cr, nil
}
