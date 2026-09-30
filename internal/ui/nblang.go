package ui

import (
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/ui/cmdhelp"
	"github.com/FelineStateMachine/012/internal/ui/nbview"
)

// What the code editor knows of nushell: nu itself, asked about the
// cell being written (nbview.Nu), with the notebook's own words, and
// F1's help on the command at the caret. nu is
// asked only where a cell could run without asking: not in 012 serve
// unless serve-shell allows it, not with shell = off, and not about a
// file from another computer until its cells are trusted. Otherwise,
// and while nu is missing, old or slow, the built-ins answer.

// nbLangKey is what a notebook's words were worked out from.
type nbLangKey struct {
	s     *sheet.Sheet
	state int // the workbook's state in its history
	words int // how many of nu's command names were known
}

// nbLang is the session's questions to nu.
func (m *Model) nbLang() *nbview.NuSession {
	if m.nb.lang == nil {
		m.nb.lang = nbview.NewNuSession(m.runner())
	}
	return m.nb.lang
}

// nbMayAsk reports whether nu may be asked about the cells: they could
// run without asking.
func (m *Model) nbMayAsk() bool {
	if m.shellOff() != "" {
		return false
	}
	return m.configString("shell", "ask") == "on" || m.macroTrusted() || !m.nb.fromFile && len(m.book().Macros()) == 0
}

// syncLang tells nu's questions about notebook s's view v whether they
// may be asked, and the notebook's words once they've changed.
func (m *Model) syncLang(s *sheet.Sheet, v *nbview.View) {
	m.nbLang().SetOn(m.nbMayAsk())
	lang, ok := v.Providers.Completer.(*nbview.Nu)
	key := nbLangKey{s, m.book().StateID(), len(m.nb.words)}
	if !ok || key == m.nb.langKey {
		return
	}
	m.nb.langKey = key
	words := m.nbWords(s)
	var names []string
	for _, w := range words {
		if name, ok := strings.CutPrefix(w.Text, "$"); ok && name != "selection" && name != "sheet." {
			names = append(names, name)
		}
	}
	lang.SetWords(names, words)
}

// nbWords are the completions of notebook s's cells: its names, the
// workbook's regions, and nu's commands once they're known.
func (m *Model) nbWords(s *sheet.Sheet) []nbview.Word {
	var out []nbview.Word
	for _, c := range s.NotebookCells() {
		name := c.Name()
		if name != "" {
			out = append(out, nbview.Word{Text: "$" + name, Desc: "a cell's output"})
		}
		for _, n := range c.Parse().Assigned() {
			if n != name && !slices.ContainsFunc(out, func(w nbview.Word) bool { return w.Text == "$"+n }) {
				out = append(out, nbview.Word{Text: "$" + n, Desc: "a cell's variable"})
			}
		}
	}
	for _, t := range m.book().Sheets() {
		for _, r := range t.Regions() {
			if r.Linked() {
				out = append(out, nbview.Word{Text: "$" + r.Name, Desc: "linked file " + r.File.Path})
			}
		}
	}
	out = append(out, nbview.Word{Text: "$selection", Desc: "the selection on the sheet shown last"},
		nbview.Word{Text: "$sheet.", Desc: "a range of a sheet: $sheet.A1:C9"})
	for _, c := range m.nb.words {
		out = append(out, nbview.Word{Text: c, Desc: "command"})
	}
	return out
}

func init() {
	register(&command{id: "nb.word_help", macro: macroNever, title: "Help on the command",
		desc:    "Show the whole help of the nushell command or flag at the caret, with its page in nushell's docs",
		enabled: func(m *Model) bool { v := m.nbView(); return v != nil && v.Editing() },
		run:     func(m *Model) tea.Cmd { return m.nbView().WordHelp() }})
}

// showWordHelp opens the help of the command at the caret, or the
// keyboard shortcuts, as F1 anywhere else, when nu has none for it.
func (m *Model) showWordHelp(msg nbview.HelpMsg) tea.Cmd {
	if msg.Help == nil {
		return m.runCommand("help")
	}
	m.openOverlay(cmdhelp.New(m.host(), *msg.Help))
	return nil
}

// Code draws a line of nushell as a notebook's cells do.
func (h host) Code(src string) string { return nbview.Highlighted(&h.m.th, src) }
