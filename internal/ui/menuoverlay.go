package ui

import (
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/ui/overlay"
	"github.com/FelineStateMachine/012/internal/ui/theme"
)

// menuHost is what an open menu acts on. The model implements it. Menus
// stay in package ui because their items are the command registry and
// the menu bar's definitions.
type menuHost interface {
	styles() *theme.Theme
	size() (width, height int)
	available(id string) bool
	shortcut(id string) string
	isChecked(id string) bool
	showBarMenu(i int)
	rightClick(x, y int)
	closeOverlay()
	// runFromOverlay closes the menu and runs a command through runCommand.
	runFromOverlay(id string) tea.Cmd
}

// menuOverlay is an open dropdown from the menu bar, or a context menu,
// with any submenus opened from it.
type menuOverlay struct {
	m      menuHost // the model, through what a menu needs of it
	bar    int      // index in barMenus of the open title; -1 for a context menu
	x, y   int      // where a context menu opened
	levels []*menuLevel
}

// menuLevel is one open box: the dropdown or a submenu.
type menuLevel struct {
	items []menuItem
	sel   int // -1 while nothing is highlighted, e.g. a submenu opened by hovering
	top   int // first item shown when the box is too tall for the screen
}

// showBarMenu opens dropdown i of barMenus.
func (m *Model) showBarMenu(i int) {
	menus := barMenus()
	if len(menus) == 0 {
		return
	}
	i = (i%len(menus) + len(menus)) % len(menus)
	m.openOverlay(&menuOverlay{m: m, bar: i, levels: []*menuLevel{newLevel(m, visibleItems(menus[i].def.items), true)}})
}

// showContextMenu opens a menu of items with its corner at x, y.
func (m *Model) showContextMenu(items []menuItem, x, y int) {
	if items = visibleItems(items); len(items) > 0 {
		m.openOverlay(&menuOverlay{m: m, bar: -1, x: x, y: y, levels: []*menuLevel{newLevel(m, items, true)}})
	}
}

func newLevel(m menuHost, items []menuItem, highlight bool) *menuLevel {
	l := &menuLevel{items: items, sel: -1}
	if highlight {
		l.step(m, 1)
	}
	return l
}

func (l *menuLevel) selectable(m menuHost, i int) bool {
	it := l.items[i]
	return !it.sep && (it.items != nil || m.available(it.cmd))
}

// step moves the highlight to the next selectable item in direction d,
// wrapping around.
func (l *menuLevel) step(m menuHost, d int) {
	n, i := len(l.items), l.sel
	if i < 0 && d < 0 {
		i = n
	}
	for range n {
		i = ((i+d)%n + n) % n
		if l.selectable(m, i) {
			l.sel = i
			return
		}
	}
}

func (o *menuOverlay) top() *menuLevel { return o.levels[len(o.levels)-1] }

func (o *menuOverlay) Indicator() string { return "MENU" }

// Key handles a key in an open menu: Up/Down highlight, Right opens a
// submenu or the next menu, Left closes a submenu or opens the previous
// menu, Enter runs, Esc closes one level, and a letter jumps to the items
// starting with it, running the item if it's the only one.
func (o *menuOverlay) Key(k tea.KeyPressMsg) tea.Cmd {
	m := o.m
	l := o.top()
	key := k.String()
	if i := barMenuFor(key); i >= 0 {
		m.showBarMenu(i)
		return nil
	}
	switch key {
	case "up", "shift+tab":
		l.step(m, -1)
	case "down", "tab":
		l.step(m, 1)
	case "home":
		l.sel = -1
		l.step(m, 1)
	case "end":
		l.sel = -1
		l.step(m, -1)
	case "right":
		if l.sel >= 0 && l.items[l.sel].items != nil {
			return o.choose(m)
		}
		if l.sel < 0 {
			l.step(m, 1)
		} else if o.bar >= 0 {
			m.showBarMenu(o.bar + 1)
		}
	case "left":
		if len(o.levels) > 1 {
			o.levels = o.levels[:len(o.levels)-1]
		} else if o.bar >= 0 {
			m.showBarMenu(o.bar - 1)
		}
	case "enter", "space":
		if l.sel < 0 {
			l.step(m, 1)
			return nil
		}
		return o.choose(m)
	case "esc":
		if len(o.levels) > 1 {
			o.levels = o.levels[:len(o.levels)-1]
		} else {
			m.closeOverlay()
		}
	case "f10":
		m.closeOverlay()
	default:
		return o.jump(m, typed(k))
	}
	return nil
}

