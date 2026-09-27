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

// File > Download: the sheet, or for SQLite the selection, written in
// the background as CSV, TSV, Excel or a SQLite table, asking before
// replacing a file or table.

func init() {
	for _, k := range fileio.Kinds() {
		if !k.CanExport() {
			continue
		}
		register(&command{
			id:    "file.download." + strings.ToLower(k.String()),
			title: "Download as " + k.String(),
			desc:  downloadDesc(k),
			run:   func(m *Model) tea.Cmd { return m.openDownload(k) },
		})
	}
}

type exportedMsg struct {
	name string
	kind fileio.Kind
	res  *fileio.ExportResult
	err  error
}

func downloadDesc(k fileio.Kind) string {
	switch k {
	case fileio.CSV, fileio.TSV:
		return "Save the values as shown, as " + strings.ToLower(k.Label()) + " (" + k.Ext() + ")"
	case fileio.XLSX:
		return "Save as an Excel workbook (.xlsx) with formulas, formats and widths"
	case fileio.SQLite:
		return "Save the sheet, or the selection, as a table in a SQLite database"
	}
	return ""
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
	if k == fileio.SQLite && m.hasRange() {
		r = m.selection()
		label = "Download " + r.String() + " as SQLite:"
	}
	m.openText(label, m.displayBase()+k.Ext(), func(m *Model, text string) tea.Cmd {
		if text == "" {
			return nil
		}
		name := text
		if filepath.Ext(name) == "" {
			name += k.Ext()
		}
		if k == fileio.SQLite {
			m.openTableName(name, r)
			return nil
		}
		return m.confirmReplace(filepath.Base(name)+" exists.", func(m *Model) tea.Cmd {
			return m.download(name, k, r, "")
		}, exists(name))
	})
	return nil
}

func exists(name string) bool {
	_, err := os.Stat(name)
	return err == nil
}

// openTableName asks for the table to write in a SQLite database.
func (m *Model) openTableName(name string, r sheet.Rect) {
	m.openText("Table in "+filepath.Base(name)+":", fileio.TableName(filepath.Base(m.displayBase())), func(m *Model, table string) tea.Cmd {
		if table == "" {
			return nil
		}
		has := false
		if exists(name) {
			ts, err := fileio.Tables(context.Background(), name)
			if err != nil {
				m.fail(fmt.Sprintf("Couldn't open %s: %v", filepath.Base(name), err))
				return nil
			}
			for _, t := range ts {
				has = has || strings.EqualFold(t.Name, table)
			}
		}
		return m.confirmReplace("Table "+table+" exists in "+filepath.Base(name)+".", func(m *Model) tea.Cmd {
			return m.download(name, fileio.SQLite, r, table)
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
		msg:  msg,
		warn: true,
		choices: []choice{
			{key: "enter", label: "Replace", run: do},
			{key: "esc", label: "Cancel", run: func(*Model) tea.Cmd { return nil }},
		},
	})
	return nil
}

// download snapshots the sheet and writes it in the background.
func (m *Model) download(name string, k fileio.Kind, r sheet.Rect, table string) tea.Cmd {
	snap := fileio.Snap(m.sheet, r, filepath.Base(m.displayBase()))
	if k == fileio.XLSX && r == (sheet.Rect{}) {
		snap = fileio.SnapBook(m.sheet) // every sheet, as Sheets' .xlsx download
	}
	return func() tea.Msg {
		res, err := fileio.Export(context.Background(), name, k, snap, fileio.ExportOptions{Table: table})
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
	m.openOverlay(&choiceBar{msg: filepath.Base(m.xfer.source) + " was imported.", choices: choices})
	return nil
}
