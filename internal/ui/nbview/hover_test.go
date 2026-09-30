package nbview

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/notebook"
)

// nu's hover, as testdata/nu records it: a command's signature and
// summary, a flag in its command's help, a variable's type, and the
// notebook's own word for a name it binds.
func TestNuHovers(t *testing.T) {
	_, n := nuFor(recordedNu(t))
	for _, c := range []struct{ word, text, desc string }{
		{"sort-by", "sort-by <...comparator: cell-path|closure> --reverse --ignore-case --natural --custom", "Sort by the given cell path or closure."},
		{"-r", "sort-by --reverse, -r", "Sort in reverse order."},
		{"--decimals", "into string --decimals <int>, -d", "Decimal digits to which to round."},
		{"$n", "$n: int", ""},
		{"$files", "$files: a cell's output", ""},
	} {
		at := strings.Index(hoverSrc, c.word+" ")
		if c.word == "$n" {
			at = strings.LastIndex(hoverSrc, c.word)
		}
		h, ok := n.Hover(context.Background(), hoverSrc, at+1)
		if !ok || h.Text != c.text || h.Desc != c.desc || hoverSrc[h.From:h.To] != c.word {
			t.Errorf("%s: %+v %v", c.word, h, ok)
		}
		if wantHelp := c.desc != ""; (h.Help != nil) != wantHelp {
			t.Errorf("%s: help %v", c.word, h.Help)
		}
	}
	n.SetWords([]string{"files"}, []Word{{Text: "$sheet.", Desc: "a range of a sheet: $sheet.A1:C9"}})
	if h, ok := n.Hover(context.Background(), "$sheet.A1:C9 | first", 3); !ok || h.Text != "$sheet.A1:C9: a range of a sheet" {
		t.Errorf("a range: %+v", h)
	}
}

func TestCommandStart(t *testing.T) {
	for _, c := range []struct{ src, flag, want string }{
		{"ls | sort-by size -r", "-r", "sort-by"},
		{"ls | each {|f| $f | sort-by x -r }", "-r", "sort-by"},
		{"let big = ls -a", "-a", "ls"},
		{"ls\n  | where ('a|b' | str contains -i x)", "-i", "str"},
		{"^git log --oneline", "--oneline", "git"},
	} {
		src := c.src
		at, ok := commandStart(src, notebook.Parse(src).Strings, strings.Index(src, c.flag))
		if !ok || !strings.HasPrefix(src[at:], c.want) {
			t.Errorf("%q: %d %q", src, at, src[at:])
		}
	}
}

// settle runs cmd and hands the view the messages it makes, and theirs,
// with typing's pauses over at once.
func settle(v *View, cmd tea.Cmd) []tea.Msg {
	var out []tea.Msg
	for _, msg := range drain(cmd) {
		if p, ok := msg.(paused); ok {
			msg = p.msg
		}
		out = append(out, msg)
		if next, ok := v.Update(msg); ok {
			out = append(out, settle(v, next)...)
		}
	}
	return out
}

// press types keys, then lets the caret rest.
func press(v *View, keys ...string) {
	var cmd tea.Cmd
	for _, k := range keys {
		cmd, _ = v.Key(key(k))
	}
	settle(v, cmd)
}

