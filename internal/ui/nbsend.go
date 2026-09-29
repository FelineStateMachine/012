package ui

import (
	"context"
	"strconv"

	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/fileio"
	"github.com/FelineStateMachine/012/internal/notebook"
	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/ui/picker"
)

// Sending a cell's output to a sheet: the output becomes a region of the
// sheet named after the cell (sheet.Region.Output), which formulas read
// as nu.name, and whose rows the cell sends as live operations
// (Workbook.ApplyLive) whenever it runs. The region's rows aren't part
// of the undo history: when undo puts it back, or it moves, it is stale
// and the cell's output is sent again (syncOutputs).

// sendCell asks where the selected cell's output goes: a new sheet named
// after the cell, or a cell of a sheet there is.
func (m *Model) sendCell() tea.Cmd {
	nb := m.sheet
	c, _ := m.nbCell()
	name := c.Name()
	if name == "" {
		i, _ := m.nbView().Selected()
		name = m.book().FreeCellName("cell" + strconv.Itoa(i+1))
		m.renameCell(nb, c, name)
	}
	if t, r, ok := m.book().Region(name); ok {
		m.note = name + " is on " + t.Name() + " at " + r.At.String() + ": nu." + name + " reads it"
		return nil
	}
	items := []picker.Item{{Title: "New sheet " + name, Name: len("New sheet " + name), Desc: "Add a sheet named " + name + " with the output at A1",
		Pick: func() tea.Cmd {
			m.closeOverlay()
			return m.sendToNewSheet(nb, name)
		}}}
	for _, t := range m.visibleSheets() {
		if t.IsNotebook() {
			continue
		}
		items = append(items, picker.Item{Title: t.Name(), Name: len(t.Name()), Detail: "pick a cell", Desc: "Put the output on " + t.Name(),
			Pick: func() tea.Cmd {
				m.closeOverlay()
				m.askSendCell(nb, name, t)
				return nil
			}})
	}
	p := m.newPicker("Send "+name+" to", "Type a sheet name", 60, items)
	p.Action = "send"
	m.openOverlay(p)
	return nil
}

// sendToNewSheet sends name's output to a new sheet named after it.
func (m *Model) sendToNewSheet(nb *sheet.Sheet, name string) tea.Cmd {
	w := m.book()
	t, err := w.AddSheet(freeSheetName(w, name), w.Index(nb)+1)
	if err != nil {
		m.fail(err.Error())
		return nil
	}
	return m.sendTo(nb, name, t, sheet.Addr{})
}

// askSendCell asks for the cell of t the output starts at.
func (m *Model) askSendCell(nb *sheet.Sheet, name string, t *sheet.Sheet) {
	at := m.tabs.Places[t].Cur
	m.openText("Send "+name+" to "+t.Name()+" at:", at.String(), func(m *Model, text string) tea.Cmd {
		a, ok := sheet.ParseAddr(text)
		if !ok {
			m.fail(text + " isn't a cell")
			return nil
		}
		return m.sendTo(nb, name, t, a)
	})
}

// sendTo makes name's output a region of t at a, and sends it there.
func (m *Model) sendTo(nb *sheet.Sheet, name string, t *sheet.Sheet, a sheet.Addr) tea.Cmd {
	if err := t.AddRegion(sheet.Region{Name: name, At: a, Output: true}); err != nil {
		m.fail(err.Error())
		return nil
	}
	m.feedOutput(name)
	m.note = "Sent " + name + " to " + sheet.Qualified(t.Name(), sheet.Rect{From: a, To: a}) + ": =SUM(nu." + name + ") reads it"
	if m.sheet != nb {
		m.showSheet(nb)
	}
	return nil
}

// cellNamed finds the cell named name among the notebooks' cells.
func (m *Model) cellNamed(name string) (notebook.Cell, bool) {
	for _, s := range m.book().Sheets() {
		cells := s.NotebookCells()
		if i, ok := notebook.Names(cells)[name]; ok {
			return cells[i], true
		}
	}
	return notebook.Cell{}, false
}

// feedOutput sends the output of the cell named name to the region it
// was sent to, if it was: its rows, none when it hasn't run, or why
// when it failed or is gone.
func (m *Model) feedOutput(name string) {
	w := m.book()
	if name == "" {
		return
	}
	_, r, ok := w.Region(name)
	if !ok || !r.Output {
		return
	}
	op := sheet.LiveOp{Region: r.Name, Reset: true}
	c, ok := m.cellNamed(r.Name)
	switch o := w.Output(c.ID); {
	case !ok:
		op = sheet.LiveOp{Region: r.Name, Err: "its cell is gone"}
	case o == nil || o.Unsaved:
	case o.Failed():
		op = sheet.LiveOp{Region: r.Name, Err: o.Err}
	default:
		rows, note, err := fileio.NUONRows(context.Background(), o.NUON, 0)
		if err != nil {
			op = sheet.LiveOp{Region: r.Name, Err: err.Error()}
			break
		}
		op.Header, op.Rows, op.Note = rows.Header, rows.Rows, note
	}
	if err := w.ApplyLive(op); err != nil {
		m.warn = err.Error()
	}
}

// syncOutputs sends again the outputs whose regions are stale.
func (m *Model) syncOutputs() {
	for _, name := range m.book().StaleOutputs() {
		m.feedOutput(name)
	}
}

// moveOutput gives a sent output its cell's new name.
func (m *Model) moveOutput(r sheet.Region, name string) {
	if err := m.book().RenameRegion(r.Name, name); err != nil {
		m.warn = err.Error()
	}
}

// onOutputRegion reports whether the active cell is in an output sent
// to the sheet.
func onOutputRegion(m *Model) bool {
	r, ok := m.sheet.RegionAt(m.cur)
	return ok && r.Output
}

// freezeOutput turns the output sent here into plain values.
func (m *Model) freezeOutput() tea.Cmd {
	r, _ := m.sheet.RegionAt(m.cur)
	if err := m.sheet.FreezeRegion(r.Name); err != nil {
		m.fail(err.Error())
		return nil
	}
	m.note = "Froze " + r.Name + " into values: the cell no longer updates them"
	return nil
}

// deleteOutput removes the output sent here.
func (m *Model) deleteOutput() tea.Cmd {
	r, _ := m.sheet.RegionAt(m.cur)
	if err := m.sheet.DeleteRegion(r.Name); err != nil {
		m.fail(err.Error())
		return nil
	}
	m.note = "Removed " + r.Name + " from " + m.sheet.Name()
	return nil
}

// regionLine is the context line on a region: a linked file's state, or
// the cell an output comes from.
func (m *Model) regionLine() string {
	r, ok := m.sheet.RegionAt(m.cur)
	if !ok {
		return ""
	}
	if r.Linked() {
		return m.linkedLine() // linked.go
	}
	from := "a notebook"
	for _, s := range m.book().Sheets() {
		if _, ok := notebook.Names(s.NotebookCells())[r.Name]; ok {
			from = s.Name()
		}
	}
	return m.th.Muted.Render("Output of " + r.Name + " from " + from + ", nu." + r.Name + " in formulas")
}
