package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
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
		{cmd: "file.new"}, {cmd: "file.open"}, {cmd: "file.import"}, sep,
		{cmd: "file.save"}, {cmd: "file.saveas"}, {title: "Download", items: downloadItems()}, sep,
		{title: "Settings", items: []menuItem{
			{cmd: "settings.decimal"}, {cmd: "settings.locale"}, {cmd: "settings.vim"}, sep,
			{cmd: "settings.theme"}, {cmd: "settings.jev_key"}, sep,
			{cmd: "settings.config_edit"}, {cmd: "settings.config_reload"},
		}}, sep,
		{cmd: "quit"},
	}},
	{title: "Edit", accel: 'e', items: []menuItem{
		{cmd: "edit.undo"}, {cmd: "edit.redo"}, sep,
		{cmd: "edit.cut"}, {cmd: "edit.copy"}, {cmd: "edit.paste"}, {cmd: "edit.paste_values", title: "Paste values only"}, sep,
		{cmd: "edit.fill_down", title: "Fill down"}, {cmd: "edit.fill_right", title: "Fill right"}, sep,
		{cmd: "edit.find"}, {cmd: "edit.replace"}, sep,
		{cmd: "clear"}, {cmd: "select.all"}, {cmd: "goto"}, sep,
		{cmd: "delete.row", title: "Delete row"}, {cmd: "delete.col", title: "Delete column"}, {cmd: "delete.selection"}, sep,
		{title: "Sheet", items: []menuItem{
			{cmd: "sheet.rename", title: "Rename"}, {cmd: "sheet.duplicate", title: "Duplicate"}, {cmd: "sheet.delete", title: "Delete"}, {cmd: "sheet.hide"}, sep,
			{cmd: "sheet.move_left", title: "Move left"}, {cmd: "sheet.move_right", title: "Move right"}, sep,
			{cmd: "sheet.next"}, {cmd: "sheet.prev"}, {cmd: "sheet.goto"},
		}},
	}},
	{title: "View", accel: 'v', items: []menuItem{
		{title: "Freeze", items: []menuItem{
			{cmd: "view.freeze_rows0", title: "No rows"}, {cmd: "view.freeze_rows1", title: "1 row"},
			{cmd: "view.freeze_rows2", title: "2 rows"}, {cmd: "view.freeze_rows_cur", title: "Up to current row"}, sep,
			{cmd: "view.freeze_cols0", title: "No columns"}, {cmd: "view.freeze_cols1", title: "1 column"},
			{cmd: "view.freeze_cols2", title: "2 columns"}, {cmd: "view.freeze_cols_cur", title: "Up to current column"},
		}}, sep,
		{cmd: "sheet.unhide"}, sep,
		{cmd: "palette", title: "Command palette"}, {cmd: "help"},
	}},
	{title: "Insert", accel: 'i', items: []menuItem{
		{cmd: "insert.row_above", title: "Row above"}, {cmd: "insert.row_below", title: "Row below"}, sep,
		{cmd: "insert.col_left", title: "Column left"}, {cmd: "insert.col_right", title: "Column right"}, sep,
		{cmd: "insert.selection"}, sep,
		{cmd: "sheet.new", title: "Sheet"}, sep,
		{cmd: "note.edit"}, {cmd: "insert.chart"}, sep,
		{cmd: "insert.checkbox"}, {cmd: "insert.dropdown"},
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
		{cmd: "format.align_left", title: "Align left"}, {cmd: "format.align_center", title: "Align center"}, {cmd: "format.align_right", title: "Align right"},
		{title: "Wrapping", items: []menuItem{
			{cmd: "format.wrap_overflow"}, {cmd: "format.wrap"}, {cmd: "format.wrap_clip"},
		}},
		{title: "Borders", items: []menuItem{
			{cmd: "format.borders_all", title: "All"}, {cmd: "format.borders_outer", title: "Outer"}, {cmd: "format.borders_inner", title: "Inner"}, sep,
			{cmd: "format.border_top", title: "Top"}, {cmd: "format.border_bottom", title: "Bottom"},
			{cmd: "format.border_left", title: "Left"}, {cmd: "format.border_right", title: "Right"}, sep,
			{cmd: "format.borders_clear", title: "None"}, sep,
			{cmd: "format.border_thin"}, {cmd: "format.border_thick"}, {cmd: "format.border_double"},
		}},
		{title: "Merge cells", items: []menuItem{
			{cmd: "format.merge_all"}, {cmd: "format.merge_horizontal"}, {cmd: "format.merge_vertical"}, sep,
			{cmd: "format.unmerge"},
		}}, sep,
		{cmd: "column.width"}, {cmd: "column.reset"}, {cmd: "row.height"}, {cmd: "row.fit"}, sep,
		{cmd: "format.conditional"}, {cmd: "format.conditional_clear"}, sep,
		{cmd: "format.clear", title: "Clear formatting"},
	}},
	{title: "Data", accel: 'd', items: []menuItem{
		{title: "Sort sheet", items: []menuItem{
			{cmd: "data.sort_sheet_az", title: "A to Z"}, {cmd: "data.sort_sheet_za", title: "Z to A"},
		}},
		{title: "Sort range", items: []menuItem{
			{cmd: "data.sort_range_az", title: "A to Z by the active column"}, {cmd: "data.sort_range_za", title: "Z to A by the active column"}, sep,
			{cmd: "data.sort_range", title: "Advanced range sorting options"},
		}}, sep,
		{cmd: "data.filter"}, {cmd: "data.filter_column"}, {cmd: "data.filter_remove"}, sep,
		{cmd: "data.pivot"}, {cmd: "data.pivot_edit"}, {cmd: "data.frequency"}, sep,
		{cmd: "data.named_ranges"}, {cmd: "data.define_name"}, {cmd: "data.protect"}, sep,
		{cmd: "data.validation"}, {cmd: "data.validation_clear"}, sep,
		{cmd: "data.precedents"}, {cmd: "data.dependents"}, sep,
		{title: "Macros", items: macroItems}, sep,
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

// anyVisible reports whether visibleItems would keep any of items,
// without making the list: the menu bar asks every frame.
func anyVisible(items []menuItem) bool {
	for _, it := range items {
		if it.items != nil && anyVisible(it.items) || !it.sep && it.items == nil && commands[it.cmd] != nil {
			return true
		}
	}
	return false
}

// barMenus lays out the menus that have visible items. Titles are
// separated by two spaces, as elsewhere in the UI.
func barMenus() []barMenu {
	var out []barMenu
	x := 0
	for i := range menuBar {
		d := &menuBar[i]
		if !anyVisible(d.items) {
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
		style, accel := m.th.MenuBar, m.th.MenuAccel
		if i == m.openBarMenu() {
			style, accel = m.th.MenuSelected, m.th.MenuAccelSelected
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
	register(&command{id: "menu", macro: macroNever, title: "Menu", desc: "Open the menu bar", run: func(m *Model) tea.Cmd {
		m.showBarMenu(0)
		return nil
	}})
	keymap["f10"] = "menu"
}
