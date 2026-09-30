package nbview

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/FelineStateMachine/012/internal/notebook"
)

// A cell of several statements: each name assigned shows as a variable,
// a comment's text as a comment, whatever it holds, and # in a string
// or a raw string is the string's.
func TestHighlightsStatements(t *testing.T) {
	src := "files = ls # all\n# big = $files\nbig = $files | where name != \"a#b\" | append r#'c # d'#"
	var got []string
	for _, s := range (Tokens{}).Highlight(context.Background(), src) {
		got = append(got, fmt.Sprintf("%s:%d", src[s.From:s.To], s.Kind))
	}
	want := fmt.Sprint([]string{"files:2", "=:5", "ls:0", "# all:6", "# big = $files:6", "big:2", "=:5", "$files:2", "|:5", "where:0",
		"!=:5", `"a#b":1`, "|:5", "append:0", "r#'c # d'#:1"})
	if fmt.Sprint(got) != want {
		t.Errorf("spans %v\nwant %v", got, want)
	}
}

// nu is asked about a cell of several statements with each `name =`
// blanked, every byte in its place, and the names its lines assign for
// the lines after declared.
func TestPrepareStatements(t *testing.T) {
	src := "files = ls # x = 1\nbig = $files | first"
	p := prepare(src, []string{"sales"})
	if body := p.text[p.at:]; body != "        ls # x = 1\n      $files | first" {
		t.Errorf("masked %q", body)
	}
	pre := p.text[:p.at]
	if !strings.Contains(pre, "let files: any = []\n") || strings.Contains(pre, "let big") || strings.Contains(pre, "let x") {
		t.Errorf("prelude %q", pre)
	}
}

// The context line says what a cell of several statements reads and
// sets: not its own variables.
func TestHeadStatements(t *testing.T) {
	h := newHost("files = ls | where type == file\n$files | where size > 1kb | sort-by size --reverse", "$files | length")
	v := newView(h, 60, 12)
	v.Select(0, false)
	if name, text := v.Head(); name != "cell 1" || text != "a nushell pipeline; sets $files" {
		t.Errorf("head %q %q", name, text)
	}
	v.Select(1, false)
	if _, text := v.Head(); text != "reads $files" {
		t.Errorf("head of the reader %q", text)
	}
	if (notebook.Cell{Source: h.cells[0].Source}).Name() != "" {
		t.Error("the cell is named")
	}
}
