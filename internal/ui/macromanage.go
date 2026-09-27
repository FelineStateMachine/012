package ui

import (
	"cmp"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/macro"
	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/ui/picker"
	"github.com/FelineStateMachine/012/internal/ui/theme"
)

// Managing macros, as Sheets' Extensions > Macros > Manage macros: a
// picker lists them with their shortcuts; Enter runs one, F2 renames it,
// F3 changes its shortcut, F4 opens its script in the user's editor, and
// Ctrl+D deletes it. The first row writes a new one. Questions go on the
// context line; the picker opens again after each.

// macrosHost is what the macro manager acts on. The model implements it.
// The manager stays in package ui: it is a few keys over a picker, and
// what they do (run, rename, edit a script) is the model's macro
// machinery.
type macrosHost interface {
	styles() *theme.Theme
	size() (width, height int)
	book() *sheet.Workbook
	closeOverlay()
	// canEditScripts reports whether this session may open an editor.
	canEditScripts() bool
	renameMacro(mc sheet.Macro)
	setMacroShortcut(mc sheet.Macro)
	editScript(name, src string, isNew bool) tea.Cmd
	macroManageItems() []picker.Item
}

// macrosPicker is the picker of saved macros with its extra keys.
type macrosPicker struct {
	m macrosHost // the model, through what the manager needs of it
	*picker.Picker
	msg string // feedback on the last action, e.g. a deletion
}

const writeMacroTitle = "+ Write a macro"

// openMacros opens the manager with the macro named sel highlighted.
func (m *Model) openMacros(sel string) {
	p := m.newPicker("Macros", "Type a macro's name", 72, m.macroManageItems())
	p.Action = "run"
	for i, pm := range p.Shown() {
		if strings.EqualFold(pm.Item.Title, sel) {
			p.Sel = i
		}
	}
	m.openOverlay(&macrosPicker{m: m, Picker: p})
}

// macroManageItems lists "Write a macro" and then every macro.
func (m *Model) macroManageItems() []picker.Item {
	var items []picker.Item
	if m.macros.editor {
		items = append(items, picker.Item{
			Title: writeMacroTitle, Name: len(writeMacroTitle), Desc: "Write a new macro script in your editor",
			Pick: func() tea.Cmd {
				m.closeOverlay()
				return m.newMacro()
			},
		})
	}
	for _, mc := range m.book().Macros() {
		lines := strings.Count(strings.TrimRight(mc.Source, "\n"), "\n") + 1
		items = append(items, picker.Item{
			Title: mc.Name, Name: len(mc.Name), Detail: strconv.Itoa(lines) + " lines", Key: shortcutLabel(mc.Key),
			Desc: "Run " + mc.Name,
			Pick: func() tea.Cmd {
				m.closeOverlay()
				return m.runMacro(mc)
			},
		})
	}
	return items
}

// current is the macro highlighted, if any.
func (p *macrosPicker) current(m macrosHost) (sheet.Macro, bool) {
	if p.Picker.Sel >= len(p.Shown()) || p.Shown()[p.Picker.Sel].Item.Title == writeMacroTitle {
		return sheet.Macro{}, false
	}
	return m.book().Macro(p.Shown()[p.Picker.Sel].Item.Title)
}

func (p *macrosPicker) Key(k tea.KeyPressMsg) tea.Cmd {
	m := p.m
	p.msg = ""
	mc, ok := p.current(m)
	switch k.String() {
	case "f2", "f3", "f4", "ctrl+d":
		if !ok {
			return nil
		}
	default:
		return p.Picker.Key(k)
	}
	switch k.String() {
	case "f2":
		m.closeOverlay()
		m.renameMacro(mc)
	case "f3":
		m.closeOverlay()
		m.setMacroShortcut(mc)
	case "f4":
		if !m.canEditScripts() {
			p.msg = "No editor in this session"
			return nil
		}
		m.closeOverlay()
		return m.editScript(mc.Name, mc.Source, false)
	case "ctrl+d":
		m.book().DeleteMacro(mc.Name)
		p.msg = "Deleted " + mc.Name + "; Ctrl+Z brings it back"
		p.Items = m.macroManageItems()
		sel := p.Picker.Sel
		p.Changed()
		p.Picker.Sel = max(min(sel, len(p.Shown())-1), 0)
	}
	return nil
}

func (p *macrosPicker) Status() (string, string) {
	m := p.m
	pairs := []string{"Enter", "run", "F2", "rename", "F3", "shortcut", "F4", "edit", "Ctrl+D", "delete", "Esc", "close"}
	if !m.canEditScripts() {
		pairs = slices.Delete(pairs, 6, 8)
	}
	if width, _ := m.size(); width < 100 {
		pairs = slices.DeleteFunc(pairs[:len(pairs)-2], func(s string) bool { return s == "shortcut" })
		pairs = slices.Insert(pairs, slices.Index(pairs, "F3")+1, "key")
	}
	keys := m.styles().KeyHints(pairs...)
	if _, ok := p.current(m); !ok {
		keys = m.styles().KeyHints("Enter", "write", "Esc", "close")
	}
	if p.msg != "" {
		return p.msg, keys
	}
	desc, _ := p.Picker.Status()
	return desc, keys
}

