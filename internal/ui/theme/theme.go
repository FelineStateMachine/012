// Package theme is the look of the UI: the style roles every view draws
// with, and the small widgets built from them (framed boxes, key chips,
// key hints). It knows nothing of the model, so any part of the UI can
// use it.
package theme

import (
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/chart"
	"github.com/FelineStateMachine/012/internal/sheet"
)

// Theme holds every style the UI draws with. Views must use these roles
// instead of creating styles inline, so the look stays consistent and a
// new theme only has to be defined here.
//
// Roles are defined on the 16 ANSI colors (New). The terminal theme draws
// them as ANSI indexes, so the sheet follows the user's terminal palette,
// with dark and light variants that only differ where ANSI colors would
// lose contrast. A color scheme (FromPalette) draws the same roles in the
// scheme's colors, corrected for contrast, on its own background.
type Theme struct {
	Indicator    lipgloss.Style // mode indicator, top right
	Recording    lipgloss.Style // the REC chip beside the mode indicator while a macro is recorded
	Header       lipgloss.Style // column letters, row numbers and the name box
	HeaderActive lipgloss.Style // header of the focused row and column
	HeaderSel    lipgloss.Style // headers of selected rows and columns
	HeaderHover  lipgloss.Style // a header under the mouse
	Handle       lipgloss.Style // a column resize handle being hovered or dragged
	Pointer      lipgloss.Style // the cell pointer
	Selection    lipgloss.Style // a range being pointed at
	Hint         lipgloss.Style // guidance on the context line
	Warning      lipgloss.Style // recoverable problems, e.g. a formula error
	Error        lipgloss.Style // ERROR mode message
	Muted        lipgloss.Style // secondary text: key hints, file lists
	Key          lipgloss.Style // emphasized text in the status line, e.g. a range
	KeyChip      lipgloss.Style // a key cap in hints, menus and the palette, e.g. " Enter "
	ErrorCell    lipgloss.Style // cells whose value is ERR or NA
	// ErrorMark is layered on an error's text, so errors show beyond
	// color: a curly underline, in the error color where the terminal
	// supports colored underlines.
	ErrorMark    lipgloss.Style
	Link         lipgloss.Style // a cell's URL or HYPERLINK label, layered on the cell's role
	Spilled      lipgloss.Style // values an array formula spilled into the cells below and right of it
	Found        lipgloss.Style // cells matching an open search
	Traced       lipgloss.Style // precedents or dependents being traced
	Argument     lipgloss.Style // the argument at the caret in a function's signature
	Progress     lipgloss.Style // the done part of an import's progress bar
	ProgressTodo lipgloss.Style // the rest of the progress bar
	// Copied marks the range on the clipboard, like Sheets' dashed border:
	// a dashed underline across every cell, layered on the cell's own
	// style, with its own text color where the cell has none.
	Copied lipgloss.Style
	// FrozenLine divides frozen rows and columns from the scrolling ones,
	// like a tmux pane border.
	FrozenLine lipgloss.Style
	// Sheet tabs on the status line, like lazygit's panel tabs: plain
	// names (so they don't read as key chips), the sheet shown in the
	// accent and bold, and a tab under the mouse, or where a dragged tab
	// would go, bold and underlined.
	Tab       lipgloss.Style
	TabActive lipgloss.Style
	TabHover  lipgloss.Style
	// FilterOn is the filter mark in the header of a column whose filter
	// hides something (the mark itself also changes, from ▾ to ▼).
	FilterOn lipgloss.Style
	// NoteMark is the mark in the top-right corner of a cell with a note,
	// like Sheets' small triangle.
	NoteMark lipgloss.Style
	// CellBorder draws the lines of Format > Borders, in the ink of the
	// text beside them rather than a color of their own, as Sheets draws
	// borders black.
	CellBorder lipgloss.Style

	// Conditional formats (rules.go), indexed by sheet.Color: a rule's
	// text color on the cell, its fill with text readable on it, and
	// RuleOn[fill*sheet.NumColors+text] a text color on a fill, in the
	// fill's own ink where the text color wouldn't read on it.
	RuleText [sheet.NumColors]lipgloss.Style
	RuleFill [sheet.NumColors]lipgloss.Style
	RuleOn   [sheet.NumColors * sheet.NumColors]lipgloss.Style
	// Invalid is layered on the text of a cell that fails its data
	// validation, as Sheets' red corner: a dotted underline, in the
	// warning color where the terminal colors underlines.
	Invalid lipgloss.Style
	// Dropdown is the ▾ at the right of a cell with a dropdown list.
	Dropdown lipgloss.Style
	// scales and shades keep the shades of color scales and of rule
	// colors drawn so far.
	scales map[scaleKey]Shade
	shades map[sheet.RuleStyle]Shade

	// Chrome: the menu bar, dropdowns, the palette and dialogs.
	MenuBar           lipgloss.Style // menu bar titles
	MenuAccel         lipgloss.Style // a title's accelerator letter
	MenuSelected      lipgloss.Style // open title, highlighted item
	MenuAccelSelected lipgloss.Style // accelerator letter of the open title
	Border            lipgloss.Style // box borders and separators
	Title             lipgloss.Style // box titles and group headings
	Disabled          lipgloss.Style // items that can't run right now
	Match             lipgloss.Style // characters matched by a search
	MatchSelected     lipgloss.Style // matched characters in the highlighted row
	Cell              lipgloss.Style // an ordinary cell: the base for bold, italic and underline

	// Bars: full-width bands behind the menu bar, formula bar, context
	// line, column headers and status line. Each line is drawn on its
	// role's background, with its foreground for text that has none, out
	// to the terminal's edge; an empty role leaves the line as it is.
	MenuBarRow      lipgloss.Style
	FormulaBarRow   lipgloss.Style
	ContextRow      lipgloss.Style
	ColumnHeaderRow lipgloss.Style
	StatusBarRow    lipgloss.Style
	RowHeader       lipgloss.Style // row numbers not focused, selected or hovered
	// Screen is the background and default text color of the whole
	// screen. Empty in the terminal theme, so the terminal's own show.
	Screen lipgloss.Style

	// Name is the theme's name, and Palette its colors: nil for the
	// terminal theme, whose colors only the terminal knows.
	Name    string
	Palette *Palette

	// Charts: see charts.go in package ui.
	ChartFrame    lipgloss.Style // a chart's border
	ChartSelected lipgloss.Style // the border of the selected chart and its resize handle
	ChartAxis     lipgloss.Style // axis lines and tick marks
	ChartLabel    lipgloss.Style // tick values, category labels and legend text
	// Series colors bars, lines, slices and legend swatches, in order;
	// SeriesBg is the same colors as backgrounds, for the lower half of a
	// pie's half blocks. SeriesANSI is the ANSI index of each, so images
	// can use the terminal's own colors.
	Series     [chart.Colors]lipgloss.Style
	SeriesBg   [chart.Colors]lipgloss.Style
	SeriesANSI [chart.Colors]int
}

