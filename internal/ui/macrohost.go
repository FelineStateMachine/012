package ui

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/FelineStateMachine/012/internal/macro"
	"github.com/FelineStateMachine/012/internal/sheet"
)

// scriptHost is what a running macro acts on: the model, through the
// same paths a user's keys take (entries, the selection, commands). Its
// methods run on the UI goroutine, inside the run's undo step; see
// macrorun.go.
type scriptHost struct {
	m *Model
}

var _ macro.Host = scriptHost{}

// maxReadCells bounds what get and get_formula return at once.
const maxReadCells = 1 << 18

// resolve finds the sheet and range a script means: "" for the
// selection, A1 or A1:B3, whole columns (B:D) or rows (3:5), or a named
// range, any of them after a sheet name.
func (h scriptHost) resolve(ref string) (*sheet.Sheet, sheet.Rect, error) {
	m := h.m
	if strings.TrimSpace(ref) == "" {
		return m.sheet, m.selection(), nil
	}
	name, rest := sheet.SplitSheet(ref)
	s := m.sheet
	if name != "" {
		if s = m.book().Lookup(name); s == nil {
			return nil, sheet.Rect{}, fmt.Errorf("there's no sheet named %s", name)
		}
	}
	if r, ok := parseWhole(rest); ok {
		return s, r, nil
	}
	if n, ok := m.sheet.LookupName(rest); ok && name == "" {
		if n.Gone() {
			return nil, sheet.Rect{}, fmt.Errorf("the range named %s was deleted", n.Name)
		}
		return n.Sheet, n.Range, nil
	}
	return nil, sheet.Rect{}, fmt.Errorf("not a cell or range: %s", ref)
}

// parseWhole parses A1, A1:B3, whole columns B:D and whole rows 3:5.
func parseWhole(s string) (sheet.Rect, bool) {
	if r, ok := sheet.ParseRange(s); ok {
		return r, true
	}
	from, to, ok := strings.Cut(strings.TrimSpace(s), ":")
	if !ok {
		return sheet.Rect{}, false
	}
	if a, ok1 := sheet.ParseCol(strings.ToUpper(from)); ok1 {
		if b, ok2 := sheet.ParseCol(strings.ToUpper(to)); ok2 {
			return sheet.NewRect(sheet.Addr{Col: a}, sheet.Addr{Col: b, Row: sheet.MaxRows - 1}), true
		}
	}
	a, err1 := strconv.Atoi(from)
	b, err2 := strconv.Atoi(to)
	if err1 != nil || err2 != nil || a < 1 || b < 1 || a > sheet.MaxRows || b > sheet.MaxRows {
		return sheet.Rect{}, false
	}
	return sheet.NewRect(sheet.Addr{Row: a - 1}, sheet.Addr{Col: sheet.MaxCols - 1, Row: b - 1}), true
}

// wholeRef writes r as a script would: B:D for whole columns, 3:5 for
// whole rows, otherwise A1 or A1:B3.
func wholeRef(r sheet.Rect) string {
	allRows := r.From.Row == 0 && r.To.Row == sheet.MaxRows-1
	allCols := r.From.Col == 0 && r.To.Col == sheet.MaxCols-1
	switch {
	case allRows && !allCols:
		return sheet.ColName(r.From.Col) + ":" + sheet.ColName(r.To.Col)
	case allCols && !allRows:
		return strconv.Itoa(r.From.Row+1) + ":" + strconv.Itoa(r.To.Row+1)
	}
	return r.String()
}

// readRange is r to read: a large range (whole columns, say) is trimmed
// to its last row and column with contents. Small ones are read whole,
// blanks and all, as written.
func readRange(s *sheet.Sheet, r sheet.Rect) (sheet.Rect, error) {
	if (r.To.Col-r.From.Col+1)*(r.To.Row-r.From.Row+1) <= 1<<12 {
		return r, nil
	}
	data, ok := s.FilledBounds(r)
	r.To = r.From
	if ok {
		r.To = data.To
	}
	if (r.To.Col-r.From.Col+1)*(r.To.Row-r.From.Row+1) > maxReadCells {
		return r, fmt.Errorf("%s has more than %d cells to read at once", r, maxReadCells)
	}
	return r, nil
}

