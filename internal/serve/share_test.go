package serve

import (
	"context"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Two sessions opening one file share its workbook: what one types the
// other shows at once, the first to quit leaves it to the other without
// asking, and stopping the server keeps the unsaved work once.
func TestSessionsShareAFile(t *testing.T) {
	key := newKey(t)
	srv, addr, dir := testServer(t, nil, key)
	saveSheet(t, filepath.Join(dir, "book.012"), "saved")
	c, err := dial(addr, key, srv.HostKey())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	var terms []*term
	for range 3 {
		tm, err := openExec(t, c, "book.012")
		if err != nil {
			t.Fatal(err)
		}
		tm.waitFor(t, "saved")
		terms = append(terms, tm)
	}
	a, b, cy := terms[0], terms[1], terms[2]
	io.WriteString(a.stdin, "\x1b[Bfrom a\r")
	b.waitFor(t, "from a", "modified")
	cy.waitFor(t, "from a")

	io.WriteString(b.stdin, "\x11") // Ctrl+Q: a and cy still have it
	b.waitClosed(t)
	a.waitFor(t, "from a")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	a.waitClosed(t)
	cy.waitClosed(t)
	files, _ := filepath.Glob(filepath.Join(dir, ".012-recovery", "*"))
	if len(files) != 1 {
		t.Fatalf("recovery files %v; want one for the shared workbook", files)
	}
	kept := 0
	for _, tm := range []*term{a, cy} {
		if strings.Contains(tm.output(), "unsaved changes kept in") {
			kept++
		}
	}
	if kept != 1 {
		t.Errorf("%d sessions said they kept the changes", kept)
	}
}
