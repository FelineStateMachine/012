package ui

import (
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/fileio"
	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/ui/picker"
)

// Commands on linked sources: Data > Linked file > Link a source makes
// one, and on its tab the commands that read a sheet's data act on the
// source instead: sorting and filtering order the rows the tab shows
// (pushed down to SQLite, streamed over Parquet), a pivot table or a
// frequency table summarizes the whole source, and Copy copies the
// active cell. Everything that would change cells says the tab is
// read-only.

func init() {
	register(&command{id: "data.link_source", macro: macroNever, title: "Link a source",
		desc: "Read a Parquet file or a SQLite table or query in place, on a tab of its own, however big: formulas and pivot tables stream over it",
		run:  (*Model).openSourcePicker})
	register(&command{id: "data.source_all", title: "Original order",
		desc:    "Show the linked source's rows in its own order, unsorted and unfiltered",
		enabled: func(m *Model) bool { info, ok := m.sheet.Source(); return ok && !info.Source.Order.IsZero() },
		run: func(m *Model) tea.Cmd {
			info, _ := m.sheet.Source()
			return m.setSourceOrder(info, nil)
		}})
	register(&command{id: "data.source_reload", macro: macroNever, title: "Read the source again",
		desc:    "Read the linked source's file again, forgetting what formulas and pivot tables found in it",
		enabled: func(m *Model) bool { return m.sheet.IsSource() },
		run: func(m *Model) tea.Cmd {
			info, _ := m.sheet.Source()
			m.sources().host.Reload(info.Name)
			m.note = "Reading " + filepath.Base(info.Source.Path) + " again"
			return nil
		}})
}

// sourceTakes are the commands that act on a source's tab, the source
// standing for the tab's cells.
var sourceTakes = map[string]bool{
	"edit.copy": true, "data.sort_sheet_az": true, "data.sort_sheet_za": true, "data.sort_range_az": true,
	"data.sort_range_za": true, "data.filter": true, "data.filter_column": true, "data.filter_remove": true,
	"data.pivot": true, "data.frequency": true, "data.source_all": true, "data.source_reload": true,
	"data.link_source": true, "data.link": true, "goto": true,
}

// sourceCommand runs id on a source's tab, reporting whether it did:
// the commands that read data act on the source, and the rest, which
// would change cells, say they can't.
func (m *Model) sourceCommand(c *command) (tea.Cmd, bool) {
	info, ok := m.sheet.Source()
	switch {
	case !ok || notebookSafe(c.id) || c.id == "data.source_all" || c.id == "data.source_reload" || c.id == "data.link_source" || c.id == "data.link":
		return nil, false
	case !sourceTakes[c.id]:
		m.note = c.title + " works on a sheet's cells: this tab is a linked source, read-only"
		return nil, true
	case !info.Known:
		m.note = "The source isn't open yet"
		return nil, true
	}
	v := m.srcView()
	_, col := v.Active()
	switch c.id {
	case "edit.copy":
		_, text := m.sourceCell(v)
		m.note = "Copied " + text
		return tea.SetClipboard(text), true
	case "data.sort_sheet_az", "data.sort_range_az", "data.sort_sheet_za", "data.sort_range_za":
		desc := c.id == "data.sort_sheet_za" || c.id == "data.sort_range_za"
		o := copyOrder(info.Source.Order)
		o.Sort = []sheet.SourceSort{{Col: col, Desc: desc}}
		return m.setSourceOrder(info, o), true
	case "data.filter", "data.filter_column":
		m.openSourceFilter(info, col)
		return nil, true
	case "data.filter_remove":
		o := copyOrder(info.Source.Order)
		o.Filter = nil
		return m.setSourceOrder(info, o), true
	case "data.pivot", "data.frequency":
		return m.sourcePivot(info, col, c.id == "data.frequency"), true
	case "goto":
		m.openText("Go to a row, or a cell such as C5000000:", "", (*Model).sourceGoto)
		return nil, true
	}
	return nil, false
}

// sourceEnabled says whether a command runs on a source's tab, when
// the tab decides it: those that would change cells don't, and those
// acting on the source do, whatever they say of a sheet.
func sourceEnabled(m *Model, id string) (on, decided bool) {
	switch {
	case notebookSafe(id), id == "data.source_all", id == "data.source_reload":
		return false, false
	case !sourceTakes[id]:
		return false, true
	case id == "data.filter_remove":
		info, _ := m.sheet.Source()
		return info.Source.Order != nil && len(info.Source.Order.Filter) > 0, true
	}
	return true, true
}

// copyOrder is a copy of o to change, never nil.
func copyOrder(o *sheet.SourceOrder) *sheet.SourceOrder {
	out := &sheet.SourceOrder{}
	if o != nil {
		out.Sort, out.Filter = slices.Clone(o.Sort), slices.Clone(o.Filter)
	}
	return out
}

// setSourceOrder orders the source's tab as o says, as an undo step.
func (m *Model) setSourceOrder(info sheet.SourceInfo, o *sheet.SourceOrder) tea.Cmd {
	if err := m.book().SetSourceOrder(info.Name, o); err != nil {
		m.fail(err.Error())
	}
	return nil
}

