package ui

import (
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/room"
	"github.com/FelineStateMachine/012/internal/sheet"
)

// Sharing a workbook between the sessions of 012 serve: every session
// that opens the same file (or joins the same named room, @name) sits
// in one room (package room) and works on its one workbook, each with
// its own cursor, scroll, selection, theme and modes. The session's
// model runs its turns (Update, View) under the room's lock (Shared,
// rooms.go), so the workbook's one mutation path orders everyone's
// steps; each step is its author's, so undo takes back only one's own
// (sheet/authors.go). When the room changes, the others are told
// (roomMsg), draw what changed and see where each other is
// (sharedraw.go). Saving is the room's: every session counts unsaved
// changes from the last save anyone made, and asks about them only
// when it's the last one there. Linked files are followed, and
// notebook cells run, by the room (nbRuns, follow.go), so they go on
// while anyone is there. See docs/terminal/ssh.md.

// shareState is a served session's part in the rooms.
type shareState struct {
	reg  *room.Registry // nil: not shared (the local app, or serve without rooms)
	link *peerLink      // the session as a participant
	seat *room.Seat     // the room of the workbook shown; nil in a workbook of its own
	want *joinReq       // a room to join once the turn is over
	seen uint64         // the room's last operation this model has looked at
	held []tea.Msg      // input held while another's step is open (a macro running)
	// entryFrom is the room's last operation when the entry being typed
	// began, to tell whether someone changed the cell meanwhile.
	entryFrom uint64
	// frame is what the frame draws of the others, and ticking is set
	// while a redraw waits for marks to go stale: sharedraw.go.
	frame   shareFrame
	ticking bool
	// still stops the ticks, so tests that run commands in line don't
	// wait on them.
	still bool
}

// joinReq is a room to join: its key, the file's name as typed, and
// for a room not yet open, its workbook (nil for a new one) and the
// file's stamp.
type joinReq struct {
	key, name string
	book      *sheet.Workbook
	stamp     stamp
	// restored is the recovery file book was read from: the room opens
	// again on it, as unsaved changes.
	restored string
}

// roomMsg tells the model its room changed.
type roomMsg struct{}

// ShareRooms makes a served session open files in the rooms of reg, as
// user: sessions opening the same file share its workbook.
func (m *Model) ShareRooms(reg *room.Registry, user string) {
	m.share.reg = reg
	m.share.link = &peerLink{name: user, kick: make(chan struct{}, 1)}
}

// shared reports whether the workbook is shown in a room with others.
func (m *Model) shared() bool { return m.share.seat != nil && m.share.seat.Others() > 0 }

// roomKey is the room of the file named name: its path on disk, or the
// name itself for a named room (@name).
func (m *Model) roomKey(name string) (string, bool) {
	if strings.HasPrefix(name, "@") {
		return name, true
	}
	p, ok := m.path("open", name)
	if !ok {
		return "", false
	}
	if abs, err := filepath.Abs(p); err == nil {
		p = abs
	}
	return p, true
}

// openShared opens the file named name, at path on disk, in its room:
// joining the room when it's open, reading the file first otherwise.
func (m *Model) openShared(name, path string) tea.Cmd {
	key, ok := m.roomKey(name)
	if !ok {
		return nil
	}
	if m.share.reg.Has(key) {
		m.share.want = &joinReq{key: key, name: name}
		return nil
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		m.share.want = &joinReq{key: key, name: name, book: sheet.New().Book()}
		return nil
	}
	return loadCmd(name, path, m.spans.Parent())
}

// joinNamed joins the named room @name, opening it on a new workbook.
func (m *Model) joinNamed(name string) {
	m.share.want = &joinReq{key: name}
}

// loadedShared takes a file read to be opened in its room.
func (m *Model) loadedShared(msg loadedMsg) {
	key, ok := m.roomKey(msg.name)
	if !ok {
		return
	}
	m.share.want = &joinReq{key: key, name: msg.name, book: msg.sheet.Book(), stamp: msg.stamp}
}

// enterRoom shows the room's workbook, as opening a file does. It runs
// in a turn of the new seat.
func (m *Model) enterRoom(seat *room.Seat, created bool, req *joinReq) tea.Cmd {
	w := seat.Book()
	s := w.Sheets()[w.Active()]
	if !s.Live() || s.Hidden() {
		s = w.Visible()[0]
	}
	m.reset(s, req.name)
	m.share.seat, m.share.seen = seat, seat.Last()
	m.nb.runs = seat.Value("nb", func() any { return &nbRuns{} }).(*nbRuns)
	switch {
	case created && req.restored != "":
		m.disk, m.saved, m.changed, m.recovered = req.stamp, -1, true, req.restored
		v := m.savedState()
		v.State = -1
		seat.SetSaved(v)
		m.note = "Restored the kept changes; save to keep them"
	case created:
		m.disk = req.stamp
		seat.SetSaved(m.savedState())
		m.offerRecovery(req.name)
	default:
		m.syncSaved()
	}
	if peers := seat.Peers(); len(peers) > 0 {
		m.note = "Sharing " + m.displayName() + " with " + peerNames(peers)
	}
	return tea.Batch(m.syncFollowers(), m.notebookSync())
}

