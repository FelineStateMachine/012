package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Two people reach one 012 serve with the system's ssh client, each in
// a terminal of their own, and open the same file: what one types the
// other sees, each sees where the other is, undo takes back only one's
// own change, the first to quit leaves without being asked, and the one
// left saves the file.
func TestServeSharesAFile(t *testing.T) {
	top := t.TempDir()
	served := filepath.Join(top, "served")
	os.Mkdir(served, 0o755)
	sshPath, args := serveSSH(t, top, served)
	args = append(args, "plan.012")

	ann := startWith(t, options{dir: top, program: sshPath}, asUser("ann", args)...)
	ann.waitFor("plan.012")
	bob := startWith(t, options{dir: top, program: sshPath}, asUser("bob", args)...)
	bob.waitFor("Sharing plan.012 with ann")
	ann.waitFor(" bob ") // on ann's status line

	ann.keys("rent", "<enter>")
	bob.waitForLine(gridRow1, "    1 ▘rent") // marked: ann just changed it
	bob.keys("<right>", "<down>", "450", "<enter>")
	ann.waitFor("450")
	// Bob's pointer is on B3 now: his initial is on row 3's header.
	ann.eventually("bob's initial on row 3", func() bool { return strings.HasPrefix(ann.line(gridRow1+2), "b   3") })

	// Ann's undo takes back her entry, not bob's.
	ann.keys("<ctrl+z>")
	bob.eventually("ann's undo", func() bool { return !strings.Contains(bob.screen(), "rent") })
	if !strings.Contains(bob.screen(), "450") {
		t.Fatalf("ann's undo took back bob's entry:\n%s", bob.screen())
	}

	bob.keys("<ctrl+q>") // ann still has it: no question
	bob.waitExit()
	ann.eventually("bob gone from the status line", func() bool { return !strings.Contains(ann.line(int(ann.rows)-1), " bob ") })
	ann.keys("<ctrl+s>")
	ann.eventually("plan.012 saved", func() bool {
		data, err := os.ReadFile(filepath.Join(served, "plan.012"))
		return err == nil && strings.Contains(string(data), "450")
	})
	ann.keys("<ctrl+q>")
	ann.waitExit()
}