// jump highlights the next item starting with letter, running it if it's
// the only one.
func (o *menuOverlay) jump(m menuHost, letter string) tea.Cmd {
	l := o.top()
	if letter == "" {
		return nil
	}
	var matches []int
	for i, it := range l.items {
		if l.selectable(m, i) && strings.HasPrefix(strings.ToLower(it.label()), strings.ToLower(letter)) {
			matches = append(matches, i)
		}
	}
	switch len(matches) {
	case 0:
		return nil
	case 1:
		l.sel = matches[0]
		return o.choose(m)
	}
	next := matches[0]
	for _, i := range matches {
		if i > l.sel {
			next = i
			break
		}
	}
	l.sel = next
	return nil
}

// choose opens the highlighted submenu or runs the highlighted command.
func (o *menuOverlay) choose(m menuHost) tea.Cmd {
	l := o.top()
	if l.sel < 0 || !l.selectable(m, l.sel) {
		return nil
	}
	it := l.items[l.sel]
	if it.items != nil {
		o.levels = append(o.levels, newLevel(m, it.items, true))
		return nil
	}
	return m.runFromOverlay(it.cmd)
}

func menuBoxID(level int) string { return "menu" + strconv.Itoa(level) }

// Mouse handles hovering and clicking items, switching menus by hovering
// or clicking the menu bar, and closing the menu on a click elsewhere.
func (o *menuOverlay) Mouse(e overlay.MouseEvent) tea.Cmd {
	m := o.m
	for lv, l := range o.levels {
		if e.Box == menuBoxID(lv) {
			return o.levelMouse(m, e, lv, l)
		}
	}
	if e.Y == menuLine && e.Box == "" {
		if t := barMenuAt(e.X); t >= 0 {
			o.barMouse(m, e, t)
			return nil
		}
	}
	if e.Kind == overlay.MousePress {
		m.closeOverlay()
		if e.Button == tea.MouseRight {
			m.rightClick(e.X, e.Y) // right-clicking elsewhere opens a menu there
		}
	}
	return nil
}

// levelMouse handles the mouse over open box lv: hovering highlights an
// item and opens its submenu, clicking chooses it, the wheel moves.
func (o *menuOverlay) levelMouse(m menuHost, e overlay.MouseEvent, lv int, l *menuLevel) tea.Cmd {
	i := l.top + e.Row - 1
	if e.Kind == overlay.MouseWheel {
		o.levels = o.levels[:lv+1]
		l.wheel(m, e.Button)
		return nil
	}
	if e.Row < 1 || i >= len(l.items) || !l.selectable(m, i) {
		return nil
	}
	switch e.Kind {
	case overlay.MouseMotion:
		if i != l.sel || len(o.levels) == lv+1 {
			o.levels, l.sel = o.levels[:lv+1], i
			if sub := l.items[i].items; sub != nil {
				o.levels = append(o.levels, newLevel(m, sub, false))
			}
		}
	case overlay.MousePress:
		o.levels, l.sel = o.levels[:lv+1], i
		return o.choose(m)
	}
	return nil
}

// barMouse handles the mouse over menu bar title t: clicking the open
// title closes its menu, and clicking or hovering another opens that one.
func (o *menuOverlay) barMouse(m menuHost, e overlay.MouseEvent, t int) {
	switch {
	case e.Kind == overlay.MousePress && t == o.bar:
		m.closeOverlay()
	case e.Kind == overlay.MousePress, e.Kind == overlay.MouseMotion && o.bar >= 0 && t != o.bar:
		m.showBarMenu(t)
	}
}

// wheel moves the highlight, which scrolls a menu too tall for the screen.
func (l *menuLevel) wheel(m menuHost, b tea.MouseButton) {
	switch b {
	case tea.MouseWheelUp:
		l.step(m, -1)
	case tea.MouseWheelDown:
		l.step(m, 1)
	}
}

