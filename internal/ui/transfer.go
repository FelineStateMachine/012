package ui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"012/internal/fileio"
	"012/internal/sheet"
)

// Data comes in through File > Import (a picker of the files around),
// File > Open or the command line, and goes out through File > Download.
// Imports run in the background: the mode indicator says WAIT, the
// status line counts rows with a progress bar, and Esc cancels. An
// imported sheet remembers its source, so Save offers to save it as a
// .012 file or export it back.

// transfer is the model's import and export state.
type transfer struct {
	job     *importJob
	lastID  int
	source  string      // the file the sheet was imported from, if any
	kind    fileio.Kind // its format
	startup string      // a file to import as the program starts
}

// importJob is an import in progress.
type importJob struct {
	id     int
	name   string
	prog   *fileio.Progress
	cancel context.CancelFunc
}

type importedMsg struct {
	id   int
	name string
	opt  fileio.Options
	res  *fileio.Result
	err  error
}

type importTickMsg struct{ id int }

type exportedMsg struct {
	name string
	kind fileio.Kind
	res  *fileio.ExportResult
	err  error
}

// importTick is how often the progress display refreshes.
const importTick = 100 * time.Millisecond

func init() {
	register(&command{id: "file.import", title: "Import",
		desc: "Import a CSV, TSV, Excel, SQLite, Parquet or 1-2-3 file, replacing this sheet",
		run:  (*Model).openImport})
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

// Import sets a file to import when the program starts, e.g. from
// "012 sales.csv".
func (m *Model) Import(name string) {
	m.xfer.startup = name
}

// startupCmd starts the import set with Import, if any.
func (m *Model) startupCmd() tea.Cmd {
	name := m.xfer.startup
	if name == "" {
		return nil
	}
	m.xfer.startup = ""
	return m.startImport(name, fileio.Options{})
}

// importable lists the files in the current directory that 012 can
// import, by name.
func importable() []string {
	entries, _ := os.ReadDir(".")
	var out []string
	for _, e := range entries {
		if _, ok := fileio.KindOf(e.Name()); ok && e.Type().IsRegular() && !strings.HasPrefix(e.Name(), ".") {
			out = append(out, e.Name())
		}
	}
	slices.SortFunc(out, func(a, b string) int { return strings.Compare(strings.ToLower(a), strings.ToLower(b)) })
	return out
}

// openImport opens the import picker: the importable files here, with
// their type and size. A typed path that isn't listed can be imported
// too.
func (m *Model) openImport() tea.Cmd {
	var items []pickItem
	for _, name := range importable() {
		k, _ := fileio.KindOf(name)
		detail, desc := k.Label(), "Import "+name+", "+strings.ToLower(k.Label()[:1])+k.Label()[1:]
		if st, err := os.Stat(name); err == nil {
			detail += "  " + fileSize(st.Size())
			desc += ", " + fileSize(st.Size()) + ", changed " + st.ModTime().Format("Jan 2 15:04")
		}
		items = append(items, pickItem{title: name, name: len(name), detail: detail, desc: desc,
			pick: func(m *Model) tea.Cmd {
				m.closeOverlay()
				return m.confirmImport(name, fileio.Options{})
			}})
	}
	p := newPicker(m, "Import", "Type to filter, or a path", 72, items)
	p.action = "import"
	p.enter = func(m *Model, query string) (tea.Cmd, bool) {
		// A path, or a name that matched nothing: import it by name.
		if query == "" || !strings.ContainsRune(query, filepath.Separator) && len(p.shown) > 0 {
			return nil, false
		}
		m.closeOverlay()
		if _, ok := fileio.KindOf(query); !ok {
			m.fail(fmt.Sprintf("Can't import %s: 012 imports %s", query, importExts()))
			return nil, true
		}
		return m.confirmImport(query, fileio.Options{}), true
	}
	m.openOverlay(p)
	return nil
}

// importExts lists the extensions 012 imports, for error messages.
func importExts() string {
	var exts []string
	for _, k := range fileio.Kinds() {
		exts = append(exts, k.Ext())
	}
	return strings.Join(exts[:len(exts)-1], ", ") + " and " + exts[len(exts)-1] + " files"
}

// fileSize writes a size the way ls -h does.
func fileSize(n int64) string {
	switch {
	case n < 1000:
		return fmt.Sprintf("%d B", n)
	case n < 1000*1000:
		return fmt.Sprintf("%.0f KB", float64(n)/1000)
	case n < 1000*1000*1000:
		return fmt.Sprintf("%.1f MB", float64(n)/1e6)
	}
	return fmt.Sprintf("%.1f GB", float64(n)/1e9)
}

// confirmImport imports name, first asking when that would replace
// unsaved changes.
func (m *Model) confirmImport(name string, opt fileio.Options) tea.Cmd {
	if !m.changed {
		return m.startImport(name, opt)
	}
	m.openOverlay(&choiceBar{
		msg:  "Importing replaces this sheet, which has unsaved changes.",
		warn: true,
		choices: []choice{
			{key: "enter", label: "Import", run: func(m *Model) tea.Cmd { return m.startImport(name, opt) }},
			{key: "esc", label: "Cancel", run: func(*Model) tea.Cmd { return nil }},
		},
	})
	return nil
}

// startImport reads name in the background.
func (m *Model) startImport(name string, opt fileio.Options) tea.Cmd {
	if m.xfer.job != nil {
		m.xfer.job.cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.xfer.lastID++
	job := &importJob{id: m.xfer.lastID, name: name, prog: fileio.NewProgress(), cancel: cancel}
	opt.Progress = job.prog
	m.xfer.job = job
	m.note = ""
	return tea.Batch(
		func() tea.Msg {
			res, err := fileio.Import(ctx, name, opt)
			return importedMsg{id: job.id, name: name, opt: opt, res: res, err: err}
		},
		tickImport(job.id),
	)
}

func tickImport(id int) tea.Cmd {
	return tea.Tick(importTick, func(time.Time) tea.Msg { return importTickMsg{id} })
}

// importing handles input while an import runs: Esc cancels it, and
// everything else waits.
func (m *Model) importing(msg tea.Msg) (tea.Cmd, bool) {
	if m.xfer.job == nil {
		return nil, false
	}
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		if msg.String() == "esc" {
			m.xfer.job.cancel()
		}
		return nil, true
	case tea.MouseClickMsg:
		if msg.Mouse().Y == contextLine {
			m.xfer.job.cancel() // the Esc chip
		}
		return nil, true
	case tea.MouseMsg, tea.PasteMsg:
		return nil, true
	}
	return nil, false
}

// handleTransfer handles import and export messages.
func (m *Model) handleTransfer(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case importTickMsg:
		if m.xfer.job != nil && m.xfer.job.id == msg.id {
			return tickImport(msg.id)
		}
	case importedMsg:
		if m.xfer.job == nil || m.xfer.job.id != msg.id {
			return nil
		}
		m.xfer.job.cancel()
		m.xfer.job = nil
		return m.handleImported(msg)
	case exportedMsg:
		if msg.err != nil {
			m.fail(fmt.Sprintf("Couldn't download %s: %v", msg.name, msg.err))
			return nil
		}
		m.note = "Downloaded " + filepath.Base(msg.name) + " (" + countRows(msg.res.Rows) + ")"
		if len(msg.res.Notes) > 0 {
			m.note += "; " + strings.Join(msg.res.Notes, "; ")
		}
	}
	return nil
}

