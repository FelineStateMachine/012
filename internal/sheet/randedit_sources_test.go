package sheet

import "fmt"

// Linked sources among the random edits (randedit_test.go): linked,
// ordered, their tabs renamed and deleted, and formulas reading them,
// so the tabs, their order in the file and the formulas naming them
// meet every other edit. Nothing answers the workbook's questions about
// them, as a file opened without a host: what reads them stays Loading…
// through undo, redo and reopening alike.

func init() { randEdits = append(randEdits, (*randBook).editSource) }

// randSources are the names the edits give sources.
var randSources = []string{"big", "huge"}

func (r *randBook) editSource(e *edits) string {
	w := r.wb
	name := randSources[e.n(len(randSources))]
	switch e.n(5) {
	case 0:
		_, err := w.AddSource(name, LinkSource{Path: name + ".parquet"}, r.sheet(e))
		return fmt.Sprintf("link source %s (%v)", name, err)
	case 1:
		o := &SourceOrder{}
		if e.n(2) == 0 {
			o.Sort = []SourceSort{{Col: e.n(3), Desc: e.n(2) == 0}}
		}
		if e.n(2) == 0 {
			o.Filter = []SourceFilter{{Col: e.n(3), Cond: Condition{Op: CondGreater, Arg: fmt.Sprint(e.n(9))}}}
		}
		err := w.SetSourceOrder(name, o)
		return fmt.Sprintf("order %s %+v (%v)", name, o, err)
	case 2:
		info, ok := w.LookupSource(name)
		if !ok {
			return "no source " + name
		}
		err := w.DeleteSheet(info.Sheet)
		return fmt.Sprintf("delete source %s (%v)", name, err)
	}
	s, a := r.sheet(e), e.addr()
	in := []string{
		"=SUM(" + name + "[amount])", `=COUNTIF(` + name + `[region],"x")`, "=ROWS(nu." + name + ")",
		"=" + name + "!B3", "=IFERROR(XLOOKUP(1," + name + "[id]," + name + "[amount]),0)",
	}[e.n(5)]
	err := s.Set(a, in)
	return fmt.Sprintf("%s!%s = %q (%v)", s.name, a, in, err)
}
