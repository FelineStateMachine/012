package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/locale"
	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/telemetry"
	"github.com/FelineStateMachine/012/internal/ui/choicebar"
	"github.com/FelineStateMachine/012/internal/ui/cmdline"
	"github.com/FelineStateMachine/012/internal/ui/filterpick"
	"github.com/FelineStateMachine/012/internal/ui/findbar"
	"github.com/FelineStateMachine/012/internal/ui/lineedit"
	"github.com/FelineStateMachine/012/internal/ui/nuprompt"
	"github.com/FelineStateMachine/012/internal/ui/picker"
	"github.com/FelineStateMachine/012/internal/ui/rules"
	"github.com/FelineStateMachine/012/internal/ui/shortcuts"
	"github.com/FelineStateMachine/012/internal/ui/sortbar"
	"github.com/FelineStateMachine/012/internal/ui/suggest"
	"github.com/FelineStateMachine/012/internal/ui/theme"
	"github.com/FelineStateMachine/012/internal/ui/themepicker"
)

// host is how components in packages of their own reach the model. It
// implements each one's Host interface, the little it acts on, so the
// model's exported methods stay what cmd/012 and serve use, and a
// component sees only its own interface.
type host struct{ m *Model }

func (m *Model) host() host { return host{m} }

var (
	_ picker.Host      = host{}
	_ cmdline.Host     = host{}
	_ nuprompt.Host    = host{}
	_ findbar.Host     = host{}
	_ themepicker.Host = host{}
	_ rules.Host       = host{}
	_ choicebar.Host   = host{}
	_ sortbar.Host     = host{}
	_ filterpick.Host  = host{}
	_ shortcuts.Host   = host{}
	_ suggest.Host     = host{}
)

// The in-package hosts the model implements itself.
var (
	_ menuHost   = (*Model)(nil)
	_ pivotHost  = (*Model)(nil)
	_ macrosHost = (*Model)(nil)
	_ namesHost  = (*Model)(nil)
	_ promptHost = (*Model)(nil)
	_ chartHost  = (*Model)(nil)
)

// Components that stay in package ui are handed interfaces of their own
// (menuHost and the like), which the model implements with unexported
// methods, these among them.

func (m *Model) styles() *theme.Theme      { return &m.th }
func (m *Model) size() (width, height int) { return m.width, m.height }
func (m *Model) available(id string) bool  { return commands[id].available(m) }
func (m *Model) sheetShown() *sheet.Sheet  { return m.sheet }
func (m *Model) canEditScripts() bool      { return m.macros.editor }

// syncChanged makes the modified flag follow the undo history.
func (m *Model) syncChanged() { m.changed = m.sheet.StateID() != m.saved }

// pickValues opens a filter's values list titled title at screen column
// x; apply gets the criteria chosen, and cancel runs after Esc.
func (m *Model) pickValues(title string, x int, values []sheet.FilterValue, cond sheet.Condition, apply func(sheet.Criteria), cancel func()) {
	fp := m.openValuesPicker(title, x, values, cond, apply)
	fp.OnCancel = cancel
}

// pointRange asks for a range on the context line; done gets it, and
// cancel runs after Esc.
func (m *Model) pointRange(label string, done func(sheet.Rect), cancel func()) {
	m.openRange(label, func(_ *Model, r sheet.Rect) tea.Cmd {
		done(r)
		return nil
	})
	m.prompt.onCancel = cancel
}

// askText asks for text on the context line, starting from initial; done
// gets the answer, and cancel runs after Esc.
func (m *Model) askText(label, initial string, done func(string), cancel func()) {
	m.openText(label, initial, func(_ *Model, text string) tea.Cmd {
		done(text)
		return nil
	})
	m.prompt.onCancel = cancel
}

// What every component with a text field needs.

func (h host) Theme() *theme.Theme       { return &h.m.th }
func (h host) Size() (width, height int) { return h.m.width, h.m.height }
func (h host) Line() *lineedit.Line      { return &h.m.line }
func (h host) Close()                    { h.m.closeOverlay() }
func (h host) Locale() *locale.Locale    { return h.m.locale() }

// The picker.

func (h host) RecordAnswer(answer string, cancelled bool) { h.m.recordAnswer(answer, cancelled) }

// newPicker returns a picker of items over the model; the caller opens it.
func (m *Model) newPicker(title, placeholder string, maxW int, items []picker.Item) *picker.Picker {
	return picker.New(m.host(), title, placeholder, maxW, items)
}

// The command line.

// Commands are the command line's completions: every registered command
// but the command line itself, by title.
func (h host) Commands() []cmdline.Item {
	ids := commandIDs()
	items := make([]cmdline.Item, 0, len(ids))
	for _, id := range ids {
		if id == "vim.command" {
			continue
		}
		c := commands[id]
		items = append(items, cmdline.Item{Word: id, Title: c.title, Desc: c.desc, Key: h.m.shortcut(id), Off: !c.available(h.m)})
	}
	return items
}

func (h host) Run(text string) (tea.Cmd, bool) { return h.m.runCmdLine(text) }
func (h host) Fail(msg string)                 { h.m.fail(msg) }
func (h host) History() *cmdline.History       { return &h.m.session.history }

// The shell prompt's host: nuprompt.Host.

func (h host) Submit(line string) tea.Cmd { return h.m.submitShell(line) }
func (h host) Words() []nuprompt.Word     { return h.m.shellWords() }
func (h host) ShellHistory() []string     { return h.m.book().ShellHistory() }
func (h host) Stop()                      { h.m.stopShell() }
func (h host) Said() string               { return h.m.shell.said }
func (h host) Running() string {
	if j := h.m.shell.running; j != nil {
		return j.name
	}
	return ""
}

// The theme picker.

func (h host) Current() string   { return h.m.prefs.Config.Theme().Pick(!h.m.prefs.light) }
func (h host) ThemesDir() string { return h.m.prefs.ThemesDir }
func (h host) Keep(name string)  { h.m.keepTheme(name) }
func (h host) KeyReleases() bool { return h.m.session.releases }

func (h host) Preview(name string) {
	h.m.prefs.preview = name
	h.m.applyTheme()
}

// The find bar.

func (h host) Book() *sheet.Workbook             { return h.m.book() }
func (h host) Trace() *telemetry.Trace           { return h.m.spans }
func (h host) At() (*sheet.Sheet, sheet.Addr)    { return h.m.sheet, h.m.cur }
func (h host) Note(msg string)                   { h.m.note = msg }
func (h host) Show(s *sheet.Sheet, a sheet.Addr) { h.m.showSheet(s); h.m.cur = a }

// Replace runs a replacement of the find bar, recorded as Find and
// replace answered with what it did, after the selection it started
// from.
func (h host) Replace(do func() string) {
	h.m.recordFlush()
	if answer := do(); answer != "" {
		h.m.changed = true
		h.m.recordDialog("edit.replace", answer)
	}
}

// Leave closes the find bar, keeping it for Ctrl+F and find next.
func (h host) Leave() {
	if f, ok := h.m.overlay.(*findbar.Bar); ok {
		h.m.find = f
	}
	h.m.closeOverlay()
}

// Press hands a click outside an overlay to the grid, as if none were
// open.
func (h host) Press(x, y int, button tea.MouseButton) tea.Cmd {
	return h.m.handlePress(tea.Mouse{X: x, Y: y, Button: button})
}