// each calls fn for the cells of ref, row by row, to read them.
func (h scriptHost) each(ref string, fn func(s *sheet.Sheet, a sheet.Addr, row int)) error {
	s, r, err := h.resolve(ref)
	if err != nil {
		return err
	}
	if r, err = readRange(s, r); err != nil {
		return err
	}
	for row := r.From.Row; row <= r.To.Row; row++ {
		for col := r.From.Col; col <= r.To.Col; col++ {
			fn(s, sheet.Addr{Col: col, Row: row}, row-r.From.Row)
		}
	}
	return nil
}

func (h scriptHost) Values(ref string) ([][]any, error) {
	var rows [][]any
	err := h.each(ref, func(s *sheet.Sheet, a sheet.Addr, row int) {
		if row == len(rows) {
			rows = append(rows, nil)
		}
		rows[row] = append(rows[row], scriptValue(s.Value(a)))
	})
	return rows, err
}

// scriptValue is a value as scripts see it.
func scriptValue(v sheet.Value) any {
	switch v.Kind {
	case sheet.Number:
		return v.Num
	case sheet.Text, sheet.Error:
		return v.Str
	case sheet.Bool:
		return v.Num != 0
	}
	return nil
}

func (h scriptHost) Formulas(ref string) ([][]string, error) {
	var rows [][]string
	err := h.each(ref, func(s *sheet.Sheet, a sheet.Addr, row int) {
		if row == len(rows) {
			rows = append(rows, nil)
		}
		f := ""
		if c := s.Cell(a); c != nil && c.IsFormula() {
			f = c.Input
		}
		rows[row] = append(rows[row], f)
	})
	return rows, err
}

func (h scriptHost) SetInput(ref, input string) error {
	s, r, err := h.resolve(ref)
	if err != nil {
		return err
	}
	if n := (r.To.Col - r.From.Col + 1) * (r.To.Row - r.From.Row + 1); n > maxReadCells {
		return fmt.Errorf("%s has more than %d cells to set at once", ref, maxReadCells)
	}
	return s.Batch(sheet.Change{Label: "set " + r.String(), Focus: r}, func() error {
		for row := r.From.Row; row <= r.To.Row; row++ {
			for col := r.From.Col; col <= r.To.Col; col++ {
				if err := s.Set(sheet.Addr{Col: col, Row: row}, input); err != nil {
					return fmt.Errorf("%s: %w", sheet.Addr{Col: col, Row: row}, err)
				}
			}
		}
		return nil
	})
}

func (h scriptHost) SetInputs(ref string, rows [][]string) error {
	s, r, err := h.resolve(ref)
	if err != nil {
		return err
	}
	return s.Batch(sheet.Change{Label: "set " + r.String(), Focus: r}, func() error {
		for i, row := range rows {
			for j, input := range row {
				a := sheet.Addr{Col: r.From.Col + j, Row: r.From.Row + i}
				if !a.Valid() {
					return fmt.Errorf("the values run off the sheet at row %d, column %d", i+1, j+1)
				}
				if err := s.Set(a, input); err != nil {
					return fmt.Errorf("%s: %w", a, err)
				}
			}
		}
		return nil
	})
}

func (h scriptHost) SetFormula(ref, f string) error {
	s, r, err := h.resolve(ref)
	if err != nil {
		return err
	}
	return s.FillEntry(r, r.From, f)
}

func (h scriptHost) Clear(ref string) error {
	s, r, err := h.resolve(ref)
	if err != nil {
		return err
	}
	s.EraseRange(r)
	return nil
}

func (h scriptHost) NumberFormat(ref string) (string, int, error) {
	s, r, err := h.resolve(ref)
	if err != nil {
		return "", 0, err
	}
	f := s.DisplayFormat(r.From)
	return f.Kind.String(), f.Decimals, nil
}

func (h scriptHost) SetNumberFormat(ref, kind string, decimals int, pattern string) error {
	s, r, err := h.resolve(ref)
	if err != nil {
		return err
	}
	k, ok := sheet.ParseFormatKind(kind)
	if !ok {
		return fmt.Errorf("no number format %q; try number, percent, currency, date, time or custom", kind)
	}
	f := sheet.Preset(k)
	if decimals >= 0 {
		f.Decimals = min(decimals, sheet.MaxDecimals)
	}
	if k == sheet.FmtCustom && pattern == "" {
		return errors.New("a custom format needs a pattern, e.g. pattern=\"0.0%\"")
	}
	f.Pattern = pattern
	s.SetFormat(r, f)
	return nil
}