// renameMacro asks for a new name on the context line.
func (m *Model) renameMacro(mc sheet.Macro) {
	m.openText("Rename macro "+mc.Name+":", mc.Name, func(m *Model, name string) tea.Cmd {
		old := mc.Name
		mc.Name = name
		if err := m.book().SaveMacro(old, mc, "rename macro "+old); err != nil {
			m.fail(err.Error())
			return nil
		}
		m.openMacros(name)
		return nil
	})
	m.prompt.indicator = "NAME"
}

// setMacroShortcut asks for the digit of a macro's shortcut.
func (m *Model) setMacroShortcut(mc sheet.Macro) {
	m.openText("Shortcut for "+mc.Name+": Ctrl+Alt+Shift+ (a digit, or empty for none):", mc.Key, func(m *Model, key string) tea.Cmd {
		if key != "" && !isShortcutDigit(key) {
			m.fail("A shortcut is one digit, 0 to 9, or nothing")
			return nil
		}
		mc.Key = key
		if err := m.book().SaveMacro(mc.Name, mc, "change the shortcut of macro "+mc.Name); err != nil {
			m.fail(err.Error())
			return nil
		}
		m.openMacros(mc.Name)
		return nil
	})
	m.prompt.indicator = "KEY"
}

// newMacro asks for a name, then opens a new script in the editor.
func (m *Model) newMacro() tea.Cmd {
	m.openText("Name for the new macro:", nextMacroName(m.book()), func(m *Model, name string) tea.Cmd {
		if err := checkNewMacroName(m.book(), name); err != nil {
			m.fail(err.Error())
			return nil
		}
		return m.editScript(strings.TrimSpace(name), newMacroTemplate, true)
	})
	m.prompt.indicator = "NAME"
	return nil
}

const newMacroTemplate = `# A macro written by hand. It runs from the top as one undo step;
# docs/macros.md lists what it can call. For example:
#
#   set("A1", "Total")
#   set_formula("B1", "SUM(B2:B100)")
#   select("B1")
#   run("format.bold")
`

// macroEditedMsg says the editor closed on a macro's script.
type macroEditedMsg struct {
	name, path, orig string
	isNew            bool
	err              error
}

// editScript opens src in the user's editor ($VISUAL, $EDITOR or vi),
// through a temporary file, and saves it as the macro name when the
// editor exits. The screen is handed to the editor meanwhile.
func (m *Model) editScript(name, src string, isNew bool) tea.Cmd {
	f, err := os.CreateTemp("", "012-macro-*.star")
	if err == nil {
		_, err = f.WriteString(src)
		if cerr := f.Close(); err == nil {
			err = cerr
		}
	}
	if err != nil {
		m.fail("Couldn't write the script for the editor: " + err.Error())
		return nil
	}
	msg := macroEditedMsg{name: name, path: f.Name(), orig: src, isNew: isNew}
	return tea.ExecProcess(editorCommand(f.Name()), func(err error) tea.Msg {
		msg.err = err
		return msg
	})
}

// editorCommand runs the user's editor on path.
func editorCommand(path string) *exec.Cmd {
	editor := strings.Fields(cmp.Or(os.Getenv("VISUAL"), os.Getenv("EDITOR"), "vi"))
	return exec.Command(editor[0], append(editor[1:], path)...)
}

// macroEdited saves the script the editor wrote, and checks it: a
// mistake is shown with its line and column, and the script is kept so
// it can be fixed.
func (m *Model) macroEdited(msg macroEditedMsg) {
	defer os.Remove(msg.path)
	if msg.err != nil {
		m.fail(fmt.Sprintf("The editor failed: %v; the script is unchanged", msg.err))
		return
	}
	data, err := os.ReadFile(msg.path)
	if err != nil {
		m.fail("Couldn't read the edited script: " + err.Error())
		return
	}
	src := string(data)
	if !msg.isNew && src == msg.orig {
		m.note = "No changes to " + msg.name
		return
	}
	before := len(m.book().Macros())
	mc, old, label := sheet.Macro{Name: msg.name, Source: src}, "", "write macro "+msg.name
	if !msg.isNew {
		cur, ok := m.book().Macro(msg.name)
		if !ok {
			m.fail("The macro " + msg.name + " is gone; the edit wasn't saved")
			return
		}
		mc, old, label = cur, cur.Name, "edit macro "+msg.name
		mc.Source = src
	}
	if err := m.book().SaveMacro(old, mc, label); err != nil {
		m.fail(err.Error())
		return
	}
	if msg.isNew {
		m.madeHere(before)
	}
	if err := macro.Check(msg.name, src); err != nil {
		m.warn = "Saved " + msg.name + ", but it has a mistake at " + err.Error()
		return
	}
	m.note = "Saved " + msg.name
}
