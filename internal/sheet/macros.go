package sheet

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"
)

// Macros are kept with the workbook, as Google Sheets keeps a
// spreadsheet's Apps Script: each is a named Starlark script with an
// optional Ctrl+Alt+Shift+digit shortcut. The engine only stores them;
// recording and running them is the UI's (internal/ui, internal/macro).
// Changing the list is an undo step like any other edit, so the file's
// modified flag follows it.

// MacroAPI is the version of the scripting API macros are written for.
// A file records it with each macro, so a later build that changes the
// API can tell old scripts from new ones.
const MacroAPI = 1

// Macro is a saved macro.
type Macro struct {
	Name   string
	Key    string // the shortcut's digit, "0" to "9", or "" for none
	Source string // the Starlark script
	API    int    // the scripting API it was written for, MacroAPI or older
}

// maxMacroName keeps names short enough for menus and the palette.
const maxMacroName = 60

// ValidMacroName reports why name can't name a macro, or nil.
func ValidMacroName(name string) error {
	switch {
	case strings.TrimSpace(name) == "":
		return errors.New("a macro needs a name")
	case utf8.RuneCountInString(name) > maxMacroName:
		return fmt.Errorf("macro names are at most %d characters", maxMacroName)
	case strings.ContainsAny(name, "\n\r\t"):
		return errors.New("macro names are one line")
	}
	return nil
}

// Macros returns the saved macros in the order they were added.
func (w *Workbook) Macros() []Macro { return slices.Clone(w.macros) }

// Macro finds a macro by name, ignoring case.
func (w *Workbook) Macro(name string) (Macro, bool) {
	if i := w.macroIndex(name); i >= 0 {
		return w.macros[i], true
	}
	return Macro{}, false
}

// MacroForKey finds the macro a shortcut digit runs.
func (w *Workbook) MacroForKey(key string) (Macro, bool) {
	for _, mc := range w.macros {
		if key != "" && mc.Key == key {
			return mc, true
		}
	}
	return Macro{}, false
}

func (w *Workbook) macroIndex(name string) int {
	return slices.IndexFunc(w.macros, func(mc Macro) bool { return strings.EqualFold(mc.Name, name) })
}

// SaveMacro stores mc in place of the macro named old, or adds it when
// old is "", as one undo step labelled label. Names are unique ignoring
// case, and so are shortcuts.
func (w *Workbook) SaveMacro(old string, mc Macro, label string) error {
	if err := ValidMacroName(mc.Name); err != nil {
		return err
	}
	mc.Name = strings.TrimSpace(mc.Name)
	if mc.API == 0 {
		mc.API = MacroAPI
	}
	at := len(w.macros)
	if old != "" {
		if at = w.macroIndex(old); at < 0 {
			return fmt.Errorf("there's no macro named %s", old)
		}
	}
	if i := w.macroIndex(mc.Name); i >= 0 && i != at {
		return fmt.Errorf("there's already a macro named %s", w.macros[i].Name)
	}
	if other, ok := w.MacroForKey(mc.Key); ok && !strings.EqualFold(other.Name, old) {
		return fmt.Errorf("Ctrl+Alt+Shift+%s already runs %s", mc.Key, other.Name)
	}
	if mc.Key != "" && (len(mc.Key) != 1 || mc.Key[0] < '0' || mc.Key[0] > '9') {
		return fmt.Errorf("a macro's shortcut is a digit, not %q", mc.Key)
	}
	w.changeMacros(label, func() {
		if at == len(w.macros) {
			w.macros = append(w.macros, mc)
		} else {
			w.macros[at] = mc
		}
	})
	return nil
}

// DeleteMacro removes the named macro as an undo step, reporting whether
// there was one.
func (w *Workbook) DeleteMacro(name string) bool {
	i := w.macroIndex(name)
	if i < 0 {
		return false
	}
	w.changeMacros("delete macro "+w.macros[i].Name, func() {
		w.macros = slices.Delete(w.macros, i, i+1)
	})
	return true
}

