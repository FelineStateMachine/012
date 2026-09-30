package room

import (
	"sync"
	"time"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// Registry is the rooms a server holds, by key: a file's path, or @name
// for a named room.
type Registry struct {
	mode  Mode
	mu    sync.Mutex
	rooms map[string]*Room
}

// NewRegistry holds rooms whose participants edit as mode says.
func NewRegistry(mode Mode) *Registry {
	return &Registry{mode: mode, rooms: map[string]*Room{}}
}

// Has reports whether a room with key is open.
func (g *Registry) Has(key string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.rooms[key] != nil
}

// Rooms is how many rooms are open.
func (g *Registry) Rooms() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	seen := map[*Room]bool{}
	for _, r := range g.rooms {
		seen[r] = true
	}
	return len(seen)
}

// Join seats p in the room with key, opening it on the workbook book
// makes when there's none (book is called with no lock held but the
// registry's, and may be nil when the room is known to be open). It
// reports whether it opened the room. A room's history is shared: undo
// takes back each participant's own steps.
func (g *Registry) Join(key string, p Participant, book func() *sheet.Workbook) (*Seat, bool) {
	return g.join(key, p, book)
}

// join is Join; with book nil, it seats p only in a room that's open.
func (g *Registry) join(key string, p Participant, book func() *sheet.Workbook) (*Seat, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	r := g.rooms[key]
	created := false
	if r == nil && book == nil {
		return nil, false
	}
	if r == nil {
		w := book()
		w.ShareHistory()
		r = &Room{key: key, reg: g, book: w, mode: g.mode}
		r.seen, r.version = w.LastOp(), w.Version()
		g.rooms[key] = r
		created = true
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nextID++
	s := &Seat{room: r, id: r.nextID, name: p.Name(), color: r.freeColor(), p: p, joined: time.Now()}
	if _, ok := p.(Agent); ok {
		s.agent = true
	}
	if len(r.seats) == 0 {
		r.writer = s.id
	}
	r.seats = append(r.seats, s)
	r.notify(s)
	return s, created
}

// JoinOpen seats p in the room with key if it's open, reporting false
// when it isn't: an agent joins only a workbook someone has open.
func (g *Registry) JoinOpen(key string, p Participant) (*Seat, bool) {
	s, _ := g.join(key, p, nil)
	return s, s != nil
}

// freeColor is the first color no one in the room has.
func (r *Room) freeColor() int {
	used := map[int]bool{}
	for _, s := range r.seats {
		used[s.color] = true
	}
	for c := range Colors {
		if !used[c] {
			return c
		}
	}
	return r.nextID % Colors
}

// alias makes the room also the one of key, a name it was saved under,
// unless another room has it.
func (g *Registry) alias(r *Room, key string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.rooms[key] == nil {
		g.rooms[key] = r
	}
}

// close forgets the room once its last person has left, reporting whether
// it did: someone may have joined since.
func (g *Registry) close(r *Room) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.keeper() != nil || r.closed {
		return false
	}
	r.closed = true
	for k, x := range g.rooms {
		if x == r {
			delete(g.rooms, k)
		}
	}
	return true
}
