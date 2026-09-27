package ui

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/FelineStateMachine/012/internal/macro"
)

// docs/reference/macro-api.md documents the scripting API and names command ids; these
// tests keep it honest.

func readMacroDoc(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile("../../docs/reference/macro-api.md")
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// TestMacroDocCommandIDs checks every command id the doc names exists and
// can run in a script.
func TestMacroDocCommandIDs(t *testing.T) {
	doc := readMacroDoc(t)
	start := strings.Index(doc, "| Id | Command |")
	end := strings.Index(doc[start:], "\n\n")
	ids := regexp.MustCompile("`([a-z_.0-9]+)`").FindAllStringSubmatch(doc[start:start+end], -1)
	if len(ids) < 20 {
		t.Fatalf("found %d ids", len(ids))
	}
	for _, id := range ids {
		if c, ok := commands[id[1]]; !ok || c.macro == macroNever {
			t.Errorf("docs/reference/macro-api.md names %q, which scripts can't run", id[1])
		}
	}
}

// TestMacroDocFunctions checks the doc covers every function scripts have.
func TestMacroDocFunctions(t *testing.T) {
	doc := readMacroDoc(t)
	for _, name := range macro.Functions() {
		if !strings.Contains(doc, "`"+name+"(") {
			t.Errorf("docs/reference/macro-api.md doesn't document %s", name)
		}
	}
}

// TestMacroDocExample runs the example script from the doc.
func TestMacroDocExample(t *testing.T) {
	doc := readMacroDoc(t)
	start := strings.Index(doc, "```python\n")
	end := strings.Index(doc[start:], "```\n\n")
	src := doc[start+len("```python\n") : start+end]
	m := newModel()
	press(t, m, "<right>", "<down>", "1", "<tab>", "x", "<tab>", "2", "<enter>", "3", "<tab>", "y", "<tab>", "4", "<enter>")
	press(t, m, "<up>", "<up>", "<shift+down>", "<shift+right>", "<shift+right>")
	script(t, m, src)
	if m.warn != "" {
		t.Fatal(m.warn)
	}
	if got := input(m, "B4") + " " + input(m, "C4") + " " + input(m, "D4"); got != "=SUM(B2:B3)  =SUM(D2:D3)" {
		t.Errorf("totals %q", got)
	}
	if c := m.sheet.Cell(addr("B4")); c == nil || !c.Style.Bold || m.cur != addr("B4") {
		t.Errorf("B4 isn't bold and active: %v", m.cur)
	}
}
