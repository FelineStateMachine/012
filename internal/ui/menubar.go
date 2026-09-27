package ui

import (
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// The menu bar follows Google Sheets: File Edit View Insert Format Data
// Help, each opened with Alt and its accelerator letter (Alt+O for Format,
// as in Sheets). Items name registered commands; an item whose command
// isn't registered is hidden, and so is a menu or submenu left empty, so
// menus fill in as features land.

// menuItem is one line of a dropdown: a command, a submenu or a separator.
type menuItem struct {
	title string // defaults to the command's title
	cmd   string
	items []menuItem // a submenu
	sep   bool
}

// sep separates groups of items.
var sep = menuItem{sep: true}

// menuDef is a menu bar title and its dropdown.
type menuDef struct {
	title string
	accel byte // lowercase accelerator letter, marked in the title
	items []menuItem
}

var menuBar = []menuDef{
	{title: "File", accel: 'f', items: []menuItem{
		{cmd: "file.new"}, {cmd: "file.open"}, {cmd: "file.save"}, {cmd: "file.saveas"}, sep,
		{cmd: "quit"},
	}},
	{title: "Edit", accel: 'e', items: []menuItem{
		{cmd: "edit.undo"}, {cmd: "edit.redo"}, sep,
		{cmd: "edit.cut"}, {cmd: "edit.copy"}, {cmd: "edit.paste"}, {cmd: "edit.paste_values", title: "Paste values only"}, sep,
		{cmd: "edit.fill_down", title: "Fill down"}, {cmd: "edit.fill_right", title: "Fill right"}, sep,
		{cmd: "edit.find"}, {cmd: "edit.replace"}, sep,
		{cmd: "clear"}, {cmd: "select.all"}, {cmd: "goto"}, sep,
		{cmd: "delete.row", title: "Delete row"}, {cmd: "delete.col", title: "Delete column"}, {cmd: "delete.selection"},
	}},
	{title: "View", accel: 'v', items: []menuItem{
		{cmd: "palette", title: "Command palette"}, {cmd: "help"},
	}},
	{title: "Insert", accel: 'i', items: []menuItem{
		{cmd: "insert.row_above", title: "Row above"}, {cmd: "insert.row_below", title: "Row below"}, sep,
		{cmd: "insert.col_left", title: "Column left"}, {cmd: "insert.col_right", title: "Column right"}, sep,
		{cmd: "insert.selection"},
	}},
	{title: "Format", accel: 'o', items: []menuItem{
		{title: "Number", items: []menuItem{
			{cmd: "format.automatic", title: "Automatic"}, {cmd: "format.plain_text", title: "Plain text"}, sep,
			{cmd: "format.number", title: "Number"}, {cmd: "format.percent", title: "Percent"}, {cmd: "format.scientific", title: "Scientific"}, sep,
			{cmd: "format.accounting", title: "Accounting"}, {cmd: "format.financial", title: "Financial"},
			{cmd: "format.currency", title: "Currency"}, {cmd: "format.currency_rounded", title: "Currency rounded"}, sep,
			{cmd: "format.date", title: "Date"}, {cmd: "format.time", title: "Time"}, {cmd: "format.datetime", title: "Date time"}, {cmd: "format.duration", title: "Duration"},
		}},
		{cmd: "format.decimals_more", title: "Increase decimal places"}, {cmd: "format.decimals_less", title: "Decrease decimal places"}, sep,
		{cmd: "format.bold", title: "Bold"}, {cmd: "format.italic", title: "Italic"}, {cmd: "format.underline", title: "Underline"}, {cmd: "format.strikethrough", title: "Strikethrough"}, sep,
		{cmd: "format.align_left", title: "Align left"}, {cmd: "format.align_center", title: "Align center"}, {cmd: "format.align_right", title: "Align right"}, sep,
		{cmd: "column.width"}, {cmd: "column.reset"}, sep,
		{cmd: "format.clear", title: "Clear formatting"},
	}},
	// Data: sort and filter will go here.
	{title: "Data", accel: 'd', items: []menuItem{
		{cmd: "data.named_ranges"}, {cmd: "data.define_name"}, sep,
		{cmd: "data.precedents"}, {cmd: "data.dependents"}, sep,
		{cmd: "jev.refresh"},
	}},
	{title: "Help", accel: 'h', items: []menuItem{
		{cmd: "palette", title: "Search the menus"}, {cmd: "help"}, {cmd: "help.functions"}, sep,
		{cmd: "help.about"},
	}},
}

// label is the text an item shows.
func (it menuItem) label() string {
	if it.title != "" {
		return it.title
	}
	if c, ok := commands[it.cmd]; ok {
		return c.title
	}
	return it.cmd
}

// visibleItems drops items whose command isn't registered and submenus
// left empty, then tidies the separators around what's left.
func visibleItems(items []menuItem) []menuItem {
	var out []menuItem
	for _, it := range items {
		switch {
		case it.sep:
			if len(out) > 0 && !out[len(out)-1].sep {
				out = append(out, it)
			}
		case it.items != nil:
			if sub := visibleItems(it.items); len(sub) > 0 {
				it.items = sub
				out = append(out, it)
			}
		case commands[it.cmd] != nil:
			out = append(out, it)
		}
	}
	if len(out) > 0 && out[len(out)-1].sep {
		out = out[:len(out)-1]
	}
	return out
}

// barMenu is a menu bar title as laid out on screen.
type barMenu struct {
	def  *menuDef
	x, w int // the title's span, including one column of padding each side
}

// barMenus lays out the menus that have visible items. Titles are
// separated by two spaces, as elsewhere in the UI.
func barMenus() []barMenu {
	var out []barMenu
	x := 0
	for i := range menuBar {
		d := &menuBar[i]
		if len(visibleItems(d.items)) == 0 {
			continue
		}
		w := len(d.title) + 2
		out = append(out, barMenu{def: d, x: x, w: w})
		x += w
	}
	return out
}

// menuBarTitles renders the menu bar with each accelerator letter marked
// and the open menu, if any, highlighted.
func (m *Model) menuBarTitles() string {
	var b strings.Builder
	for i, bm := range barMenus() {
		style, accel := m.th.menuBar, m.th.menuAccel
		if i == m.openBarMenu() {
			style, accel = m.th.menuSelected, m.th.menuAccelSelected
		}
		t := bm.def.title
		k := strings.IndexByte(strings.ToLower(t), bm.def.accel)
		b.WriteString(style.Render(" "+t[:k]) + accel.Render(t[k:k+1]) + style.Render(t[k+1:]+" "))
	}
	return b.String()
}

// openBarMenu is the index in barMenus of the open dropdown, or -1.
func (m *Model) openBarMenu() int {
	if o, ok := m.overlay.(*menuOverlay); ok {
		return o.bar
	}
	return -1
}

// barMenuAt returns the index in barMenus of the title at column x, or -1.
func barMenuAt(x int) int {
	for i, bm := range barMenus() {
		if x >= bm.x && x < bm.x+bm.w {
			return i
		}
	}
	return -1
}

// barMenuFor returns the index in barMenus of the menu Alt+key opens, or
// -1 if key isn't one.
func barMenuFor(key string) int {
	letter, ok := strings.CutPrefix(key, "alt+")
	if !ok || len(letter) != 1 {
		return -1
	}
	for i, bm := range barMenus() {
		if bm.def.accel == letter[0] {
			return i
		}
	}
	return -1
}

func init() {
	register(&command{id: "menu", title: "Menu", desc: "Open the menu bar", run: func(m *Model) tea.Cmd {
		m.showBarMenu(0)
		return nil
	}})
	keymap["f10"] = "menu"
}

// menuOverlay is an open dropdown from the menu bar, or a context menu,
// with any submenus opened from it.
type menuOverlay struct {
	bar    int // index in barMenus of the open title; -1 for a context menu
	x, y   int // where a context menu opened
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
	m.openOverlay(&menuOverlay{bar: i, levels: []*menuLevel{m.newLevel(visibleItems(menus[i].def.items), true)}})
}

// showContextMenu opens a menu of items with its corner at x, y.
func (m *Model) showContextMenu(items []menuItem, x, y int) {
	if items = visibleItems(items); len(items) > 0 {
		m.openOverlay(&menuOverlay{bar: -1, x: x, y: y, levels: []*menuLevel{m.newLevel(items, true)}})
	}
}

func (m *Model) newLevel(items []menuItem, highlight bool) *menuLevel {
	l := &menuLevel{items: items, sel: -1}
	if highlight {
		l.step(m, 1)
	}
	return l
}

func (l *menuLevel) selectable(m *Model, i int) bool {
	it := l.items[i]
	return !it.sep && (it.items != nil || commands[it.cmd].available(m))
}

// step moves the highlight to the next selectable item in direction d,
// wrapping around.
func (l *menuLevel) step(m *Model, d int) {
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

func (o *menuOverlay) indicator() string { return "MENU" }

// key handles a key in an open menu: Up/Down highlight, Right opens a
// submenu or the next menu, Left closes a submenu or opens the previous
// menu, Enter runs, Esc closes one level, and a letter jumps to the items
// starting with it, running the item if it's the only one.
func (o *menuOverlay) key(m *Model, k tea.KeyPressMsg) tea.Cmd {
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
func (o *menuOverlay) jump(m *Model, letter string) tea.Cmd {
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
func (o *menuOverlay) choose(m *Model) tea.Cmd {
	l := o.top()
	if l.sel < 0 || !l.selectable(m, l.sel) {
		return nil
	}
	it := l.items[l.sel]
	if it.items != nil {
		o.levels = append(o.levels, m.newLevel(it.items, true))
		return nil
	}
	return m.runFromOverlay(it.cmd)
}

func menuBoxID(level int) string { return "menu" + strconv.Itoa(level) }

// mouse handles hovering and clicking items, switching menus by hovering
// or clicking the menu bar, and closing the menu on a click elsewhere.
func (o *menuOverlay) mouse(m *Model, e mouseEvent) tea.Cmd {
	for lv, l := range o.levels {
		if e.box != menuBoxID(lv) {
			continue
		}
		i := l.top + e.row - 1
		if e.kind == mouseWheel {
			o.levels = o.levels[:lv+1]
			l.wheel(m, e.button)
			return nil
		}
		if e.row < 1 || i >= len(l.items) || !l.selectable(m, i) {
			return nil
		}
		switch e.kind {
		case mouseMotion:
			if i != l.sel || len(o.levels) == lv+1 {
				o.levels, l.sel = o.levels[:lv+1], i
				if sub := l.items[i].items; sub != nil {
					o.levels = append(o.levels, m.newLevel(sub, false))
				}
			}
		case mousePress:
			o.levels, l.sel = o.levels[:lv+1], i
			return o.choose(m)
		}
		return nil
	}
	if e.y == menuLine && e.box == "" {
		if t := barMenuAt(e.x); t >= 0 {
			switch {
			case e.kind == mousePress && t == o.bar:
				m.closeOverlay()
			case e.kind == mousePress, e.kind == mouseMotion && o.bar >= 0 && t != o.bar:
				m.showBarMenu(t)
			}
			return nil
		}
	}
	if e.kind == mousePress {
		m.closeOverlay()
		if e.button == tea.MouseRight {
			m.rightClick(e.x, e.y) // right-clicking elsewhere opens a menu there
		}
	}
	return nil
}

// wheel moves the highlight, which scrolls a menu too tall for the screen.
func (l *menuLevel) wheel(m *Model, b tea.MouseButton) {
	switch b {
	case tea.MouseWheelUp:
		l.step(m, -1)
	case tea.MouseWheelDown:
		l.step(m, 1)
	}
}

func (o *menuOverlay) status(m *Model) (string, string) {
	keys := m.keyHints("Up/Down", "move", "Enter", "choose", "Esc", "close")
	if o.bar >= 0 {
		keys = m.keyHints("Arrows", "move", "Enter", "choose", "Esc", "close")
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

// layout places the dropdown under its title (or a context menu at the
// mouse) and each submenu beside the item that opened it, flipping left or
// up where the screen runs out. The status line stays visible.
func (o *menuOverlay) layout(m *Model) []box {
	boxes := make([]box, 0, len(o.levels))
	maxRows := max(m.height-4, 3)
	for i, l := range o.levels {
		rows := min(len(l.items), maxRows)
		if l.sel >= 0 {
			l.top = clamp(l.top, l.sel-rows+1, l.sel)
		}
		l.top = clamp(l.top, 0, len(l.items)-rows)
		lines := m.dropdown(l, rows)
		w, h := ansi.StringWidth(lines[0]), len(lines)
		var x, y int
		switch {
		case i > 0:
			p := boxes[i-1]
			x, y = p.x+p.width(), p.y+o.levels[i-1].sel-o.levels[i-1].top
			// Open to the right; else to the left; and if neither fits,
			// against the screen's right edge, where it covers the parent's
			// shortcuts rather than its labels.
			if x+w > m.width {
				x = p.x - w
				if x < 0 {
					x = m.width - w
				}
			}
		case o.bar >= 0:
			x, y = barMenus()[o.bar].x, menuLine+1
		default:
			x, y = o.x, o.y
			if y+h > m.height-1 {
				y = o.y - h
			}
		}
		x = clamp(x, 0, m.width-w)
		y = clamp(y, 0, m.height-1-h)
		boxes = append(boxes, box{id: menuBoxID(i), x: x, y: y, lines: lines})
	}
	return boxes
}

// dropdown draws a menu box: each item with its shortcut right-aligned, a
// › for submenus, separators between groups, unavailable items dimmed and
// arrows in the border when the items don't all fit.
func (m *Model) dropdown(l *menuLevel, rows int) []string {
	lw, kw := 0, 0
	for _, it := range l.items {
		lw = max(lw, ansi.StringWidth(it.label()))
		kw = max(kw, ansi.StringWidth(m.itemKey(it, false)))
	}
	inner := max(lw+kw+5, 18)
	out := make([]string, 0, rows)
	for i := l.top; i < l.top+rows; i++ {
		it := l.items[i]
		if it.sep {
			out = append(out, sepRow)
			continue
		}
		style := m.th.menuBar
		switch {
		case i == l.sel:
			style = m.th.menuSelected
		case !l.selectable(m, i):
			style = m.th.disabled
		}
		k := m.itemKey(it, !l.selectable(m, i))
		if it.items != nil || i == l.sel {
			// The highlight runs unbroken across the row.
			k = style.Render(ansi.Strip(k))
		}
		out = append(out, cells(style, " "+it.label(), inner-ansi.StringWidth(k)-1)+k+style.Render(" "))
	}
	var up, down string
	if l.top > 0 {
		up = "▲"
	}
	if l.top+rows < len(l.items) {
		down = "▼"
	}
	return m.frame(inner, up, down, out)
}

// itemKey is what an item shows on the right: its shortcut as a key chip
// (plain when the item is unavailable), or › for a submenu.
func (m *Model) itemKey(it menuItem, disabled bool) string {
	k := shortcut(it.cmd)
	switch {
	case it.items != nil:
		return "›"
	case k == "":
		return ""
	case disabled:
		return m.th.disabled.Render(" " + k + " ")
	}
	return m.chip(k)
}
