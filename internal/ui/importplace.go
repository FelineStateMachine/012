package ui

import (
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/fileio"
	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/ui/picker"
	"github.com/FelineStateMachine/012/internal/ui/transfer"
)

// Where File > Import puts what it reads, as Sheets' Import location:
// into new sheets after the others, in place of the sheet shown, or in
// place of the whole spreadsheet (what File > Open and the command line
// do). A new, empty spreadsheet is just replaced. Inserting or replacing
// a sheet is one undo step, and the file stays the one being edited.

// The places are transfer.Book, transfer.NewSheets and transfer.Sheet.

// pristine reports whether the spreadsheet is new and untouched, so an
// import may simply replace it.
func (m *Model) pristine() bool {
	return !m.changed && m.filename == "" && m.xfer.Source == "" && m.book().Len() == 1 &&
		m.sheet.Len() == 0 && len(m.sheet.Charts()) == 0
}

// askImportPlace imports name, asking first where it goes unless the
// spreadsheet is new and empty.
func (m *Model) askImportPlace(name string) tea.Cmd {
	if m.pristine() {
		return m.startImport(name, fileio.Options{}, transfer.Book)
	}
	k, _ := fileio.KindOf(name)
	base := filepath.Base(name)
	insert, as := "Insert new sheet", " as a new sheet"
	if k.HoldsSheets() {
		insert, as = "Insert new sheets", " as new sheets"
	}
	item := func(title, detail, desc string, run func(m *Model) tea.Cmd) picker.Item {
		return picker.Item{Title: title, Name: len(title), Detail: detail, Desc: desc, Pick: func() tea.Cmd {
			m.closeOverlay()
			return run(m)
		}}
	}
	items := []picker.Item{item(insert, "after the others", "Add "+base+as,
		func(m *Model) tea.Cmd { return m.startImport(name, fileio.Options{}, transfer.NewSheets) })}
	if !k.HoldsSheets() {
		items = append(items, item("Replace current sheet", m.sheet.Name(), "Put "+base+" in place of "+m.sheet.Name()+", keeping its name",
			func(m *Model) tea.Cmd { return m.startImport(name, fileio.Options{}, transfer.Sheet) }))
	}
	detail := "open it instead"
	if m.changed {
		detail = "unsaved changes"
	}
	items = append(items, item("Replace spreadsheet", detail, "Open "+base+" instead, as File > Open does",
		func(m *Model) tea.Cmd { return m.confirmImport(name, fileio.Options{}) }))
	p := m.newPicker("Import "+base, "Import location", 60, items)
	p.Action = "import"
	m.openOverlay(p)
	return nil
}

// placeImport puts an imported workbook where the import was asked to go
// and says what it did, or reports false to replace the spreadsheet.
func (m *Model) placeImport(msg transfer.ImportedMsg, what string) bool {
	res := msg.Res
	label := "import " + filepath.Base(msg.Name)
	switch msg.Place {
	case transfer.NewSheets:
		ins, err := m.book().InsertBook(res.Sheet.Book(), label)
		if err != nil {
			m.fail(err.Error())
			return true
		}
		show := res.Sheet
		if show.Hidden() {
			show = ins.Sheets[0]
		}
		m.afterSheetsChange(show, m.book().Index(m.sheet))
		m.note = "Imported " + what + " as " + sheetList(ins.Sheets) + " (" + transfer.Rows(res.Rows) + ")"
		if ins.Names > 0 {
			m.note += "; " + plural(ins.Names, "1 named range", transfer.Thousands(ins.Names)+" named ranges") + " left out, their names taken"
		}
	case transfer.Sheet:
		i := m.book().Index(m.sheet)
		if err := m.book().ReplaceSheet(m.sheet, res.Sheet, label); err != nil {
			m.fail(err.Error())
			return true
		}
		m.afterSheetsChange(res.Sheet, i)
		m.note = "Imported " + what + " into " + res.Sheet.Name() + " (" + transfer.Rows(res.Rows) + ")"
	default:
		return false
	}
	m.changed = true
	if len(res.Notes) > 0 {
		m.note += "; " + strings.Join(res.Notes, "; ")
	}
	return true
}

// sheetList names sheets for a note: "Sales", "Sales and Q3", or "3
// sheets, Sales to Q3".
func sheetList(sheets []*sheet.Sheet) string {
	switch n := len(sheets); n {
	case 1:
		return sheets[0].Name()
	case 2:
		return sheets[0].Name() + " and " + sheets[1].Name()
	default:
		return transfer.Thousands(n) + " sheets, " + sheets[0].Name() + " to " + sheets[n-1].Name()
	}
}
