package ui

import (
	"context"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/fileio"
	"github.com/FelineStateMachine/012/internal/live"
	"github.com/FelineStateMachine/012/internal/paged"
	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/ui/srcview"
)

// Linked sources (Data > Linked file > Link a source): a Parquet file
// or a SQLite table or query read in place on a tab of its own. The
// workbook's paged.Host answers what formulas and pivot tables ask of
// the sources, its jobs run on goroutines of their own, a few at a
// time, and their answers come back as messages that recalculate what
// waited for them. The tab shown reads its rows a page at a time
// (paged.Pages) as it scrolls. Which sources the host reads is
// reconciled with the workbook after every update (syncSources), as
// linked files are (follow.go), and their files are looked at every
// live.Interval, a source read again once its file has changed.
//
// In a room of 012 serve the host is the room's, as its linked files
// and notebook runs are (share.go): whoever keeps the room links the
// sources, runs the jobs and looks at the files, and the jobs' answers
// reach whoever keeps it when they're done (roomOwned). Every
// participant reads the pages of the tab it shows itself.

// sourceHost is a workbook's host and the jobs it has queued: the
// model's own, or the room's.
type sourceHost struct {
	host    *paged.Host
	links   string            // the links given to the host, to notice a change
	failed  map[string]string // why a source can't be linked, as told to the workbook
	queue   []*paged.Job      // the host's jobs waiting for a turn
	running int
}

// sourceState is the linked sources' part of the model.
type sourceState struct {
	run     *sourceHost
	pages   map[string]*paged.Pages        // the rows of each source's tab, by the source's name
	views   map[*sheet.Sheet]*srcview.View // each source tab's view
	ticking bool
	dragBar bool // the scrollbar's thumb is being dragged
}

// sourceInterval is how often sources' files are looked at; tests lower
// it.
var sourceInterval = live.Interval

// sourceParallel is how many of the host's jobs run at once: each may
// read a whole source.
const sourceParallel = 3

type (
	sourceJobMsg struct {
		host *paged.Host
		j    *paged.Job
	}
	sourcePageMsg struct {
		p *paged.Pages
		j *paged.PageJob
	}
	sourceTickMsg struct{ host *paged.Host }
)

// sources is the workbook's host and jobs.
func (m *Model) sources() *sourceHost {
	if m.src.run == nil {
		m.src.run = &sourceHost{}
	}
	return m.src.run
}

// keepsSources reports whether this model runs the host's jobs: its own
// workbook's, or a room's it keeps.
func (m *Model) keepsSources() bool {
	seat := m.share.seat
	return seat == nil || seat.Keeper()
}

// closeSources lets the workbook's sources go, as another is opened:
// the host too, unless others share it.
func (m *Model) closeSources() {
	if r := m.src.run; r != nil && r.host != nil && !m.shared() {
		r.host.Close()
	}
	for _, p := range m.src.pages {
		p.Close()
	}
}

// syncSources gives the host the workbook's sources, keeps the pages of
// the source shown, and runs what's queued.
func (m *Model) syncSources() tea.Cmd {
	w, r := m.book(), m.sources()
	infos := w.Sources()
	if len(infos) == 0 && r.host == nil {
		return nil
	}
	if r.host == nil {
		r.host = paged.NewHost(sheet.MaxCells())
		w.SetSources(r.host)
	}
	keeper := m.keepsSources()
	if keeper {
		m.linkSources(infos)
	}
	m.syncPages()
	if v := m.srcView(); v != nil {
		v.Prefetch()
	}
	cmds := []tea.Cmd{m.runPageJobs()}
	if keeper {
		cmds = append(cmds, m.runSourceJobs())
		if !m.src.ticking && len(infos) > 0 {
			m.src.ticking = true
			h := r.host
			cmds = append(cmds, tea.Tick(sourceInterval, func(time.Time) tea.Msg { return sourceTickMsg{h} }))
		}
	}
	return tea.Batch(cmds...)
}

