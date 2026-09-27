package e2e

import (
	"testing"
)

// Typing =su shows matching functions under the formula bar; Tab inserts
// one, and the context line then shows its signature.
func TestAutocompleteAndSignature(t *testing.T) {
	s := start(t, "")
	s.keys("2", "<enter>", "3", "<enter>", "=sum")
	s.waitFor("│ SUM ")
	s.waitFor("Sum of numbers")
	s.keys("<tab>")
	s.waitForEntry("=SUM(")
	s.waitFor("SUM(value1, [value2, ...])")
	s.keys("A1", ",", "A2", ")", "<enter>")
	s.waitForLine(gridRow1+2, numRow(3, "5"))
}

// A named range is defined from the selection, used in a formula, found
// by Go to, and follows inserted rows.
func TestNamedRanges(t *testing.T) {
	s := start(t, "")
	s.keys("10", "<enter>", "20", "<enter>", "=SUM(Pair)", "<enter>")
	s.waitFor("#NAME?")
	s.keys("<ctrl+home>", "<shift+down>", "<alt+d>", "d", "<enter>")
	s.waitFor("Name for A1:A2:")
	s.keys("Pair", "<enter>")
	s.waitForName("Pair")
	s.waitForLine(gridRow1+2, numRow(3, "30"))

	s.keys("<ctrl+home>", "<ctrl+alt+=>")
	s.waitForLine(gridRow1+3, numRow(4, "30"))
	s.keys("<f5>", "pair", "<enter>")
	s.waitForName("Pair")
	s.keys("<alt+d>", "n")
	s.waitFor("A2:A3")
	s.waitFor("│ Pair            A2:A3")
}

// Alt+, jumps to what a formula reads; Esc comes back.
func TestTracePrecedents(t *testing.T) {
	s := start(t, "")
	s.keys("1", "<enter>", "2", "<enter>", "=A1+A2", "<enter>", "<up>")
	s.keys("<alt+,>")
	s.waitFor("2 precedents of A3: A1, A2")
	s.waitForName("A1")
	s.keys("<alt+,>")
	s.waitForName("A2")
	s.keys("<esc>")
	s.waitForBar("A3", "=A1+A2")
}
