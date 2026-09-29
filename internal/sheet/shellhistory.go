package sheet

import "slices"

// The lines typed at a notebook's prompt, which Up and Down recall. They
// belong to the workbook and are saved with it ("shellHistory", which
// needs no version), so each notebook remembers its own. Adding one isn't
// an undo step.

// maxShellHistory is how many lines the history keeps.
const maxShellHistory = 100

// ShellHistory returns the lines typed at the prompt, oldest first.
func (w *Workbook) ShellHistory() []string { return slices.Clone(w.shellHistory) }

// AddShellHistory puts line at the end of the history, moving it there
// if it's already in it.
func (w *Workbook) AddShellHistory(line string) {
	if line == "" {
		return
	}
	w.shellHistory = slices.DeleteFunc(w.shellHistory, func(l string) bool { return l == line })
	w.shellHistory = append(w.shellHistory, line)
	if n := len(w.shellHistory); n > maxShellHistory {
		w.shellHistory = slices.Clone(w.shellHistory[n-maxShellHistory:])
	}
}

// readShellHistory keeps a file's history, its last maxShellHistory
// lines.
func (w *Workbook) readShellHistory(lines []string) {
	for _, l := range lines {
		w.AddShellHistory(l)
	}
}