func countRows(n int) string {
	if n == 1 {
		return "1 row"
	}
	return thousands(n) + " rows"
}

func thousands(n int) string {
	s := fmt.Sprint(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

func (m *Model) handleImported(msg importedMsg) tea.Cmd {
	var need *fileio.ErrNeedTable
	switch {
	case errors.Is(msg.err, context.Canceled):
		m.note = "Import of " + filepath.Base(msg.name) + " cancelled"
		return nil
	case errors.As(msg.err, &need):
		m.openTablePicker(msg.name, need.Tables)
		return nil
	case msg.err != nil:
		m.fail(fmt.Sprintf("Couldn't import %s: %v", filepath.Base(msg.name), msg.err))
		return nil
	}
	jev := m.jev
	m.reset(msg.res.Sheet, "")
	m.jev = jev
	m.xfer.source, m.xfer.kind = msg.name, msg.res.Kind
	what := filepath.Base(msg.name)
	switch {
	case msg.opt.Table != "":
		what = msg.opt.Table + " from " + what
	case msg.opt.Query != "":
		what = "the query from " + what
	}
	m.note = "Imported " + what + " (" + countRows(msg.res.Rows) + ")"
	if len(msg.res.Notes) > 0 {
		m.note += "; " + strings.Join(msg.res.Notes, "; ")
	}
	return nil
}

// openTablePicker asks which table of a SQLite database to import, or
// for a query.
func (m *Model) openTablePicker(name string, tables []fileio.TableInfo) {
	var items []pickItem
	for _, t := range tables {
		detail := countRows(t.Rows) + ", " + plural(len(t.Cols), "1 column", fmt.Sprintf("%d columns", len(t.Cols)))
		if t.View {
			detail = "view, " + detail
		}
		desc := "Import " + t.Name + ": " + strings.Join(t.Cols, ", ")
		items = append(items, pickItem{title: t.Name, name: len(t.Name), detail: detail, desc: desc,
			pick: func(m *Model) tea.Cmd {
				m.closeOverlay()
				return m.startImport(name, fileio.Options{Table: t.Name})
			}})
	}
	first := "table"
	if len(tables) > 0 {
		first = tables[0].Name
	}
	items = append(items, pickItem{title: "Run a query", name: len("Run a query"), detail: "SELECT …",
		desc: "Type a SQL query and import its results",
		pick: func(m *Model) tea.Cmd {
			m.closeOverlay()
			m.openText("SQL query:", "SELECT * FROM "+quoteSQL(first), func(m *Model, text string) tea.Cmd {
				if text == "" {
					return nil
				}
				return m.startImport(name, fileio.Options{Query: text})
			})
			m.prompt.indicator = "SQL"
			return nil
		}})
	p := newPicker(m, "Import from "+filepath.Base(name), "Type to filter tables", 72, items)
	p.action = "import"
	m.openOverlay(p)
}

// quoteSQL quotes a table name when it needs it.
func quoteSQL(s string) string {
	if fileio.TableName(s) == s {
		return s
	}
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

// importLine is the context line during an import.
func (m *Model) importLine() string {
	return "Importing " + filepath.Base(m.xfer.job.name) + "…   " + m.keyHints("Esc", "cancel")
}

// importStatus is the status line during an import: the file, the rows
// read, and a bar when the total is known.
func (m *Model) importStatus() string {
	job := m.xfer.job
	rows, frac := job.prog.Get()
	left := m.th.key.Render("Importing " + filepath.Base(job.name))
	right := countRows(rows)
	if frac >= 0 {
		pct := fmt.Sprintf(" %3d%%", int(frac*100))
		w := clamp(m.width-ansi.StringWidth(left)-ansi.StringWidth(right)-len(pct)-6, 0, 30)
		if w >= 8 {
			done := int(frac * float64(w))
			right += "  " + m.th.progress.Render(strings.Repeat("━", done)) +
				m.th.progressTodo.Render(strings.Repeat("─", w-done)) + pct
		} else {
			right += pct
		}
	}
	return m.spread(left, right)
}

// progressBar is the terminal's own progress indicator (OSC 9;4), shown
// in the tab or title bar where supported.
func (m *Model) progressBar() *tea.ProgressBar {
	if m.xfer.job == nil {
		return nil
	}
	if _, frac := m.xfer.job.prog.Get(); frac >= 0 {
		return tea.NewProgressBar(tea.ProgressBarDefault, int(frac*100))
	}
	return tea.NewProgressBar(tea.ProgressBarIndeterminate, 0)
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
