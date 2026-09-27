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

	"github.com/FelineStateMachine/012/internal/confine"
	"github.com/FelineStateMachine/012/internal/fileio"
	"github.com/FelineStateMachine/012/internal/ui/theme"
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

// importTick is how often the progress display refreshes.
const importTick = 100 * time.Millisecond

func init() {
	register(&command{id: "file.import", title: "Import",
		desc: "Import a " + importNouns() + " file, replacing this sheet",
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

// openImport opens the import picker: the importable files here, with
// their type and size. A typed path that isn't listed can be imported
// too.
func (m *Model) openImport() tea.Cmd {
	var items []pickItem
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
	m.note = ""
	path, ok := m.path("import", name)
	if !ok {
		return nil
	}
	return m.xfer.start(name, path, opt)
}

// start reads name, at path on disk, in the background, replacing any
// import running.
func (x *transfer) start(name, path string, opt fileio.Options) tea.Cmd {
	if x.job != nil {
		x.job.cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	x.lastID++
	job := &importJob{id: x.lastID, name: name, prog: fileio.NewProgress(), cancel: cancel}
	opt.Progress = job.prog
	x.job = job
	return tea.Batch(
		func() tea.Msg {
			res, err := fileio.Import(ctx, path, opt)
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
	m.note = "Import of " + filepath.Base(m.xfer.cancel()) + " cancelled"
}

// cancel stops the import and returns the name of its file.
func (x *transfer) cancel() string {
	job := x.job
	job.cancel()
	x.job = nil
	return job.name
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
	m.reset(msg.res.Sheet, "")
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
	// A long import may finish while the terminal is in the background.
	return m.term.notify("Imported " + what)
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

// line is the context line during an import.
func (x *transfer) line(th *theme.Theme) string {
	return "Importing " + filepath.Base(x.job.name) + "…   " + th.KeyHints("Esc", "cancel")
}

// status is the status line during an import, width wide: the file, the
// rows read, and a bar when the total is known.
func (x *transfer) status(th *theme.Theme, width int) (left, right string) {
	rows, frac := x.job.prog.Get()
	left = th.Key.Render("Importing " + filepath.Base(x.job.name))
	right = countRows(rows) + " read"
	if frac < 0 {
		return left, right
	}
	pct := fmt.Sprintf(" %3d%%", int(frac*100))
	w := clamp(width-ansi.StringWidth(left)-ansi.StringWidth(right)-len(pct)-6, 0, 30)
	if w < 8 {
		return left, right + pct
	}
	done := int(frac * float64(w))
	return left, right + "  " + th.Progress.Render(strings.Repeat("━", done)) +
		th.ProgressTodo.Render(strings.Repeat("─", w-done)) + pct
}

// progressBar is the terminal's own progress indicator (OSC 9;4), shown
// in the tab or title bar where supported.
func (x *transfer) progressBar() *tea.ProgressBar {
	if x.job == nil {
		return nil
	}
	if _, frac := x.job.prog.Get(); frac >= 0 {
		return tea.NewProgressBar(tea.ProgressBarDefault, int(frac*100))
	}
	return tea.NewProgressBar(tea.ProgressBarIndeterminate, 0)
}
