package sheet

import (
	"fmt"
	"strings"

	"github.com/FelineStateMachine/012/internal/notebook"
)

// The edits TestRandomEdits draws from, each one undo step (or none,
// when it's refused), described for the log.
var randEdits = []func(r *randBook, e *edits) string{
	(*randBook).editSet, (*randBook).editSet, (*randBook).editSet,
	(*randBook).editErase, (*randBook).editPaste, (*randBook).editFill,
	(*randBook).editSeries, (*randBook).editSort, (*randBook).editInsert,
	(*randBook).editDelete, (*randBook).editFormat, (*randBook).editStyle,
	(*randBook).editMerge, (*randBook).editMove, (*randBook).editLayout,
	(*randBook).editNote, (*randBook).editRule, (*randBook).editRegion,
	(*randBook).editSheets, (*randBook).editView,
}

func (r *randBook) edit(e *edits) string {
	return randEdits[e.n(len(randEdits))](r, e)
}

// sheet picks one of the grid sheets (not the notebook nor a source) to
// edit.
func (r *randBook) sheet(e *edits) *Sheet {
	var grids []*Sheet
	for _, s := range r.wb.sheets {
		if !s.IsNotebook() && !s.IsSource() {
			grids = append(grids, s)
		}
	}
	return grids[e.n(len(grids))]
}

// input is something to type into a cell: numbers, text, dates and
// formulas that reach across ranges, sheets, names, regions and spills.
func (r *randBook) input(e *edits) string {
	ref := func() string {
		a := e.addr()
		if e.n(4) == 0 {
			return "$" + ColName(a.Col) + "$" + fmt.Sprint(a.Row+1)
		}
		return a.String()
	}
	rng := func() string { return e.rect().String() }
	other := []string{"Sheet1", "S2"}[e.n(2)]
	forms := []func() string{
		func() string { return fmt.Sprint(e.n(20) - 5) },
		func() string { return fmt.Sprintf("%d.5", e.n(10)) },
		func() string { return []string{"a", "b", "x y", "TRUE", "2026-01-05", "10%"}[e.n(6)] },
		func() string { return "=" + ref() + "+" + ref() },
		func() string { return "=SUM(" + rng() + ")" },
		func() string { return "=" + other + "!" + ref() + "*2" },
		func() string { return "=SUM(" + other + "!" + rng() + ")" },
		func() string { return "=IFERROR(" + ref() + "/" + ref() + ",0)" },
		func() string { return `=COUNTIF(` + rng() + `,">2")` },
		func() string { return "=SEQUENCE(" + fmt.Sprint(1+e.n(3)) + ")" },
		func() string { return "=SORT(" + rng() + ")" },
		func() string { return "=VLOOKUP(" + ref() + "," + rng() + ",1,FALSE)" },
		func() string { return "=SUM(nu.r1)" },
		func() string { return "=SUM(Total)*2" },
		func() string { return "=INDEX(" + rng() + ",1,1)" },
		func() string { return "=MAX(" + ColName(e.n(randCols)) + ":" + ColName(e.n(randCols)) + ")" },
		func() string { return "=CONCAT(" + ref() + "," + ref() + ")" },
		func() string { return "=ABS(" + ref() + ")+1" },
		func() string { return "=" + ref() + "&\"\"" },
		func() string { return "" },
	}
	return forms[e.n(len(forms))]()
}

func (r *randBook) editSet(e *edits) string {
	s, a, in := r.sheet(e), e.addr(), r.input(e)
	err := s.Set(a, in)
	return fmt.Sprintf("%s!%s = %q (%v)", s.name, a, in, err)
}

func (r *randBook) editErase(e *edits) string {
	s, rg := r.sheet(e), e.rect()
	s.EraseRange(rg)
	return fmt.Sprintf("erase %s!%s", s.name, rg)
}

func (r *randBook) editPaste(e *edits) string {
	src, dst := r.sheet(e), r.sheet(e)
	from, to := e.rect(), e.rect()
	values := e.n(3) == 0
	_, err := dst.Paste(src.Copy(from), to, values)
	return fmt.Sprintf("copy %s!%s, paste (values %v) %s!%s (%v)", src.name, from, values, dst.name, to, err)
}

func (r *randBook) editFill(e *edits) string {
	s, rg := r.sheet(e), e.rect()
	var err error
	if e.n(2) == 0 {
		_, err = s.FillDown(rg)
		return fmt.Sprintf("fill down %s!%s (%v)", s.name, rg, err)
	}
	_, err = s.FillRight(rg)
	return fmt.Sprintf("fill right %s!%s (%v)", s.name, rg, err)
}

