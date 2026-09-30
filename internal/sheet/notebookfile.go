package sheet

import (
	"bufio"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/FelineStateMachine/012/internal/notebook"
)

// A notebook tab in the file: a sheet marked "tab": "notebook", its
// cells one per line, each with its output as NUON text when the caps
// (OutputCaps) keep it, or "unsaved" when they don't. None of it needs a
// version: earlier builds ignore the fields and open the tab as an
// empty sheet.
//
//	"tab": "notebook",
//	"reactive": true,
//	"notebookCells": [
//	  {"kind": "note", "source": "# Sales"},
//	  {"source": "sales = open sales.csv", "output": "[[Region, Units]; [North, 120]]"},
//	  {"source": "$sales | where Units > 500", "error": "Column not found", "detail": "help: ..."},
//	  {"source": "ls **/*", "unsaved": true}
//	]
type fileNotebookCell struct {
	Kind    string `json:"kind,omitempty"`
	Source  string `json:"source"`
	Output  string `json:"output,omitempty"`
	Error   string `json:"error,omitempty"`
	Detail  string `json:"detail,omitempty"`
	Note    string `json:"note,omitempty"`
	Unsaved bool   `json:"unsaved,omitempty"`
}

// notebookTab is what "tab" says of a notebook.
const notebookTab = "notebook"

// maxSavedError caps what a file keeps of a failed run's message.
const maxSavedError = 4 << 10

// writeNotebook writes a notebook tab's fields.
func (s *Sheet) writeNotebook(b *bufio.Writer, indent string) error {
	if !s.regions.notebook {
		return nil
	}
	b.WriteString(",\n" + indent + `"tab": "notebook"`)
	if s.regions.reactive {
		b.WriteString(",\n" + indent + `"reactive": true`)
	}
	if len(s.regions.cells) == 0 {
		return nil
	}
	w := s.wb
	kept := notebook.Kept(s.regions.cells, w.Output, w.OutputCaps())
	b.WriteString(",\n" + indent + `"notebookCells": [`)
	for i, c := range s.regions.cells {
		fc := fileNotebookCell{Source: c.Source}
		if c.Kind == notebook.Note {
			fc.Kind = c.Kind.String()
		}
		if o := w.Output(c.ID); o != nil {
			fc.Error, fc.Detail, fc.Note = clip(o.Err), clip(o.Detail), clip(o.Note)
			switch {
			case kept[c.ID]:
				fc.Output = string(o.NUON)
			case o.NUON != nil || o.Unsaved:
				fc.Unsaved = true
			}
		}
		raw, err := json.Marshal(fc)
		if err != nil {
			return err
		}
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(b, "\n%s  %s", indent, raw)
	}
	b.WriteString("\n" + indent + "]")
	return nil
}

func clip(s string) string {
	if len(s) > maxSavedError {
		return strings.ToValidUTF8(s[:maxSavedError], "") + "…"
	}
	return s
}

// UnsavedOutputs names the cells whose outputs a file written now would
// leave out, over the caps.
func (w *Workbook) UnsavedOutputs() []string {
	var out []string
	for _, s := range w.sheets {
		kept := notebook.Kept(s.regions.cells, w.Output, w.OutputCaps())
		for i, c := range s.regions.cells {
			if o := w.Output(c.ID); o != nil && o.NUON != nil && !kept[c.ID] {
				name := c.Name()
				if name == "" {
					name = "cell " + fmt.Sprint(i+1)
				}
				out = append(out, name)
			}
		}
	}
	return out
}

// readNotebook reads a notebook tab's cells and their outputs, as its
// cells would have left them.
func (s *Sheet) readNotebook(f fileSheet) error {
	if f.Tab != notebookTab {
		if len(f.NotebookCells) > 0 {
			return fmt.Errorf("notebook cells on a sheet that isn't a notebook")
		}
		return nil
	}
	w := s.wb
	s.regions.notebook, s.regions.reactive = true, f.Reactive
	outputs := map[int]*notebook.Output{}
	for i, fc := range f.NotebookCells {
		kind, ok := notebook.ParseKind(fc.Kind)
		if !ok {
			return fmt.Errorf("cell %d: invalid kind %q", i+1, fc.Kind)
		}
		c := notebook.Cell{ID: w.NewCellID(), Kind: kind, Source: fc.Source}
		s.regions.cells = append(s.regions.cells, c)
		if fc.Output == "" && fc.Error == "" && !fc.Unsaved {
			continue
		}
		o := &notebook.Output{Err: fc.Error, Detail: fc.Detail, Note: fc.Note, Unsaved: fc.Unsaved}
		if fc.Output != "" && !fc.Unsaved {
			o.NUON = []byte(fc.Output)
		}
		w.SetOutput(c.ID, o)
		outputs[c.ID] = w.Output(c.ID)
	}
	notebook.Settle(s.regions.cells, outputs)
	return nil
}

// oldRegion is a command region of a notebook sheet as earlier builds
// wrote it, which opening the file converts: its command becomes a
// code cell of the workbook's notebook tab, and its table stays on the
// sheet as the cell's output sent there.
type oldRegion struct {
	s  *Sheet
	fr fileRegion
}

// convertOld turns the command regions of a file into code cells of a
// notebook tab, made after the last sheet that held them, and their
// tables into sent outputs where the tables were, saying so in the load
// notes.
func (w *Workbook) convertOld(old []oldRegion) {
	if len(old) == 0 {
		return
	}
	nb := w.Notebook()
	if nb == nil {
		nb = w.newSheet(w.freeName("Notebook"))
		nb.regions.notebook = true
		w.insert(nb, w.Index(old[len(old)-1].s)+1)
	}
	var from []string
	for _, o := range old {
		fr := o.fr
		src := fr.Command
		switch {
		case fr.Input != "":
			src = "$sheet." + quoteRef(fr.Input) + " | do {\n" + src + "\n}"
		case len(notebook.Parse(src).Stmts) > 1: // one statement, so the name is its output's
			src = "do {\n" + src + "\n}"
		}
		nb.regions.cells = append(nb.regions.cells, notebook.Cell{ID: w.NewCellID(), Source: fr.Name + " = " + src})
		at, _ := ParseAddr(fr.At)
		at.Row = min(at.Row+1, MaxRows-1) // the table was under the label line
		r := Region{Name: fr.Name, At: at, Output: true}
		o.s.regions.list = append(o.s.regions.list, r)
		o.s.meta(nameKey(r.Name)).stale = true
		o.s.regionsStale = true
		if name := o.s.name; len(from) == 0 || from[len(from)-1] != name {
			from = append(from, name)
		}
	}
	w.nb.notes = append(w.nb.notes, fmt.Sprintf("%s's shell commands are now code cells of %s; their tables stay where they were, sent from the cells, and show once the cells run",
		strings.Join(from, ", "), nb.name))
}

// quoteRef writes Sheet!A1:B2 as $sheet. reads it: the sheet's name in
// quotes when it isn't a plain word.
func quoteRef(ref string) string {
	sheet, rest := SplitSheet(ref)
	if sheet == "" {
		return rest
	}
	for i := range len(sheet) {
		if c := sheet[i]; !isLetter(c) && !isDigit(c) && c != '_' {
			return "'" + strings.ReplaceAll(sheet, "'", "''") + "'!" + rest
		}
	}
	return sheet + "!" + rest
}
