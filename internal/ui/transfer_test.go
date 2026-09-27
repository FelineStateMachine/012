package ui

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/fileio"
	"github.com/FelineStateMachine/012/internal/sheet"
)

func writeFile(t *testing.T, name, text string) {
	t.Helper()
	os.MkdirAll(filepath.Dir(name), 0o755)
	if err := os.WriteFile(name, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func pickerTitles(t *testing.T, m *Model) []string {
	var out []string
	for _, pm := range openPicker(t, m).shown {
		out = append(out, pm.item.title+" | "+pm.item.detail)
	}
	return out
}

func TestImportPicker(t *testing.T) {
	t.Chdir(t.TempDir())
	writeFile(t, "sales.csv", "Region,Total\nNorth,\"$1,200\"\n")
	writeFile(t, "notes.txt", "not importable")
	writeFile(t, "Old.WK1", "x")
	writeFile(t, "sub/deep.tsv", "a\tb\n")
	m := newModel()
	m.runCommand("file.import")
	got := strings.Join(pickerTitles(t, m), "\n")
	want := "Old.WK1 | Lotus 1-2-3 worksheet     1 B\nsales.csv | Comma-separated values   28 B"
	if got != want {
		t.Errorf("picker lists\n%s\nwant\n%s", got, want)
	}
	if !strings.Contains(screen(m), "Import") || !strings.Contains(line(m, m.height-1), "Import sales.csv") &&
		!strings.Contains(line(m, m.height-1), "Import Old.WK1") {
		t.Errorf("screen:\n%s", screen(m))
	}
	press(t, m, "sal", "<enter>")
	if m.overlay != nil || m.mode != modeReady {
		t.Fatalf("after import: mode %v overlay %T err %q", m.mode, m.overlay, m.errMsg)
	}
	if v := m.sheet.Value(addr("B2")); v.Num != 1200 {
		t.Errorf("B2 = %+v", v)
	}
	if l := line(m, contextLine); l != "Imported sales.csv (2 rows)" {
		t.Errorf("context line %q", l)
	}
	if !strings.HasPrefix(line(m, m.height-1), "sales.csv") || m.changed {
		t.Errorf("status %q changed %v", line(m, m.height-1), m.changed)
	}

	// A typed path imports a file that isn't listed.
	m.runCommand("file.import")
	press(t, m, "sub/deep.tsv", "<enter>")
	if input(m, "B1") != "b" || m.xfer.source != "sub/deep.tsv" {
		t.Errorf("typed path: B1 %q source %q err %q", input(m, "B1"), m.xfer.source, m.errMsg)
	}
	// Unknown types and broken files say what's wrong.
	m.runCommand("file.import")
	press(t, m, "notes.txt", "<enter>")
	if m.mode != modeError || !strings.Contains(m.errMsg, "Can't import notes.txt: 012 imports .csv, .tsv") {
		t.Errorf("txt: %v %q", m.mode, m.errMsg)
	}
	press(t, m, "<esc>")
	m.runCommand("file.import")
	press(t, m, "old", "<enter>")
	if m.mode != modeError || m.errMsg != "Couldn't import Old.WK1: not a Lotus 1-2-3 worksheet: the file is empty" &&
		!strings.Contains(m.errMsg, "Couldn't import Old.WK1: not a Lotus 1-2-3 worksheet") {
		t.Errorf("wk1: %v %q", m.mode, m.errMsg)
	}
}

func TestImportAsksBeforeReplacingChanges(t *testing.T) {
	t.Chdir(t.TempDir())
	writeFile(t, "a.csv", "1\n")
	m := newModel()
	press(t, m, "keep me", "<enter>")
	m.runCommand("file.import")
	press(t, m, "<enter>")
	if _, ok := m.overlay.(*choiceBar); !ok || !strings.Contains(line(m, contextLine), "Importing replaces this sheet, which has unsaved changes.") {
		t.Fatalf("no warning: %q", line(m, contextLine))
	}
	press(t, m, "<esc>")
	if input(m, "A1") != "keep me" {
		t.Errorf("Esc imported anyway")
	}
	m.runCommand("file.import")
	press(t, m, "<enter>", "<enter>")
	if input(m, "A1") != "1" {
		t.Errorf("Enter didn't import: %q", input(m, "A1"))
	}
}

func TestOpenImportsOtherFormats(t *testing.T) {
	t.Chdir(t.TempDir())
	writeFile(t, "data.tsv", "x\t2\n")
	m := newModel()
	press(t, m, "<ctrl+o>", "data.tsv", "<enter>")
	if input(m, "B1") != "2" || m.filename != "" || m.xfer.source != "data.tsv" {
		t.Errorf("open: B1 %q file %q source %q", input(m, "B1"), m.filename, m.xfer.source)
	}
	// Save offers a .012 file or exporting back.
	press(t, m, "<ctrl+s>")
	if l := line(m, contextLine); !strings.Contains(l, "data.tsv was imported.") || !strings.Contains(l, "Save as data.012") || !strings.Contains(l, "Download as TSV") {
		t.Errorf("save choices %q", l)
	}
	press(t, m, "<enter>")
	if m.mode != modePrompt || string(m.buf) != "data.012" {
		t.Errorf("save as %v %q", m.mode, string(m.buf))
	}
	press(t, m, "<enter>")
	if m.filename != "data.012" {
		t.Errorf("saved as %q", m.filename)
	}
	press(t, m, "<ctrl+s>") // now a native file: saves directly
	if m.overlay != nil || m.mode != modeReady {
		t.Errorf("second save asked again")
	}
}

func TestSQLiteTablePicker(t *testing.T) {
	t.Chdir(t.TempDir())
	db, err := sql.Open("sqlite", "shop.db")
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{
		"CREATE TABLE orders (id INTEGER, total REAL)", "INSERT INTO orders VALUES (1, 9.5), (2, 3)",
		"CREATE TABLE people (name TEXT)", "INSERT INTO people VALUES ('Ada')",
	} {
		if _, err := db.Exec(s); err != nil {
			t.Fatal(err)
		}
	}
	db.Close()
	m := newModel()
	m.runCommand("file.import")
	press(t, m, "<enter>")
	got := strings.Join(pickerTitles(t, m), "\n")
	if got != "orders | 2 rows, 2 columns\npeople | 1 row, 1 column\nRun a query | SELECT …" {
		t.Errorf("tables:\n%s", got)
	}
	if !strings.Contains(screen(m), "Import from shop.db") {
		t.Errorf("title missing:\n%s", screen(m))
	}
	press(t, m, "<enter>")
	if m.sheet.Value(addr("B2")).Num != 9.5 || line(m, contextLine) != "Imported orders from shop.db (3 rows)" {
		t.Errorf("B2 %+v, context %q", m.sheet.Value(addr("B2")), line(m, contextLine))
	}

	m.runCommand("file.import")
	press(t, m, "<enter>", "query", "<enter>")
	if m.mode != modePrompt || string(m.buf) != "SELECT * FROM orders" {
		t.Fatalf("query prompt %v %q", m.mode, string(m.buf))
	}
	press(t, m, "<ctrl+a>")
	m.buf, m.bufPos = []rune("SELECT sum(total) AS s FROM orders"), len("SELECT sum(total) AS s FROM orders")
	press(t, m, "<enter>")
	if m.sheet.Value(addr("A2")).Num != 12.5 {
		t.Errorf("query A2 %+v err %q", m.sheet.Value(addr("A2")), m.errMsg)
	}
}

func TestDownload(t *testing.T) {
	t.Chdir(t.TempDir())
	m := newModel()
	m.runCommand("file.download.csv")
	if m.mode == modePrompt || line(m, contextLine) != "Nothing to download: the sheet is empty" {
		t.Errorf("empty sheet: %q", line(m, contextLine))
	}
	press(t, m, "Item", "<tab>", "Cost", "<enter>", "Rent", "<tab>", "$1,450", "<enter>", "Total", "<tab>", "=B2*2", "<enter>")

	m.runCommand("file.download.csv")
	if l := line(m, contextLine); !strings.HasPrefix(l, "Download as CSV: SHEET1.csv") {
		t.Errorf("prompt %q", l)
	}
	press(t, m, "bills", "<enter>")
	data, err := os.ReadFile("bills.csv")
	if err != nil || string(data) != "Item,Cost\nRent,\"$1,450\"\nTotal,\"$2,900\"\n" {
		t.Errorf("bills.csv %q %v", data, err)
	}
	if l := line(m, contextLine); l != "Downloaded bills.csv (3 rows); 1 formula saved as values" {
		t.Errorf("note %q", l)
	}

	// Downloading over a file asks first.
	m.runCommand("file.download.csv")
	press(t, m, "bills", "<enter>")
	if l := line(m, contextLine); !strings.Contains(l, "bills.csv exists.") || !strings.Contains(l, "Replace") {
		t.Fatalf("no replace question: %q", l)
	}
	press(t, m, "<esc>")

	m.runCommand("file.download.xlsx")
	press(t, m, "<enter>")
	if _, err := os.Stat("SHEET1.xlsx"); err != nil {
		t.Errorf("xlsx: %v %q", err, m.errMsg)
	}

	// SQLite asks for a table, and exports the selection when there is
	// one.
	press(t, m, "<ctrl+home>", "<shift+down>")
	m.runCommand("file.download.sqlite")
	if l := line(m, contextLine); !strings.HasPrefix(l, "Download A1:A2 as SQLite: SHEET1.sqlite") {
		t.Errorf("sqlite prompt %q", l)
	}
	press(t, m, "<enter>")
	if l := line(m, contextLine); !strings.HasPrefix(l, "Table in SHEET1.sqlite: SHEET1") {
		t.Errorf("table prompt %q", l)
	}
	press(t, m, "<enter>")
	if l := line(m, contextLine); l != "Downloaded SHEET1.sqlite (1 row)" {
		t.Errorf("sqlite note %q err %q", l, m.errMsg)
	}
	m.runCommand("file.download.sqlite")
	press(t, m, "<enter>", "<enter>")
	if l := line(m, contextLine); !strings.Contains(l, "Table SHEET1 exists in SHEET1.sqlite.") {
		t.Errorf("no table question: %q", l)
	}
	press(t, m, "<enter>")
	ts, _ := fileio.Tables(context.Background(), "SHEET1.sqlite")
	if len(ts) != 1 || ts[0].Rows != 1 {
		t.Errorf("tables %+v", ts)
	}
}

func TestImportProgress(t *testing.T) {
	m := newModel()
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 16})
	cancelled := false
	prog := fileio.NewProgress()
	m.xfer.job = &importJob{id: 7, name: "data/big.csv", prog: prog, cancel: func() { cancelled = true }}
	prog.Report(1234, 0, 0)
	if ind := m.indicator(); ind != "WAIT" {
		t.Errorf("indicator %q", ind)
	}
	if l := line(m, contextLine); l != "Importing big.csv…    Esc  cancel" {
		t.Errorf("context %q", l)
	}
	if l := line(m, m.height-1); !strings.HasPrefix(l, "Importing big.csv") || !strings.HasSuffix(l, "1,234 rows read") {
		t.Errorf("status %q", l)
	}
	if pb := m.View().ProgressBar; pb == nil || pb.State != tea.ProgressBarIndeterminate {
		t.Errorf("terminal progress %+v", pb)
	}
	prog.Report(4096, 1, 2)
	l := line(m, m.height-1)
	if l != "Importing big.csv           4,096 rows read  ━━━━━━━━━━━━━━━───────────────  50%" {
		t.Errorf("status %q", l)
	}
	if pb := m.View().ProgressBar; pb == nil || pb.Value != 50 {
		t.Errorf("terminal progress %+v", pb)
	}
	// Keys wait; Esc cancels.
	press(t, m, "x", "<down>")
	if m.cur != (sheet.Addr{}) || m.mode != modeReady {
		t.Errorf("keys went through: %v %v", m.cur, m.mode)
	}
	press(t, m, "<esc>")
	if !cancelled {
		t.Error("Esc didn't cancel")
	}
	if m.xfer.job != nil || line(m, contextLine) != "Import of big.csv cancelled" {
		t.Errorf("after cancel: %q", line(m, contextLine))
	}
	// A stale result from an earlier import is ignored.
	send(m, importedMsg{id: 3, name: "old.csv", res: &fileio.Result{Sheet: sheet.New()}})
	if m.xfer.source != "" {
		t.Errorf("stale import applied")
	}
}

func TestImportOnStart(t *testing.T) {
	t.Chdir(t.TempDir())
	writeFile(t, "start.csv", "7\n")
	m := New(sheet.New(), "")
	m.Import("start.csv")
	run(m, m.Init())
	if input(m, "A1") != "7" || m.displayName() != "start.csv" {
		t.Errorf("A1 %q name %q err %q", input(m, "A1"), m.displayName(), m.errMsg)
	}
}

// An import that finishes while the terminal is in the background says so
// with a desktop notification.
func TestImportNotifiesWhenBlurred(t *testing.T) {
	m := newModel()
	send(m, tea.BlurMsg{})
	m.xfer.job = &importJob{id: 5, name: "big.csv", prog: fileio.NewProgress(), cancel: func() {}}
	_, cmd := m.Update(importedMsg{id: 5, name: "big.csv", res: &fileio.Result{Sheet: sheet.New(), Rows: 3}})
	var notified bool
	for _, msg := range flatten(cmd) {
		if raw, ok := msg.(tea.RawMsg); ok && strings.Contains(fmt.Sprint(raw.Msg), "012: Imported big.csv") {
			notified = true
		}
	}
	if !notified {
		t.Error("no notification for a background import")
	}
}
