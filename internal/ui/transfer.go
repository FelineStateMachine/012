package ui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/confine"
	"github.com/FelineStateMachine/012/internal/fileio"
	"github.com/FelineStateMachine/012/internal/ui/picker"
	"github.com/FelineStateMachine/012/internal/ui/transfer"
)

// Data comes in through File > Import (a picker of the files around),
// File > Open or the command line, and goes out through File > Download.
// Imports run in the background: the mode indicator says WAIT, the
// status line counts rows with a progress bar, and Esc cancels. An
// imported sheet remembers its source, so Save offers to save it as a
// .012 file or export it back. The import running and its progress are
// package transfer's; here is what the model does with them.

func init() {
	register(&command{id: "file.import", macro: macroNever, title: "Import",
		desc: "Import a " + importNouns() + " file as new sheets, or in place of this sheet or spreadsheet",
		run:  (*Model).openImport})
}

// importNouns lists the formats 012 imports: "CSV, TSV, ... or 1-2-3".
func importNouns() string {
	var nouns []string
	for _, k := range fileio.Kinds() {
		nouns = append(nouns, k.Noun())
	}
	return strings.Join(nouns[:len(nouns)-1], ", ") + " or " + nouns[len(nouns)-1]
}

// Import sets a file to import when the program starts, e.g. from
// "012 sales.csv".
func (m *Model) Import(name string) {
	m.xfer.Startup = name
}

// startupCmd starts the import set with Import, if any.
func (m *Model) startupCmd() tea.Cmd {
	name := m.xfer.Startup
	if name == "" {
		return nil
	}
	m.xfer.Startup = ""
	return m.startImport(name, fileio.Options{}, transfer.Book)
}

// importable lists the files in the directory names are relative to
// (the current one, or the served one) that 012 can import, by name,
// with their sizes.
func importable(root confine.Root) ([]string, map[string]int64) {
	dir, err := root.Resolve(".")
	if err != nil {
		return nil, nil
	}
	entries, _ := os.ReadDir(dir)
	var out []string
	sizes := map[string]int64{}
	for _, e := range entries {
		if _, ok := fileio.KindOf(e.Name()); ok && e.Type().IsRegular() && !strings.HasPrefix(e.Name(), ".") {
			out = append(out, e.Name())
			if info, err := e.Info(); err == nil {
				sizes[e.Name()] = info.Size()
			}
		}
	}
	slices.SortFunc(out, func(a, b string) int { return strings.Compare(strings.ToLower(a), strings.ToLower(b)) })
	return out, sizes
}

// openImport opens the import Picker: the importable files here, with
// their type and size. A typed path that isn't listed can be imported
// too.
func (m *Model) openImport() tea.Cmd {
	var items []picker.Item
	names, sizes := importable(m.root)
	lw := 0 // types line up, and sizes after them
	for _, name := range names {
		k, _ := fileio.KindOf(name)
		lw = max(lw, len(k.Label()))
	}
	for _, name := range names {
		k, _ := fileio.KindOf(name)
		detail, desc := k.Label(), "Import "+name+", "+strings.ToLower(k.Label()[:1])+k.Label()[1:]
		if size, ok := sizes[name]; ok {
			detail = fmt.Sprintf("%-*s %6s", lw, k.Label(), fileSize(size))
			desc += ", " + fileSize(size)
		}
		items = append(items, picker.Item{Title: name, Name: len(name), Detail: detail, Desc: desc,
			Pick: func() tea.Cmd {
				m.closeOverlay()
				return m.askImportPlace(name)
			}})
	}
	p := m.newPicker("Import", "Type to filter, or a path", 72, items)
	p.Action = "import"
	p.Enter = func(query string) (tea.Cmd, bool) {
		// A path, or a name that matched nothing: import it by name.
		if query == "" || !strings.ContainsRune(query, filepath.Separator) && len(p.Shown()) > 0 {
			return nil, false
		}
		m.closeOverlay()
		if _, ok := fileio.KindOf(query); !ok {
			m.fail(fmt.Sprintf("Can't import %s: 012 imports %s", query, importExts()))
			return nil, true
		}
		return m.askImportPlace(query), true
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
		return m.startImport(name, opt, transfer.Book)
	}
	m.ask(question{
		msg:  "Importing replaces this spreadsheet, which has unsaved changes.",
		warn: true,
		choices: []choice{
			{key: "enter", label: "Import", run: func(m *Model) tea.Cmd { return m.startImport(name, opt, transfer.Book) }},
			{key: "esc", label: "Cancel", run: func(*Model) tea.Cmd { return nil }},
		},
	})
	return nil
}

// startImport reads name in the background.
func (m *Model) startImport(name string, opt fileio.Options, place transfer.Place) tea.Cmd {
	m.note = ""
	path, ok := m.path("import", name)
	if !ok {
		return nil
	}
	if opt.Locale == nil {
		opt.Locale = m.locale()
	}
	return m.xfer.Start(name, path, opt, place, m.spans.Parent())
}

