package ui

import (
	tea "charm.land/bubbletea/v2"
)

// . repeats the last change made in NORMAL mode, as vim does: the keys
// of the operator (x, dd, cc, s, p, o, i, a, = and the rest marked
// change in vimkeys.go) with its count and register, and when it started
// an entry, the keys typed into it up to the one that finished it. A
// count before . replaces the change's own (3. after dd deletes three
// rows).

// The binding gets repeatChange here rather than in its table, which
// repeatChange reads through the key handlers.
func init() {
	b := vimNormal["."]
	b.do = repeatChange
	vimNormal["."] = b
}

// vimChange is a change . can repeat.
type vimChange struct {
	keys  []tea.Msg // key presses and pastes, after any count or register
	count int
	reg   rune
}

// vimChanged remembers c as the change . repeats, once any entry it
// started is finished.
func (m *Model) vimChanged(c *vimChange) {
	if m.editing() {
		m.vim.inserting = c
		return
	}
	m.vim.last = c
}

// vimCapture adds a key or paste to the entry a change started, before
// the model handles it.
func (m *Model) vimCapture(msg tea.Msg) {
	v := &m.vim
	if v.inserting == nil || v.replaying {
		return
	}
	switch msg.(type) {
	case tea.KeyPressMsg, tea.PasteMsg:
		v.inserting.keys = append(v.inserting.keys, msg)
	}
}

// vimSettle finishes the change taking an entry's keys once the entry
// is accepted or cancelled.
func (m *Model) vimSettle() {
	if v := &m.vim; v.inserting != nil && !m.editing() {
		v.last, v.inserting = v.inserting, nil
	}
}

// repeatChange is .: it replays the last change's keys, with the count
// typed before . when there is one.
func repeatChange(m *Model, _ int) tea.Cmd {
	v := &m.vim
	c := v.last
	if c == nil {
		m.note = "No change to repeat yet"
		return nil
	}
	count := c.count
	if v.count > 0 {
		count = v.count
	}
	v.reset()
	v.count, v.reg = count, c.reg
	v.replaying = true
	defer func() { v.replaying = false }()
	cmds := make([]tea.Cmd, 0, len(c.keys))
	for _, msg := range c.keys {
		switch msg := msg.(type) {
		case tea.KeyPressMsg:
			cmds = append(cmds, m.handleKey(msg))
		case tea.PasteMsg:
			m.handlePaste(msg.Content)
		}
	}
	return tea.Batch(cmds...)
}
