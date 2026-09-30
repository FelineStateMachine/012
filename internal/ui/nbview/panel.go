package nbview

import (
	"slices"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/ui/overlay"
	"github.com/FelineStateMachine/012/internal/ui/theme"
)

// What the notebook puts on the control panel: the context line's
// hints and messages, and the completions' box.

// KeyLabel is how a key of Keys shows: "d d" as dd, a letter as it is
// typed (G is Shift+g), others as chips show them.
func KeyLabel(k string) string {
	if a, b, ok := strings.Cut(k, " "); ok {
		return a + b
	}
	if len(k) == 1 {
		return k
	}
	return theme.KeyLabel(k)
}

// keyFor is the key bound to command id in keys, the shortest first.
func keyFor(keys map[string]string, id string) string {
	var found []string
	for k, c := range keys {
		if c == id {
			found = append(found, k)
		}
	}
	slices.SortFunc(found, func(a, b string) int {
		if len(a) != len(b) {
			return len(a) - len(b)
		}
		return strings.Compare(a, b)
	})
	if len(found) == 0 {
		return ""
	}
	return KeyLabel(found[0])
}

// hints are key hints for commands, leaving out those without a key.
func (v *View) hints(keys map[string]string, pairs ...string) string {
	var out []string
	for i := 0; i+1 < len(pairs); i += 2 {
		if k := keyFor(keys, pairs[i]); k != "" {
			out = append(out, k, pairs[i+1])
		}
	}
	return v.h.Theme().KeyHints(out...)
}

// ContextLine is what the context line says in the notebook, at its
// left and its right: a problem, a key waiting for its pair, or what
// the selection is, and the keys that apply that the toolbar doesn't
// show.
func (v *View) ContextLine() (string, string) {
	th := v.h.Theme()
	switch {
	case v.full != nil:
		return v.full.ContextLine(th)
	case v.edit.on:
		right := v.hints(v.EditKeys, "nb.command_mode", "done") + "  " + th.KeyHints("Tab", "complete")
		if d := v.Diagnostic(); d != "" {
			return th.Warning.Render(d), right
		}
		if h, ok := v.hovered(); ok {
			if h.Help != nil {
				right = v.hints(v.EditKeys, "nb.word_help", "help")
			}
			return hoverLine(th, h, v.width-ansi.StringWidth(right)-6), right
		}
		return "", right
	case v.pending == "d":
		return th.Hint.Render("d again deletes " + v.selectedWords()), th.KeyHints("Esc", "cancel")
	case v.pending == "0":
		return th.Hint.Render("0 again restarts: every output is cleared"), th.KeyHints("Esc", "cancel")
	}
	c, ok := v.Cell()
	if !ok {
		return "", v.hints(v.Keys, "nb.insert_below", "add a cell")
	}
	if from, to := v.Range(); to > from {
		return th.Hint.Render(v.selectedWords() + " selected"), v.hints(v.Keys, "nb.delete", "delete", "nb.move_up", "move up", "nb.move_down", "down")
	}
	st := v.h.State(c.ID)
	if st.Problem != "" {
		return th.Warning.Render(st.Problem), ""
	}
	if o := v.h.Output(c.ID); o.Failed() && !v.onOut {
		return th.Warning.Render("Failed: " + o.Err), v.hints(v.Keys, "nb.edit", "edit")
	}
	if v.onOut {
		enter := "full-screen"
		if v.shown(c).isGrid() {
			enter = "work in it"
		}
		return th.Muted.Render(v.outputSummary(c)), v.hints(v.Keys, "nb.edit", enter, "nb.toggle_output", "hide",
			"nb.toggle_whole", "whole", "nb.send", "to a sheet")
	}
	name, text := v.Head()
	return th.Muted.Render(name + ": " + text), v.hints(v.Keys, "nb.edit", "edit") + "  " + th.KeyHints("Shift+↑↓", "select")
}

// selectedWords names the selected cells: "the cell", "3 cells".
func (v *View) selectedWords() string {
	if from, to := v.Range(); to > from {
		return itoa(to-from+1) + " cells"
	}
	return "the cell"
}

// Rule is the context line drawn at width: left, then a rule, then
// right, the rule closing off the toolbar above the cells.
func (v *View) Rule(left, right string, width int) string {
	th := v.h.Theme()
	lw, rw := ansi.StringWidth(left), ansi.StringWidth(right)
	if lw+rw+4 > width {
		right, rw = "", 0
	}
	if left != "" {
		left = " " + ansi.Truncate(left, width-2, "…") + " "
		lw = ansi.StringWidth(left)
	}
	if right != "" {
		right = " " + right + " "
		rw += 2
	}
	return left + th.Border.Render(strings.Repeat("─", max(width-lw-rw, 0))) + right
}

// first line is at screen row top.
func (v *View) Boxes(top int) []overlay.Box {
	if !v.edit.on || len(v.edit.comp) == 0 {
		return nil
	}
	lines := v.completionBox(v.h.Theme())
	x, y, ok := v.Cursor()
	if !ok || len(lines) == 0 {
		return nil
	}
	caret := top + y
	y = caret + 1
	if y+len(lines) > top+v.height {
		y = max(caret-len(lines), 0) // no room below: above the caret
	}
	x = max(min(x, v.width-ansi.StringWidth(lines[0])), 0)
	return []overlay.Box{{ID: "nbcomplete", X: x, Y: y, Lines: lines}}
}
