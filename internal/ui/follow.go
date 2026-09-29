package ui

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/fileio"
	"github.com/FelineStateMachine/012/internal/live"
	"github.com/FelineStateMachine/012/internal/sheet"
)

// Following linked regions (Data > Linked file, File > Import's Follow
// the file): each region of the workbook gets a live.Source, polled one
// poll at a time on a command's goroutine every live.Interval (at once
// while it has more to read), and each update comes back as a message
// and is applied to the workbook as a sheet.LiveOp, the change stream,
// outside the undo history. What follows which region is reconciled
// with the workbook after every update (syncFollowers), so undo, redo,
// opening a file, pausing and unlinking need nothing of their own.

// followState is the linked regions' part of the model.
type followState struct {
	by map[int]*follower // by region ID
	// trusted is set once the user agreed to follow the files a
	// workbook from elsewhere links outside its folder; asked once the
	// question was put.
	trusted, asked bool
}

// follower is a region's source and where its polling is.
type follower struct {
	id  int
	key string // the source it was made for, to notice a change
	src live.Source
	// busy is set while a poll runs, waiting while one is scheduled;
	// gone once the region is gone, closing the source when its poll
	// returns. reload is set from asking the source to read whole until
	// it has, and pending when that was asked while a poll ran.
	busy, waiting, gone, reload, pending bool
}

// followInterval is how often a region's source is polled; tests lower
// it.
var followInterval = live.Interval

// followTickMsg says it's time to poll f; followMsg is what a poll found.
type (
	followTickMsg struct{ f *follower }
	followMsg     struct {
		f  *follower
		u  live.Update
		ok bool
	}
)

// sourceKey identifies what a region reads, and where from.
func (m *Model) sourceKey(src sheet.LinkSource) string {
	return fmt.Sprintf("%s\x00%s\x00%s\x00%s\x00%d\x00%s", src.Path, src.Format, src.Table, src.Query, src.Window, m.filename)
}

// syncFollowers makes the followers match the workbook's regions:
// sources for new ones, none for those gone, a whole read for stale
// ones, and the next poll for each that follows.
func (m *Model) syncFollowers() tea.Cmd {
	regions := m.book().LinkedRegions()
	if len(regions) == 0 && len(m.follow.by) == 0 {
		return nil
	}
	if m.follow.by == nil {
		m.follow.by = map[int]*follower{}
	}
	seen := map[int]bool{}
	var cmds []tea.Cmd
	for _, r := range regions {
		seen[r.ID] = true
		if cmd := m.syncFollower(r); cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	for id, f := range m.follow.by {
		if !seen[id] {
			m.dropFollower(f)
		}
	}
	return tea.Batch(cmds...)
}

// syncFollower keeps r's follower in step with r.
func (m *Model) syncFollower(r sheet.LinkedRegion) tea.Cmd {
	f := m.follow.by[r.ID]
	if f != nil && f.key != m.sourceKey(r.Source) {
		m.dropFollower(f)
		f = nil
	}
	if f == nil {
		if !m.linkTrusted(r) {
			m.askLinkTrust()
			return nil
		}
		if f = m.newFollower(r); f == nil {
			return nil
		}
	}
	if r.Stale && !f.reload {
		f.reload = true
		if f.busy {
			f.pending = true
		} else {
			f.src.Reload()
		}
	}
	if f.busy || f.waiting || r.Paused {
		return nil
	}
	f.waiting = true
	if r.Stale {
		return func() tea.Msg { return followTickMsg{f} }
	}
	return tea.Tick(followInterval, func(_ time.Time) tea.Msg { return followTickMsg{f} })
}

// newFollower makes r's source, or shows why it can't.
func (m *Model) newFollower(r sheet.LinkedRegion) *follower {
	name := m.linkName(r.Source.Path)
	k, ok := live.KindOf(name, r.Source.Format)
	if !ok {
		m.linkFailed(r.ID, "012 doesn't read "+filepath.Ext(name)+" files")
		return nil
	}
	path, err := m.root.Resolve(name)
	if err != nil {
		m.linkFailed(r.ID, m.root.Scrub(err.Error()))
		return nil
	}
	src := live.NewFile(filepath.Base(name), path, k, fileio.Options{Table: r.Source.Table, Query: r.Source.Query, Locale: m.locale()})
	f := &follower{id: r.ID, key: m.sourceKey(r.Source), src: src, reload: true}
	m.follow.by[r.ID] = f
	return f
}

// linkFailed shows err in the region id, which can't be followed.
func (m *Model) linkFailed(id int, err string) {
	if r, ok := m.book().LinkedRegion(id); ok && r.Err == err {
		return
	}
	m.book().ApplyLive(sheet.LiveOp{Link: id, Err: err})
}

// dropFollower lets a follower go, now or when its poll returns.
func (m *Model) dropFollower(f *follower) {
	delete(m.follow.by, f.id)
	f.gone = true
	if !f.busy {
		f.src.Close()
	}
}

// pollFollower polls f's source on a goroutine of its own.
func pollFollower(f *follower) tea.Cmd {
	f.busy = true
	return func() tea.Msg {
		u, ok := f.src.Poll(context.Background())
		return followMsg{f, u, ok}
	}
}

// handleFollow handles a follower's tick and what its poll found.
func (m *Model) handleFollow(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case followTickMsg:
		f := msg.f
		f.waiting = false
		if m.follow.by[f.id] != f {
			f.src.Close() // of a workbook no longer open
			return nil
		}
		if r, ok := m.book().LinkedRegion(f.id); !ok || r.Paused {
			return nil
		}
		return pollFollower(f)
	case followMsg:
		f := msg.f
		f.busy = false
		switch {
		case f.gone || m.follow.by[f.id] != f:
			f.src.Close()
			return nil
		case f.pending:
			f.pending = false
			f.src.Reload() // asked while it polled: read it whole now
			return pollFollower(f)
		}
		if msg.ok {
			m.applyUpdate(f, msg.u)
		}
		if msg.u.More && !f.gone {
			return pollFollower(f)
		}
	}
	return nil
}

