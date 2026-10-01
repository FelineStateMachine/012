package e2e

import (
	"strings"
	"testing"
)

// An array formula spills, its spilled cells refuse edits and say where
// they come from, a cell in the way makes it #REF! until cleared, and the
// file keeps only the formula, which spills again when it opens.
func TestArraySpill(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	s := start(t, dir)
	s.keys("pear", "<enter>", "apple", "<enter>", "fig", "<enter>")
	s.keys("<ctrl+home>", "<right>", "=SORT(A1:A3)", "<enter>")
	s.eventually("the sorted spill", func() bool {
		return strings.Contains(s.line(gridRow1), "apple") && strings.Contains(s.line(gridRow1+1), "fig") &&
			strings.Contains(s.line(gridRow1+2), "pear")
	})
	s.keys("<down>") // B3, a spilled cell
	s.waitFor("Spilled from B1")
	s.keys("x")
	s.waitFor("B3 shows part of the array B1 spills")

	// A cell in the way blocks the array, and clearing it lets it spill.
	s.keys("<esc>", "<f5>", "B1", "<enter>", "=SORT(A1:A4)", "<enter>")
	s.keys("<f5>", "A4", "<enter>", "kiwi", "<enter>")
	s.eventually("four sorted", func() bool { return strings.Contains(s.line(gridRow1+3), "pear") })
	s.keys("<f5>", "B6", "<enter>", "stop", "<enter>", "<f5>", "B5", "<enter>", "=SEQUENCE(2)", "<enter>", "<up>")
	s.waitFor("would overwrite data in B6")
	s.keys("<down>", "<delete>", "<up>")
	s.eventually("spilled once cleared", func() bool { return strings.Contains(s.line(gridRow1+5), "2") })

	// The file keeps the formulas; their arrays spill again on opening.
	s.keys("<ctrl+s>", "fruit", "<enter>")
	s.eventually("saved", func() bool { return s.title() == "012 - fruit.012" })
	s.keys("<ctrl+q>")
	s.waitExit()
	r := start(t, dir, "fruit.012")
	r.eventually("spill after reopening", func() bool {
		return strings.Contains(r.line(gridRow1), "apple") && strings.Contains(r.line(gridRow1+2), "kiwi")
	})
}
