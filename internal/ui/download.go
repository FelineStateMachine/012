package ui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/fileio"
	"github.com/FelineStateMachine/012/internal/sheet"
)

// File > Download: the workbook, or for formats with tables the selection,
// written in the background in any format fileio exports, asking before
// replacing a file or table.

func init() {
	for _, k := range fileio.Kinds() {
		if !k.CanExport() {
			continue
		}
		register(&command{
			id:    downloadID(k),
			title: "Download as " + k.String(),
			desc:  k.About(),
			run:   func(m *Model) tea.Cmd { return m.openDownload(k) },
			macro: macroNever,
		})
	}
}

// downloadID is the command downloading in format k.
func downloadID(k fileio.Kind) string { return "file.download." + strings.ToLower(k.String()) }

// downloadItems is the File > Download menu: every format 012 exports.
func downloadItems() []menuItem {
	var items []menuItem
	for _, k := range fileio.Kinds() {
		if k.CanExport() {
			items = append(items, menuItem{cmd: downloadID(k), title: k.MenuTitle()})
		}
	}
	return items
}

type exportedMsg struct {
	name string
	kind fileio.Kind
	res  *fileio.ExportResult
	err  error
}

// displayBase is the name downloads and saves start from: the sheet's
// file or the imported file, without its extension.
func (m *Model) displayBase() string {
	name := m.filename
	if name == "" {
		name = m.xfer.source
	}
	if name == "" {
		return "SHEET1"
	}
	return strings.TrimSuffix(name, filepath.Ext(name))
}

// openDownload asks where to download the sheet as k.
func (m *Model) openDownload(k fileio.Kind) tea.Cmd {
	if _, ok := m.sheet.UsedRange(); !ok {
		m.note = "Nothing to download: the sheet is empty"
		return nil
	}
	r := sheet.Rect{}
	label := "Download as " + k.String() + ":"
	if k.HasTables() && m.hasRange() {
		r = m.selection()
		label = "Download " + r.String() + " as " + k.String() + ":"
	}
	m.openText(label, m.displayBase()+k.Ext(), func(m *Model, text string) tea.Cmd {
		return m.downloadFile(k, r, text)
	})
	return nil
}

// downloadFile downloads r (the whole sheet when zero) as k to a file
// named for Download or :w, asking before replacing a file or table.
func (m *Model) downloadFile(k fileio.Kind, r sheet.Rect, text string) tea.Cmd {
	if text == "" {
		return nil
	}
	name := text
	if filepath.Ext(name) == "" {
		name += k.Ext()
	}
	path, ok := m.path("download to", name)
	if !ok {
		return nil
	}
	if k.HasTables() {
		m.openTableName(name, path, k, r)
		return nil
	}
	return m.confirmReplace(filepath.Base(name)+" exists.", func(m *Model) tea.Cmd {
		return m.download(name, path, k, r, "")
	}, exists(path))
}

func exists(name string) bool {
	_, err := os.Stat(name)
	return err == nil
}

// openTableName asks for the table to write in a database of kind k,
// name at path on disk.
func (m *Model) openTableName(name, path string, k fileio.Kind, r sheet.Rect) {
	m.openText("Table in "+filepath.Base(name)+":", fileio.TableName(filepath.Base(m.displayBase())), func(m *Model, table string) tea.Cmd {
		if table == "" {
			return nil
		}
		has := false
		if exists(path) {
			ts, err := fileio.Tables(context.Background(), path)
			if err != nil {
				m.fail(fmt.Sprintf("Couldn't open %s: %v", filepath.Base(name), err))
				return nil
			}
			for _, t := range ts {
				has = has || strings.EqualFold(t.Name, table)
			}
		}
		return m.confirmReplace("Table "+table+" exists in "+filepath.Base(name)+".", func(m *Model) tea.Cmd {
			return m.download(name, path, k, r, table)
		}, has)
	})
}

// confirmReplace runs do, first asking on the context line when it
// would replace something.
func (m *Model) confirmReplace(msg string, do func(*Model) tea.Cmd, replaces bool) tea.Cmd {
	if !replaces {
		return do(m)
	}
	m.openOverlay(&choiceBar{
		m:    m,
		msg:  msg,
		warn: true,
		choices: []choice{
			{key: "enter", label: "Replace", run: do},
			{key: "esc", label: "Cancel", run: func(*Model) tea.Cmd { return nil }},
		},
	})
	return nil
}

// download snapshots the sheet and writes it, as name at path on disk,
// in the background.
func (m *Model) download(name, path string, k fileio.Kind, r sheet.Rect, table string) tea.Cmd {
	snap := fileio.Snap(m.sheet, r, filepath.Base(m.displayBase()))
	if k.HoldsSheets() && r == (sheet.Rect{}) {
		snap = fileio.SnapBook(m.sheet) // every sheet, as Sheets' .xlsx download
	}
	return func() tea.Msg {
		res, err := fileio.Export(context.Background(), path, k, snap, fileio.ExportOptions{Table: table})
		return exportedMsg{name: name, kind: k, res: res, err: err}
	}
}

// saveImported is Save for a sheet imported from another format: save it
// as a .012 file (keeping formulas and formatting), or export it back.
func (m *Model) saveImported() tea.Cmd {
	k := m.xfer.kind
	choices := []choice{{key: "enter", label: "Save as " + filepath.Base(m.displayBase()) + sheet.FileExt, run: (*Model).openSave}}
	if k.CanExport() {
		choices = append(choices, choice{key: "e", label: "Download as " + k.String(), run: func(m *Model) tea.Cmd { return m.openDownload(k) }})
	}
	choices = append(choices, choice{key: "esc", label: "Cancel", run: func(m *Model) tea.Cmd {
		m.quitAfterSave = false
		return nil
	}})
	m.openOverlay(&choiceBar{m: m, msg: filepath.Base(m.xfer.source) + " was imported.", choices: choices})
	return nil
}
