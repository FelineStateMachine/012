package theme

import "github.com/FelineStateMachine/012/internal/sheet"

// Cell borders are drawn with box-drawing characters, so lines of
// different weights meet in the joints a terminal font draws for them.

// Junction returns the character drawn where cell edges meet, given the
// line of each arm: up, down, left and right (LineNone for no arm). A
// lone arm draws the whole line through, and no arm a space. Unicode
// joins double lines only to single ones: a single arm on the same axis
// as a double draws double, and where a double line meets a thick one
// the joint is drawn thick, the heavier look, as Unicode has no joint of
// the two (╠ between ┃ lines reads as a stray mark). The double line
// then starts beside the joint with a thick stub (see Bridges).
func Junction(up, down, left, right sheet.Line) string {
	arms := joinable([4]sheet.Line{up, down, left, right})
	n := 0
	for _, a := range arms {
		if a != sheet.LineNone {
			n++
		}
	}
	switch {
	case n == 0:
		return " "
	case n == 1 && (arms[0] != sheet.LineNone || arms[1] != sheet.LineNone):
		arms[0], arms[1] = max(arms[0], arms[1]), max(arms[0], arms[1])
	case n == 1:
		arms[2], arms[3] = max(arms[2], arms[3]), max(arms[2], arms[3])
	}
	if g, ok := junctions[arms]; ok {
		return g
	}
	for i, a := range arms { // not in Unicode: draw it light
		arms[i] = min(a, sheet.LineThin)
	}
	return junctions[arms]
}

// Bridges reports which of a junction's horizontal arms are double lines
// it draws thick, meeting a thick line: the line beside the joint on
// that side starts with a thick stub (━═══), so the joint's arm runs on
// into it rather than stopping at a gap.
func Bridges(up, down, left, right sheet.Line) (onLeft, onRight bool) {
	if !mixed([4]sheet.Line{up, down, left, right}) {
		return false, false
	}
	return left == sheet.LineDouble, right == sheet.LineDouble
}

// Mixed reports whether a junction of these arms joins double and thick
// lines, which it draws thick.
func Mixed(up, down, left, right sheet.Line) bool {
	return mixed([4]sheet.Line{up, down, left, right})
}

// mixed reports whether a junction joins double and thick lines.
func mixed(arms [4]sheet.Line) bool {
	double, thick := false, false
	for _, a := range arms {
		double = double || a == sheet.LineDouble
		thick = thick || a == sheet.LineThick
	}
	return double && thick
}

// joinable turns the arms of a junction with a double line into ones
// Unicode has a character for.
func joinable(arms [4]sheet.Line) [4]sheet.Line {
	if mixed(arms) {
		for i, a := range arms {
			if a == sheet.LineDouble {
				arms[i] = sheet.LineThick
			}
		}
		return arms
	}
	double := false
	for _, a := range arms {
		double = double || a == sheet.LineDouble
	}
	if !double {
		return arms
	}
	for _, axis := range [2][2]int{{0, 1}, {2, 3}} {
		x, y := &arms[axis[0]], &arms[axis[1]]
		if *x != sheet.LineNone && *y != sheet.LineNone && *x != *y {
			*x, *y = sheet.LineDouble, sheet.LineDouble
		}
	}
	return arms
}

