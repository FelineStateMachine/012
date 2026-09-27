package theme

import "github.com/FelineStateMachine/012/internal/sheet"

// Cell borders are drawn with box-drawing characters, so lines of
// different weights meet in the joints a terminal font draws for them.

// Junction returns the character drawn where cell edges meet, given the
// line of each arm: up, down, left and right (LineNone for no arm). A
// lone arm draws the whole line through, and no arm a space. Unicode
// joins double lines only to single ones, so heavy arms meeting a double
// one draw double, as does a single arm on the same axis as a double.
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

// joinable turns the arms of a junction with a double line into ones
// Unicode has a character for.
func joinable(arms [4]sheet.Line) [4]sheet.Line {
	double := false
	for _, a := range arms {
		double = double || a == sheet.LineDouble
	}
	if !double {
		return arms
	}
	for i, a := range arms {
		if a == sheet.LineThick {
			arms[i] = sheet.LineDouble
		}
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
