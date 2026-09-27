package ui

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/ui/picker"
)

// What scripts do to the selection, the sheets and through commands; the
// cells are in macrohost.go.

func (h scriptHost) Selection() (string, string) {
	return wholeRef(h.m.selection()), h.m.cur.String()
}

func (h scriptHost) Select(ref, active string) error {
	m := h.m
	s, r, err := h.resolve(ref)
	if err != nil {
		return err
	}
	if s.Hidden() {
		return errors.New(hiddenMsg(s))
	}
	m.showSheet(s)
	m.selectRect(r)
	if active == "" {
		return nil
	}
	a, ok := sheet.ParseAddr(active)
	if !ok || !r.Contains(a) {
		return fmt.Errorf("the active cell %s isn't in %s", active, ref)
	}
	m.setActive(r, a)
	return nil
}

// setActive makes a, inside the selected range r, the active cell, with
// the far corner of r as the other end of the selection.
func (g *grid) setActive(r sheet.Rect, a sheet.Addr) {
	far := func(v, lo, hi int) int {
		if v == lo {
			return hi
		}
		return lo
	}
	g.cur = a
	switch g.whole {
	case wholeCols:
		g.ext = sheet.Addr{Col: far(a.Col, r.From.Col, r.To.Col), Row: a.Row}
	case wholeRows:
		g.ext = sheet.Addr{Col: a.Col, Row: far(a.Row, r.From.Row, r.To.Row)}
	case wholeNone:
		g.ext = sheet.Addr{Col: far(a.Col, r.From.Col, r.To.Col), Row: far(a.Row, r.From.Row, r.To.Row)}
	}
}

func (h scriptHost) Move(cols, rows int) error {
	m := h.m
	to := sheet.Addr{Col: m.cur.Col + cols, Row: m.cur.Row + rows}
	if !to.Valid() {
		return fmt.Errorf("moving %d, %d from %s goes off the sheet", cols, rows, m.cur)
	}
	m.cur = to
	m.clearSelection()
	return nil
}

func (h scriptHost) Extend(cols, rows int, whole string) error {
	m := h.m
	to := sheet.Addr{Col: m.cur.Col + cols, Row: m.cur.Row + rows}
	if !to.Valid() {
		return fmt.Errorf("extending %d, %d from %s goes off the sheet", cols, rows, m.cur)
	}
	m.selecting, m.ext, m.whole = true, to, wholeNone
	switch whole {
	case "columns":
		m.whole = wholeCols
	case "rows":
		m.whole = wholeRows
	}
	return nil
}

// jumpKeys are the keys Jump's targets stand for.
var jumpKeys = map[string]string{
	"up": "ctrl+up", "down": "ctrl+down", "left": "ctrl+left", "right": "ctrl+right",
	"home": "home", "start": "ctrl+home", "end": "ctrl+end",
}

func (h scriptHost) Jump(to string, extend bool) error {
	key, ok := jumpKeys[to]
	if !ok {
		return fmt.Errorf("can't jump to %q", to)
	}
	if extend {
		key = strings.Replace(key, "+", "+shift+", 1)
		if !strings.Contains(key, "+") {
			key = "shift+" + key
		}
	}
	h.m.moveKey(key)
	return nil
}

func (h scriptHost) Enter(text string, fill bool, origin string) error {
	m := h.m
	if origin != "" {
		from, ok := sheet.ParseAddr(origin)
		if !ok {
			return fmt.Errorf("origin %q isn't a cell", origin)
		}
		var err error
		if text, err = sheet.ShiftEntry(text, m.cur.Col-from.Col, m.cur.Row-from.Row); err != nil {
			return err
		}
	}
	if fill {
		r := m.selection()
		m.writeChecked("Fill", func() (sheet.Rect, error) { return r, m.sheet.FillEntry(r, m.cur, text) })
		return h.failed()
	}
	if bad := m.sheet.CheckEntry(m.cur, text); bad != nil && bad.Reject {
		return bad
	}
	if err := m.sheet.Set(m.cur, text); err != nil {
		return err
	}
	m.clearSelection()
	return nil
}

