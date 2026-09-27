package ui

import (
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"012/internal/chart"
	"012/internal/sheet"
)

// theme holds every style the UI draws with. Views must use these roles
// instead of creating styles inline, so the look stays consistent and a
// new theme only has to be defined here.
//
// Colors are the 16 ANSI colors, so the sheet follows the user's terminal
// palette; dark and light variants only differ where ANSI colors would
// lose contrast.
type theme struct {
	indicator    lipgloss.Style // mode indicator, top right
	header       lipgloss.Style // column letters, row numbers and the name box
	headerActive lipgloss.Style // header of the focused row and column
	headerSel    lipgloss.Style // headers of selected rows and columns
	headerHover  lipgloss.Style // a header under the mouse
	handle       lipgloss.Style // a column resize handle being hovered or dragged
	pointer      lipgloss.Style // the cell pointer
	selection    lipgloss.Style // a range being pointed at
	hint         lipgloss.Style // guidance on the context line
	warning      lipgloss.Style // recoverable problems, e.g. a formula error
	error        lipgloss.Style // ERROR mode message
	muted        lipgloss.Style // secondary text: key hints, file lists
	key          lipgloss.Style // emphasized text in the status line, e.g. a range
	keyChip      lipgloss.Style // a key cap in hints, menus and the palette, e.g. " Enter "
	errorCell    lipgloss.Style // cells whose value is ERR or NA
	// errorMark is layered on an error's text, so errors show beyond
	// color: a curly underline, in the error color where the terminal
	// supports colored underlines.
	errorMark    lipgloss.Style
	link         lipgloss.Style // a cell's URL or HYPERLINK label, layered on the cell's role
	found        lipgloss.Style // cells matching an open search
	traced       lipgloss.Style // precedents or dependents being traced
	argument     lipgloss.Style // the argument at the caret in a function's signature
	progress     lipgloss.Style // the done part of an import's progress bar
	progressTodo lipgloss.Style // the rest of the progress bar
	// copied marks the range on the clipboard, like Sheets' dashed border:
	// a dashed underline across every cell, layered on the cell's own
	// style, with its own text color where the cell has none.
	copied lipgloss.Style
	// frozenLine divides frozen rows and columns from the scrolling ones,
	// like a tmux pane border.
	frozenLine lipgloss.Style
	// filterOn is the filter mark in the header of a column whose filter
	// hides something (the mark itself also changes, from ▾ to ▼).
	filterOn lipgloss.Style

	// Chrome: the menu bar, dropdowns, the palette and dialogs.
	menuBar           lipgloss.Style // menu bar titles
	menuAccel         lipgloss.Style // a title's accelerator letter
	menuSelected      lipgloss.Style // open title, highlighted item
	menuAccelSelected lipgloss.Style // accelerator letter of the open title
	border            lipgloss.Style // box borders and separators
	title             lipgloss.Style // box titles and group headings
	disabled          lipgloss.Style // items that can't run right now
	match             lipgloss.Style // characters matched by a search
	matchSelected     lipgloss.Style // matched characters in the highlighted row
	cell              lipgloss.Style // an ordinary cell: the base for bold, italic and underline

	// Charts: see charts.go.
	chartFrame    lipgloss.Style // a chart's border
	chartSelected lipgloss.Style // the border of the selected chart and its resize handle
	chartAxis     lipgloss.Style // axis lines and tick marks
	chartLabel    lipgloss.Style // tick values, category labels and legend text
	// series colors bars, lines, slices and legend swatches, in order;
	// seriesBg is the same colors as backgrounds, for the lower half of a
	// pie's half blocks. seriesANSI is the ANSI index of each, so images
	// can use the terminal's own colors.
	series     [chart.Colors]lipgloss.Style
	seriesBg   [chart.Colors]lipgloss.Style
	seriesANSI [chart.Colors]int
}

// imageID is the style of an image's Unicode placeholders: the
// foreground color is not a color but the image's id, in the 256-color
// palette, which is how the terminal knows which image to draw there.
func (t theme) imageID(id int) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(id))
}

