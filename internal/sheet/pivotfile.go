package sheet

import (
	"encoding/json"
	"fmt"

	"github.com/FelineStateMachine/012/internal/formula"
)

// filePivot is a pivot table in the file (version 5): its definition, on
// one line after the cells of its sheet, never its results, which are
// computed again on loading:
//
//	"pivot": {"source": "Sales!A1:D200", "rows": [{"column": "B"}],
//	  "values": [{"column": "D", "summarize": "sum"}], "rowTotals": true, "columnTotals": true}
type filePivot struct {
	Source       string            `json:"source"`
	Rows         []filePivotGroup  `json:"rows,omitempty"`
	Columns      []filePivotGroup  `json:"columns,omitempty"`
	Values       []filePivotValue  `json:"values,omitempty"`
	Filters      []filePivotFilter `json:"filters,omitempty"`
	RowTotals    bool              `json:"rowTotals"`
	ColumnTotals bool              `json:"columnTotals"`
}

type filePivotGroup struct {
	Column string `json:"column"`
	Order  string `json:"order,omitempty"`  // "desc" for Z to A
	SortBy int    `json:"sortBy,omitempty"` // by the total of this value, from 1
}

type filePivotValue struct {
	Column    string `json:"column"`
	Summarize string `json:"summarize"`
	ShowAs    string `json:"showAs,omitempty"`
	Name      string `json:"name,omitempty"`
}

type filePivotFilter struct {
	Column string `json:"column"`
	fileCriteria
}

// hasPivots reports whether any sheet has a pivot table, which needs
// version 5.
func (w *Workbook) hasPivots() bool {
	for _, s := range w.sheets {
		if s.pivot.def != nil {
			return true
		}
	}
	return false
}

func encodePivot(p *Pivot) ([]byte, error) {
	rng := p.Range.String()
	if p.Lost {
		rng = "#REF!"
	}
	fp := filePivot{Source: formula.QuoteSheet(p.Source) + "!" + rng, RowTotals: p.RowTotals, ColumnTotals: p.ColumnTotals}
	groups := func(gs []PivotGroup) []filePivotGroup {
		var out []filePivotGroup
		for _, g := range gs {
			fg := filePivotGroup{Column: ColName(g.Col), SortBy: g.SortBy}
			if g.Desc {
				fg.Order = "desc"
			}
			out = append(out, fg)
		}
		return out
	}
	fp.Rows, fp.Columns = groups(p.Rows), groups(p.Columns)
	for _, v := range p.Values {
		fp.Values = append(fp.Values, filePivotValue{Column: ColName(v.Col), Summarize: v.Summarize.String(), ShowAs: v.ShowAs.String(), Name: v.Name})
	}
	for _, f := range p.Filters {
		cr := f.Criteria
		fp.Filters = append(fp.Filters, filePivotFilter{Column: ColName(f.Col),
			fileCriteria: fileCriteria{Hidden: cr.Hidden, Condition: cr.Cond.Op.String(), Value: cr.Cond.Arg}})
	}
	return json.Marshal(fp)
}

func decodePivot(fp filePivot) (*Pivot, error) {
	sheet, rest := SplitSheet(fp.Source)
	if sheet == "" {
		return nil, fmt.Errorf("pivot source %q names no sheet", fp.Source)
	}
	p := &Pivot{Source: sheet, RowTotals: fp.RowTotals, ColumnTotals: fp.ColumnTotals, Lost: rest == "#REF!"}
	if !p.Lost {
		r, ok := ParseRange(rest)
		if !ok {
			return nil, fmt.Errorf("invalid pivot source %q", fp.Source)
		}
		p.Range = r
	}
	var err error
	if p.Rows, err = decodeGroups(fp.Rows); err != nil {
		return nil, err
	}
	if p.Columns, err = decodeGroups(fp.Columns); err != nil {
		return nil, err
	}
	for _, fv := range fp.Values {
		v, err := decodeValue(fv)
		if err != nil {
			return nil, err
		}
		p.Values = append(p.Values, v)
	}
	for _, ff := range fp.Filters {
		col, ok := ParseCol(ff.Column)
		if !ok {
			return nil, fmt.Errorf("invalid pivot filter column %q", ff.Column)
		}
		op, ok := ParseCondOp(ff.Condition)
		if !ok {
			return nil, fmt.Errorf("unknown pivot filter condition %q", ff.Condition)
		}
		p.Filters = append(p.Filters, PivotFilter{Col: col, Criteria: Criteria{Hidden: ff.Hidden, Cond: Condition{Op: op, Arg: ff.Value}}})
	}
	return p, nil
}

func decodeGroups(fgs []filePivotGroup) ([]PivotGroup, error) {
	var out []PivotGroup
	for _, fg := range fgs {
		col, ok := ParseCol(fg.Column)
		if !ok {
			return nil, fmt.Errorf("invalid pivot column %q", fg.Column)
		}
		out = append(out, PivotGroup{Col: col, Desc: fg.Order == "desc", SortBy: max(fg.SortBy, 0)})
	}
	return out, nil
}

func decodeValue(fv filePivotValue) (PivotValue, error) {
	col, ok := ParseCol(fv.Column)
	if !ok {
		return PivotValue{}, fmt.Errorf("invalid pivot value column %q", fv.Column)
	}
	f, ok := ParseSummarize(fv.Summarize)
	if !ok {
		return PivotValue{}, fmt.Errorf("unknown pivot summary %q", fv.Summarize)
	}
	show, ok := ParseShowAs(fv.ShowAs)
	if !ok {
		return PivotValue{}, fmt.Errorf("unknown pivot show as %q", fv.ShowAs)
	}
	return PivotValue{Col: col, Summarize: f, ShowAs: show, Name: fv.Name}, nil
}
