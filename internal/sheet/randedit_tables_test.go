package sheet

import "fmt"

// Tables among the random edits (randedit_test.go): made, renamed,
// resized, styled and removed, and formulas reading them by column,
// so their growth, header renames and bindings meet every other edit.

func init() { randEdits = append(randEdits, (*randBook).editTable) }

// randTables are the names the edits give tables.
var randTables = []string{"Sales", "Costs"}

func (r *randBook) editTable(e *edits) string {
	s, name := r.sheet(e), randTables[e.n(len(randTables))]
	w := s.wb
	switch e.n(6) {
	case 0:
		rg := e.rect()
		err := s.CreateTable(name, rg)
		return fmt.Sprintf("table %s on %s!%s (%v)", name, s.name, rg, err)
	case 1:
		to := randTables[e.n(len(randTables))]
		err := w.RenameTable(name, to)
		return fmt.Sprintf("rename table %s to %s (%v)", name, to, err)
	case 2:
		rg := e.rect()
		err := w.ResizeTable(name, rg)
		return fmt.Sprintf("resize table %s to %s (%v)", name, rg, err)
	case 3:
		err := w.RemoveTable(name)
		return fmt.Sprintf("remove table %s (%v)", name, err)
	case 4:
		banded, header := e.n(2) == 0, e.n(2) == 0
		err := w.SetTableStyle(name, banded, header)
		return fmt.Sprintf("style table %s %v %v (%v)", name, banded, header, err)
	}
	a := e.addr()
	in := []string{
		"=SUM(" + name + "[Column1])", "=" + name + "[@Column2]*2", "=ROWS(" + name + "[#All])",
		"=COUNTA(" + name + ")", "=SUM(" + name + "[[#Headers],[#Data],[Column1]:[Column2]])",
	}[e.n(5)]
	err := s.Set(a, in)
	return fmt.Sprintf("%s!%s = %q (%v)", s.name, a, in, err)
}
