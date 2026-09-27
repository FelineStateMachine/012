package ui

import (
	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/ui/rowtext"
)

// grid is the sheet as the window shows it: which sheet, the active cell,
// the scroll position, the window size and the selection. Its methods map
// rows and columns to the screen and back (panes.go), move around the
// sheet and select (selection.go); they know nothing of modes, overlays
// or input, which belong to Model. Model embeds it, so m.sheet and m.cur
// read as the model's own.
type grid struct {
	sheet         *sheet.Sheet   // the sheet shown; its workbook is the file
	cur           sheet.Addr     // the active cell
	top, left     int            // first visible scrolling row and column
	width, height int            // the window
	shapes        rowtext.Shapes // how the rows drawn are laid out; see bands.go

	// The selection: see selection.go.
	selecting bool
	ext       sheet.Addr // the moving corner of the selection
	whole     wholeKind

	// entered is the cell of a merge the active cell last moved or was
	// clicked into, before it moved to the merge's top-left: stepping
	// off the merge sideways goes on along its row, and up or down along
	// its column, as in Sheets.
	entered sheet.Addr
}

func (g *grid) book() *sheet.Workbook { return g.sheet.Book() }