// changeMacros runs fn, which changes the macro list, as an undo step.
func (w *Workbook) changeMacros(label string, fn func()) {
	s := w.sheets[w.Active()]
	w.change(s, label, Rect{}, func() {
		if st := w.hist.open; st != nil && st.macros == nil {
			before := slices.Clone(w.macros)
			st.macros = &before
		}
		w.macros = slices.Clone(w.macros) // steps keep the old slice
		fn()
	})
}

// MacroOrigin identifies the computer the workbook's macros were made or
// trusted on, as saved in the file; "" when unknown. The UI compares it
// with its own to decide whether to ask before running a macro from a
// file made elsewhere.
func (w *Workbook) MacroOrigin() string { return w.macroOrigin }

// SetMacroOrigin records where the macros were made or trusted. It isn't
// an edit: it changes nothing a user sees, and is saved with the next
// save.
func (w *Workbook) SetMacroOrigin(origin string) { w.macroOrigin = origin }

// Begin opens an undo step that stays open until the returned function
// is called, for changes made over several calls that undo together,
// such as a macro run. Changes in between join it as in a Batch, and
// Undo and Redo do nothing while it is open. Call Settle to see the
// values of formulas changed so far.
func (w *Workbook) Begin(c Change) (end func()) {
	w.begin(c.Sheet, c.Label, c.Focus)
	done := false
	return func() {
		if !done {
			done = true
			w.finish()
		}
	}
}

// Settle recalculates what the open step has changed so far, so formulas
// read inside it (by a macro, or a command it runs) see current values.
// Outside a step there is nothing pending.
func (w *Workbook) Settle() {
	h := &w.hist
	if h.open == nil || len(h.dirty) == 0 && !w.structural {
		return
	}
	dirty := h.dirty
	h.dirty = nil
	if w.structural {
		w.structural = false
		w.recalcAll()
	} else {
		w.recalc(dirty)
	}
}

// fileMacro is a macro, one per line, its script a JSON string:
//
//	{"name": "Totals", "key": "1", "api": 1, "source": "select(\"B9\")\nenter(\"=SUM(B2:B8)\")\n"}
type fileMacro struct {
	Name   string `json:"name"`
	Key    string `json:"key,omitempty"`
	API    int    `json:"api"`
	Source string `json:"source"`
}

// macrosLines is the "macros" field: a list with one macro per line.
func (w *Workbook) macrosLines() string {
	var b strings.Builder
	b.WriteString(`"macros": [`)
	for i, mc := range w.macros {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, "\n    {\"name\": %s, ", plainJSON(mc.Name))
		if mc.Key != "" {
			fmt.Fprintf(&b, "\"key\": %s, ", plainJSON(mc.Key))
		}
		fmt.Fprintf(&b, "\"api\": %d, \"source\": %s}", mc.API, plainJSON(mc.Source))
	}
	b.WriteString("\n  ]")
	return b.String()
}

// plainJSON is s as a JSON string, leaving <, > and & as they are so
// formulas in scripts stay readable.
func plainJSON(s string) string {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	enc.Encode(s)
	return strings.TrimSuffix(b.String(), "\n")
}

// readMacros keeps a file's macros. A macro whose name clashes or is
// invalid is an error, as for named ranges; its script isn't checked
// until it runs.
func (w *Workbook) readMacros(macros []fileMacro) error {
	for _, fm := range macros {
		if err := ValidMacroName(fm.Name); err != nil {
			return fmt.Errorf("macro %q: %w", fm.Name, err)
		}
		if w.macroIndex(fm.Name) >= 0 {
			return fmt.Errorf("macro %q is defined twice", fm.Name)
		}
		key := fm.Key
		if _, taken := w.MacroForKey(key); taken || len(key) != 1 || key[0] < '0' || key[0] > '9' {
			key = "" // a clash or a key this build can't bind: keep the macro, drop the shortcut
		}
		w.macros = append(w.macros, Macro{Name: fm.Name, Key: key, Source: fm.Source, API: max(fm.API, 1)})
	}
	return nil
}
