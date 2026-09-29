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

// keyLabel is how a key of Keys shows: "d d" as dd, others as chips do.
func keyLabel(k string) string {
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
	return keyLabel(found[0])
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

// ContextLine is what the context line says in the notebook: a problem,
// a key waiting for its pair, or the keys that apply.
func (v *View) ContextLine() (string, string) {
	th := v.h.Theme()
	switch {
	case v.full != nil:
		return v.full.ContextLine(th)
	case v.edit.on:
		if d := v.Diagnostic(); d != "" {
			return th.Warning.Render(d), v.hints(v.EditKeys, "nb.run_next", "run", "nb.command_mode", "done")
		}
		return v.hints(v.EditKeys, "nb.run_next", "run", "nb.run", "run here") + "   " + th.KeyHints("Tab", "complete"),
			v.hints(v.EditKeys, "nb.command_mode", "done")
	case v.pending == "d":
		return th.Hint.Render("d again deletes the cell"), th.KeyHints("Esc", "cancel")
	case v.pending == "0":
		return th.Hint.Render("0 again restarts: every output is cleared"), th.KeyHints("Esc", "cancel")
	}
	c, ok := v.Cell()
	if !ok {
		return v.hints(v.Keys, "nb.insert_below", "add a cell"), ""
	}
	st := v.h.State(c.ID)
	if st.Problem != "" {
		return th.Warning.Render(st.Problem), ""
	}
	if o := v.h.Output(c.ID); o.Failed() && !v.onOut {
		return th.Warning.Render("Failed: " + o.Err), v.hints(v.Keys, "nb.edit", "edit")
	}
	if v.onOut {
		return v.hints(v.Keys, "nb.open_output", "open", "nb.toggle_output", "all or less", "nb.send", "send to a sheet"), ""
	}
	return v.hints(v.Keys, "nb.edit", "edit", "nb.run_next", "run", "nb.insert_above", "add above", "nb.insert_below", "below",
		"nb.delete", "delete", "nb.send", "send to a sheet"), ""
}

// Boxes are the completions' box, under the caret, for a body whose
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
