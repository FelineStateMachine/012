package ui

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/fileio"
	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/ui/picker"
	"github.com/FelineStateMachine/012/internal/ui/tabstrip"
	"github.com/FelineStateMachine/012/internal/ui/transfer"
)

// Linked files: Data > Linked file links a file at the active cell, and
// File > Import can follow the file in a new sheet; either way the
// file's table becomes a linked region, read-only, following the file
// (follow.go). The same menu pauses and resumes following, reads the
// file again, sets how many rows to keep, and unlinks, keeping the rows
// as values.

func init() {
	register(&command{id: "data.link", macro: macroNever, title: "Link a table",
		desc: "Show a file's table at the active cell, following the file as it grows or is rewritten",
		run:  (*Model).openLinkPicker})
	register(&command{id: "data.link_follow", macro: macroNever, title: "Follow",
		desc:    "Follow the linked file under the pointer as it changes, or pause following it",
		enabled: hasLinked, checked: func(m *Model) bool { r, ok := m.currentLinked(); return ok && !r.Paused },
		run: func(m *Model) tea.Cmd {
			r, _ := m.currentLinked()
			m.book().PauseLinked(r.Name, !r.Paused)
			m.note = "Following " + filepath.Base(r.Source.Path)
			if !r.Paused {
				m.note = "Paused following " + filepath.Base(r.Source.Path)
			}
			return nil
		}})
	register(&command{id: "data.link_reload", macro: macroNever, title: "Read again",
		desc: "Read the linked file under the pointer again, whole", enabled: hasLinked,
		run: func(m *Model) tea.Cmd {
			r, _ := m.currentLinked()
			m.book().ReloadLinked(r.Name)
			return nil
		}})
	register(&command{id: "data.link_rows", macro: macroNever, title: "Rows to keep",
		desc: "Keep every row of the linked file under the pointer, or only the last ones, as tail -f does", enabled: hasLinked,
		run: func(m *Model) tea.Cmd {
			r, _ := m.currentLinked()
			m.askFollowRows(filepath.Base(r.Source.Path), func(m *Model, window int) tea.Cmd {
				src := r.Source
				src.Window = window
				if err := m.book().SetLinkSource(r.Name, src); err != nil {
					m.fail(err.Error())
				}
				return nil
			})
			return nil
		}})
	register(&command{id: "data.unlink", title: "Unlink",
		desc: "Keep the linked file's rows as values and stop following it", enabled: hasLinked,
		run: func(m *Model) tea.Cmd {
			r, _ := m.currentLinked()
			if err := m.book().Unlink(r.Name); err != nil {
				m.fail(err.Error())
				return nil
			}
			m.note = "Unlinked " + filepath.Base(r.Source.Path) + "; its rows are values now"
			return nil
		}})
}

// linkedItems are Data > Linked file's items.
var linkedItems = []menuItem{
	{cmd: "data.link"}, sep, {cmd: "data.link_follow"}, {cmd: "data.link_reload"}, {cmd: "data.link_rows"}, sep, {cmd: "data.unlink"},
}

func hasLinked(m *Model) bool { _, ok := m.currentLinked(); return ok }

// currentLinked is the region the pointer is in, or the sheet's only
// one.
func (m *Model) currentLinked() (sheet.LinkedRegion, bool) {
	if r, ok := m.sheet.LinkedAt(m.cur); ok {
		return r, true
	}
	if rs := m.sheet.LinkedRegions(); len(rs) == 1 {
		return rs[0], true
	}
	return sheet.LinkedRegion{}, false
}

// openLinkPicker asks for the file to link at the active cell: the
// importable files here, or a path typed.
func (m *Model) openLinkPicker() tea.Cmd {
	at := m.cur
	var items []picker.Item
	names, _ := importable(m.root)
	for _, name := range names {
		k, _ := fileio.KindOf(name)
		items = append(items, picker.Item{Title: name, Name: len(name), Detail: k.Label(), Desc: "Link " + name + " at " + at.String(),
			Pick: func() tea.Cmd {
				m.closeOverlay()
				return m.linkFile(name, func(m *Model, src sheet.LinkSource) { m.linkAt(at, src) })
			}})
	}
	p := m.newPicker("Link a table", "Type to filter, or a path", 72, items)
	p.Action = "link"
	p.Enter = func(query string) (tea.Cmd, bool) {
		if query == "" || !strings.ContainsRune(query, filepath.Separator) && len(p.Shown()) > 0 {
			return nil, false
		}
		m.closeOverlay()
		return m.linkFile(query, func(m *Model, src sheet.LinkSource) { m.linkAt(at, src) }), true
	}
	m.openOverlay(p)
	return nil
}