// applyUpdate applies what f's poll found to its region.
func (m *Model) applyUpdate(f *follower, u live.Update) {
	if u.Reset {
		f.reload = false
	}
	_, err := m.book().ApplyLive(u.Op(f.id))
	if errors.Is(err, sheet.ErrNoLinked) {
		m.dropFollower(f)
	}
}

// linkName is the name a region's path stands for, as the user would
// type it (relative to the working folder, or the served one).
func (m *Model) linkName(path string) string { return linkNameIn(path, m.filename) }

// linkNameIn is the name path stands for in the workbook named workbook:
// a relative path is relative to its folder.
func linkNameIn(path, workbook string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(filepath.Dir(workbook), path)
}

// linkPath is what a region keeps for the file typed as name, in the
// workbook named workbook: the path relative to the workbook's folder,
// or absolute when the name is and the file isn't in that folder or
// below.
func linkPath(name, workbook string) string {
	dir := filepath.Dir(workbook)
	if !filepath.IsAbs(name) && !filepath.IsAbs(dir) {
		if rel, err := filepath.Rel(dir, name); err == nil {
			return rel
		}
		return name
	}
	absName, err1 := filepath.Abs(name)
	absDir, err2 := filepath.Abs(dir)
	if err1 != nil || err2 != nil {
		return name
	}
	if rel, err := filepath.Rel(absDir, absName); err == nil && !outside(rel) {
		return rel
	}
	return absName
}

// rebaseLinks keeps the regions' paths naming the same files when the
// workbook named from is to be named to, perhaps in another folder.
func (m *Model) rebaseLinks(from, to string) {
	if filepath.Dir(from) == filepath.Dir(to) {
		return
	}
	m.book().RebaseLinks(func(p string) string { return linkPath(linkNameIn(p, from), to) })
}

// outside reports whether a region's path leaves its workbook's folder.
func outside(path string) bool {
	return filepath.IsAbs(path) || path == ".." || strings.HasPrefix(path, ".."+string(filepath.Separator))
}

// linkTrusted reports whether r may be followed without asking: its
// file is in the workbook's folder or below, or the workbook's links
// were made or trusted on this computer, or the user said so.
func (m *Model) linkTrusted(r sheet.LinkedRegion) bool {
	o := m.book().LinkOrigin()
	return !outside(r.Source.Path) || m.follow.trusted || o != "" && o == m.macros.machine
}

// askLinkTrust asks once whether to follow the files a workbook from
// elsewhere links outside its folder, when nothing else is being asked.
func (m *Model) askLinkTrust() {
	if m.follow.asked || m.mode != modeReady || m.overlay != nil {
		return
	}
	m.follow.asked = true
	var names []string
	for _, r := range m.book().LinkedRegions() {
		if !m.linkTrusted(r) {
			names = append(names, r.Source.Path)
		}
	}
	m.ask(question{
		msg:  "This spreadsheet, made on another computer, follows files outside its folder: " + strings.Join(names, ", ") + ".",
		warn: true,
		desc: "Following reads those files as they change; not following leaves their regions empty",
		choices: []choice{
			{key: "enter", label: "Follow them", run: func(m *Model) tea.Cmd { m.trustLinks(); return nil }},
			{key: "esc", label: "Don't follow", run: func(m *Model) tea.Cmd {
				for _, r := range m.book().LinkedRegions() {
					if !m.linkTrusted(r) {
						m.linkFailed(r.ID, "Not followed: the spreadsheet was made on another computer")
					}
				}
				return nil
			}},
		},
	})
}

// trustLinks trusts the workbook's links from now on, on this computer.
func (m *Model) trustLinks() {
	m.follow.trusted = true
	if m.macros.machine != "" {
		m.book().SetLinkOrigin(m.macros.machine)
	}
}
