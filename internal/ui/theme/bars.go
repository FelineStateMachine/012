package theme

import "math"

// Data bars are drawn with the left eighth blocks, so a bar ends within
// a column and reads without color: the blank columns it covers are
// blocks in its color (RuleText), and text it runs under is drawn in
// reverse video in its color (BarOn).

// blocks are a column covered from the left by 0 to 8 eighths.
var blocks = [9]string{" ", "▏", "▎", "▍", "▌", "▋", "▊", "▉", "█"}

// Block is the glyph of a column a bar covers n eighths of, from the
// left: a space for none, a full block for all.
func Block(n int) string { return blocks[min(max(n, 0), 8)] }

// BarCover is how much of each of w columns a bar frac (0 to 1) of
// their width long covers, in eighths. A bar of a number above its
// shortest point is at least an eighth long, so it shows.
func BarCover(frac float64, w int) []int {
	out := make([]int, max(w, 0))
	n := int(math.Round(min(max(frac, 0), 1) * float64(8*w)))
	if n == 0 && frac > 0 {
		n = 1
	}
	for i := range out {
		out[i] = min(max(n-8*i, 0), 8)
	}
	return out
}