// ImageID is the style of an image's Unicode placeholders: the
// foreground color is not a color but the image's id, in the 256-color
// palette, which is how the terminal knows which image to draw there.
func (t *Theme) ImageID(id int) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(id))
}

// Text adds a cell's bold, italic, underline and strikethrough to base,
// one of the cell roles (cell, pointer, selection, errorCell), so text
// styles show through the pointer and selection colors.
func (t *Theme) Text(base lipgloss.Style, st sheet.Style) lipgloss.Style {
	if st.Bold {
		base = base.Bold(true)
	}
	if st.Italic {
		base = base.Italic(true)
	}
	if st.Underline {
		base = base.Underline(true)
	}
	if st.Strikethrough {
		base = base.Strikethrough(true)
	}
	return base
}

// New returns the terminal theme's dark or light variant.
func New(dark bool) Theme {
	// Contrast was checked against the reference palettes in e2e: text on
	// colored backgrounds stays at or above roughly 4.5:1.
	headerBg, headerFg := lipgloss.BrightBlack, lipgloss.BrightWhite
	selFg, muted, match := lipgloss.Black, lipgloss.BrightBlack, lipgloss.Yellow
	bar := lipgloss.Cyan
	filterFg := lipgloss.Yellow
	noteFg := lipgloss.Yellow
	borderFg := lipgloss.White
	if !dark {
		borderFg = lipgloss.Black
		noteFg = lipgloss.Magenta
		headerBg, headerFg = lipgloss.White, lipgloss.Black
		selFg, muted, match = lipgloss.BrightWhite, lipgloss.Black, lipgloss.Blue
		filterFg = lipgloss.Blue
		bar = lipgloss.Blue
	}
	// Links are blue, as in Sheets; plain blue is too dark on a dark
	// background.
	link := lipgloss.BrightBlue
	// Series colors, most distinct first. On light backgrounds yellow and
	// cyan fade, so they come last or not at all.
	series := [chart.Colors]ansi.BasicColor{lipgloss.Cyan, lipgloss.Magenta, lipgloss.Yellow, lipgloss.Green, lipgloss.BrightBlue, lipgloss.BrightRed}
	if !dark {
		link = lipgloss.Blue
		series = [chart.Colors]ansi.BasicColor{lipgloss.Blue, lipgloss.Magenta, lipgloss.Green, lipgloss.Red, lipgloss.Cyan, lipgloss.BrightBlack}
	}
	accent := lipgloss.NewStyle().Background(lipgloss.Cyan).Foreground(lipgloss.Black)
	t := Theme{
		Indicator:    accent.Bold(true),
		Recording:    lipgloss.NewStyle().Background(lipgloss.Red).Foreground(selFg).Bold(true),
		Header:       lipgloss.NewStyle().Background(headerBg).Foreground(headerFg),
		HeaderActive: accent.Bold(true),
		HeaderSel:    lipgloss.NewStyle().Background(lipgloss.Blue).Foreground(selFg),
		HeaderHover:  lipgloss.NewStyle().Background(headerBg).Foreground(lipgloss.Cyan).Bold(true),
		Handle:       lipgloss.NewStyle().Background(headerBg).Foreground(lipgloss.Cyan).Bold(true),
		Pointer:      accent,
		Selection:    lipgloss.NewStyle().Background(lipgloss.Blue).Foreground(selFg),
		Hint:         lipgloss.NewStyle().Foreground(muted),
		Warning:      lipgloss.NewStyle().Foreground(lipgloss.Yellow),
		Error:        lipgloss.NewStyle().Foreground(lipgloss.BrightRed).Bold(true),
		Muted:        lipgloss.NewStyle().Foreground(muted),
		Key:          lipgloss.NewStyle().Bold(true),
		KeyChip:      lipgloss.NewStyle().Background(headerBg).Foreground(headerFg),
		ErrorCell:    lipgloss.NewStyle().Foreground(lipgloss.Red),
		ErrorMark:    lipgloss.NewStyle().UnderlineStyle(lipgloss.UnderlineCurly).UnderlineColor(lipgloss.Red),
		Link:         lipgloss.NewStyle().Foreground(link).Underline(true),
		Spilled:      lipgloss.NewStyle().Foreground(bar),
		Found:        lipgloss.NewStyle().Background(lipgloss.Yellow).Foreground(lipgloss.Black),
		Traced:       lipgloss.NewStyle().Background(lipgloss.Green).Foreground(lipgloss.Black),
		Argument:     lipgloss.NewStyle().Bold(true).Underline(true),
		Progress:     lipgloss.NewStyle().Foreground(bar),
		ProgressTodo: lipgloss.NewStyle().Foreground(lipgloss.BrightBlack),
		Copied: lipgloss.NewStyle().Foreground(lipgloss.Magenta).
			UnderlineStyle(lipgloss.UnderlineDashed).UnderlineSpaces(true),
		FrozenLine: lipgloss.NewStyle().Foreground(lipgloss.BrightBlack),
		Tab:        lipgloss.NewStyle(),
		TabActive:  accent.Bold(true),
		TabHover:   lipgloss.NewStyle().Foreground(bar).Bold(true).Underline(true),
		FilterOn:   lipgloss.NewStyle().Background(headerBg).Foreground(filterFg).Bold(true),
		NoteMark:   lipgloss.NewStyle().Foreground(noteFg),
		CellBorder: lipgloss.NewStyle().Foreground(borderFg),

		MenuBar:           lipgloss.NewStyle(),
		MenuAccel:         lipgloss.NewStyle().Underline(true),
		MenuSelected:      accent,
		MenuAccelSelected: accent.Underline(true),
		Border:            lipgloss.NewStyle().Foreground(lipgloss.BrightBlack),
		Title:             lipgloss.NewStyle().Bold(true),
		Disabled:          lipgloss.NewStyle().Foreground(lipgloss.BrightBlack), // exempt from contrast, like Sheets
		Match:             lipgloss.NewStyle().Foreground(match).Bold(true),
		MatchSelected:     accent.Bold(true).Underline(true),
		Cell:              lipgloss.NewStyle(),

		RowHeader: lipgloss.NewStyle().Background(headerBg).Foreground(headerFg),
		Name:      Terminal,

		ChartFrame:    lipgloss.NewStyle().Foreground(lipgloss.BrightBlack),
		ChartSelected: lipgloss.NewStyle().Foreground(lipgloss.Cyan).Bold(true),
		ChartAxis:     lipgloss.NewStyle().Foreground(lipgloss.BrightBlack),
		ChartLabel:    lipgloss.NewStyle().Foreground(muted),

		Invalid:  lipgloss.NewStyle().UnderlineStyle(lipgloss.UnderlineDotted).UnderlineColor(lipgloss.Yellow),
		Dropdown: lipgloss.NewStyle().Foreground(muted),
		scales:   map[scaleKey]Shade{},
		shades:   map[sheet.RuleStyle]Shade{},
	}
	ruleRoles(&t, dark)
	for i, c := range series {
		t.Series[i] = lipgloss.NewStyle().Foreground(c)
		t.SeriesBg[i] = lipgloss.NewStyle().Background(c)
		t.SeriesANSI[i] = int(c)
	}
	return t
}
