package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/confine"
	"github.com/FelineStateMachine/012/internal/room"
	"github.com/FelineStateMachine/012/internal/sheet"
)

// seshion is a served session in a room, driven as its program would:
// every message through Shared.
type seshion struct {
	t  *testing.T
	sh *Shared
	m  *Model
}

// servedRooms is a served directory and the rooms of its sessions.
type servedRooms struct {
	t    *testing.T
	dir  string
	root confine.Root
	reg  *room.Registry
	all  []*seshion
}

func newRooms(t *testing.T, mode room.Mode) *servedRooms {
	dir := t.TempDir()
	root, err := confine.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	return &servedRooms{t: t, dir: dir, root: root, reg: room.NewRegistry(mode)}
}

// open starts user's session on name, as ssh -t host name does, and
// catches everyone up.
func (r *servedRooms) open(user, name string) *seshion {
	m := New(sheet.New(), "")
	m.Serve(r.root, []string{"TERM=xterm-256color"})
	m.ShareRooms(r.reg, user)
	m.OpenOnStart(name)
	m.SetMachine("here")
	m.share.still, m.nb.still = true, true
	s := &seshion{t: r.t, sh: InRooms(Guard(m)), m: m}
	s.run(s.sh.Init())
	s.send(tea.WindowSizeMsg{Width: 80, Height: 20})
	r.all = append(r.all, s)
	r.sync()
	return s
}

// sync delivers roomMsg to every session its room told, until none is
// waiting: what Shared.Attach does for a program.
func (r *servedRooms) sync() {
	for range 10 {
		idle := true
		for _, s := range r.all {
			select {
			case <-s.m.share.link.kick:
				idle = false
				s.send(roomMsg{})
			default:
			}
		}
		if idle {
			return
		}
	}
}

func (s *seshion) update(msg tea.Msg) tea.Cmd {
	_, cmd := s.sh.Update(msg)
	return cmd
}

func (s *seshion) run(cmd tea.Cmd) tea.Msg { return runVia(s.update, cmd) }

func (s *seshion) send(msg tea.Msg) tea.Msg { return s.run(s.update(msg)) }

func (s *seshion) press(keys ...string) tea.Msg {
	s.t.Helper()
	return pressTo(s.t, s.send, keys...)
}

func (s *seshion) screen() string {
	var out string
	s.sh.turn(func() { out = ansi.Strip(s.m.View().Content) })
	return out
}

// Two sessions opening one file share its workbook: what one types the
// other sees, where each is shows on the other's screen, and each has
// its own cursor.
func TestSharedSessionsSeeEachOther(t *testing.T) {
	r := newRooms(t, room.Edit)
	writeSheet(t, filepath.Join(r.dir, "budget.012"), "rent")
	ann := r.open("ann", "budget.012")
	bob := r.open("bob", "budget.012")
	if ann.m.book() != bob.m.book() {
		t.Fatal("two workbooks")
	}
	if !strings.Contains(bob.m.note, "Sharing budget.012 with ann") {
		t.Errorf("bob's note %q", bob.m.note)
	}
	ann.press("<down>", "1200", "<enter>")
	r.sync()
	if got := bob.m.sheet.Value(addr("A2")).Num; got != 1200 {
		t.Fatalf("bob sees A2 = %v", got)
	}
	if !strings.Contains(bob.screen(), "1200") {
		t.Errorf("bob's screen:\n%s", bob.screen())
	}
	bob.press("<right>", "<right>")
	r.sync()
	if bob.m.cur != addr("C1") || ann.m.cur != addr("A3") {
		t.Errorf("cursors: ann %v, bob %v", ann.m.cur, bob.m.cur)
	}
	scr := ann.screen()
	if !strings.Contains(scr, "b   1  rent") {
		t.Errorf("bob's initial isn't on row 1's header:\n%s", scr)
	}
	if !strings.Contains(scr, " bob ") {
		t.Errorf("the status line doesn't list bob:\n%s", scr)
	}
	// Ann goes to bob's cell from Who's here.
	run(ann.m, nil)
	ann.run(ann.m.runCommand("share.who"))
	ann.press("<enter>")
	ann.screen() // a frame, which works out where the others are
	if ann.m.cur != addr("C1") || !strings.Contains(ansi.Strip(ann.m.shareLine()), "bob  is here") {
		t.Errorf("ann at %v:\n%s", ann.m.cur, ann.screen())
	}
}

