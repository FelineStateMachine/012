package ui

import (
	"strconv"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/sheet"
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
	promptWidth
)

// prompt is a question on the context line, such as a file name or a
// range to act on.
type prompt struct {
	kind      promptKind
	label     string
	indicator string
	fresh     bool // the default is still showing; typing replaces it
	secret    bool // the answer is shown masked, e.g. an API key
	typing    bool // range prompts: the user is typing instead of pointing
	onText    func(m *Model, text string) tea.Cmd
	onRange   func(m *Model, r sheet.Rect) tea.Cmd
	onCancel  func(m *Model)
	files     []string // the file list shown by File Open
}

// pointing reports whether a range prompt is taking the arrows.
func (m *Model) pointing() bool {
	return m.mode == modePrompt && m.prompt.kind == promptRange && !m.prompt.typing
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
	m.openPrompt(&prompt{kind: promptText, label: label, indicator: "EDIT", fresh: true, onText: onText}, initial)
}

// openRange asks for a range, starting from the current selection.
func (m *Model) openRange(label string, onRange func(*Model, sheet.Rect) tea.Cmd) {
	r := m.selection()
	m.openPrompt(&prompt{kind: promptRange, label: label, indicator: "POINT", onRange: onRange}, "")
	m.point = pointer{anchor: r.From, at: r.To, anchored: r.From != r.To}
}

func (m *Model) closePrompt() {
	m.prompt = nil
	m.cancelEntry()
}

// key handles a key while the prompt is open.
func (p *prompt) key(m *Model, k tea.KeyPressMsg) tea.Cmd {
	switch key := k.String(); {
	case key == "esc":
		p.cancel(m)
	case key == "enter":
		return p.accept(m)
	case m.pointing():
		p.pointKey(m, k)
	case p.kind == promptWidth && (key == "left" || key == "right"):
		p.stepWidth(m, key)
	default:
		p.typeKey(m, k)
	}
	return nil
}

// cancel backs out: of a range being stretched, or of the prompt.
func (p *prompt) cancel(m *Model) {
	if m.pointing() && m.point.anchored {
		m.point.anchored = false
		return
	}
	if p.onCancel != nil {
		p.onCancel(m)
	}
	m.quitAfterSave = false // cancelling Save as cancels Save and quit
	m.closePrompt()
	m.recordAnswer("", true)
}

// accept closes the prompt and acts on the answer.
func (p *prompt) accept(m *Model) tea.Cmd {
	text := strings.TrimSpace(m.line.Text())
	m.closePrompt()
	if p.kind != promptRange {
		cmd := p.onText(m, text)
		m.recordAnswer(text, false)
		return cmd
	}
	r := m.point.rect()
	if p.typing {
		var ok bool
		if r, ok = sheet.ParseRange(text); !ok {
			m.fail("Invalid range: " + text)
			return nil
		}
	}
	cmd := p.onRange(m, r)
	m.recordAnswer(r.String(), false)
	return cmd
}

// pointKey moves or stretches the range being pointed at; typing switches
// to typing the range instead.
func (p *prompt) pointKey(m *Model, k tea.KeyPressMsg) {
	key := k.String()
	switch {
	case m.pointMoveKey(key):
	case key == ":":
		m.point.anchor, m.point.anchored = m.point.at, true
	case typed(k) != "":
		p.typing = true
		m.line.Insert(typed(k))
	}
}

// stepWidth changes a width by one, previewing it live.
func (p *prompt) stepWidth(m *Model, key string) {
	w, _ := strconv.Atoi(m.line.Text())
	if key == "left" {
		w--
	} else {
		w++
	}
	w = clamp(w, 1, 240)
	m.line.Clear()
	p.fresh = false
	m.line.Insert(strconv.Itoa(w))
	m.setWidths(w) // live preview
}

// typeKey edits the answer; the first key typed replaces the default.
func (p *prompt) typeKey(m *Model, k tea.KeyPressMsg) {
	if p.fresh && typed(k) != "" {
		m.line.Clear()
	}
	p.fresh = false
	if p.kind == promptWidth && !isDigits(typed(k)) {
		return
	}
	m.line.Key(k)
}

// paste types pasted text into the answer.
func (p *prompt) paste(m *Model, text string) {
	if p.fresh {
		m.line.Clear()
		p.fresh = false
	}
	m.line.Insert(text)
}

// line is the prompt as the context line shows it, e.g. "Save as:
// budget.012", and the keys or choices that go with it.
func (p *prompt) line(m *Model) (left, right string) {
	switch {
	case m.pointing():
		return p.prefix() + m.th.Selection.Render(m.point.text()),
			m.th.KeyHints("Arrows", "move", "Shift+arrows", "extend", "Enter", "apply", "Esc", "cancel")
	case len(p.files) > 0:
		return p.prefix() + m.line.Text(), m.th.Muted.Render(strings.Join(p.files, "  "))
	case p.kind == promptWidth:
		return p.prefix() + m.line.Text(), m.th.KeyHints("Left/Right", "adjust", "Enter", "apply", "Esc", "cancel")
	case p.secret:
		return p.prefix() + mask(len(m.line.Buf)), m.th.KeyHints("Enter", "save", "Esc", "cancel")
	}
	return p.prefix() + m.line.Text(), m.th.KeyHints("Enter", "apply", "Esc", "cancel")
}

func (p *prompt) prefix() string {
	return p.label + " "
}

// mask stands for n typed characters of a secret.
func mask(n int) string { return strings.Repeat("•", n) }

// head is the answer before the caret, as shown.
func (p *prompt) head(m *Model) string {
	if p.secret {
		return mask(m.line.Pos)
	}
	return m.line.Head()
}

func isDigits(s string) bool {
	return !strings.ContainsFunc(s, func(r rune) bool { return !unicode.IsDigit(r) })
}
