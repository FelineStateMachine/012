// Package room shares workbooks between the people 012 serve hosts: a
// room is one workbook, opened by every session that opens the same
// file (or joins the same named room), and each participant has a seat
// in it. The server holds the workbook, so there is no merging: every
// participant acts on it in turn, under the room's lock, and the steps
// they make are ordered by the workbook's one mutation path (Batch and
// its undo steps), each attributed to its author, as a stream of
// operations (sheet.Op) the room hands every participant. A participant
// is anything with a name that can be told the room changed
// (Participant): a served session's model. The interface stays that
// small so an MCP client attached to the server can take part the same
// way (ROADMAP.md, Agents). See docs/contributing/architecture.md.
package room

import (
	"slices"
	"sync"
	"time"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// Participant is one who takes part in a room. The room asks only this
// of it; everything else a participant does, it does through its Seat.
type Participant interface {
	// Name is who they are to the others: the SSH user of a session.
	Name() string
	// Notify tells them the room changed: someone else's operations,
	// where the others are, who writes, a save, a message posted for
	// them. It's called with the room locked, so it must neither block
	// nor call back into the room: they look at what changed on their
	// next turn (Seat.Do).
	Notify()
}

// Mode is who may change a room's workbook.
type Mode int

const (
	// Edit lets every participant edit.
	Edit Mode = iota
	// View lets one participant, the writer, edit; the others follow.
	// The writer hands the role over explicitly (Seat.HandTo).
	View
)

// Colors is how many colors participants are told apart by.
const Colors = 6

// Room is one shared workbook and the seats of those in it.
type Room struct {
	key string
	reg *Registry

	mu     sync.Mutex
	book   *sheet.Workbook
	seats  []*Seat // in the order they joined
	nextID int
	mode   Mode
	writer int // the seat that writes in View mode
	saved  Saved
	values map[string]any
	inbox  []any
	closed bool

	// seen is the last operation the room has looked at, version the
	// workbook's when a turn last ended, and marks the latest
	// operations' cells, for who changed what.
	seen    uint64
	version uint64
	marks   []Mark
	// alias is a key the room was saved under this turn, for the
	// registry once the room is unlocked.
	alias string
}

// Saved is the room's file as last saved or opened: what every session
// in it counts unsaved changes from.
type Saved struct {
	State   int    // the workbook's StateID then
	Outputs int    // its OutputsChanged then
	Name    string // the file's name, as typed, "" for none
	Key     string // the file's room key, its path: the room is its room too
	Stamp   any    // the file on disk then, the UI's to compare
}

// Presence is where a participant is.
type Presence struct {
	Sheet     *sheet.Sheet
	Cursor    sheet.Addr
	Selection sheet.Rect
	Editing   bool // typing into the cell at Cursor
}

// Seat is a participant's place in a room.
type Seat struct {
	room     *Room
	id       int
	name     string
	color    int
	p        Participant
	joined   time.Time
	presence Presence
	moved    bool // presence changed this turn
	left     bool
}

// Peer is what a participant sees of another: who, in what color,
// where, and whether they write.
type Peer struct {
	ID       int
	Name     string
	Color    int
	Presence Presence
	Writing  bool
	Joined   time.Time
}

// Mark is an operation's place in the workbook: who changed which
// cells, for showing whose change a cell holds.
type Mark struct {
	Seq    uint64
	Author int
	Name   string
	Color  int
	Kind   sheet.OpKind
	Label  string
	Sheet  *sheet.Sheet
	Rect   sheet.Rect
	At     time.Time
}

// maxMarks is how many of the latest operations the room remembers the
// cells of.
const maxMarks = 256

// ID identifies the seat in the room: the author of its steps.
func (s *Seat) ID() int { return s.id }

// Name is the participant's name.
func (s *Seat) Name() string { return s.name }

// Color is the participant's color, from 0 to Colors-1.
func (s *Seat) Color() int { return s.color }

// Key is the room's key: the file's path, or @name for a named room.
func (s *Seat) Key() string { return s.room.key }

// Do runs fn as the participant's turn: with the room locked, the
// workbook's steps made in the participant's name. When the turn ends,
// the others are told of what changed.
func (s *Seat) Do(fn func(w *sheet.Workbook)) {
	r := s.room
	r.mu.Lock()
	defer r.endTurn(s)
	r.book.SetAuthor(s.id)
	fn(r.book)
}

// endTurn takes note of the operations the turn made, and tells the
// others when anything changed.
func (r *Room) endTurn(by *Seat) {
	changed := r.catchUp()
	if v := r.book.Version(); v != r.version {
		r.version, changed = v, true
	}
	if changed || by.moved {
		by.moved = false
		r.notify(by)
	}
	alias := r.alias
	r.alias = ""
	r.mu.Unlock()
	if alias != "" {
		r.reg.alias(r, alias)
	}
}

// catchUp marks the cells of the operations since the room last looked.
func (r *Room) catchUp() bool {
	ops, last := r.book.OpsSince(r.seen)
	if last == r.seen {
		return false
	}
	r.seen = last
	now := time.Now()
	for _, o := range ops {
		if o.Tabs || o.Macros || o.Sheet == nil {
			continue
		}
		m := Mark{Seq: o.Seq, Author: o.Author, Kind: o.Kind, Label: o.Label, Sheet: o.Sheet, Rect: o.Focus, At: now}
		if s := r.seat(o.Author); s != nil {
			m.Name, m.Color = s.name, s.color
		}
		r.marks = append(r.marks, m)
	}
	if len(r.marks) > maxMarks {
		r.marks = slices.Delete(r.marks, 0, len(r.marks)-maxMarks)
	}
	return true
}

// notify tells every seat but by that the room changed.
func (r *Room) notify(by *Seat) {
	for _, s := range r.seats {
		if s != by {
			s.p.Notify()
		}
	}
}

// seat is the seat with id, nil when it left.
func (r *Room) seat(id int) *Seat {
	for _, s := range r.seats {
		if s.id == id {
			return s
		}
	}
	return nil
}

// Book is the room's workbook. Only use it in a turn (Do).
func (s *Seat) Book() *sheet.Workbook { return s.room.book }

// Move sets where the participant is, for the others to see once the
// turn ends. Call it in a turn.
func (s *Seat) Move(p Presence) {
	if p != s.presence {
		s.presence, s.moved = p, true
	}
}

// Peers are the others in the room, in the order they joined. Call it
// in a turn.
func (s *Seat) Peers() []Peer {
	var out []Peer
	for _, o := range s.room.seats {
		if o != s {
			out = append(out, o.peer())
		}
	}
	return out
}

func (s *Seat) peer() Peer {
	return Peer{ID: s.id, Name: s.name, Color: s.color, Presence: s.presence, Writing: s.Writing(), Joined: s.joined}
}

// Others is how many others are in the room. Call it in a turn.
func (s *Seat) Others() int { return len(s.room.seats) - 1 }

// Marks are the operations since seq, oldest first, whoever made them.
// Call it in a turn.
func (s *Seat) Marks(since uint64) []Mark {
	m := s.room.marks
	i, _ := slices.BinarySearchFunc(m, since+1, func(x Mark, seq uint64) int { return int(x.Seq) - int(seq) })
	return m[i:]
}

// Last is the latest operation's number. Call it in a turn.
func (s *Seat) Last() uint64 { return s.room.seen }

// Mode is who may edit the room's workbook.
func (s *Seat) Mode() Mode { return s.room.mode }

// Writing reports whether the participant may edit: in an Edit room
// everyone, in a View room the writer.
func (s *Seat) Writing() bool { return s.room.mode == Edit || s.room.writer == s.id }

// Writer is the writer's name in a View room, "" in an Edit room.
func (s *Seat) Writer() string {
	if s.room.mode == Edit {
		return ""
	}
	if w := s.room.seat(s.room.writer); w != nil {
		return w.name
	}
	return ""
}

// HandTo hands the writer's role to the participant with id, reporting
// false when this one doesn't write or there's no such participant.
// Call it in a turn.
func (s *Seat) HandTo(id int) bool {
	r := s.room
	if r.mode != View || r.writer != s.id || r.seat(id) == nil {
		return false
	}
	r.writer, s.moved = id, true
	return true
}

// Saved is the room's file as last saved or opened. Call it in a turn.
func (s *Seat) Saved() Saved { return s.room.saved }

// SetSaved records a save, or the file the room was opened from, for
// every session. Call it in a turn.
func (s *Seat) SetSaved(v Saved) {
	s.room.saved, s.moved = v, true
	if v.Key != "" && v.Key != s.room.key {
		s.room.alias = v.Key
	}
}

// Value is the room's value for key, made by make the first time: state
// every participant shares, such as the notebook cells running. Call it
// in a turn.
func (s *Seat) Value(key string, make func() any) any {
	r := s.room
	if r.values == nil {
		r.values = map[string]any{}
	}
	v, ok := r.values[key]
	if !ok {
		v = make()
		r.values[key] = v
	}
	return v
}

// Keeper reports whether the participant keeps what the room runs on
// its own (linked files followed, notebook cells running): the one who
// has been there longest. Call it in a turn.
func (s *Seat) Keeper() bool { return len(s.room.seats) > 0 && s.room.seats[0] == s }

// Post leaves msg for the keeper, whoever it is when it takes it: what
// the room runs reports back this way rather than to the participant
// that started it, who may have left. It may be called from any
// goroutine, outside a turn.
func (s *Seat) Post(msg any) {
	r := s.room
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed || len(r.seats) == 0 {
		return
	}
	r.inbox = append(r.inbox, msg)
	r.seats[0].p.Notify()
}

// Take is what was posted for the keeper, if this is the keeper. Call
// it in a turn.
func (s *Seat) Take() []any {
	if !s.Keeper() || len(s.room.inbox) == 0 {
		return nil
	}
	in := s.room.inbox
	s.room.inbox = nil
	return in
}

// Leave gives up the seat, reporting whether it was the last: the room
// closes then, and its workbook is the participant's alone.
func (s *Seat) Leave() (last bool) {
	r := s.room
	r.mu.Lock()
	if s.left {
		r.mu.Unlock()
		return false
	}
	s.left = true
	i := slices.Index(r.seats, s)
	r.seats = slices.Delete(r.seats, i, i+1)
	empty := len(r.seats) == 0
	if !empty {
		if r.mode == View && r.writer == s.id {
			r.writer = r.seats[0].id
		}
		r.notify(nil)
	}
	r.mu.Unlock()
	if empty {
		return r.reg.close(r)
	}
	return false
}
