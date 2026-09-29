package theme

import "charm.land/lipgloss/v2"

// Syntax is a kind of token in a code cell, which Theme.Code styles.
type Syntax int

const (
	SyntaxCommand  Syntax = iota // a command's name: ls, where, open
	SyntaxString                 // a string: "a", 'b', a bare word argument
	SyntaxVariable               // $name, $in
	SyntaxNumber                 // numbers, sizes, durations, dates
	SyntaxKeyword                // let, def, if, true, null
	SyntaxOperator               // |, ==, and, =
	SyntaxComment                // # to the end of the line
	NumSyntax
)

// codeRoles are the notebooks' roles.
func codeRoles(t *Theme, dark bool) {
	muted := lipgloss.BrightBlack
	command, number := lipgloss.Cyan, lipgloss.Yellow
	if !dark {
		muted = lipgloss.Black
		command, number = lipgloss.Blue, lipgloss.Red
	}
	t.Code = [NumSyntax]lipgloss.Style{
		SyntaxCommand:  lipgloss.NewStyle().Foreground(command),
		SyntaxString:   lipgloss.NewStyle().Foreground(lipgloss.Green),
		SyntaxVariable: lipgloss.NewStyle().Foreground(lipgloss.Magenta),
		SyntaxNumber:   lipgloss.NewStyle().Foreground(number),
		SyntaxKeyword:  lipgloss.NewStyle().Bold(true),
		SyntaxOperator: lipgloss.NewStyle().Bold(true),
		SyntaxComment:  lipgloss.NewStyle().Foreground(muted).Italic(true),
	}
	t.CellHead = lipgloss.NewStyle().Foreground(muted)
	t.CellBar = lipgloss.NewStyle().Foreground(lipgloss.Blue)
	t.CellBarEdit = lipgloss.NewStyle().Foreground(lipgloss.Green)
	t.OutputHead = lipgloss.NewStyle().Bold(true).Underline(true)
	t.Stale = lipgloss.NewStyle().Foreground(lipgloss.Yellow).Italic(true)
	if !dark {
		t.Stale = t.Stale.Foreground(lipgloss.Magenta)
	}
}