// Undo takes back one's own changes, and says whose later change is in
// the way when it can't.
func TestSharedUndoIsYourOwn(t *testing.T) {
	r := newRooms(t, room.Edit)
	ann := r.open("ann", "@plan")
	bob := r.open("bob", "@plan")
	ann.press("mine", "<enter>")
	r.sync()
	bob.press("<right>", "theirs", "<enter>")
	r.sync()
	ann.press("<ctrl+z>")
	r.sync()
	s := ann.m.sheet
	if s.Filled(addr("A1")) || s.Value(addr("B1")).Str != "theirs" {
		t.Fatalf("after ann's undo: A1 %v, B1 %v", s.Value(addr("A1")), s.Value(addr("B1")))
	}
	if !strings.Contains(bob.screen(), "theirs") || strings.Contains(bob.screen(), "mine") {
		t.Errorf("bob's screen:\n%s", bob.screen())
	}
	ann.press("<ctrl+y>")
	r.sync()
	bob.press("<left>", "<up>", "bob's", "<enter>")
	r.sync()
	ann.press("<ctrl+z>")
	if !strings.Contains(ann.m.warn, "Can't undo") || !strings.Contains(ann.m.warn, "bob") || s.Value(addr("A1")).Str != "bob's" {
		t.Errorf("ann's undo: %q, A1 %v", ann.m.warn, s.Value(addr("A1")))
	}
	if ann.m.displayName() != "@plan" {
		t.Errorf("named %q", ann.m.displayName())
	}
}

// An entry typed over a cell someone else changed meanwhile goes in, in
// the server's order, and both are told.
func TestSharedConflictingEntries(t *testing.T) {
	r := newRooms(t, room.Edit)
	ann := r.open("ann", "@c")
	bob := r.open("bob", "@c")
	ann.press("a", "nn")
	bob.press("bob", "<enter>")
	r.sync()
	if !strings.Contains(ann.m.warn, "bob changed A1 while you type") {
		t.Errorf("ann wasn't told: %q", ann.m.warn)
	}
	ann.press("<enter>")
	r.sync()
	if ann.m.sheet.Value(addr("A1")).Str != "ann" || !strings.Contains(ann.m.warn, "replaced bob's change to A1") {
		t.Errorf("A1 %v, ann's warning %q", ann.m.sheet.Value(addr("A1")), ann.m.warn)
	}
	bob.press("<up>")
	if !strings.Contains(bob.screen(), "Changed by") {
		t.Errorf("bob's context line:\n%s", bob.screen())
	}
}

// Saving is the room's: a save by one clears the others' modified mark,
// and quitting while others remain asks nothing.
func TestSharedSaveAndQuit(t *testing.T) {
	r := newRooms(t, room.Edit)
	ann := r.open("ann", "notes.012")
	bob := r.open("bob", "notes.012")
	bob.press("x", "<enter>")
	r.sync()
	if !ann.m.changed || !bob.m.changed {
		t.Fatalf("modified: ann %v, bob %v", ann.m.changed, bob.m.changed)
	}
	ann.press("<ctrl+s>")
	r.sync()
	if ann.m.changed || bob.m.changed {
		t.Errorf("after ann saved, modified: ann %v, bob %v", ann.m.changed, bob.m.changed)
	}
	if _, err := os.Stat(filepath.Join(r.dir, "notes.012")); err != nil {
		t.Fatal(err)
	}
	bob.press("y", "<enter>")
	r.sync()
	if msg := ann.press("<ctrl+q>"); msg == nil {
		t.Errorf("ann asked before quitting, bob still there: %q", ann.screen())
	}
	if ann.m.LeaveRoom() {
		t.Error("ann was the last")
	}
	r.sync()
	if !bob.m.LeaveRoom() || !bob.m.Unsaved() {
		t.Error("bob, last, doesn't keep his unsaved change")
	}
}

// In a one-writer room the others follow: they can't edit until the
// writer hands writing over.
func TestSharedOneWriter(t *testing.T) {
	r := newRooms(t, room.View)
	ann := r.open("ann", "@talk")
	bob := r.open("bob", "@talk")
	bob.press("hello")
	if bob.m.mode != modeReady || !strings.Contains(bob.m.warn, "ann writes here") {
		t.Fatalf("bob typed: mode %v, %q", bob.m.mode, bob.m.warn)
	}
	if !strings.Contains(bob.screen(), "ann writes") {
		t.Errorf("bob's status line:\n%s", bob.screen())
	}
	ann.run(ann.m.runCommand("share.hand"))
	ann.press("<enter>")
	r.sync()
	bob.press("hello", "<enter>")
	if bob.m.sheet.Value(addr("A1")).Str != "hello" {
		t.Errorf("bob, writing, couldn't type: %q", bob.m.warn)
	}
	ann.press("<down>", "no", "<enter>")
	if ann.m.sheet.Filled(addr("A2")) {
		t.Error("ann typed after handing over")
	}
}