func (o *menuOverlay) Status() (string, string) {
	m := o.m
	keys := m.styles().KeyHints("Up/Down", "move", "Enter", "choose", "Esc", "close")
	if o.bar >= 0 {
		keys = m.styles().KeyHints("Arrows", "move", "Enter", "choose", "Esc", "close")
	}
	l := o.top()
	if l.sel < 0 {
		return "", keys
	}
	it := l.items[l.sel]
	if it.items != nil {
		names := make([]string, 0, len(it.items))
		for _, sub := range it.items {
			if !sub.sep {
				names = append(names, sub.label())
			}
		}
		return strings.Join(names, ", "), keys
	}
	return commands[it.cmd].desc, keys
}

// Layout places the dropdown under its title (or a context menu at the
// mouse) and each submenu beside the item that opened it, flipping left or
// up where the screen runs out. The status line stays visible.
func (o *menuOverlay) Layout() []overlay.Box {
	m := o.m
	width, height := m.size()
	boxes := make([]overlay.Box, 0, len(o.levels))
	maxRows := max(height-4, 3)
	for i, l := range o.levels {
		rows := min(len(l.items), maxRows)
		if l.sel >= 0 {
			l.top = clamp(l.top, l.sel-rows+1, l.sel)
		}
		l.top = clamp(l.top, 0, len(l.items)-rows)
		lines := l.lines(m, rows)
		w, h := ansi.StringWidth(lines[0]), len(lines)
		var x, y int
		switch {
		case i > 0:
			p := boxes[i-1]
			x, y = p.X+p.Width(), p.Y+o.levels[i-1].sel-o.levels[i-1].top
			// Open to the right; else to the left; and if neither fits,
			// against the screen's right edge, where it covers the parent's
			// shortcuts rather than its labels.
			if x+w > width {
				x = p.X - w
				if x < 0 {
					x = width - w
				}
			}
		case o.bar >= 0:
			x, y = barMenus()[o.bar].x, menuLine+1
		default:
			x, y = o.x, o.y
			if y+h > height-1 {
				y = o.y - h
			}
		}
		x = clamp(x, 0, width-w)
		y = clamp(y, 0, height-1-h)
		boxes = append(boxes, overlay.Box{ID: menuBoxID(i), X: x, Y: y, Lines: lines})
	}
	return boxes
}

// lines draws the box: each item with its shortcut right-aligned, a
// › for submenus, separators between groups, unavailable items dimmed and
// arrows in the border when the items don't all fit.
func (l *menuLevel) lines(m menuHost, rows int) []string {
	lw, kw := 0, 0
	for _, it := range l.items {
		lw = max(lw, ansi.StringWidth(it.label()))
		kw = max(kw, ansi.StringWidth(it.key(m, false)))
	}
	inner := max(lw+kw+5, 18)
	out := make([]string, 0, rows)
	for i := l.top; i < l.top+rows; i++ {
		it := l.items[i]
		if it.sep {
			out = append(out, theme.SepRow)
			continue
		}
		style := m.styles().MenuBar
		switch {
		case i == l.sel:
			style = m.styles().MenuSelected
		case !l.selectable(m, i):
			style = m.styles().Disabled
		}
		k := it.key(m, !l.selectable(m, i))
		if it.items != nil || i == l.sel {
			// The highlight runs unbroken across the row.
			k = style.Render(ansi.Strip(k))
		}
		out = append(out, theme.Cells(style, " "+it.label(), inner-ansi.StringWidth(k)-1)+k+style.Render(" "))
	}
	var up, down string
	if l.top > 0 {
		up = "▲"
	}
	if l.top+rows < len(l.items) {
		down = "▼"
	}
	return m.styles().Frame(inner, up, down, out)
}

// isChecked reports whether the command is a setting that is on.
func (m *Model) isChecked(id string) bool {
	c := commands[id]
	return c != nil && c.checked != nil && c.checked(m)
}

// key is what an item shows on the right: its shortcut as a key chip
// (plain when the item is unavailable), › for a submenu, or ✓ for a
// setting that is on.
func (it menuItem) key(m menuHost, disabled bool) string {
	k := m.shortcut(it.cmd)
	switch {
	case it.items != nil:
		return "›"
	case k == "" && m.isChecked(it.cmd):
		return "✓"
	case k == "":
		return ""
	case disabled:
		return m.styles().Disabled.Render(" " + k + " ")
	}
	return m.styles().Chip(k)
}