func (h scriptHost) PasteText(text string) error {
	m := h.m
	if !m.pasteText(text) {
		entry := strings.TrimSpace(text)
		if bad := m.sheet.CheckEntry(m.cur, entry); bad != nil && bad.Reject {
			return fmt.Errorf("Paste undone: %w", bad)
		}
		if err := m.sheet.Set(m.cur, entry); err != nil {
			return err
		}
	}
	return h.failed()
}

// failed is the script's error for what a command just reported to the
// user in ERROR mode, if it did.
func (h scriptHost) failed() error {
	if h.m.mode == modeError {
		return h.failure()
	}
	return nil
}

// failure turns an error a command reported to the user into the
// script's error, leaving the screen as it was.
func (h scriptHost) failure() error {
	m := h.m
	msg := m.errMsg
	m.errMsg, m.mode = "", modeReady
	return errors.New(msg)
}

func (h scriptHost) Sheets() []string {
	var names []string
	for _, s := range h.m.book().Sheets() {
		names = append(names, s.Name())
	}
	return names
}

func (h scriptHost) ActiveSheet() string { return h.m.sheet.Name() }

func (h scriptHost) ActivateSheet(name string) error {
	s := h.m.book().Lookup(name)
	if s == nil {
		return fmt.Errorf("there's no sheet named %s", name)
	}
	if s.Hidden() {
		return errors.New(hiddenMsg(s))
	}
	h.m.showSheet(s)
	return nil
}

func (h scriptHost) AddSheet(name string) (string, error) {
	m := h.m
	s, err := m.book().AddSheet(name, m.book().Index(m.sheet)+1)
	if err != nil {
		return "", err
	}
	m.showSheet(s)
	return s.Name(), nil
}

func (h scriptHost) MoveSheet(pos int) error {
	m := h.m
	if pos < 1 || pos > m.book().Len() {
		return fmt.Errorf("position %d isn't between 1 and %d", pos, m.book().Len())
	}
	m.book().MoveSheet(m.sheet, pos-1)
	return nil
}

func (h scriptHost) SetWidth(cols string, width int) error {
	r, ok := parseWhole(cols)
	if !ok {
		if c, isCol := sheet.ParseCol(strings.ToUpper(cols)); isCol {
			r, ok = sheet.Rect{From: sheet.Addr{Col: c}, To: sheet.Addr{Col: c}}, true
		}
	}
	if !ok {
		return fmt.Errorf("not a column or columns: %s", cols)
	}
	if width < 1 || width > 240 {
		return errors.New("column width must be between 1 and 240")
	}
	for c := r.From.Col; c <= r.To.Col; c++ {
		h.m.sheet.SetColWidth(c, width)
	}
	return nil
}

func (h scriptHost) SetHeight(rows string, height int) error {
	r, ok := parseWhole(rows)
	if !ok {
		if n, err := strconv.Atoi(strings.TrimSpace(rows)); err == nil && n >= 1 && n <= sheet.MaxRows {
			r, ok = sheet.Rect{From: sheet.Addr{Row: n - 1}, To: sheet.Addr{Row: n - 1}}, true
		}
	}
	if !ok {
		return fmt.Errorf("not a row or rows: %s", rows)
	}
	if height < 0 || height > sheet.MaxRowHeight {
		return fmt.Errorf("row height must be between 1 and %d lines, or 0 to fit", sheet.MaxRowHeight)
	}
	h.m.sheet.SetRowHeight(r.From.Row, r.To.Row, height)
	return nil
}

