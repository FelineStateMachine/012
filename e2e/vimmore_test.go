package e2e

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// . repeats a change, "a keeps a copy of its own, marks and two quotes
// jump back, cc clears a row to retype, Up on the : line recalls a line,
// and :w! writes over a file changed on disk.
func TestVimRepeatRegistersMarks(t *testing.T) {
	dir := t.TempDir()
	s := start(t, dir)
	vimOn(s)
	s.keys("i", "a", "<tab>", "i", "1", "<enter>")
	s.keys("i", "b", "<tab>", "i", "2", "<enter>")
	s.keys("i", "c", "<tab>", "i", "3", "<enter>")
	s.waitForBar("A4", "")

	// A mark on B1, a jump away and back, then back to the row jumped from.
	s.keys("gg", "l", "mq", "G", "`q")
	s.waitForBar("B1", "1")
	s.keys("''")
	s.waitForBar("A3", "c")

	// x and . clear two cells; "ayy keeps row 1 while dd fills the
	// clipboard, and "ap pastes row 1 back as a new row.
	s.keys("gg", "l", "x", "j", ".")
	s.waitForBar("B2", "")
	s.keys("gg", `"ayy`, "j", "dd", "G", "0", `"ap`)
	s.waitForBar("A3", "a")

	// cc clears the row and starts typing; . does it again below.
	s.keys("gg", "cc", "top", "<enter>")
	s.waitForBar("A2", "c")
	s.keys(".")
	s.waitForBar("A3", "a")
	s.keys("k")
	s.waitForBar("A2", "top")

	// The : line remembers what ran.
	s.keys(":B3", "<enter>", "gg", ":", "<up>")
	s.waitFor(":B3")
	s.keys("<enter>")
	s.waitForBar("B3", "")

	s.keys(":w vim", "<enter>")
	s.eventually("saved", func() bool { return s.title() == "012 - vim.012" })
	name := filepath.Join(dir, "vim.012")
	later := time.Now().Add(time.Minute)
	if err := os.WriteFile(name, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(name, later, later); err != nil {
		t.Fatal(err)
	}
	s.keys("gg", "0", "x", ":w", "<enter>")
	s.waitFor("changed on disk")
	s.keys("<esc>", ":w!", "<enter>")
	s.eventually("written", func() bool {
		data, _ := os.ReadFile(name)
		return string(data) != "{}" && s.title() == "012 - vim.012"
	})
}