func (r *randBook) editSeries(e *edits) string {
	s, src := r.sheet(e), e.rect()
	dst := src
	if e.n(2) == 0 {
		dst.To.Row = min(dst.To.Row+1+e.n(6), randRows-1)
	} else {
		dst.To.Col = min(dst.To.Col+1+e.n(3), randCols-1)
	}
	_, err := s.FillSeries(src, dst)
	return fmt.Sprintf("series %s!%s to %s (%v)", s.name, src, dst, err)
}

func (r *randBook) editSort(e *edits) string {
	s, rg := r.sheet(e), e.rect()
	keys := []SortKey{{Col: rg.From.Col + e.n(rg.To.Col-rg.From.Col+1), Desc: e.n(2) == 0}}
	if e.n(2) == 0 {
		keys = append(keys, SortKey{Col: rg.From.Col + e.n(rg.To.Col-rg.From.Col+1)})
	}
	s.SortRange(rg, keys)
	return fmt.Sprintf("sort %s!%s by %v", s.name, rg, keys)
}

func (r *randBook) editInsert(e *edits) string {
	s, at, n := r.sheet(e), e.n(randRows), 1+e.n(3)
	if e.n(2) == 0 {
		err := s.InsertRows(at, n)
		return fmt.Sprintf("insert %d rows at %s!%d (%v)", n, s.name, at+1, err)
	}
	at %= randCols
	err := s.InsertCols(at, n)
	return fmt.Sprintf("insert %d cols at %s!%s (%v)", n, s.name, ColName(at), err)
}

func (r *randBook) editDelete(e *edits) string {
	s, at, n := r.sheet(e), e.n(randRows), 1+e.n(3)
	if e.n(2) == 0 {
		s.DeleteRows(at, n)
		return fmt.Sprintf("delete %d rows at %s!%d", n, s.name, at+1)
	}
	at %= randCols
	s.DeleteCols(at, n)
	return fmt.Sprintf("delete %d cols at %s!%s", n, s.name, ColName(at))
}

func (r *randBook) editFormat(e *edits) string {
	s, rg := r.sheet(e), e.rect()
	f := Format{Kind: FormatKind(e.n(int(FmtSize) + 1))}
	if f.Kind.HasDecimals() {
		f.Decimals = e.n(4)
	}
	switch e.n(3) {
	case 0:
		rg = colRect(rg.From.Col, rg.To.Col)
	case 1:
		rg = rowRect(rg.From.Row, rg.To.Row)
	}
	s.SetFormat(rg, f)
	return fmt.Sprintf("format %s!%s %s", s.name, rg, f.Kind)
}

func (r *randBook) editStyle(e *edits) string {
	s, rg := r.sheet(e), e.rect()
	switch e.n(4) {
	case 0:
		s.SetStyle(rg, func(st *Style) { st.Bold = !st.Bold })
		return fmt.Sprintf("bold %s!%s", s.name, rg)
	case 1:
		k, l := BorderKind(e.n(int(BorderNone)+1)), Line(1+e.n(3))
		s.SetBorders(rg, k, l)
		return fmt.Sprintf("borders %d %d %s!%s", k, l, s.name, rg)
	case 2:
		s.SetStyle(rg, func(st *Style) { st.Wrap = WrapOn })
		return fmt.Sprintf("wrap %s!%s", s.name, rg)
	}
	s.ClearFormatting(rg)
	return fmt.Sprintf("clear formatting %s!%s", s.name, rg)
}

func (r *randBook) editMerge(e *edits) string {
	s, rg := r.sheet(e), e.rect()
	if e.n(3) == 0 {
		n := s.Unmerge(rg)
		return fmt.Sprintf("unmerge %s!%s (%d)", s.name, rg, n)
	}
	k := MergeKind(e.n(3))
	err := s.Merge(rg, k)
	return fmt.Sprintf("merge %d %s!%s (%v)", k, s.name, rg, err)
}

func (r *randBook) editMove(e *edits) string {
	src, dst := r.sheet(e), r.sheet(e)
	rg, to := e.rect(), e.addr()
	_, err := src.MoveTo(dst, rg, to)
	return fmt.Sprintf("move %s!%s to %s!%s (%v)", src.name, rg, dst.name, to, err)
}

