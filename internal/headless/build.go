package headless

import (
	"fmt"
	"strings"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// ChartSpec is a chart to add, as Insert > Chart makes one: over Data,
// with the header row, category labels and title guessed as Sheets
// guesses them unless given.
type ChartSpec struct {
	Data   string `json:"data" jsonschema:"the range to chart, e.g. Q3!A1:C9"`
	Type   string `json:"type,omitempty" jsonschema:"column, bar, line, pie, area or scatter; column when empty"`
	Title  string `json:"title,omitempty" jsonschema:"the title; guessed from the series when empty"`
	At     string `json:"at,omitempty" jsonschema:"the cell under the chart's top-left corner; right of the data when empty"`
	Width  int    `json:"width,omitempty" jsonschema:"the width in terminal cells, 20 to 240; 60 when 0"`
	Height int    `json:"height,omitempty" jsonschema:"the height in terminal cells, 8 to 120; 18 when 0"`
	ByRow  bool   `json:"by_row,omitempty" jsonschema:"series run along rows rather than down columns"`
	Stack  string `json:"stack,omitempty" jsonschema:"stacked or percent, for column, bar and area charts"`
}

// ChartMade says which chart AddChart made: its sheet and number, as
// describe lists it.
type ChartMade struct {
	Sheet  string `json:"sheet"`
	Number int    `json:"number"`
	Title  string `json:"title"`
}

// AddChart adds a chart as spec says.
func AddChart(w *sheet.Workbook, spec ChartSpec) (ChartMade, error) {
	t, err := Resolve(w, spec.Data)
	if err != nil {
		return ChartMade{}, err
	}
	s, r := t.Sheet, targetRect(t)
	c := s.GuessChart(r)
	if spec.Type != "" {
		typ, ok := sheet.ParseChartType(spec.Type)
		if !ok {
			return ChartMade{}, fmt.Errorf("no chart type %q: column, bar, line, pie, area or scatter", spec.Type)
		}
		c.Type = typ
	}
	if spec.Title != "" {
		c.Title = spec.Title
	}
	c.ByRow = spec.ByRow
	if c.Stack, err = parseStack(spec.Stack); err != nil {
		return ChartMade{}, err
	}
	c.W, c.H = orDefault(spec.Width, 60), orDefault(spec.Height, 18)
	c.At = sheet.Addr{Col: min(r.To.Col+2, sheet.MaxCols-1), Row: r.From.Row}
	if spec.At != "" {
		cs, a, err := ResolveCell(w, spec.At)
		if err != nil {
			return ChartMade{}, err
		}
		if cs != s {
			return ChartMade{}, fmt.Errorf("a chart sits on its data's sheet, %s", s.Name())
		}
		c.At = a
	}
	i := s.AddChart(c)
	return ChartMade{Sheet: s.Name(), Number: i + 1, Title: ChartTitle(s.Charts()[i])}, nil
}

func parseStack(s string) (sheet.ChartStack, error) {
	switch strings.ToLower(s) {
	case "", "none":
		return sheet.StackNone, nil
	case "stacked":
		return sheet.StackNormal, nil
	case "percent":
		return sheet.StackPercent, nil
	}
	return 0, fmt.Errorf("stack %q: stacked or percent", s)
}

func orDefault(v, d int) int {
	if v <= 0 {
		return d
	}
	return v
}

// PivotSpec is a pivot table to add, as Insert > Pivot table makes one:
// a new sheet summarizing Source, whose first row names its columns.
type PivotSpec struct {
	Source  string       `json:"source" jsonschema:"the data, header row included, e.g. Sales!A1:F99"`
	Name    string       `json:"name,omitempty" jsonschema:"the new sheet's name; Pivot Table N when empty"`
	Rows    []string     `json:"rows,omitempty" jsonschema:"columns whose values become rows, by header or letter"`
	Columns []string     `json:"columns,omitempty" jsonschema:"columns whose values become columns, by header or letter"`
	Values  []PivotValue `json:"values" jsonschema:"columns to summarize"`
	Totals  bool         `json:"totals,omitempty" jsonschema:"add grand totals"`
}

// PivotValue is a column to summarize, and how.
type PivotValue struct {
	Column    string `json:"column" jsonschema:"by header or letter"`
	Summarize string `json:"summarize,omitempty" jsonschema:"sum, counta, count, countunique, average, max or min; sum when empty"`
}

// AddPivot adds a pivot table's sheet after its source's, returning its
// name.
func AddPivot(w *sheet.Workbook, spec PivotSpec) (string, error) {
	t, err := Resolve(w, spec.Source)
	if err != nil {
		return "", err
	}
	src, r := t.Sheet, targetRect(t)
	p := sheet.Pivot{RowTotals: spec.Totals, ColumnTotals: spec.Totals}
	for _, name := range spec.Rows {
		col, err := columnOf(src, r, name, true)
		if err != nil {
			return "", err
		}
		p.Rows = append(p.Rows, sheet.PivotGroup{Col: col})
	}
	for _, name := range spec.Columns {
		col, err := columnOf(src, r, name, true)
		if err != nil {
			return "", err
		}
		p.Columns = append(p.Columns, sheet.PivotGroup{Col: col})
	}
	if len(spec.Values) == 0 {
		return "", fmt.Errorf("name a column to summarize")
	}
	for _, v := range spec.Values {
		col, err := columnOf(src, r, v.Column, true)
		if err != nil {
			return "", err
		}
		sum := sheet.SumBy
		if v.Summarize != "" {
			var ok bool
			if sum, ok = sheet.ParseSummarize(strings.ToLower(v.Summarize)); !ok {
				return "", fmt.Errorf("summarize %q: sum, counta, count, countunique, average, max or min", v.Summarize)
			}
		}
		p.Values = append(p.Values, sheet.PivotValue{Col: col, Summarize: sum})
	}
	s, err := w.CreatePivot(src, r, spec.Name, p)
	if err != nil {
		return "", err
	}
	return s.Name(), nil
}