// linkSources gives the host the sources it may read, their files
// resolved, and tells the workbook why the others can't be.
func (m *Model) linkSources(infos []sheet.SourceInfo) {
	r := m.sources()
	var links []paged.Linked
	var key strings.Builder
	for _, info := range infos {
		src := info.Source
		path, why := m.sourcePath(src)
		if why != "" {
			if r.failed[info.Name] != why {
				if r.failed == nil {
					r.failed = map[string]string{}
				}
				r.failed[info.Name] = why
				m.book().SetSourceShape(info.Name, sheet.SourceShape{}, why)
			}
			continue
		}
		delete(r.failed, info.Name)
		spec := fileio.SourceSpec{Path: path, Format: src.Format, Table: src.Table, Query: src.Query}
		links = append(links, paged.Linked{Name: info.Name, Spec: spec})
		key.WriteString(info.Name + "\x00" + path + "\x00" + src.Format + "\x00" + src.Table + "\x00" + src.Query + "\x01")
	}
	if key.String() != r.links {
		r.links = key.String()
		r.host.Link(links)
	}
}

// sourcePath is the file a source reads, or why it may not be read:
// outside the served folder, or outside the workbook's from a workbook
// made on another computer, until the user trusts it (follow.go).
func (m *Model) sourcePath(src sheet.LinkSource) (string, string) {
	if outside(src.Path) && !m.macroTrusted() {
		m.askLinkTrust()
		return "", "Not read: the spreadsheet was made on another computer"
	}
	path, err := m.root.Resolve(m.linkName(src.Path))
	if err != nil {
		return "", m.root.Scrub(err.Error())
	}
	return path, ""
}

// runSourceJobs starts the host's jobs, a few at a time.
func (m *Model) runSourceJobs() tea.Cmd {
	r := m.sources()
	h := r.host
	r.queue = append(r.queue, h.Jobs()...)
	var cmds []tea.Cmd
	for r.running < sourceParallel && len(r.queue) > 0 {
		j := r.queue[0]
		r.queue = r.queue[1:]
		r.running++
		parent := m.spans.Parent()
		cmds = append(cmds, m.roomOwned(func() tea.Msg {
			span := parent.Start("source " + j.String())
			j.Run(context.Background())
			span.End()
			return sourceJobMsg{h, j}
		}))
	}
	return tea.Batch(cmds...)
}

// runPageJobs reads the pages the tab shown asked for.
func (m *Model) runPageJobs() tea.Cmd {
	var cmds []tea.Cmd
	for _, p := range m.src.pages {
		for _, j := range p.Jobs() {
			cmds = append(cmds, func() tea.Msg {
				j.Run(context.Background())
				return sourcePageMsg{p, j}
			})
		}
	}
	return tea.Batch(cmds...)
}

// handleSource takes what a job found, a page read, or a tick.
func (m *Model) handleSource(msg tea.Msg) tea.Cmd {
	r := m.sources()
	switch msg := msg.(type) {
	case sourceJobMsg:
		if msg.host != r.host {
			return nil // the workbook it was for is gone
		}
		r.running--
		paged.Apply(m.book(), r.host.Store(msg.j))
	case sourcePageMsg:
		if m.src.pages[pagesName(m.src.pages, msg.p)] == msg.p {
			msg.p.Store(msg.j)
		}
	case sourceTickMsg:
		m.src.ticking = false
		if msg.host == r.host && m.keepsSources() {
			r.host.Poll()
		}
	}
	return nil
}

// pagesName is the name p is kept by, "" when it isn't kept.
func pagesName(pages map[string]*paged.Pages, p *paged.Pages) string {
	for name, q := range pages {
		if q == p {
			return name
		}
	}
	return ""
}

// syncPages keeps the pages of the source tab shown in step with its
// source: made once it's open, made again when it's read again or its
// order changes; those of other tabs let go.
func (m *Model) syncPages() {
	info, ok := m.sheet.Source()
	for name, p := range m.src.pages {
		if !ok || !strings.EqualFold(name, info.Name) {
			p.Close()
			delete(m.src.pages, name)
		}
	}
	if !ok {
		return
	}
	h, gen, open := m.sources().host.Source(info.Name)
	p := m.src.pages[info.Name]
	switch {
	case !open:
		if p != nil {
			p.Close()
			delete(m.src.pages, info.Name)
		}
	case p == nil || !p.Same(h, gen, info.Source.Order):
		reordered := p != nil && p.Of(h, gen)
		if p != nil {
			p.Close()
		}
		if m.src.pages == nil {
			m.src.pages = map[string]*paged.Pages{}
		}
		m.src.pages[info.Name] = paged.NewPages(h, gen, info.Source.Order)
		if v := m.src.views[m.sheet]; v != nil {
			if reordered {
				v.Reset() // other rows in other places: back to the first
			}
			v.Refit()
		}
	}
}