// linkFile asks what to read of name (a table of a database) and how
// many rows to keep, then has place make the region.
func (m *Model) linkFile(name string, place func(*Model, sheet.LinkSource)) tea.Cmd {
	k, ok := fileio.KindOf(name)
	if !ok {
		m.fail(fmt.Sprintf("Can't link %s: 012 reads %s", name, importExts()))
		return nil
	}
	src := sheet.LinkSource{Path: linkPath(name, m.filename)}
	rows := func(src sheet.LinkSource) {
		m.askFollowRows(filepath.Base(name), func(m *Model, window int) tea.Cmd {
			src.Window = window
			place(m, src)
			return nil
		})
	}
	if !k.HasTables() {
		rows(src)
		return nil
	}
	path, ok := m.path("link", name)
	if !ok {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	tables, err := fileio.Tables(ctx, path)
	switch {
	case err != nil:
		m.fail(fmt.Sprintf("Couldn't read %s: %v", filepath.Base(name), m.root.Scrub(err.Error())))
	case len(tables) == 1:
		src.Table = tables[0].Name
		rows(src)
	default:
		var items []picker.Item
		for _, t := range tables {
			items = append(items, picker.Item{Title: t.Name, Name: len(t.Name), Detail: transfer.Rows(t.Rows), Desc: "Link " + t.Name,
				Pick: func() tea.Cmd {
					m.closeOverlay()
					src.Table = t.Name
					rows(src)
					return nil
				}})
		}
		m.openOverlay(m.newPicker("Link from "+filepath.Base(name), "Type to filter tables", 60, items))
	}
	return nil
}

// askFollowRows asks how many rows a linked file keeps: all of them, up
// to max-cells, or the last so many, then calls done with the window (0
// for all).
func (m *Model) askFollowRows(what string, done func(*Model, int) tea.Cmd) {
	m.ask(question{
		msg:  "Follow " + what + ":",
		desc: "Keep every row up to max-cells, or only the newest, older ones dropping as new ones arrive",
		choices: []choice{
			{key: "enter", label: "Keep every row", run: func(m *Model) tea.Cmd { return done(m, 0) }},
			{key: "l", label: "Keep the last rows", run: func(m *Model) tea.Cmd {
				m.openText("Keep the last how many rows:", "1000", func(m *Model, text string) tea.Cmd {
					n, err := strconv.Atoi(strings.ReplaceAll(strings.TrimSpace(text), ",", ""))
					if err != nil || n < 1 {
						m.fail("Type a number of rows, 1 or more")
						return nil
					}
					return done(m, n)
				})
				return nil
			}},
			{key: "esc", label: "Cancel", run: func(*Model) tea.Cmd { return nil }},
		},
	})
}

// linkAt makes a region reading src at a on the sheet shown.
func (m *Model) linkAt(a sheet.Addr, src sheet.LinkSource) {
	before := len(m.book().LinkedRegions())
	if _, err := m.sheet.AddLinked(a, src); err != nil {
		m.fail(err.Error())
		return
	}
	m.linkedHere(before)
	m.note = "Following " + filepath.Base(src.Path) + " at " + a.String()
}

// followInNewSheet makes a sheet after the one shown with a region at
// A1 reading src, as File > Import's Follow the file does.
func (m *Model) followInNewSheet(src sheet.LinkSource) {
	before := len(m.book().LinkedRegions())
	base := strings.TrimSuffix(filepath.Base(src.Path), filepath.Ext(src.Path))
	if src.Table != "" {
		base = src.Table
	}
	name := freeSheetName(m.book(), base)
	var s *sheet.Sheet
	err := m.book().Batch(sheet.Change{Label: "follow " + filepath.Base(src.Path), Sheet: m.sheet, Tabs: true}, func() error {
		var err error
		if s, err = m.book().AddSheet(name, m.book().Index(m.sheet)+1); err != nil {
			return err
		}
		_, err = s.AddLinked(sheet.Addr{}, src)
		return err
	})
	if err != nil {
		m.fail(err.Error())
		return
	}
	m.afterSheetsChange(s, m.book().Index(s))
	m.linkedHere(before)
	m.note = "Following " + filepath.Base(src.Path) + " in " + s.Name()
}

// linkedHere trusts the workbook on this computer once the user links a
// file, unless regions or macros came from elsewhere untrusted.
func (m *Model) linkedHere(before int) {
	w := m.book()
	if m.macroTrusted() || before == 0 && len(w.Macros()) == 0 && len(w.RunOrder()) == 0 {
		m.trustHere()
	}
}

// freeSheetName is base made a valid sheet name no sheet has: "Log",
// "Log 2".
func freeSheetName(w *sheet.Workbook, base string) string {
	base = strings.Map(func(r rune) rune {
		if strings.ContainsRune(`:\/?*[]'`, r) {
			return '_'
		}
		return r
	}, strings.TrimSpace(base))
	if r := []rune(base); len(r) > 28 {
		base = string(r[:28])
	}
	if sheet.ValidSheetName(base) != nil {
		base = "Linked"
	}
	name := base
	for n := 2; w.Lookup(name) != nil; n++ {
		name = base + " " + strconv.Itoa(n)
	}
	return name
}

// linkedLine says what the linked region under the pointer is doing:
// "● Following log.csv  1,204 rows, updated 12:03:04".
func (m *Model) linkedLine() string {
	r, ok := m.sheet.LinkedAt(m.cur)
	if !ok {
		return ""
	}
	name := filepath.Base(r.Source.Path)
	if r.Err != "" {
		line := m.th.Warning.Render("! " + name + ": " + r.Err)
		if r.Rows > 0 {
			line += m.th.Muted.Render(linkedCounts(r))
		}
		return line
	}
	state := liveMark(r) + " Following "
	if r.Paused {
		state = liveMark(r) + " Paused "
	}
	return m.th.Muted.Render(state) + m.th.Key.Render(name) + m.th.Muted.Render(linkedCounts(r))
}

// linkedCounts is a region's rows and last update, for linkedLine.
func linkedCounts(r sheet.LinkedRegion) string {
	out := "  " + transfer.Rows(r.Rows)
	if r.Source.Window > 0 {
		out += " (the last " + transfer.Thousands(r.Source.Window) + ")"
	}
	if r.Dropped > 0 {
		out += ", " + transfer.Thousands(r.Dropped) + " older dropped"
	}
	if !r.Updated.IsZero() {
		out += ", updated " + r.Updated.Local().Format(time.TimeOnly)
	}
	if r.Note != "" {
		out += "; " + r.Note
	}
	return out
}

// liveMark is the glyph a linked region's state shows as, beside its
// tab's name and on the context line: tabstrip.Mark.
func liveMark(r sheet.LinkedRegion) string { return tabstrip.Mark(r.Paused, r.Err != "") }
