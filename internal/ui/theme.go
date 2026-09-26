package ui

import (
	"charm.land/lipgloss/v2"
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
	key          lipgloss.Style // a key name inside a hint, e.g. "Enter"
	errorCell    lipgloss.Style // cells whose value is ERR or NA
	// copied marks the range on the clipboard, like Sheets' dashed border:
	// a dashed underline across every cell, layered on the cell's own
	// style, with its own text color where the cell has none.
	copied lipgloss.Style

	// Chrome: the menu bar, dropdowns, the palette and dialogs.
	menuBar           lipgloss.Style // menu bar titles
	menuAccel         lipgloss.Style // a title's accelerator letter
	menuSelected      lipgloss.Style // open title, highlighted item or button
	menuAccelSelected lipgloss.Style // accelerator letter of the open title
	border            lipgloss.Style // box borders and separators
	title             lipgloss.Style // box titles and group headings
	disabled          lipgloss.Style // items that can't run right now
	match             lipgloss.Style // characters matched by a search
	matchSelected     lipgloss.Style // matched characters in the highlighted row
	button            lipgloss.Style // dialog buttons without focus
}

func newTheme(dark bool) theme {
	// Contrast was checked against the reference palettes in e2e: text on
	// colored backgrounds stays at or above roughly 4.5:1.
	headerBg, headerFg := lipgloss.BrightBlack, lipgloss.BrightWhite
	selFg, muted, match := lipgloss.Black, lipgloss.BrightBlack, lipgloss.Yellow
	if !dark {
		headerBg, headerFg = lipgloss.White, lipgloss.Black
		selFg, muted, match = lipgloss.BrightWhite, lipgloss.Black, lipgloss.Blue
	}
	accent := lipgloss.NewStyle().Background(lipgloss.Cyan).Foreground(lipgloss.Black)
	return theme{
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
		errorCell:    lipgloss.NewStyle().Foreground(lipgloss.Red),
		copied: lipgloss.NewStyle().Foreground(lipgloss.Magenta).
			UnderlineStyle(lipgloss.UnderlineDashed).UnderlineSpaces(true),

		menuBar:           lipgloss.NewStyle(),
		menuAccel:         lipgloss.NewStyle().Underline(true),
		menuSelected:      accent,
		menuAccelSelected: accent.Underline(true),
		border:            lipgloss.NewStyle().Foreground(lipgloss.BrightBlack),
		title:             lipgloss.NewStyle().Bold(true),
		disabled:          lipgloss.NewStyle().Foreground(lipgloss.BrightBlack), // exempt from contrast, like Sheets
		match:             lipgloss.NewStyle().Foreground(match).Bold(true),
		matchSelected:     accent.Bold(true).Underline(true),
		button:            lipgloss.NewStyle().Background(headerBg).Foreground(headerFg),
	}
}
