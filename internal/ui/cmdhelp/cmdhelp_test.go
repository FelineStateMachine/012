package cmdhelp

import (
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/nushell"
	"github.com/FelineStateMachine/012/internal/ui/theme"
)

type fakeHost struct {
	th     theme.Theme
	w, h   int
	closed bool
}

func (f *fakeHost) Theme() *theme.Theme    { return &f.th }
func (f *fakeHost) Size() (int, int)       { return f.w, f.h }
func (f *fakeHost) Close()                 { f.closed = true }
func (f *fakeHost) Code(src string) string { return src }

var sortBy = nushell.Help{
	Name: "sort-by", Desc: "Sort by the given cell path or closure.", Usage: "sort-by {flags} <...comparator>",
	Flags: []nushell.Flag{{Short: "-r", Long: "--reverse", Desc: "Sort in reverse order."},
		{Short: "-i", Long: "--ignore-case", Desc: "Sort string-based data case-insensitively."}},
	Params:   []nushell.Param{{Name: "...comparator", Type: "oneof<cell-path, closure>", Desc: "The cell path(s) or closure(s) to compare elements by."}},
	Types:    []string{"table | table"},
	Examples: []nushell.Example{{Desc: "Sort files by modified date.", Code: "ls | sort-by modified"}, {Desc: "Reversed.", Code: "ls | sort-by name -r"}},
}

func boxText(v *View) []string {
	lines := v.Layout()[0].Lines
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = ansi.Strip(l)
	}
	return out
}

// The help says what the command does, links its docs, and lists its
// usage, flags, parameters, types and examples, scrolling in a box
// that fits the screen.
func TestHelpShowsTheCommand(t *testing.T) {
	f := &fakeHost{th: theme.New(true), w: 80, h: 16}
	v := New(f, sortBy)
	lines := boxText(v)
	box := strings.Join(lines, "\n")
	for _, want := range []string{"┌─ sort-by ", "Sort by the given cell path", "Docs: https://www.nushell.sh/commands/docs/sort-by.html", "Usage", "-r, --reverse"} {
		if !strings.Contains(box, want) {
			t.Errorf("no %q in\n%s", want, box)
		}
	}
	for _, l := range lines {
		if w := ansi.StringWidth(l); w > f.w {
			t.Errorf("a line %d wide: %q", w, l)
		}
	}
	if total := len(v.Lines()) + 1; !strings.Contains(lines[len(lines)-1], "of "+strconv.Itoa(total)) {
		t.Errorf("the count: %q", lines[len(lines)-1])
	}
	v.Key(tea.KeyPressMsg{Code: tea.KeyEnd})
	if box := strings.Join(boxText(v), "\n"); !strings.Contains(box, "ls | sort-by name -r") {
		t.Errorf("scrolled to the end\n%s", box)
	}
	v.Key(tea.KeyPressMsg{Code: tea.KeyEscape})
	if !f.closed {
		t.Error("Esc didn't close")
	}
}