// importing handles input while an import runs: Esc cancels it, and
// everything else waits.
func (m *Model) importing(msg tea.Msg) (tea.Cmd, bool) {
	if !m.xfer.Busy() {
		return nil, false
	}
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		if msg.String() == "esc" {
			m.cancelImport()
		}
		return nil, true
	case tea.MouseClickMsg:
		if msg.Mouse().Y == contextLine {
			m.cancelImport() // the Esc chip
		}
		return nil, true
	case tea.MouseMsg, tea.PasteMsg:
		return nil, true
	}
	return nil, false
}

// cancelImport stops the import at once; its result, if it still
// arrives, is ignored.
func (m *Model) cancelImport() {
	m.note = "Import of " + filepath.Base(m.xfer.Cancel()) + " cancelled"
}

// handleTransfer handles import and export messages.
func (m *Model) handleTransfer(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case transfer.TickMsg, transfer.ImportedMsg:
		res, done, cmd := m.xfer.Update(msg)
		if done {
			return m.handleImported(res)
		}
		return cmd
	case exportedMsg:
		if msg.err != nil {
			m.fail(fmt.Sprintf("Couldn't download %s: %v", msg.name, msg.err))
			return nil
		}
		m.note = "Downloaded " + filepath.Base(msg.name) + " (" + transfer.Rows(msg.res.Rows) + ")"
		if len(msg.res.Notes) > 0 {
			m.note += "; " + strings.Join(msg.res.Notes, "; ")
		}
	}
	return nil
}

func (m *Model) handleImported(msg transfer.ImportedMsg) tea.Cmd {
	var need *fileio.ErrNeedTable
	switch {
	case errors.Is(msg.Err, context.Canceled):
		m.note = "Import of " + filepath.Base(msg.Name) + " cancelled"
		return nil
	case errors.As(msg.Err, &need):
		m.openTablePicker(msg.Name, need.Tables, msg.Place)
		return nil
	case msg.Err != nil:
		m.fail(fmt.Sprintf("Couldn't import %s: %v", filepath.Base(msg.Name), msg.Err))
		return nil
	}
	if msg.Stream {
		m.streamed(msg)
		return m.term.notify("Read standard input")
	}
	what := filepath.Base(msg.Name)
	switch {
	case msg.Opt.Table != "":
		what = msg.Opt.Table + " from " + what
	case msg.Opt.Query != "":
		what = "the query from " + what
	}
	if m.placeImport(msg, what) {
		return m.term.notify("Imported " + what)
	}
	m.reset(msg.Res.Sheet, "")
	m.xfer.Source, m.xfer.Kind = msg.Name, msg.Res.Kind
	m.note = "Imported " + what + " (" + transfer.Rows(msg.Res.Rows) + ")"
	if len(msg.Res.Notes) > 0 {
		m.note += "; " + strings.Join(msg.Res.Notes, "; ")
	}
	// A long import may finish while the terminal is in the background.
	return m.term.notify("Imported " + what)
}

// openTablePicker asks which table of a SQLite database to import, or
// for a query.
func (m *Model) openTablePicker(name string, tables []fileio.TableInfo, place transfer.Place) {
	var items []picker.Item
	for _, t := range tables {
		detail := transfer.Rows(t.Rows) + ", " + plural(len(t.Cols), "1 column", fmt.Sprintf("%d columns", len(t.Cols)))
		if t.View {
			detail = "view, " + detail
		}
		desc := "Import " + t.Name + ": " + strings.Join(t.Cols, ", ")
		items = append(items, picker.Item{Title: t.Name, Name: len(t.Name), Detail: detail, Desc: desc,
			Pick: func() tea.Cmd {
				m.closeOverlay()
				return m.startImport(name, fileio.Options{Table: t.Name}, place)
			}})
	}
	first := "table"
	if len(tables) > 0 {
		first = tables[0].Name
	}
	items = append(items, picker.Item{Title: "Run a query", Name: len("Run a query"), Detail: "SELECT …",
		Desc: "Type a SQL query and import its results",
		Pick: func() tea.Cmd {
			m.closeOverlay()
			m.openText("SQL query:", "SELECT * FROM "+quoteSQL(first), func(m *Model, text string) tea.Cmd {
				if text == "" {
					return nil
				}
				return m.startImport(name, fileio.Options{Query: text}, place)
			})
			m.prompt.indicator = "SQL"
			return nil
		}})
	p := m.newPicker("Import from "+filepath.Base(name), "Type to filter tables", 72, items)
	p.Action = "import"
	m.openOverlay(p)
}

// quoteSQL quotes a table name when it needs it.
func quoteSQL(s string) string {
	if fileio.TableName(s) == s {
		return s
	}
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}
