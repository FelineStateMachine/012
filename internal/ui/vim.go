package ui

import (
	"slices"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// Vim keys (File > Settings > Vim keys), in the spirit of sc-im and
// VisiData: in READY mode, which the indicator calls NORMAL, letters move
// and act instead of starting an entry. Counts repeat a motion or say how
// many rows or cells an operator takes (5j, 3dd, 4x); v and V select
// cells or whole rows (VISUAL); : opens a command line (COMMAND). Keys
// vim doesn't claim keep their Sheets meaning, so arrows, Ctrl+S, Alt
// menus and F-keys work as always. Every action but moving and selecting
// runs a registered command; the bindings are tables in vimkeys.go.
// Registers are in vimregs.go, marks in vimmarks.go and . in
// vimrepeat.go.

// vimState is where a vim key sequence has got to.
type vimState struct {
	count  int    // the count typed so far, 0 for none
	keys   string // a sequence begun but not finished, e.g. "d" or "g"
	visual visualKind
	// rows is the clip dd or yy last copied: pasting it inserts rows.
	rows *sheet.Clip
	reg  rune // the register named with " for the next operator, 0 for none
	// typed are the keys of the sequence so far, after any count and
	// register, kept for . to repeat.
	typed []tea.Msg
	regs  map[rune]vimRegister // named, numbered and small-delete registers
	marks map[rune]mark
	back  *mark // where the last jump left from, for '' and ``
	// last is the change . repeats; inserting is one still taking the
	// keys of the entry it started (i, o, cc).
	last, inserting *vimChange
	replaying       bool // . is replaying last
	clipPaste       bool // "+p asked the terminal for the clipboard
}

type visualKind int

const (
	visualNone visualKind = iota
	visualCells
	visualRows
)

// maxCount caps a count, so a slip of the finger can't loop for long.
const maxCount = 9999

// n is the count, 1 when none was typed.
func (v *vimState) n() int { return max(v.count, 1) }

func (v *vimState) reset() { v.count, v.keys, v.reg, v.typed = 0, "", 0, nil }

// pending is what's been typed of an unfinished sequence, e.g. "3d" or
// "a2y.
func (v *vimState) pending() string {
	p := v.keys
	if v.count > 0 {
		p = strconv.Itoa(v.count) + p
	}
	if v.reg != 0 {
		p = `"` + string(v.reg) + p
	}
	return p
}

// argPrefixes are the keys whose next key is a name rather than a
// command: a register after ", a mark after m, ' and `.
var argPrefixes = map[string]string{
	`"`: "register", "m": "set a mark", "'": "go to a mark's row", "`": "go to a mark",
}

// vimKey names a key press as the vim tables do: the character typed,
// or the key's name with its modifiers, e.g. "G", "$", "ctrl+d", "up".
func vimKey(k tea.KeyPressMsg) string {
	if t := typed(k); t != "" && t != " " {
		return t
	}
	return k.String()
}

// vimActive reports whether vim keys have READY mode.
func (m *Model) vimActive() bool {
	return m.prefs.vim && m.mode == modeReady
}

// visual reports whether a visual selection is in progress; a selection
// made any other way (a click, Shift+arrows, a command) ends it.
func (m *Model) visual() visualKind {
	if !m.selecting {
		m.vim.visual = visualNone
	}
	return m.vim.visual
}

// vimKeyPress handles a key in NORMAL or VISUAL mode and reports whether
// it took it. Keys it leaves go to the Sheets bindings.
func (m *Model) vimKeyPress(k tea.KeyPressMsg) (tea.Cmd, bool) {
	v := &m.vim
	key := vimKey(k)
	if key == "esc" {
		return m.vimEscape()
	}
	if _, ok := argPrefixes[v.keys]; ok {
		return m.vimArgKey(key), true
	}
	if _, ok := argPrefixes[key]; ok && v.keys == "" {
		v.keys = key
		return nil, true
	}
	if d := digit(key); v.keys == "" && (d > 0 || d == 0 && v.count > 0) {
		v.count = min(v.count*10+d, maxCount)
		return nil, true
	}
	v.typed = append(v.typed, k)
	seq := v.keys + key
	if mv, ok := vimMotions[seq]; ok {
		if jumpMotions[seq] {
			m.jumped()
		}
		m.vimMove(mv)
		v.reset()
		return nil, true
	}
	table := vimNormal
	if m.visual() != visualNone {
		table = vimVisual
	}
	if b, ok := table[seq]; ok {
		cmd := m.runBinding(b)
		v.reset()
		return cmd, true
	}
	if vimPrefix(table, seq) {
		v.keys = seq
		return nil, true
	}
	begun := v.pending() != ""
	v.reset()
	// A broken sequence and stray letters do nothing, as in vim; other
	// keys (arrows, Ctrl+S, F1) fall through to their Sheets meaning.
	return nil, begun || typed(k) != ""
}

// vimEscape cancels a sequence being typed, then a visual selection, then
// deselects, as Esc does in Sheets.
func (m *Model) vimEscape() (tea.Cmd, bool) {
	switch {
	case m.vim.pending() != "":
		m.vim.reset()
	case m.visual() != visualNone:
		m.vim.visual = visualNone
		m.clearSelection()
	default:
		return m.runCommand("select.none"), true
	}
	return nil, true
}

func digit(key string) int {
	if len(key) == 1 && key[0] >= '0' && key[0] <= '9' {
		return int(key[0] - '0')
	}
	return -1
}

// vimPrefix reports whether seq begins a longer binding or motion.
func vimPrefix(table map[string]vimBinding, seq string) bool {
	for k := range table {
		if len(k) > len(seq) && strings.HasPrefix(k, seq) {
			return true
		}
	}
	for k := range vimMotions {
		if len(k) > len(seq) && strings.HasPrefix(k, seq) {
			return true
		}
	}
	return false
}

// motion moves a, the active cell or a visual selection's moving corner;
// counted is whether a count was typed, which some motions read as a row.
type motion func(m *Model, a *sheet.Addr, n int, counted bool)

// vimMove applies a motion: in NORMAL mode to the active cell, dropping
// any selection; in VISUAL mode to the moving corner.
func (m *Model) vimMove(mv motion) {
	a := &m.cur
	if m.visual() != visualNone {
		a = &m.ext
	}
	mv(m, a, m.vim.n(), m.vim.count > 0)
	*a = clampAddr(*a)
	a.Row = m.visibleRow(a.Row)
	if m.visual() == visualNone {
		m.clearSelection()
	}
	m.entry.tabbing = false
	m.clampView()
}

// nav repeats a Sheets movement key n times.
func nav(key string) motion {
	return func(m *Model, a *sheet.Addr, n int, _ bool) {
		for range n {
			before := *a
			m.navigate(key, a)
			if *a == before {
				return // at an edge; the rest of the count does nothing
			}
		}
	}
}

// toRow goes to row n when a count was typed, else to the row first()
// picks.
func toRow(first func(m *Model) int) motion {
	return func(m *Model, a *sheet.Addr, n int, counted bool) {
		if counted {
			a.Row = n - 1
		} else {
			a.Row = first(m)
		}
	}
}

// screenLine goes to a row on screen: pick chooses from the rows shown.
func screenLine(pick func(rows []int, n int) int) motion {
	return func(m *Model, a *sheet.Addr, n int, _ bool) {
		var rows []int
		for _, r := range m.screenRows() {
			if fr, _ := m.frozen(); r != divider && r >= fr {
				rows = append(rows, r)
			}
		}
		if len(rows) > 0 {
			a.Row = rows[clamp(pick(rows, n), 0, len(rows)-1)]
		}
	}
}

// halfPage moves the cell and the view half a screen, d = 1 down or -1 up.
func halfPage(d int) motion {
	return func(m *Model, a *sheet.Addr, n int, _ bool) {
		rows := d * max(m.scrollRows()/2, 1) * n
		a.Row = m.stepRow(a.Row, rows)
		m.top = m.stepRow(m.top, rows)
	}
}

// vimMotions move in NORMAL and VISUAL modes. The arrows and Sheets'
// movement keys are here too, so they move a visual selection's corner
// rather than dropping it.
var vimMotions = map[string]motion{
	"h": nav("left"), "j": nav("down"), "k": nav("up"), "l": nav("right"),
	"left": nav("left"), "down": nav("down"), "up": nav("up"), "right": nav("right"),
	"w": nav("ctrl+right"), "b": nav("ctrl+left"),
	"ctrl+left": nav("ctrl+left"), "ctrl+right": nav("ctrl+right"), "ctrl+up": nav("ctrl+up"), "ctrl+down": nav("ctrl+down"),
	"pgup": nav("pgup"), "pgdown": nav("pgdown"), "home": nav("home"), "end": nav("end"),
	"ctrl+home": nav("ctrl+home"), "ctrl+end": nav("ctrl+end"),
	"0": nav("home"), "^": nav("home"),
	"$": func(m *Model, a *sheet.Addr, _ int, _ bool) {
		last := sheet.Addr{Col: sheet.MaxCols - 1, Row: a.Row}
		if !m.sheet.Filled(last) {
			last = m.sheet.Edge(last, -1, 0)
		}
		a.Col = last.Col
	},
	"gg": toRow(func(*Model) int { return 0 }),
	"G": toRow(func(m *Model) int {
		used, _ := m.sheet.UsedRange()
		return used.To.Row
	}),
	"H":      screenLine(func(_ []int, n int) int { return n - 1 }),
	"M":      screenLine(func(rows []int, _ int) int { return (len(rows) - 1) / 2 }),
	"L":      screenLine(func(rows []int, n int) int { return len(rows) - n }),
	"ctrl+d": halfPage(1),
	"ctrl+u": halfPage(-1),
}

// vimLine is the context line while a sequence is being typed (the keys
// so far, as vim's showcmd, and the keys that finish it) or in VISUAL
// mode (what applies).
func (m *Model) vimLine() string {
	if p := m.vim.pending(); p != "" {
		return m.th.Key.Render(p) + "   " + m.th.KeyHints(append(m.vimNext(), "Esc", "cancel")...)
	}
	if m.vim.visual == visualRows {
		return m.th.KeyHints("d", "delete rows", "y", "copy rows", "o", "other end", ":", "command", "Esc", "back")
	}
	return m.th.KeyHints("d", "delete", "y", "copy", "p", "paste", "o", "other corner", "V", "rows", "Esc", "back")
}

// vimNext lists the keys that finish the sequence begun, and what they
// do, as pairs for KeyHints: after d, "d" and "cut rows".
func (m *Model) vimNext() []string {
	switch m.vim.keys {
	case "":
		return nil // just a count or a register: anything may follow
	case `"`:
		return []string{"a-z", "named", "0", "last copy", "1-9", "rows deleted", "-", "cells deleted", "+", "system clipboard"}
	case "m":
		return []string{"a-z", "mark the cell"}
	case "'", "`":
		return []string{"a-z", argPrefixes[m.vim.keys], m.vim.keys, "back"}
	}
	table := vimNormal
	if m.visual() != visualNone {
		table = vimVisual
	}
	var seqs []string
	for seq := range table {
		if len(seq) > len(m.vim.keys) && strings.HasPrefix(seq, m.vim.keys) {
			seqs = append(seqs, seq)
		}
	}
	if m.vim.keys == "g" {
		seqs = append(seqs, "gg")
	}
	slices.Sort(seqs)
	var out []string
	for _, seq := range seqs {
		what := "first row"
		if b, ok := table[seq]; ok {
			what = strings.ToLower(commands[b.id].title)
		}
		out = append(out, seq[len(m.vim.keys):], what)
	}
	return out
}

// startVisual selects from the active cell: cells, or whole rows.
func (m *Model) startVisual(kind visualKind) {
	if m.visual() == visualNone {
		m.selecting, m.ext = true, m.cur
	}
	m.vim.visual = kind
	m.whole = wholeNone
	if kind == visualRows {
		m.whole = wholeRows
	}
}

// spanRows selects n whole rows from the active cell's, for an operator
// with a count (3dd).
func (m *Model) spanRows(n int) {
	m.ext = sheet.Addr{Col: m.cur.Col, Row: min(m.cur.Row+n-1, sheet.MaxRows-1)}
	m.selecting, m.whole = true, wholeRows
}

// spanCols selects n cells from the active cell rightwards (4x).
func (m *Model) spanCols(n int) {
	m.ext = sheet.Addr{Col: min(m.cur.Col+n-1, sheet.MaxCols-1), Row: m.cur.Row}
	m.selecting, m.whole = n > 1, wholeNone
}
