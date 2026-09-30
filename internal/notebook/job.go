package notebook

import "fmt"

// Run is what running a code cell takes that the notebook knows: what
// nu runs, and the variables it reads from other cells. The screen and
// the headless runner each add what only they know: linked files, the
// selection and ranges of sheets.
type Run struct {
	// Command is what nu runs (Source.Command), and Exports the
	// variables it hands back beside the output.
	Command string
	Exports []string
	// Tables are the other cells' outputs and variables it reads, by
	// name, as NUON, and Reads the Seqs of the outputs they came from.
	Tables map[string][]byte
	Reads  map[string]int
	// Others are the names it reads that no cell gives: linked files,
	// or nothing nu will say so of.
	Others []string
	// Ranges are the ranges of sheets it reads, and Selection whether it
	// reads $selection.
	Ranges    []SheetRef
	Selection bool
}

// Prepare works out what running cell i takes, reading the outputs of
// the cells it reads; the error says why it can't run: what it reads
// hasn't run or failed.
func Prepare(cells []Cell, i int, output func(id int) *Output) (Run, error) {
	src := cells[i].Parse()
	cmd, exports, ranges := src.Command()
	run := Run{Command: cmd, Exports: exports, Tables: map[string][]byte{}, Reads: map[string]int{}, Ranges: ranges, Selection: src.ReadsSelection()}
	vars := Vars(cells)
	for _, name := range src.Refs() {
		j, ok := vars[name]
		if !ok || j == i {
			run.Others = append(run.Others, name)
			continue
		}
		o := output(cells[j].ID)
		switch {
		case o.Failed():
			return run, fmt.Errorf("it reads $%s, which failed", name)
		case o == nil || o.Unsaved || !Readable(cells[j], name, o):
			return run, fmt.Errorf("it reads $%s, which hasn't run", name)
		}
		run.Tables[name], run.Reads[name] = o.NUON, o.Seq
		if cells[j].Name() != name {
			run.Tables[name] = o.Vars[name]
		}
	}
	return run, nil
}
