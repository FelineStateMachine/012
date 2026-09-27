package ui

import (
	"errors"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/macro"
	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/ui/picker"
)

// Macros, as Google Sheets' Extensions > Macros, under Data > Macros:
// record what you do (with absolute or relative references), save it
// with a name and an optional Ctrl+Alt+Shift+digit shortcut, run it from
// the menu, the palette or its shortcut, and manage saved macros. Macros
// are Starlark scripts kept in the .012 file (see docs/macros.md);
// recorded ones are scripts too, readable and editable. Recording is in
// macrorec.go, running in macrorun.go, what scripts act on in
// macrohost.go, and the manager in macromanage.go.

// macroItems is Data > Macros.
var macroItems = []menuItem{
	{cmd: "macro.record"}, {cmd: "macro.record_relative"}, {cmd: "macro.stop"}, {cmd: "macro.discard"}, sep,
	{cmd: "macro.run"}, {cmd: "macro.manage"}, {cmd: "macro.new"},
}

func init() {
	idle := func(m *Model) bool { return m.rec == nil && m.macros.run == nil }
	recording := func(m *Model) bool { return m.rec != nil }
	register(
		&command{id: "macro.record", title: "Record macro", macro: macroNever, enabled: idle,
			desc: "Record what you do as a macro that selects the same cells (absolute references)",
			run:  func(m *Model) tea.Cmd { m.startRecording(false); return nil }},
		&command{id: "macro.record_relative", title: "Record macro with relative references", macro: macroNever, enabled: idle,
			desc: "Record what you do as a macro that moves from wherever the active cell is",
			run:  func(m *Model) tea.Cmd { m.startRecording(true); return nil }},
		&command{id: "macro.stop", title: "Stop and save recording", macro: macroNever, enabled: recording,
			desc: "Stop recording and save the macro with a name and an optional shortcut",
			run:  (*Model).stopRecording},
		&command{id: "macro.discard", title: "Discard recording", macro: macroNever, enabled: recording,
			desc: "Stop recording without saving; what you did stays done",
			run: func(m *Model) tea.Cmd {
				m.rec = nil
				m.note = "Recording discarded"
				return nil
			}},
		&command{id: "macro.run", title: "Run macro", macro: macroNever,
			enabled: func(m *Model) bool { return idle(m) && len(m.book().Macros()) > 0 },
			desc:    "Pick a saved macro and run it",
			run: func(m *Model) tea.Cmd {
				p := newPicker(m, "Run macro", "Type a macro's name", 60, macroRunItems(m))
				m.openOverlay(p)
				return nil
			}},
		&command{id: "macro.manage", title: "Manage macros", macro: macroNever, enabled: idle,
			desc: "Rename, delete, give shortcuts to or edit the saved macros",
			run:  func(m *Model) tea.Cmd { m.openMacros(""); return nil }},
		&command{id: "macro.new", title: "Write a macro", macro: macroNever,
			enabled: func(m *Model) bool { return idle(m) && m.macros.editor },
			desc:    "Write a macro script in your editor ($VISUAL or $EDITOR)",
			run:     (*Model).newMacro},
	)
	// Ctrl+Alt+Shift+digit arrives as Ctrl+Alt with the shifted symbol
	// from terminals without the kitty keyboard protocol (US layout).
	for i, sym := range "!@#$%^&*()" {
		d := string("1234567890"[i])
		keyAliases["ctrl+alt+"+string(sym)] = "ctrl+alt+shift+" + d
		keyAliases["ctrl+alt+shift+"+string(sym)] = "ctrl+alt+shift+" + d
	}
}

// macroShortcut returns the digit of a macro shortcut key.
func macroShortcut(key string) (string, bool) {
	d, ok := strings.CutPrefix(key, "ctrl+alt+shift+")
	return d, ok && len(d) == 1 && d[0] >= '0' && d[0] <= '9'
}

// shortcutLabel is how a macro's shortcut is shown, e.g. Ctrl+Alt+Shift+1.
func shortcutLabel(key string) string {
	if key == "" {
		return ""
	}
	return "Ctrl+Alt+Shift+" + key
}

// runShortcut runs the macro a Ctrl+Alt+Shift+digit key is for, and
// reports whether the key is one.
func (m *Model) runShortcut(key string) (tea.Cmd, bool) {
	d, ok := macroShortcut(canonicalKey(key))
	if !ok {
		return nil, false
	}
	mc, found := m.book().MacroForKey(d)
	if !found {
		m.note = shortcutLabel(d) + " runs no macro; give one a shortcut in Data > Macros > Manage macros"
		return nil, true
	}
	return m.runMacro(mc), true
}

// macroRunItems lists the saved macros for the run picker and the
// palette; picking one runs it.
func macroRunItems(m *Model) []picker.Item {
	var items []picker.Item
	for _, mc := range m.book().Macros() {
		items = append(items, picker.Item{
			Title: mc.Name, Name: len(mc.Name), Detail: "Data › Macros", Key: shortcutLabel(mc.Key),
			Desc: "Run the macro " + mc.Name, Off: m.rec != nil,
			Pick: func() tea.Cmd {
				m.closeOverlay()
				return m.runMacro(mc)
			},
		})
	}
	return items
}

