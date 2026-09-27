package cmdline

import "strings"

// maxHistory is how many lines the history keeps.
const maxHistory = 100

// History is the lines run on the command line, oldest first. It lasts
// as long as the session: each `012 serve` session has its own.
type History struct {
	lines []string
}

// Add puts line at the end of the history, moving it there if it's
// already in it.
func (h *History) Add(line string) {
	if line == "" {
		return
	}
	for i, l := range h.lines {
		if l == line {
			h.lines = append(h.lines[:i], h.lines[i+1:]...)
			break
		}
	}
	h.lines = append(h.lines, line)
	if len(h.lines) > maxHistory {
		h.lines = h.lines[len(h.lines)-maxHistory:]
	}
}

// Lines are the lines in the history, oldest first.
func (h *History) Lines() []string { return h.lines }

// browse is a walk through the history with Up and Down, as vim's: only
// lines starting with what was typed before the first Up.
type browse struct {
	on     bool
	at     int    // the line shown, len(lines) for what was typed
	prefix string // what was typed
}

// step moves d lines through h (-1 older, 1 newer) from what's shown,
// skipping lines that don't start with the prefix, and returns the text
// to show and whether there was a line to move to.
func (b *browse) step(h *History, typed string, d int) (string, bool) {
	if !b.on {
		*b = browse{on: true, at: len(h.lines), prefix: typed}
	}
	for i := b.at + d; i >= 0 && i <= len(h.lines); i += d {
		if i == len(h.lines) {
			b.at = i
			return b.prefix, true
		}
		if strings.HasPrefix(h.lines[i], b.prefix) {
			b.at = i
			return h.lines[i], true
		}
	}
	return "", false
}
