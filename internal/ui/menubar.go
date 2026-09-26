package ui

import (
	"strings"
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
		{cmd: "clear"}, {cmd: "select.all"}, {cmd: "goto"}, sep,
		{cmd: "delete.row", title: "Delete row"}, {cmd: "delete.col", title: "Delete column"},
	}},
	{title: "View", accel: 'v', items: []menuItem{
		{cmd: "palette", title: "Command palette"}, {cmd: "help"},
	}},
	{title: "Insert", accel: 'i', items: []menuItem{
		{cmd: "insert.row_above", title: "Row above"}, {cmd: "insert.row_below", title: "Row below"}, sep,
		{cmd: "insert.col_left", title: "Column left"}, {cmd: "insert.col_right", title: "Column right"},
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
	// Data: sort, filter and named ranges will go here.
	{title: "Data", accel: 'd'},
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
	return -1
}