// text adds a cell's bold, italic, underline and strikethrough to base,
// one of the cell roles (cell, pointer, selection, errorCell), so text
// styles show through the pointer and selection colors.
func (t theme) text(base lipgloss.Style, st sheet.Style) lipgloss.Style {
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

func newTheme(dark bool) theme {
	// Contrast was checked against the reference palettes in e2e: text on
	// colored backgrounds stays at or above roughly 4.5:1.
	headerBg, headerFg := lipgloss.BrightBlack, lipgloss.BrightWhite
	selFg, muted, match := lipgloss.Black, lipgloss.BrightBlack, lipgloss.Yellow
	bar := lipgloss.Cyan
	filterFg := lipgloss.Yellow
	if !dark {
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
	t := theme{
		indicator:    accent.Bold(true),
		header:       lipgloss.NewStyle().Background(headerBg).Foreground(headerFg),
		headerActive: accent.Bold(true),
		headerSel:    lipgloss.NewStyle().Background(lipgloss.Blue).Foreground(selFg),
		headerHover:  lipgloss.NewStyle().Background(headerBg).Foreground(lipgloss.Cyan).Bold(true),
		handle:       lipgloss.NewStyle().Background(headerBg).Foreground(lipgloss.Cyan).Bold(true),
		pointer:      accent,
		selection:    lipgloss.NewStyle().Background(lipgloss.Blue).Foreground(selFg),
		hint:         lipgloss.NewStyle().Foreground(muted),
		warning:      lipgloss.NewStyle().Foreground(lipgloss.Yellow),
		error:        lipgloss.NewStyle().Foreground(lipgloss.BrightRed).Bold(true),
		muted:        lipgloss.NewStyle().Foreground(muted),
		key:          lipgloss.NewStyle().Bold(true),
		keyChip:      lipgloss.NewStyle().Background(headerBg).Foreground(headerFg),
		errorCell:    lipgloss.NewStyle().Foreground(lipgloss.Red),
		errorMark:    lipgloss.NewStyle().UnderlineStyle(lipgloss.UnderlineCurly).UnderlineColor(lipgloss.Red),
		link:         lipgloss.NewStyle().Foreground(link).Underline(true),
		found:        lipgloss.NewStyle().Background(lipgloss.Yellow).Foreground(lipgloss.Black),
		traced:       lipgloss.NewStyle().Background(lipgloss.Green).Foreground(lipgloss.Black),
		argument:     lipgloss.NewStyle().Bold(true).Underline(true),
		progress:     lipgloss.NewStyle().Foreground(bar),
		progressTodo: lipgloss.NewStyle().Foreground(lipgloss.BrightBlack),
		copied: lipgloss.NewStyle().Foreground(lipgloss.Magenta).
			UnderlineStyle(lipgloss.UnderlineDashed).UnderlineSpaces(true),
		frozenLine: lipgloss.NewStyle().Foreground(lipgloss.BrightBlack),
		filterOn:   lipgloss.NewStyle().Background(headerBg).Foreground(filterFg).Bold(true),

		menuBar:           lipgloss.NewStyle(),
		menuAccel:         lipgloss.NewStyle().Underline(true),
		menuSelected:      accent,
		menuAccelSelected: accent.Underline(true),
		border:            lipgloss.NewStyle().Foreground(lipgloss.BrightBlack),
		title:             lipgloss.NewStyle().Bold(true),
		disabled:          lipgloss.NewStyle().Foreground(lipgloss.BrightBlack), // exempt from contrast, like Sheets
		match:             lipgloss.NewStyle().Foreground(match).Bold(true),
		matchSelected:     accent.Bold(true).Underline(true),
		cell:              lipgloss.NewStyle(),

		chartFrame:    lipgloss.NewStyle().Foreground(lipgloss.BrightBlack),
		chartSelected: lipgloss.NewStyle().Foreground(lipgloss.Cyan).Bold(true),
		chartAxis:     lipgloss.NewStyle().Foreground(lipgloss.BrightBlack),
		chartLabel:    lipgloss.NewStyle().Foreground(muted),
	}
	for i, c := range series {
		t.series[i] = lipgloss.NewStyle().Foreground(c)
		t.seriesBg[i] = lipgloss.NewStyle().Background(c)
		t.seriesANSI[i] = int(c)
	}
	return t
}