// runMacro runs mc, asking first when this file's macros came from
// another computer.
func (m *Model) runMacro(mc sheet.Macro) tea.Cmd {
	switch {
	case m.macros.run != nil:
		return nil
	case m.rec != nil:
		m.note = "Stop recording before running a macro"
		return nil
	case m.macroTrusted():
		return m.startMacro(mc)
	}
	m.openOverlay(&choiceBar{
		m:    m,
		msg:  "Trust this file's macros?",
		desc: "They were made on another computer and can change the file. Trusting covers all of them from now on.",
		warn: true,
		choices: []choice{
			{key: "enter", label: "Run", run: func(m *Model) tea.Cmd {
				m.trustHere()
				return m.startMacro(mc)
			}},
			{key: "esc", label: "Cancel", run: func(*Model) tea.Cmd { return nil }},
		},
	})
	return nil
}

// macroTrusted reports whether this file's macros may run without
// asking: they were made or trusted on this computer, or the user said
// so this session.
func (m *Model) macroTrusted() bool {
	o := m.book().MacroOrigin()
	return m.macros.trusted || o != "" && o == m.macros.machine
}

// trustHere trusts the file's macros from now on, on this computer: the
// origin saved with the file becomes this one.
func (m *Model) trustHere() {
	m.macros.trusted = true
	if m.macros.machine != "" {
		m.book().SetMacroOrigin(m.macros.machine)
	}
}

// AllowEditor lets the user edit macro scripts in their own editor
// ($VISUAL or $EDITOR), which runs as a program of theirs with the
// screen handed over. Only the local app allows it: a session served
// over SSH must not start programs on the server.
func (m *Model) AllowEditor() { m.macros.editor = true }

// SetMachine tells the model which computer it runs on, as an id kept in
// the user's configuration, so macros saved here run without asking and
// macros from files made elsewhere ask once.
func (m *Model) SetMachine(id string) { m.macros.machine = id }

// madeHere is called when the user saves a macro of their own: it trusts
// the file's macros unless others came from elsewhere untrusted.
func (m *Model) madeHere(before int) {
	if before == 0 || m.macroTrusted() {
		m.trustHere()
	}
}

// stopRecording asks for the new macro's name and then its shortcut.
// Esc at either question goes back to recording.
func (m *Model) stopRecording() tea.Cmd {
	r := m.rec
	r.flush(m)
	if len(r.actions) == 0 {
		m.rec = nil
		m.note = "Nothing was recorded, so there's no macro to save"
		return nil
	}
	m.openText("Save macro as:", nextMacroName(m.book()), func(m *Model, name string) tea.Cmd {
		if err := checkNewMacroName(m.book(), name); err != nil {
			m.fail(err.Error() + "; still recording")
			return nil
		}
		m.askShortcut(name)
		return nil
	})
	m.prompt.indicator = "NAME"
	m.prompt.onCancel = stillRecording
	return nil
}

func stillRecording(m *Model) { m.note = "Still recording" }

// checkNewMacroName reports why name can't name a new macro.
func checkNewMacroName(w *sheet.Workbook, name string) error {
	if err := sheet.ValidMacroName(name); err != nil {
		return err
	}
	if mc, ok := w.Macro(strings.TrimSpace(name)); ok {
		return errors.New("There's already a macro named " + mc.Name)
	}
	return nil
}

// askShortcut asks for the digit of the new macro's shortcut, offering
// the first one free.
func (m *Model) askShortcut(name string) {
	m.openText("Shortcut Ctrl+Alt+Shift+ (a digit, or empty for none):", freeShortcut(m.book()), func(m *Model, key string) tea.Cmd {
		if key != "" && !isShortcutDigit(key) {
			m.fail("A shortcut is one digit, 0 to 9, or nothing; still recording")
			return nil
		}
		m.saveRecording(name, key)
		return nil
	})
	m.prompt.indicator = "KEY"
	m.prompt.onCancel = stillRecording
}

func isShortcutDigit(s string) bool { return len(s) == 1 && s[0] >= '0' && s[0] <= '9' }

// saveRecording saves the recording as a macro and stops recording.
func (m *Model) saveRecording(name, key string) {
	r := m.rec
	before := len(m.book().Macros())
	mc := sheet.Macro{Name: name, Key: key, Source: macro.Source(recordingHeader(r.relative), r.actions)}
	if err := m.book().SaveMacro("", mc, "record macro "+strings.TrimSpace(name)); err != nil {
		m.fail(err.Error() + "; still recording")
		return
	}
	m.rec = nil
	m.madeHere(before)
	m.note = "Saved macro " + strings.TrimSpace(name)
	if key != "" {
		m.note += "; " + shortcutLabel(key) + " runs it"
	} else {
		m.note += "; run it from Data > Macros or the palette"
	}
}

// nextMacroName is the first free "Macro N".
func nextMacroName(w *sheet.Workbook) string {
	for i := 1; ; i++ {
		if name := "Macro " + strconv.Itoa(i); !hasMacro(w, name) {
			return name
		}
	}
}

func hasMacro(w *sheet.Workbook, name string) bool {
	_, ok := w.Macro(name)
	return ok
}

// freeShortcut is the first digit, 1 to 9 then 0, no macro uses.
func freeShortcut(w *sheet.Workbook) string {
	for _, d := range "1234567890" {
		if _, taken := w.MacroForKey(string(d)); !taken {
			return string(d)
		}
	}
	return ""
}