// The caret resting on a command shows its signature on the context
// line, F1 asks for its help, and a problem at the caret wins the line.
func TestHoverOnTheContextLine(t *testing.T) {
	f := recordedNu(t)
	h := newHost(hoverSrc)
	v := newView(h, 120, 20)
	v.EditKeys["f1"] = "nb.word_help"
	_, n := nuFor(f)
	v.Providers = Providers{Highlighter: n, Checker: n, Hoverer: n}
	settle(v, v.StartEdit())
	press(v, "home", "right", "right", "right", "right", "right", "right", "right", "right", "right", "right")
	left, right := v.ContextLine()
	if got := ansi.Strip(left); !strings.HasPrefix(got, "sort-by <...comparator: cell-path|closure>") || !strings.Contains(got, "Sort by the given") {
		t.Errorf("context line %q", got)
	}
	if !strings.Contains(ansi.Strip(right), "F1  help") {
		t.Errorf("keys %q", ansi.Strip(right))
	}
	msgs := settle(v, v.WordHelp())
	if len(msgs) != 1 || msgs[0].(HelpMsg).Help == nil || msgs[0].(HelpMsg).Help.Name != "sort-by" {
		t.Errorf("F1: %v", msgs)
	}

	v.edit.diags, v.edit.diagFor = []Diagnostic{{From: 19, To: 26, Msg: "Something's wrong."}}, v.edit.text()
	if left, _ := v.ContextLine(); ansi.Strip(left) != "Something's wrong." {
		t.Errorf("the problem gives way: %q", ansi.Strip(left))
	}
	v.edit.diags = nil

	cmd, _ := v.Key(key("home")) // the answer about the word left is dropped
	if _, ok := v.Update(hoveredMsg{view: v, moves: v.edit.moves - 1, src: v.edit.text(), hover: Hover{Text: "stale", To: 99}, ok: true}); !ok || v.edit.hover.Text == "stale" {
		t.Error("a stale answer was kept")
	}
	settle(v, cmd)
	if left, _ := v.ContextLine(); strings.Contains(ansi.Strip(left), "sort-by") {
		t.Errorf("the caret left the word: %q", ansi.Strip(left))
	}
}

// F1 on no command asks for the shortcuts.
func TestWordHelpWithoutAWord(t *testing.T) {
	h := newHost("ls ")
	v := newView(h, 80, 20)
	settle(v, v.StartEdit())
	msgs := settle(v, v.WordHelp())
	if len(msgs) != 1 || msgs[0].(HelpMsg).Help != nil {
		t.Errorf("F1 after a space: %v", msgs)
	}
}

// A $name the notebook binds says its cell and its value's shape,
// without asking nu.
func TestCellHover(t *testing.T) {
	h := newHost("files = ls", "x = 4\ny = [1 2 3]", "$files | append $x | append $y | append $gone")
	h.outs[1] = &notebook.Output{Count: 1, NUON: []byte("[{name: a, size: 1, type: file}, {name: b, size: 2, type: dir}]")}
	h.outs[2] = &notebook.Output{Count: 2, NUON: []byte("[1, 2, 3]"), Vars: map[string][]byte{"x": []byte("4")}}
	v := newView(h, 100, 30)
	v.Select(2, false)
	settle(v, v.StartEdit())
	src := v.edit.text()
	for word, want := range map[string]string{
		"$files": "$files: table, 2 rows × 3 columns from cell 1",
		"$x":     "$x: int from cell 2",
		"$y":     "$y: list, 3 items from cell 2",
	} {
		v.edit.area.Pos = len([]rune(src[:strings.Index(src, word)+2]))
		settle(v, v.rest())
		if left, _ := v.ContextLine(); ansi.Strip(left) != want {
			t.Errorf("%s: %q", word, ansi.Strip(left))
		}
	}
	h.outs[1] = nil
	v.edit.area.Pos = 2
	settle(v, v.rest())
	if left, _ := v.ContextLine(); ansi.Strip(left) != "$files: from cell 1, not run yet" {
		t.Errorf("not run: %q", ansi.Strip(left))
	}
	v.edit.area.Pos = len([]rune(src)) - 2
	settle(v, v.rest())
	if left, _ := v.ContextLine(); ansi.Strip(left) != "" {
		t.Errorf("a name no cell binds: %q", ansi.Strip(left))
	}
}

// A long signature gives up its flags, then its description, to fit.
func TestHoverLineFits(t *testing.T) {
	th := newHost().Theme()
	h := Hover{Text: "sort-by <...comparator: cell-path> --reverse --ignore-case", Brief: "sort-by <...comparator> …", Desc: "Sort by the given cell path or closure."}
	for room, want := range map[int]string{
		200: "sort-by <...comparator: cell-path> --reverse --ignore-case  Sort by the given cell path or closure.",
		90:  "sort-by <...comparator: cell-path> --reverse …  Sort by the given cell path or closure.",
		60:  "sort-by <...comparator> …  Sort by the given cell path or c…",
		40:  "sort-by <...comparator> …  Sort by the …",
	} {
		if got := ansi.Strip(hoverLine(th, h, room)); got != want || ansi.StringWidth(got) > room {
			t.Errorf("%d: %q", room, got)
		}
	}
}