// openSourceFilter asks for a condition on column col of the source,
// replacing the column's own.
func (m *Model) openSourceFilter(info sheet.SourceInfo, col int) {
	var cond sheet.Condition
	o := info.Source.Order
	if o != nil {
		for _, f := range o.Filter {
			if f.Col == col {
				cond = f.Cond
			}
		}
	}
	title := "Filter " + sheet.ColName(col) + "  " + m.sourceCol(info, col)
	p := m.openValuesPicker(title, m.srcView().ColX(col), nil, cond, func(cr sheet.Criteria) {
		o := copyOrder(info.Source.Order)
		o.Filter = slices.DeleteFunc(o.Filter, func(f sheet.SourceFilter) bool { return f.Col == col })
		if cr.Cond.Op != sheet.CondNone {
			o.Filter = append(o.Filter, sheet.SourceFilter{Col: col, Cond: cr.Cond})
		}
		m.setSourceOrder(info, o)
	})
	p.OnlyCondition()
}

// sourcePivot makes a pivot table over the whole source on a new sheet,
// or with frequency a frequency table of column col.
func (m *Model) sourcePivot(info sheet.SourceInfo, col int, frequency bool) tea.Cmd {
	src := info.Sheet
	r := sheet.Rect{To: sheet.Addr{Col: len(info.Shape.Cols) - 1, Row: sheet.MaxRows - 1}}
	p, name := sheet.NewPivot(src, r), ""
	if frequency {
		p = sheet.FrequencyPivot(src, r, col)
		name = "Frequency of " + m.sourceCol(info, col)
	}
	start := m.sheet.StateID()
	s, err := m.book().CreatePivot(src, r, name, p)
	if err != nil {
		m.fail(err.Error())
		return nil
	}
	m.showSheet(s)
	if !frequency {
		m.openPivotEditor(start, src)
	}
	return nil
}

// openSourcePicker asks for the Parquet file or SQLite database to link
// as a source: those here, or a path typed.
func (m *Model) openSourcePicker() tea.Cmd {
	var items []picker.Item
	names, _ := importable(m.root)
	for _, name := range names {
		k, _ := fileio.KindOf(name)
		if k != fileio.Parquet && k != fileio.SQLite {
			continue
		}
		items = append(items, picker.Item{Title: name, Name: len(name), Detail: k.Label(), Desc: "Link " + name + " as a source",
			Pick: func() tea.Cmd {
				m.closeOverlay()
				return m.linkSource(name)
			}})
	}
	p := m.newPicker("Link a source", "Type to filter, or a path", 72, items)
	p.Action = "link"
	p.Enter = func(query string) (tea.Cmd, bool) {
		if query == "" || len(p.Shown()) > 0 && !filepath.IsAbs(query) {
			return nil, false
		}
		m.closeOverlay()
		return m.linkSource(query), true
	}
	m.openOverlay(p)
	return nil
}

// linkSource links the file typed as name as a source, asking which
// table of a database with several.
func (m *Model) linkSource(name string) tea.Cmd {
	spec := fileio.SourceSpec{Path: name}
	if k, err := fileio.SourceKind(spec); err != nil {
		m.fail(fmt.Sprintf("Can't link %s: %v", filepath.Base(name), err))
		return nil
	} else if k == fileio.Parquet {
		m.addSource(sheet.LinkSource{Path: linkPath(name, m.filename)})
		return nil
	}
	src := sheet.LinkSource{Path: linkPath(name, m.filename)}
	query := picker.Item{Title: "A query…", Name: len("A query"), Desc: "Link what a SELECT reads from " + filepath.Base(name),
		Pick: func() tea.Cmd {
			m.closeOverlay()
			m.openText("Link the rows of the query:", "SELECT * FROM ", func(m *Model, text string) tea.Cmd {
				src.Query = text
				m.addSource(src)
				return nil
			})
			return nil
		}}
	m.chooseTable(name, src, []picker.Item{query}, m.addSource)
	return nil
}

// addSource adds a tab linking src after the sheet shown, and shows it.
func (m *Model) addSource(src sheet.LinkSource) {
	before := len(m.book().Sources())
	s, err := m.book().AddSource("", src, m.sheet)
	if err != nil {
		m.fail(err.Error())
		return
	}
	m.afterSheetsChange(s, m.book().Index(s))
	m.linkedHere(before)
	m.note = "Linked " + filepath.Base(src.Path) + " as " + s.Name() + ": formulas read it as " + s.Name() + "[column]"
}

// sourceGoto moves the active cell on a source's tab to where text
// says: a row number, or a column's letters and a row number, as the
// tab numbers its rows (the source's first row is 2).
func (m *Model) sourceGoto(text string) tea.Cmd {
	v := m.srcView()
	if v == nil {
		return nil
	}
	text = strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(text), ",", ""))
	letters := strings.TrimRightFunc(text, unicode.IsDigit)
	row, err := strconv.ParseInt(text[len(letters):], 10, 64)
	_, col := v.Active()
	if letters != "" {
		c, ok := sheet.ParseCol(letters)
		if !ok {
			err = strconv.ErrSyntax
		}
		col = c
	}
	if err != nil || row < 1 {
		m.fail("Type a row, such as 5000000, or a cell, such as C5000000")
		return nil
	}
	v.MoveTo(max(row-2, 0), col)
	return nil
}
