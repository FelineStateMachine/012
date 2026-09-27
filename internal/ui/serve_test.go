package ui

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/confine"
	"github.com/FelineStateMachine/012/internal/fileio"
	"github.com/FelineStateMachine/012/internal/sheet"
)

// writeSheet saves a one-cell sheet at path.
func writeSheet(t *testing.T, path, a1 string) {
	t.Helper()
	s := sheet.New()
	s.Set(addr("A1"), a1)
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := s.Write(f); err != nil {
		t.Fatal(err)
	}
}

// A served session reads and writes only inside its directory, whatever
// the server's working directory is.
func TestServedFilesStayInside(t *testing.T) {
	top := t.TempDir()
	served, elsewhere := filepath.Join(top, "served"), filepath.Join(top, "elsewhere")
	os.Mkdir(served, 0o755)
	os.Mkdir(elsewhere, 0o755)
	writeSheet(t, filepath.Join(served, "inside.012"), "in")
	writeSheet(t, filepath.Join(elsewhere, "outside.012"), "out")
	os.WriteFile(filepath.Join(served, "data.csv"), []byte("a,b\n1,2\n"), 0o644)
	os.WriteFile(filepath.Join(elsewhere, "other.csv"), []byte("x\n"), 0o644)
	os.Symlink("../elsewhere/outside.012", filepath.Join(served, "link.012"))
	t.Chdir(elsewhere)

	root, err := confine.New(served)
	if err != nil {
		t.Fatal(err)
	}
	m := newModel()
	m.Serve(root, []string{"TERM=xterm-256color"})

	// File > Open lists the served directory's sheets.
	press(t, m, "<ctrl+o>")
	if m.prompt == nil || !slices.Equal(m.prompt.files, []string{"inside.012", "link.012"}) {
		t.Fatalf("Open lists %v", m.prompt.files)
	}
	press(t, m, "inside", "<enter>")
	if m.mode != modeReady || input(m, "A1") != "in" || m.filename != "inside.012" {
		t.Fatalf("open inside: mode %v A1 %q file %q err %q", m.mode, input(m, "A1"), m.filename, m.errMsg)
	}

	for _, name := range []string{"../elsewhere/outside", "link", filepath.Join(elsewhere, "outside")} {
		press(t, m, "<ctrl+o>", name, "<enter>")
		if m.mode != modeError || !strings.Contains(m.errMsg, "outside the served directory") {
			t.Errorf("open %s: mode %v, %q", name, m.mode, m.errMsg)
		}
		if !filepath.IsAbs(name) && strings.Contains(m.errMsg, top) {
			t.Errorf("the error names the server's directories: %q", m.errMsg)
		}
		press(t, m, "<esc>")
	}

	// Save as writes into the served directory, not the working one.
	press(t, m, "<ctrl+s>")
	if _, err := os.Stat(filepath.Join(served, "inside.012")); err != nil {
		t.Fatal(err)
	}
	run(m, m.openSave())
	press(t, m, "copy", "<enter>")
	if _, err := os.Stat(filepath.Join(served, "copy.012")); err != nil {
		t.Errorf("Save as copy: %v (%q)", err, m.errMsg)
	}
	if _, err := os.Stat(filepath.Join(elsewhere, "copy.012")); err == nil {
		t.Error("Save as wrote to the working directory")
	}
	run(m, m.openSave())
	press(t, m, "../escaped", "<enter>")
	if m.mode != modeError {
		t.Errorf("Save as ../escaped: mode %v", m.mode)
	}
	press(t, m, "<esc>")
	if _, err := os.Stat(filepath.Join(top, "escaped.012")); err == nil {
		t.Error("Save as escaped the served directory")
	}

	// The import picker lists the served directory's files.
	names, _ := importable(m.root)
	if !slices.Equal(names, []string{"data.csv"}) {
		t.Errorf("importable = %v", names)
	}
	run(m, m.startImport("../elsewhere/other.csv", fileio.Options{}, placeBook))
	if m.mode != modeError || m.xfer.job != nil {
		t.Errorf("import outside: mode %v job %v", m.mode, m.xfer.job)
	}
}

// Saving over a file someone else wrote since it was opened asks first.
func TestSaveAsksWhenFileChanged(t *testing.T) {
	t.Chdir(t.TempDir())
	writeSheet(t, "book.012", "first")
	s, err := readSheet("book.012")
	if err != nil {
		t.Fatal(err)
	}
	m := New(s, "book.012")
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 20})

	// Nobody else wrote it: saving just saves.
	press(t, m, "mine", "<enter>", "<ctrl+s>")
	if m.overlay != nil || m.changed {
		t.Fatalf("plain save: overlay %T changed %v", m.overlay, m.changed)
	}

	// Another session saves the file.
	writeSheet(t, "book.012", "theirs, longer")
	later := time.Now().Add(time.Minute)
	os.Chtimes("book.012", later, later)

	press(t, m, "<up>", "again", "<enter>", "<ctrl+s>")
	if _, ok := m.overlay.(*choiceBar); !ok || !strings.Contains(line(m, contextLine), "book.012 changed on disk since it was opened") {
		t.Fatalf("context line %q, overlay %T", line(m, contextLine), m.overlay)
	}
	press(t, m, "<esc>")
	if !m.changed {
		t.Error("cancelled save cleared the modified flag")
	}
	if got, _ := readSheet("book.012"); a1Of(got) != "theirs, longer" {
		t.Error("cancelled save wrote the file")
	}

	// Asked again, Enter overwrites; saving after that doesn't ask.
	press(t, m, "<ctrl+s>", "<enter>")
	if m.overlay != nil || m.changed {
		t.Fatalf("overwrite: overlay %T changed %v", m.overlay, m.changed)
	}
	if got, _ := readSheet("book.012"); a1Of(got) != "again" {
		t.Errorf("A1 on disk %q", a1Of(got))
	}
	press(t, m, "<up>", "more", "<enter>", "<ctrl+s>")
	if m.overlay != nil || m.changed {
		t.Errorf("save after overwrite: overlay %T changed %v", m.overlay, m.changed)
	}

	// A file deleted meanwhile is written again without asking.
	os.Remove("book.012")
	press(t, m, "<up>", "back", "<enter>", "<ctrl+s>")
	if got, _ := readSheet("book.012"); m.overlay != nil || a1Of(got) != "back" {
		t.Errorf("save after delete: overlay %T, A1 on disk %q", m.overlay, a1Of(got))
	}

	// Quitting with Save and quit waits for the answer, then quits.
	writeSheet(t, "book.012", "theirs again, longer")
	press(t, m, "<up>", "last", "<enter>")
	quit := press(t, m, "<ctrl+q>", "<enter>")
	if quit != nil || !strings.Contains(line(m, contextLine), "changed on disk") {
		t.Fatalf("Save and quit didn't ask: %v, %q", quit, line(m, contextLine))
	}
	if _, ok := press(t, m, "<enter>").(tea.QuitMsg); !ok {
		t.Error("Overwrite didn't finish Save and quit")
	}
}

func a1Of(s *sheet.Sheet) string {
	if s == nil || s.Cell(addr("A1")) == nil {
		return ""
	}
	return s.Cell(addr("A1")).Input
}

func readSheet(name string) (*sheet.Sheet, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return sheet.Read(f)
}