// junctions are Unicode's box-drawing characters by the line of their
// arms: up, down, left, right.
var junctions = map[[4]sheet.Line]string{
	{0, 0, 1, 1}: "─",
	{0, 0, 1, 2}: "╼",
	{0, 0, 2, 1}: "╾",
	{0, 0, 2, 2}: "━",
	{0, 0, 3, 3}: "═",
	{0, 1, 0, 1}: "┌",
	{0, 1, 0, 2}: "┍",
	{0, 1, 0, 3}: "╒",
	{0, 1, 1, 0}: "┐",
	{0, 1, 1, 1}: "┬",
	{0, 1, 1, 2}: "┮",
	{0, 1, 2, 0}: "┑",
	{0, 1, 2, 1}: "┭",
	{0, 1, 2, 2}: "┯",
	{0, 1, 3, 0}: "╕",
	{0, 1, 3, 3}: "╤",
	{0, 2, 0, 1}: "┎",
	{0, 2, 0, 2}: "┏",
	{0, 2, 1, 0}: "┒",
	{0, 2, 1, 1}: "┰",
	{0, 2, 1, 2}: "┲",
	{0, 2, 2, 0}: "┓",
	{0, 2, 2, 1}: "┱",
	{0, 2, 2, 2}: "┳",
	{0, 3, 0, 1}: "╓",
	{0, 3, 0, 3}: "╔",
	{0, 3, 1, 0}: "╖",
	{0, 3, 1, 1}: "╥",
	{0, 3, 3, 0}: "╗",
	{0, 3, 3, 3}: "╦",
	{1, 0, 0, 1}: "└",
	{1, 0, 0, 2}: "┕",
	{1, 0, 0, 3}: "╘",
	{1, 0, 1, 0}: "┘",
	{1, 0, 1, 1}: "┴",
	{1, 0, 1, 2}: "┶",
	{1, 0, 2, 0}: "┙",
	{1, 0, 2, 1}: "┵",
	{1, 0, 2, 2}: "┷",
	{1, 0, 3, 0}: "╛",
	{1, 0, 3, 3}: "╧",
	{1, 1, 0, 0}: "│",
	{1, 1, 0, 1}: "├",
	{1, 1, 0, 2}: "┝",
	{1, 1, 0, 3}: "╞",
	{1, 1, 1, 0}: "┤",
	{1, 1, 1, 1}: "┼",
	{1, 1, 1, 2}: "┾",
	{1, 1, 2, 0}: "┥",
	{1, 1, 2, 1}: "┽",
	{1, 1, 2, 2}: "┿",
	{1, 1, 3, 0}: "╡",
	{1, 1, 3, 3}: "╪",
	{1, 2, 0, 0}: "╽",
	{1, 2, 0, 1}: "┟",
	{1, 2, 0, 2}: "┢",
	{1, 2, 1, 0}: "┧",
	{1, 2, 1, 1}: "╁",
	{1, 2, 1, 2}: "╆",
	{1, 2, 2, 0}: "┪",
	{1, 2, 2, 1}: "╅",
	{1, 2, 2, 2}: "╈",
	{2, 0, 0, 1}: "┖",
	{2, 0, 0, 2}: "┗",
	{2, 0, 1, 0}: "┚",
	{2, 0, 1, 1}: "┸",
	{2, 0, 1, 2}: "┺",
	{2, 0, 2, 0}: "┛",
	{2, 0, 2, 1}: "┹",
	{2, 0, 2, 2}: "┻",
	{2, 1, 0, 0}: "╿",
	{2, 1, 0, 1}: "┞",
	{2, 1, 0, 2}: "┡",
	{2, 1, 1, 0}: "┦",
	{2, 1, 1, 1}: "╀",
	{2, 1, 1, 2}: "╄",
	{2, 1, 2, 0}: "┩",
	{2, 1, 2, 1}: "╃",
	{2, 1, 2, 2}: "╇",
	{2, 2, 0, 0}: "┃",
	{2, 2, 0, 1}: "┠",
	{2, 2, 0, 2}: "┣",
	{2, 2, 1, 0}: "┨",
	{2, 2, 1, 1}: "╂",
	{2, 2, 1, 2}: "╊",
	{2, 2, 2, 0}: "┫",
	{2, 2, 2, 1}: "╉",
	{2, 2, 2, 2}: "╋",
	{3, 0, 0, 1}: "╙",
	{3, 0, 0, 3}: "╚",
	{3, 0, 1, 0}: "╜",
	{3, 0, 1, 1}: "╨",
	{3, 0, 3, 0}: "╝",
	{3, 0, 3, 3}: "╩",
	{3, 3, 0, 0}: "║",
	{3, 3, 0, 1}: "╟",
	{3, 3, 0, 3}: "╠",
	{3, 3, 1, 0}: "╢",
	{3, 3, 1, 1}: "╫",
	{3, 3, 3, 0}: "╣",
	{3, 3, 3, 3}: "╬",
}
