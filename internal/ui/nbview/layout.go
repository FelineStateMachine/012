package nbview

import (
	"strings"

	"github.com/FelineStateMachine/012/internal/notebook"
	"github.com/FelineStateMachine/012/internal/ui/lineedit"
)

// Where things go across the body, as in Jupyter: a bar marking the
// active cell, the prompts ([1]: and Out[1]:) right-aligned in a column
// of their own, then a code cell's box with its text two columns in,
// and a ▶ after the box. Outputs and notes' text line up with the code.
//
//	▌    [1]: ╭─ files ──────────────── ✓ <1s ─╮
//	          │ ls                              │ ▶
//	          ╰─────────────────────────────────╯
//	  Out[1]:   name       type  size
const (
	promptW = 8               // [12]: and Out[12]:, right-aligned
	gutter  = 1 + promptW + 1 // the bar, the prompt, a space: where a box starts
	textX   = gutter + 2      // where code, notes and outputs start
	runW    = 2               // " ▶" after a box
	boxW    = 4               // a box's sides and the spaces inside them
)

// content is how wide a cell's code is drawn: inside its box.
func (v *View) content() int { return max(v.width-gutter-boxW-runW, 8) }

// outWidth is how wide outputs and notes are drawn.
func (v *View) outWidth() int { return max(v.width-textX-1, 8) }

// block is what a cell takes on the screen, in lines: a box's borders
// (box is 1 for a code cell, or a note being edited), its source, its
// output, and a blank line after.
type block struct {
	box, src, out int
}

func (b block) lines() int { return 2*b.box + b.src + b.out + 1 }

// rowKind is what a line of a block is.
type rowKind int

const (
	rowTop    rowKind = iota // the box's top border
	rowSrc                   // a line of source, or of a note
	rowBottom                // the box's bottom border
	rowOut                   // a line of output
	rowGap                   // the blank line after the cell
)

// row is what line k of the block is, and which of its kind.
func (b block) row(k int) (rowKind, int) {
	switch {
	case k < b.box:
		return rowTop, 0
	case k < b.box+b.src:
		return rowSrc, k - b.box
	case k < 2*b.box+b.src:
		return rowBottom, 0
	case k < 2*b.box+b.src+b.out:
		return rowOut, k - 2*b.box - b.src
	}
	return rowGap, 0
}

// boxed reports whether cell i is drawn in a box: code, or a note being
// edited.
func (v *View) boxed(i int, c notebook.Cell) bool {
	return c.Kind == notebook.Code || v.edit.on && i == v.sel
}

// blockOf lays out cell i.
func (v *View) blockOf(i int, c notebook.Cell) block {
	var b block
	if v.boxed(i, c) {
		b.box = 1
	}
	switch {
	case v.edit.on && i == v.sel:
		b.src = len(v.edit.rows(v.content()))
	case c.Kind == notebook.Note:
		b.src = len(v.noteLines(c))
	default:
		b.src = len(lineedit.Wrap([]rune(c.Source), v.content()))
	}
	b.out = v.outHeight(c)
	return b
}

// outHeight is how many lines cell c's output takes: none, one while
// it's folded, or its window.
func (v *View) outHeight(c notebook.Cell) int {
	sh := v.shown(c)
	switch f := v.foldOf(c.ID); {
	case sh.kind == outNone:
		return 0
	case f.hidden:
		return 1
	default:
		return sh.height(f.whole, v.outWidth())
	}
}

// noteLines are a note cell's lines as drawn.
func (v *View) noteLines(c notebook.Cell) []string {
	if strings.TrimSpace(c.Source) == "" {
		return []string{v.h.Theme().Muted.Render("An empty note: Enter writes it, in Markdown")}
	}
	return markdown(v.h.Theme(), c.Source, v.outWidth())
}
