package ui

import (
	"strconv"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/ui/lineedit"
	"github.com/FelineStateMachine/012/internal/ui/theme"
)

// Prompts are questions on the context line, answered by typing (a file
// name, a width) or by pointing at a range with the arrows, as in 1-2-3.
// The mode indicator says what's being asked, e.g. EDIT, POINT or WIDTH.
// The text is edited in Model.line; a range is pointed at with
// Model.point.

type promptKind int

const (
	promptText promptKind = iota
	promptRange
	promptWidth // a size: digits only, and Left/Right preview one less or more
)

// prompt is a question on the context line, such as a file name or a
// range to act on.
type prompt struct {
	kind      promptKind
	label     string
	indicator string
	fresh     bool // the default is still showing; typing replaces it
	secret    bool // the answer is shown masked, e.g. an API key
	breaks    bool // Shift+Enter and Alt+Enter type a line break, shown as noteBreak
	typing    bool // range prompts: the user is typing instead of pointing
	onText    func(text string) tea.Cmd
	onRange   func(r sheet.Rect) tea.Cmd
	onCancel  func()
	// resize previews a size prompt's answer, from 1 to maxSize.
	resize  func(n int)
	maxSize int
	files   []string // the file list shown by File Open
}

// promptHost is what a prompt acts on. The model implements it. Prompts
// stay in package ui because a prompt is one of the model's modes rather
// than an overlay: a range is pointed at with the grid's own movement
// and drawn by the grid, and the answer goes to a callback over the
// model.
type promptHost interface {
	styles() *theme.Theme
	// editLine is the shared edit line the answer is typed in.
	editLine() *lineedit.Line
	// pointer is the range being pointed at, which the grid draws.
	pointer() *pointer
	// pointMoveKey moves or stretches the pointer as the grid's keys
	// do, and reports whether key was one of them.
	pointMoveKey(key string) bool
	closePrompt()
	fail(msg string)
	// recordAnswer tells a macro recording the answer to the question
	// of the command that asked.
	recordAnswer(answer string, cancelled bool)
	// cancelQuit stops Quit waiting on the save a cancelled prompt was
	// part of: cancelling Save as cancels Save and quit.
	cancelQuit()
}

func (m *Model) editLine() *lineedit.Line { return &m.line }
func (m *Model) pointer() *pointer        { return &m.point }
func (m *Model) cancelQuit()              { m.quitAfterSave = false }

// pointing reports whether a range prompt is taking the arrows.
func (p *prompt) pointing() bool { return p.kind == promptRange && !p.typing }

// pointing reports whether a range prompt is taking the arrows.
func (m *Model) pointing() bool {
	return m.mode == modePrompt && m.prompt.pointing()
}

func (m *Model) openPrompt(p *prompt, initial string) {
	m.mode = modePrompt
	m.prompt = p
	m.line.Clear()
	m.entry.hint = ""
	m.line.Insert(initial)
}

// openText asks for text, starting from initial, which typing replaces.
func (m *Model) openText(label, initial string, onText func(*Model, string) tea.Cmd) {
	done := func(text string) tea.Cmd { return onText(m, text) }
	m.openPrompt(&prompt{kind: promptText, label: label, indicator: "EDIT", fresh: true, onText: done}, initial)
}

// openRange asks for a range, starting from the current selection.
func (m *Model) openRange(label string, onRange func(*Model, sheet.Rect) tea.Cmd) {
	r := m.selection()
	done := func(r sheet.Rect) tea.Cmd { return onRange(m, r) }
	m.openPrompt(&prompt{kind: promptRange, label: label, indicator: "POINT", onRange: done}, "")
	m.point = pointer{anchor: r.From, at: r.To, anchored: r.From != r.To}
}

func (m *Model) closePrompt() {
	m.prompt = nil
	m.cancelEntry()
}

// key handles a key while the prompt is open.
func (p *prompt) key(m promptHost, k tea.KeyPressMsg) tea.Cmd {
	switch key := k.String(); {
	case key == "esc":
		p.cancel(m)
	case key == "enter":
		return p.accept(m)
	case p.breaks && (key == "shift+enter" || key == "alt+enter"):
		p.fresh = false
		m.editLine().Insert(noteBreak)
	case p.pointing():
		p.pointKey(m, k)
	case p.kind == promptWidth && (key == "left" || key == "right"):
		p.stepWidth(m, key)
	default:
		p.typeKey(m, k)
	}
	return nil
}

