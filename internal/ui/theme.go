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
	header       lipgloss.Style // column letters and row numbers
	headerActive lipgloss.Style // header of the focused row and column
	headerSel    lipgloss.Style // headers of selected rows and columns
	headerHover  lipgloss.Style // a header under the mouse
	handle       lipgloss.Style // a column resize handle being hovered or dragged
	pointer      lipgloss.Style // the cell pointer
	selection    lipgloss.Style // a range being pointed at
	menuSelected lipgloss.Style // highlighted menu item
	hint         lipgloss.Style // guidance on the third panel line
	warning      lipgloss.Style // recoverable problems, e.g. a formula error
	error        lipgloss.Style // ERROR mode message
	muted        lipgloss.Style // secondary text: key hints, file lists
	key          lipgloss.Style // a key name inside a hint, e.g. "Enter"
	errorCell    lipgloss.Style // cells whose value is ERR or NA
}

func newTheme(dark bool) theme {
	// Contrast was checked against the reference palettes in e2e: text on
	// colored backgrounds stays at or above roughly 4.5:1.
	headerBg, headerFg := lipgloss.BrightBlack, lipgloss.BrightWhite
	selFg, muted := lipgloss.Black, lipgloss.BrightBlack
	if !dark {
		headerBg, headerFg = lipgloss.White, lipgloss.Black
		selFg, muted = lipgloss.BrightWhite, lipgloss.Black
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
		menuSelected: accent.Bold(true),
		hint:         lipgloss.NewStyle().Foreground(muted),
		warning:      lipgloss.NewStyle().Foreground(lipgloss.Yellow),
		error:        lipgloss.NewStyle().Foreground(lipgloss.BrightRed).Bold(true),
		muted:        lipgloss.NewStyle().Foreground(muted),
		key:          lipgloss.NewStyle().Bold(true),
		errorCell:    lipgloss.NewStyle().Foreground(lipgloss.Red),
	}
}