func (r *randBook) editLayout(e *edits) string {
	s := r.sheet(e)
	if e.n(2) == 0 {
		c, w := e.n(randCols), 3+e.n(20)
		s.SetColWidth(c, w)
		return fmt.Sprintf("width %s!%s %d", s.name, ColName(c), w)
	}
	row, h := e.n(randRows), e.n(4)
	s.SetRowHeight(row, row, h)
	return fmt.Sprintf("height %s!%d %d", s.name, row+1, h)
}

func (r *randBook) editNote(e *edits) string {
	s, a := r.sheet(e), e.addr()
	text := []string{"", "note", "another"}[e.n(3)]
	err := s.SetNote(a, text)
	return fmt.Sprintf("note %s!%s %q (%v)", s.name, a, text, err)
}

func (r *randBook) editRule(e *edits) string {
	s, rg := r.sheet(e), e.rect()
	switch e.n(3) {
	case 0:
		err := s.AddCondFormat(CondFormat{Ranges: []Rect{rg}, Op: RuleGreater, Args: [2]string{fmt.Sprint(e.n(9))}, Style: RuleStyle{Bold: true}})
		return fmt.Sprintf("conditional format %s!%s (%v)", s.name, rg, err)
	case 1:
		err := s.AddValidation(Validation{Ranges: []Rect{rg}, Kind: ValidList, Items: []string{"a", "b"}})
		return fmt.Sprintf("validation %s!%s (%v)", s.name, rg, err)
	}
	s.ClearCondFormats(rg)
	s.ClearValidations(rg)
	return fmt.Sprintf("clear rules %s!%s", s.name, rg)
}

func (r *randBook) editRegion(e *edits) string {
	_, _, has2 := r.wb.Region("r2")
	switch e.n(5) {
	case 0:
		cells := r.nb.NotebookCells()
		if len(cells) > 2 {
			cells = cells[:2]
		} else {
			cells = append(cells, notebook.Cell{Source: "r2 = $r1 | first 2"})
		}
		r.nb.SetNotebookCells("edit cells", cells)
		return fmt.Sprintf("notebook cells %d", len(cells))
	case 1:
		if has2 {
			err := r.wb.deleteRegion("r2")
			return fmt.Sprintf("delete r2 (%v)", err)
		}
		s := r.sheet(e)
		err := r.addOutput(s, "r2", e)
		return fmt.Sprintf("send r2 to %s (%v)", s.name, err)
	case 2:
		if has2 {
			s, _, _ := r.wb.Region("r2")
			err := s.FreezeRegion("r2")
			return fmt.Sprintf("freeze r2 (%v)", err)
		}
	}
	name := "r1"
	if has2 && e.n(2) == 0 {
		name = "r2"
	}
	r.feed(name, e)
	return "feed " + name
}

// deleteRegion deletes the region name, wherever it is.
func (w *Workbook) deleteRegion(name string) error {
	s, _, ok := w.Region(name)
	if !ok {
		return ErrNoRegion
	}
	return s.DeleteRegion(name)
}

func (r *randBook) editSheets(e *edits) string {
	s := r.sheet(e)
	switch e.n(4) {
	case 0:
		n, err := r.wb.AddSheet("", e.n(len(r.wb.sheets)+1))
		if err != nil {
			return fmt.Sprintf("add sheet (%v)", err)
		}
		return "add sheet " + n.name
	case 1:
		if s.name == "Sheet1" || s.name == "S2" {
			return "keep " + s.name
		}
		err := r.wb.DeleteSheet(s)
		return fmt.Sprintf("delete sheet %s (%v)", s.name, err)
	case 2:
		name := s.name + "x"
		if strings.HasPrefix(s.name, "Sheet1") || s.name == "S2" {
			return "keep the name " + s.name
		}
		err := r.wb.RenameSheet(s, name)
		return fmt.Sprintf("rename %s (%v)", s.name, err)
	}
	rg := e.rect()
	err := r.wb.EditName("Total", "Total", s, rg)
	return fmt.Sprintf("Total = %s!%s (%v)", s.name, rg, err)
}

func (r *randBook) editView(e *edits) string {
	s := r.sheet(e)
	switch e.n(3) {
	case 0:
		rows, cols := e.n(3), e.n(3)
		s.SetFrozen(rows, cols)
		return fmt.Sprintf("freeze %s %d %d", s.name, rows, cols)
	case 1:
		rg := e.rect()
		s.CreateFilter(rg)
		return fmt.Sprintf("filter %s!%s", s.name, rg)
	}
	s.RemoveFilter()
	return "remove filter " + s.name
}
