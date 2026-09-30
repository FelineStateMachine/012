package serve

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"charm.land/ssh"

	"github.com/FelineStateMachine/012/internal/confine"
	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/ui"
)

// Opening a file named on the ssh command line, and keeping a session's
// unsaved work when it ends without the user quitting. See docs/terminal/ssh.md.

// fileArg is the file an exec request names (ssh -t host file.012), ""
// for a shell request. The request is never run: its one word is a file
// name, which must resolve inside the served directory as names typed in
// File > Open do. allowSession has refused anything but one word.
func (s *Server) fileArg(sess ssh.Session) (string, error) {
	if sess.RawCommand() == "" {
		return "", nil
	}
	args := sess.Command()
	if len(args) != 1 {
		return "", errors.New("give one file name: ssh -t host name.012")
	}
	name := args[0]
	if name == "" || strings.ContainsFunc(name, unicode.IsControl) {
		return "", errors.New("that's not a file name")
	}
	names := []string{name}
	if filepath.Ext(name) == "" {
		names = append(names, name+sheet.FileExt) // as the session will open it
	}
	for _, n := range names {
		if _, err := s.root.Resolve(n); err != nil {
			return "", fmt.Errorf("can't open %s: %s", name, s.root.Scrub(err.Error()))
		}
	}
	return name, nil
}

// ending is how a session ended, and where its unsaved work was kept.
type ending struct {
	idle, stopped bool      // it idled out; the server stopped
	crash         *ui.Crash // it stopped on an internal error
	kept          string    // the recovery file, relative to the served directory
	keptFor       string    // the file name it was kept for, "" for a book never saved
	err           error     // keeping it failed
	others        string    // who still has the shared workbook open, "" when nobody
}

// keep writes the unsaved work of the program g ran to a recovery file
// when the session ended by idling out, by the server stopping or on an
// internal error; when the user quits, the app has already asked about
// unsaved changes.
func (e *ending) keep(g *ui.Guarded) {
	if !e.idle && !e.stopped && e.crash == nil {
		return
	}
	e.keptFor = g.Model().Filename()
	e.kept, e.err = g.Keep(time.Now())
}

// tell says on the client's terminal why the session ended and where its
// unsaved work went.
func (e ending) tell(w io.Writer, idle time.Duration, root confine.Root) {
	switch {
	case e.stopped:
		fmt.Fprint(w, "012: the server is stopping\r\n")
	case e.idle:
		fmt.Fprintf(w, "012: closed after %v without input\r\n", idle)
	case e.crash != nil:
		fmt.Fprint(w, "012: stopped on an internal error; the server's log has a report\r\n")
	}
	switch {
	case e.others != "" && (e.idle || e.crash != nil):
		fmt.Fprintf(w, "012: the workbook stays open with %s\r\n", e.others)
	case e.err != nil:
		fmt.Fprintf(w, "012: couldn't keep the unsaved changes: %s\r\n", root.Scrub(e.err.Error()))
	case e.kept != "" && e.keptFor == "":
		fmt.Fprintf(w, "012: unsaved changes kept in %s; connect again to restore them\r\n", e.kept)
	case e.kept != "":
		fmt.Fprintf(w, "012: unsaved changes kept in %s; open %s again to restore them\r\n", e.kept, e.keptFor)
	}
}
