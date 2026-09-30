package room

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// who is a participant counting the times it was told the room changed.
type who struct {
	name  string
	kicks atomic.Int64
}

func (w *who) Name() string { return w.name }
func (w *who) Notify()      { w.kicks.Add(1) }

func addr(s string) sheet.Addr {
	a, _ := sheet.ParseAddr(s)
	return a
}

func newBook() *sheet.Workbook { return sheet.New().Book() }

// Sessions opening the same key share one workbook: each turn's steps
// are its author's, the others are told, and the room remembers who
// changed which cells, in order.
func TestRoomSharesOneWorkbook(t *testing.T) {
	g := NewRegistry(Edit)
	ann, bob := &who{name: "ann"}, &who{name: "bob"}
	a, created := g.Join("/srv/budget.012", ann, newBook)
	if !created {
		t.Fatal("the first join didn't open the room")
	}
	b, created := g.Join("/srv/budget.012", bob, func() *sheet.Workbook { t.Fatal("opened twice"); return nil })
	if created || a.Book() != b.Book() || a.Color() == b.Color() {
		t.Fatalf("created %v, same book %v, colors %d %d", created, a.Book() == b.Book(), a.Color(), b.Color())
	}
	before := bob.kicks.Load()
	a.Do(func(w *sheet.Workbook) { w.Sheets()[0].Set(addr("A1"), "ann's") })
	if bob.kicks.Load() == before {
		t.Error("bob wasn't told of ann's edit")
	}
	b.Do(func(w *sheet.Workbook) { w.Sheets()[0].Set(addr("B1"), "bob's") })
	var marks []Mark
	a.Do(func(*sheet.Workbook) { marks = a.Marks(0) })
	if len(marks) != 2 || marks[0].Name != "ann" || marks[1].Name != "bob" || marks[1].Rect.From != addr("B1") || marks[0].Seq >= marks[1].Seq {
		t.Fatalf("marks %+v", marks)
	}
	// Undo is each author's own.
	a.Do(func(w *sheet.Workbook) { w.Undo() })
	s := a.Book().Sheets()[0]
	if s.Filled(addr("A1")) || s.Value(addr("B1")).Str != "bob's" {
		t.Errorf("after ann's undo: A1 %v, B1 %v", s.Value(addr("A1")), s.Value(addr("B1")))
	}
}

// Presence reaches the others; a turn that changes nothing tells no one.
func TestRoomPresence(t *testing.T) {
	g := NewRegistry(Edit)
	ann, bob := &who{name: "ann"}, &who{name: "bob"}
	a, _ := g.Join("@standup", ann, newBook)
	b, _ := g.Join("@standup", bob, nil)
	before := bob.kicks.Load()
	a.Do(func(*sheet.Workbook) {})
	if bob.kicks.Load() != before {
		t.Error("told of nothing")
	}
	a.Do(func(w *sheet.Workbook) { a.Move(Presence{Sheet: w.Sheets()[0], Cursor: addr("C3")}) })
	if bob.kicks.Load() == before {
		t.Error("not told ann moved")
	}
	var peers []Peer
	b.Do(func(*sheet.Workbook) { peers = b.Peers() })
	if len(peers) != 1 || peers[0].Name != "ann" || peers[0].Presence.Cursor != addr("C3") {
		t.Errorf("peers %+v", peers)
	}
}

// In a View room one writes: the first in, until they hand it over or
// leave.
func TestRoomWriter(t *testing.T) {
	g := NewRegistry(View)
	a, _ := g.Join("k", &who{name: "ann"}, newBook)
	b, _ := g.Join("k", &who{name: "bob"}, nil)
	c, _ := g.Join("k", &who{name: "cy"}, nil)
	a.Do(func(*sheet.Workbook) {
		if !a.Writing() || b.Writing() || a.Writer() != "ann" {
			t.Errorf("writing: ann %v, bob %v", a.Writing(), b.Writing())
		}
		if b.HandTo(c.ID()) {
			t.Error("bob handed over what he doesn't hold")
		}
		if !a.HandTo(b.ID()) || !b.Writing() || a.Writing() {
			t.Error("ann didn't hand over to bob")
		}
	})
	b.Leave()
	a.Do(func(*sheet.Workbook) {
		if !a.Writing() {
			t.Error("the writer left, and ann, there longest, doesn't write")
		}
	})
}

// What is posted goes to the keeper, the one there longest, and to the
// next one once the keeper leaves; the last to leave closes the room.
func TestRoomKeeperAndLeaving(t *testing.T) {
	g := NewRegistry(Edit)
	ann, bob := &who{name: "ann"}, &who{name: "bob"}
	a, _ := g.Join("k", ann, newBook)
	b, _ := g.Join("k", bob, nil)
	b.Post("done")
	var got []any
	b.Do(func(*sheet.Workbook) { got = b.Take() })
	if got != nil {
		t.Errorf("bob took %v", got)
	}
	if a.Leave() {
		t.Fatal("ann was last")
	}
	b.Do(func(*sheet.Workbook) { got = b.Take() })
	if len(got) != 1 || got[0] != "done" {
		t.Errorf("bob, keeper now, took %v", got)
	}
	if !g.Has("k") || !b.Leave() || g.Has("k") || g.Rooms() != 0 {
		t.Error("the last to leave didn't close the room")
	}
	// A save under another name makes the room that file's too.
	c, _ := g.Join("@r", &who{name: "cy"}, newBook)
	c.Do(func(*sheet.Workbook) { c.SetSaved(Saved{Name: "r.012", Key: "/srv/r.012"}) })
	if !g.Has("/srv/r.012") || g.Rooms() != 1 {
		t.Error("saving didn't alias the room")
	}
}

// Many participants taking turns at once: every step lands, in one
// order, and the race detector finds nothing.
func TestRoomTurnsAtOnce(t *testing.T) {
	g := NewRegistry(Edit)
	var wg sync.WaitGroup
	const n, edits = 6, 50
	for i := range n {
		s, _ := g.Join("k", &who{name: fmt.Sprint(i)}, newBook)
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range edits {
				s.Do(func(w *sheet.Workbook) {
					w.Sheets()[0].Set(sheet.Addr{Col: i, Row: j}, fmt.Sprint(j))
					s.Move(Presence{Cursor: sheet.Addr{Col: i, Row: j}})
				})
			}
		}()
	}
	wg.Wait()
	s, _ := g.Join("k", &who{name: "last"}, nil)
	s.Do(func(w *sheet.Workbook) {
		if got := w.Sheets()[0].Len(); got != n*edits {
			t.Errorf("%d cells", got)
		}
		if s.Last() != n*edits {
			t.Errorf("%d operations", s.Last())
		}
	})
}