func (h scriptHost) Fill(to string, rows, cols int) error {
	m := h.m
	src := m.selection()
	dst := src
	switch {
	case to != "":
		s, r, err := h.resolve(to)
		if err != nil {
			return err
		}
		if s != m.sheet {
			return errors.New("fill stays on the sheet shown")
		}
		dst = r
	case rows > 0:
		dst.To.Row += rows
	case rows < 0:
		dst.From.Row += rows
	case cols > 0:
		dst.To.Col += cols
	case cols < 0:
		dst.From.Col += cols
	}
	if !dst.From.Valid() || !dst.To.Valid() {
		return errors.New("the fill goes off the sheet")
	}
	got, ok := m.writeChecked("Fill", func() (sheet.Rect, error) { return m.sheet.FillSeries(src, dst) })
	if !ok {
		return h.failed()
	}
	m.selectRect(got)
	return nil
}

// Run runs a command as if picked from the palette, answering the
// question it asks, if any.
func (h scriptHost) Run(id string, answer *string) error {
	m := h.m
	c, ok := commands[id]
	switch {
	case !ok:
		return fmt.Errorf("no command %q; the palette (Ctrl+K) lists them, and docs/reference/macro-api.md how to find ids", id)
	case c.macro == macroNever:
		return fmt.Errorf("%s (%s) can't run in a macro", c.title, id)
	case !c.available(m):
		return fmt.Errorf("%s can't run now", c.title)
	}
	m.macros.cmds = append(m.macros.cmds, m.runCommand(id))
	return h.answer(c, answer)
}

// answer answers the question a command left open, or backs out of it
// and reports that it needed one.
func (h scriptHost) answer(c *command, answer *string) error {
	m := h.m
	switch {
	case m.mode == modeError:
		return h.failure()
	case m.mode == modePrompt && answer != nil:
		p := m.prompt
		m.line.Clear()
		m.line.Insert(*answer)
		p.fresh, p.typing = false, p.kind == promptRange
		m.macros.cmds = append(m.macros.cmds, p.accept(m))
	case m.mode == modePrompt:
		label := m.prompt.label
		m.prompt.cancel(m)
		return fmt.Errorf("%s asks %q: give run(%q, answer=...)", c.title, strings.TrimSuffix(label, ":"), c.id)
	case m.overlay != nil:
		return h.choose(c, answer)
	}
	if m.mode == modeError {
		return h.failure()
	}
	if m.mode != modeReady {
		m.closeOverlay()
		return fmt.Errorf("%s asks more than one question; it can't run in a macro", c.title)
	}
	return nil
}

// choose answers a choice bar with the key or label of a choice; other
// overlays (dialogs, pickers) can't be answered by a script.
func (h scriptHost) choose(c *command, answer *string) error {
	m := h.m
	if p, ok := m.overlay.(*picker.Picker); ok && p.Answers {
		return h.pickAnswer(c, p, answer)
	}
	bar, ok := m.overlay.(*choiceBar)
	if !ok {
		m.closeOverlay()
		return fmt.Errorf("%s opens a dialog; it can't run in a macro", c.title)
	}
	if answer != nil {
		i := slices.IndexFunc(bar.choices, func(ch choice) bool {
			return strings.EqualFold(ch.key, *answer) || strings.EqualFold(ch.label, *answer)
		})
		if i >= 0 {
			m.macros.cmds = append(m.macros.cmds, bar.choose(m, bar.choices[i]))
			if m.mode == modeError {
				return h.failure()
			}
			return nil
		}
	}
	m.closeOverlay()
	return fmt.Errorf("%s asks %q: give run(%q, answer=...) with one of its keys", c.title, bar.msg, c.id)
}

// pickAnswer answers a picker that asks a command's question with the
// title of one of its items.
func (h scriptHost) pickAnswer(c *command, p *picker.Picker, answer *string) error {
	m := h.m
	if answer != nil {
		if cmd, ok := p.Answer(*answer); ok {
			m.macros.cmds = append(m.macros.cmds, cmd)
			if m.mode == modeError {
				return h.failure()
			}
			return nil
		}
	}
	m.closeOverlay()
	var titles []string
	for _, it := range p.Items {
		titles = append(titles, it.Title)
	}
	return fmt.Errorf("%s asks for one of %s: give run(%q, answer=...)", c.title, strings.Join(titles, ", "), c.id)
}
