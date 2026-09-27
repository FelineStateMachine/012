// Package choicebar is a small question asked on the context line and
// answered with a key, the way terminal programs confirm things, e.g.
// "You have unsaved changes.  Enter Save and quit  D Discard  Esc Cancel".
// It takes the keyboard like an overlay but draws no box, and knows the
// UI only through Host.
package choicebar

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/ui/overlay"
	"github.com/FelineStateMachine/012/internal/ui/theme"
)

// Host is what a choice bar needs of the UI.
type Host interface {
	Theme() *theme.Theme
	// Close closes the open overlay.
	Close()
	// RecordAnswer tells a macro recording the key chosen, for the
	// command that asked; Esc cancels it.
	RecordAnswer(answer string, cancelled bool)
}

// Choice is one answer: a key as tea reports it ("enter", "d"), its
// label, and what choosing it does, after the bar has closed.
type Choice struct {
	Key   string
	Label string
	Run   func() tea.Cmd
}

// Bar is the question and its choices.
type Bar struct {
	h       Host
	Msg     string
	Warn    bool   // the message is a warning, e.g. about losing work
	Desc    string // more on the status line, when the question needs it
	Choices []Choice
}

// New returns a bar asking msg.
func New(h Host, msg string, warn bool, desc string, choices []Choice) *Bar {
	return &Bar{h: h, Msg: msg, Warn: warn, Desc: desc, Choices: choices}
}

func (b *Bar) Indicator() string        { return "MENU" }
func (b *Bar) Layout() []overlay.Box    { return nil }
func (b *Bar) Status() (string, string) { return b.Desc, "" }

func (b *Bar) Key(k tea.KeyPressMsg) tea.Cmd {
	key := strings.ToLower(k.String())
	for i, ch := range b.Choices {
		if key == ch.Key {
			return b.Choose(i)
		}
	}
	return nil
}

// Find is the index of the choice with key or label answer, ignoring
// case, or -1.
func (b *Bar) Find(answer string) int {
	for i, ch := range b.Choices {
		if strings.EqualFold(ch.Key, answer) || strings.EqualFold(ch.Label, answer) {
			return i
		}
	}
	return -1
}

// Choose closes the bar and runs choice i.
func (b *Bar) Choose(i int) tea.Cmd {
	ch := b.Choices[i]
	b.h.Close()
	cmd := ch.Run()
	b.h.RecordAnswer(ch.Key, ch.Key == "esc")
	return cmd
}

// Mouse runs a choice when its key chip or label is clicked. A click
// anywhere else cancels, like Esc.
func (b *Bar) Mouse(e overlay.MouseEvent) tea.Cmd {
	if e.Kind != overlay.MousePress {
		return nil
	}
	if e.Y == overlay.ContextLine {
		x := ansi.StringWidth(b.prefix())
		for i, ch := range b.Choices {
			w := ansi.StringWidth(b.item(ch))
			if e.X >= x && e.X < x+w {
				return b.Choose(i)
			}
			x += w + len(gap)
		}
	}
	b.h.Close()
	return nil
}

const gap = "   "

func (b *Bar) prefix() string {
	if b.Warn {
		return b.h.Theme().Warning.Render(b.Msg) + gap
	}
	return b.Msg + gap
}

func (b *Bar) item(ch Choice) string {
	return b.h.Theme().Chip(theme.KeyLabel(ch.Key)) + " " + ch.Label
}

// ContextLine renders the bar for the context line.
func (b *Bar) ContextLine() (string, string) {
	items := make([]string, len(b.Choices))
	for i, ch := range b.Choices {
		items[i] = b.item(ch)
	}
	return b.prefix() + strings.Join(items, gap), ""
}