// restoreShared opens the room again on kept changes, when nobody else
// is in it: others are working on the workbook as it is.
func (m *Model) restoreShared(seat *room.Seat, msg restoredMsg) {
	if peers := seat.Peers(); len(peers) > 0 {
		m.warn = peerNames(peers) + " have " + m.displayName() + " open: restore the kept changes once you're the only one here"
		return
	}
	m.share.want = &joinReq{key: seat.Key(), name: msg.name, book: msg.sheet.Book(), stamp: m.disk, restored: msg.file}
}

// savedState is the room's file as this model last saved or opened it.
func (m *Model) savedState() room.Saved {
	key := ""
	if m.filename != "" {
		key, _ = m.roomKey(m.filename)
	}
	return room.Saved{State: m.book().StateID(), Outputs: m.book().OutputsChanged(), Name: m.filename, Key: key, Stamp: m.disk}
}

// syncSaved takes the room's file as anyone last saved it: its name,
// and what unsaved changes count from.
func (m *Model) syncSaved() {
	v := m.share.seat.Saved()
	m.saved, m.nb.saved = v.State, v.Outputs
	if st, ok := v.Stamp.(stamp); ok {
		m.disk = st
	}
	if v.Name != m.filename {
		m.filename = v.Name
	}
	m.changed = m.book().StateID() != m.saved || m.book().OutputsChanged() != m.nb.saved
}

// roomChanged catches up with the room: what was posted for the keeper,
// others' operations, a save, a sheet deleted from under the view.
func (m *Model) roomChanged() tea.Cmd {
	seat := m.share.seat
	if seat == nil {
		return nil
	}
	var cmds []tea.Cmd
	for _, msg := range seat.Take() {
		_, cmd := m.Update(msg)
		cmds = append(cmds, cmd)
	}
	m.othersChanged(seat.Marks(m.share.seen))
	m.share.seen = seat.Last()
	m.syncSaved()
	if !m.sheet.Live() || m.sheet.Hidden() {
		m.afterSheetsChange(nil, m.book().Active())
	}
	m.cur = clampAddr(m.cur)
	return tea.Batch(cmds...)
}

// othersChanged says on the context line when someone else changed the
// cell being typed into, or the one the pointer is on.
func (m *Model) othersChanged(marks []room.Mark) {
	seat := m.share.seat
	for _, mk := range marks {
		if mk.Author == seat.ID() || mk.Sheet != m.sheet || !mk.Rect.Contains(m.cur) {
			continue
		}
		switch {
		case m.mode == modeEnter || m.mode == modeEdit:
			m.warn = mk.Name + " changed " + m.cur.String() + " while you type: Enter puts yours over theirs, Esc keeps theirs"
		case m.mode == modeReady:
			m.note = mk.Name + " changed " + m.cur.String()
		}
	}
}

// presence is where the user is, for the others.
func (m *Model) presence() room.Presence {
	sel := sheet.Rect{From: m.cur, To: m.cur}
	if r, ok := m.highlight(); ok {
		sel = r
	}
	return room.Presence{Sheet: m.sheet, Cursor: m.active(), Selection: sel, Editing: m.mode == modeEnter || m.mode == modeEdit}
}

// mayEdit reports whether the user may change the workbook: always,
// but in a one-writer room only the writer. When not, it says who
// writes.
func (m *Model) mayEdit() bool {
	seat := m.share.seat
	if seat == nil || seat.Writing() {
		return true
	}
	m.warn = seat.Writer() + " writes here and you follow: they can hand writing to you"
	return false
}

// LeaveRoom gives up the session's seat as it ends, reporting whether
// it was the last one there (or had a workbook of its own), whose
// unsaved changes are then its to keep: others keep a room's workbook
// open. The last one out stops what the room ran.
func (m *Model) LeaveRoom() bool {
	seat := m.share.seat
	if seat == nil {
		m.closeStreams()
		return true
	}
	m.share.seat = nil
	if !seat.Leave() {
		return false
	}
	m.closeStreams()
	return true
}

// Others is how many others share the session's workbook, and their
// names.
func (m *Model) Others() (int, string) {
	if m.share.seat == nil {
		return 0, ""
	}
	var n int
	var names string
	m.share.seat.Do(func(*sheet.Workbook) {
		peers := m.share.seat.Peers()
		n, names = len(peers), peerNames(peers)
	})
	return n, names
}

// peerNames lists peers' names: "ann", "ann and bob", "ann, bob and cy".
func peerNames(peers []room.Peer) string {
	names := make([]string, len(peers))
	for i, p := range peers {
		names[i] = p.Name
	}
	switch len(names) {
	case 0:
		return ""
	case 1:
		return names[0]
	}
	return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
}

// peerLink is a served session as a room's participant: telling it the
// room changed kicks a goroutine that sends its program a roomMsg
// (Shared.Attach).
type peerLink struct {
	name string
	kick chan struct{}
}

func (p *peerLink) Name() string { return p.name }

func (p *peerLink) Notify() {
	select {
	case p.kick <- struct{}{}:
	default:
	}
}
