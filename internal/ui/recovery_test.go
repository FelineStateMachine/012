package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/FelineStateMachine/012/internal/confine"
)

// servedModel is a model served from a fresh directory, which it
// returns.
func servedModel(t *testing.T) (*Model, string) {
	t.Helper()
	dir := t.TempDir()
	root, err := confine.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	m := newModel()
	m.Serve(root, []string{"TERM=xterm-256color"})
	return m, root.Dir()
}

func TestRecoveryKey(t *testing.T) {
	for name, want := range map[string]string{
		"":               "(untitled)",
		"book.012":       "book",
		"book":           "book",
		"sub/a b.012":    "sub%2Fa%20b",
		"./x/../y.012":   "y",
		"(untitled).012": "%28untitled%29",
	} {
		if got := recoveryKey(name); got != want {
			t.Errorf("recoveryKey(%q) = %q, want %q", name, got, want)
		}
	}
}

// Only changed books that hold something are kept, only when served (or
// given a recovery directory, recovery tests in crash_test.go),
// and at most recoveryKeep per name, newest kept.
func TestRecoverKeepsFew(t *testing.T) {
	local := newModel()
	press(t, local, "1", "<enter>")
	if _, err := local.Recover(time.Now()); err == nil {
		t.Error("a local model kept a recovery file")
	}

	m, dir := servedModel(t)
	if m.Unsaved() {
		t.Error("a new sheet is unsaved")
	}
	press(t, m, "x", "<enter>", "<up>", "<delete>")
	if !m.changed || m.Unsaved() {
		t.Errorf("an emptied sheet: changed %v, unsaved %v", m.changed, m.Unsaved())
	}
	press(t, m, "42", "<enter>")
	m.filename = "book.012"
	at := time.Date(2026, 9, 27, 14, 2, 3, 0, time.Local)
	var names []string
	for i := range 5 {
		name, err := m.Recover(at.Add(time.Duration(i/2) * time.Second))
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, name)
	}
	if names[0] != filepath.Join(RecoveryDir, "book-20260927-140203.012") || names[1] != filepath.Join(RecoveryDir, "book-20260927-140203-2.012") {
		t.Errorf("names %v", names)
	}
	// Another name's files, and one that only starts like this one's,
	// are left alone.
	os.WriteFile(filepath.Join(dir, RecoveryDir, "book-20260101-000000-20260101-000000.012"), nil, 0o600)
	os.WriteFile(filepath.Join(dir, RecoveryDir, "other-20200101-000000.012"), nil, 0o600)
	if _, err := m.Recover(at.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	files := recoveries(filepath.Join(dir, RecoveryDir), "book")
	var got []string
	for _, f := range files {
		got = append(got, filepath.Base(f.path))
	}
	want := []string{"book-20260927-140204-2.012", "book-20260927-140205.012", "book-20260927-150203.012"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("kept %v, want %v", got, want)
	}
	for _, other := range []string{"book-20260101-000000-20260101-000000.012", "other-20200101-000000.012"} {
		if _, err := os.Stat(filepath.Join(dir, RecoveryDir, other)); err != nil {
			t.Errorf("%s removed", other)
		}
	}
}

// A served session starting on a new sheet offers an untitled book's
// kept changes: Esc keeps them for later, D deletes them.
func TestRecoveryOfferUntitled(t *testing.T) {
	m, dir := servedModel(t)
	press(t, m, "kept", "<enter>")
	file, err := m.Recover(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"<esc>", "d"} {
		m, _ := servedModel(t)
		m.root, _ = confine.New(dir)
		m.OpenOnStart("")
		run(m, m.Init())
		if !strings.Contains(line(m, contextLine), "Unsaved changes to an untitled sheet were kept.") {
			t.Fatalf("no offer: %q", line(m, contextLine))
		}
		press(t, m, key)
		_, err := os.Stat(filepath.Join(dir, file))
		if key == "<esc>" && err != nil || key == "d" && err == nil {
			t.Errorf("%s: recovery file there: %v", key, err == nil)
		}
		if input(m, "A1") != "" {
			t.Errorf("%s restored it", key)
		}
	}
}

// Opening a file with kept changes offers them; restoring makes them
// unsaved changes to that file, and saving them removes the recovery
// file. A local model without a recovery directory never offers.
func TestRecoveryRestore(t *testing.T) {
	m, dir := servedModel(t)
	writeSheet(t, filepath.Join(dir, "book.012"), "on disk")
	m.OpenOnStart("book")
	run(m, m.Init())
	press(t, m, "mine", "<enter>")
	file, err := m.Recover(time.Now())
	if err != nil {
		t.Fatal(err)
	}

	m, _ = servedModel(t)
	m.root, _ = confine.New(dir)
	m.OpenOnStart("book.012")
	run(m, m.Init())
	if input(m, "A1") != "on disk" || !strings.Contains(line(m, contextLine), "Unsaved changes to book.012 were kept.") {
		t.Fatalf("A1 %q, context %q", input(m, "A1"), line(m, contextLine))
	}
	press(t, m, "<enter>")
	if input(m, "A1") != "mine" || !m.changed || m.filename != "book.012" || m.recovered == "" {
		t.Fatalf("restore: A1 %q, changed %v, file %q", input(m, "A1"), m.changed, m.filename)
	}
	if !strings.Contains(line(m, contextLine), "Restored the kept changes") {
		t.Errorf("context %q", line(m, contextLine))
	}
	// Undoing to the restored state still differs from the file.
	press(t, m, "x", "<enter>", "<ctrl+z>")
	if !m.changed {
		t.Error("undo marked the restored book saved")
	}
	press(t, m, "<ctrl+s>")
	if m.overlay != nil || m.changed {
		t.Fatalf("save: overlay %T, changed %v, %q", m.overlay, m.changed, line(m, contextLine))
	}
	if got, _ := readSheet(filepath.Join(dir, "book.012")); a1Of(got) != "mine" {
		t.Errorf("saved A1 %q", a1Of(got))
	}
	if _, err := os.Stat(filepath.Join(dir, file)); err == nil {
		t.Error("saving kept the recovery file")
	}

	// Locally, nothing is offered.
	os.MkdirAll(filepath.Join(dir, RecoveryDir), 0o700)
	os.WriteFile(filepath.Join(dir, RecoveryDir, "book-20260101-000000.012"), nil, 0o600)
	t.Chdir(dir)
	local := newModel()
	press(t, local, "<ctrl+o>", "book", "<enter>")
	if local.overlay != nil || input(local, "A1") != "mine" {
		t.Errorf("local open: overlay %T, A1 %q", local.overlay, input(local, "A1"))
	}
}