// cancel backs out: of a range being stretched, or of the prompt.
func (p *prompt) cancel(m promptHost) {
	if pt := m.pointer(); p.pointing() && pt.anchored {
		pt.anchored = false
		return
	}
	m.cancelQuit()
	// Closed first, so onCancel can return to what opened the prompt, as
	// the chart editor does.
	m.closePrompt()
	if p.onCancel != nil {
		p.onCancel()
	}
	m.recordAnswer("", true)
}

// accept closes the prompt and acts on the answer.
func (p *prompt) accept(m promptHost) tea.Cmd {
	text := strings.TrimSpace(m.editLine().Text())
	m.closePrompt()
	if p.kind != promptRange {
		cmd := p.onText(text)
		m.recordAnswer(text, false)
		return cmd
	}
	r := m.pointer().rect()
	if p.typing {
		var ok bool
		if r, ok = sheet.ParseRange(text); !ok {
			m.fail("Invalid range: " + text)
			return nil
		}
	}
	cmd := p.onRange(r)
	m.recordAnswer(r.String(), false)
	return cmd
}

// pointKey moves or stretches the range being pointed at; typing switches
// to typing the range instead.
func (p *prompt) pointKey(m promptHost, k tea.KeyPressMsg) {
	key := k.String()
	switch pt := m.pointer(); {
	case m.pointMoveKey(key):
	case key == ":":
		pt.anchor, pt.anchored = pt.at, true
	case typed(k) != "":
		p.typing = true
		m.editLine().Insert(typed(k))
	}
}

// stepWidth changes a width by one, previewing it live.
func (p *prompt) stepWidth(m promptHost, key string) {
	line := m.editLine()
	w, _ := strconv.Atoi(line.Text())
	if key == "left" {
		w--
	} else {
		w++
	}
	w = clamp(w, 1, p.maxSize)
	line.Clear()
	p.fresh = false
	line.Insert(strconv.Itoa(w))
	p.resize(w)
}

// typeKey edits the answer; the first key typed replaces the default.
func (p *prompt) typeKey(m promptHost, k tea.KeyPressMsg) {
	line := m.editLine()
	if p.fresh && typed(k) != "" {
		line.Clear()
	}
	p.fresh = false
	if p.kind == promptWidth && !isDigits(typed(k)) {
		return
	}
	line.Key(k)
}

// paste types pasted text into the answer.
func (p *prompt) paste(m promptHost, text string) {
	line := m.editLine()
	if p.fresh {
		line.Clear()
		p.fresh = false
	}
	line.Insert(text)
}

// line is the prompt as the context line shows it, e.g. "Save as:
// budget.012", and the keys or choices that go with it.
func (p *prompt) line(m promptHost) (left, right string) {
	th, text := m.styles(), m.editLine().Text()
	switch {
	case p.pointing():
		return p.prefix() + th.Selection.Render(m.pointer().text()),
			th.KeyHints("Arrows", "move", "Shift+arrows", "extend", "Enter", "apply", "Esc", "cancel")
	case len(p.files) > 0:
		return p.prefix() + text, th.Muted.Render(strings.Join(p.files, "  "))
	case p.kind == promptWidth:
		return p.prefix() + text, th.KeyHints("Left/Right", "adjust", "Enter", "apply", "Esc", "cancel")
	case p.secret:
		return p.prefix() + mask(len(m.editLine().Buf)), th.KeyHints("Enter", "save", "Esc", "cancel")
	case p.breaks:
		return p.prefix() + text, th.KeyHints("Enter", "save", "Shift+Enter", "new line", "Esc", "cancel")
	}
	return p.prefix() + text, th.KeyHints("Enter", "apply", "Esc", "cancel")
}

func (p *prompt) prefix() string {
	return p.label + " "
}

// mask stands for n typed characters of a secret.
func mask(n int) string { return strings.Repeat("•", n) }

// head is the answer before the caret, as shown.
func (p *prompt) head(m promptHost) string {
	if p.secret {
		return mask(m.editLine().Pos)
	}
	return m.editLine().Head()
}

func isDigits(s string) bool {
	return !strings.ContainsFunc(s, func(r rune) bool { return !unicode.IsDigit(r) })
}
