package sheet

import (
	"errors"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Linked files: regions (region.go) whose rows come from a file the UI
// follows as it grows or is rewritten. Its anchor is its table's first
// cell, the file's first row, which stays; the rows under it are the
// file's, all of them up to max-cells, or the last Window of them. Its rows arrive only as live operations
// (live.go) and are never part of the undo state (see region.go for
// why), so a linked file is read again whenever its cells may no longer
// be the file's: undo put it back, it moved, or a cell blocking it was
// cleared.

// LinkSource is what a linked file reads, as the file keeps it.
type LinkSource struct {
	// Path is the file, as the UI resolves it: relative to the
	// workbook's folder when it can be (see RebaseLinks).
	Path string
	// Format is the name of the file's format ("CSV", "NUON"), or "" to
	// tell it by the extension.
	Format string
	// Table is a SQLite table to read, or Query a query to run.
	Table, Query string
	// Window keeps the last Window rows under the header, dropping older
	// ones, as tail -f does; 0 keeps every row, up to max-cells.
	Window int
}

// liveMeta is what the rows arriving have made of a linked file, kept
// beside its region and never undone.
type liveMeta struct {
	data, dropped int // rows arrived since the last reset, rows the window dropped
	updated       time.Time
	err, note     string
	paused        bool
	stale         bool // to be read again, whole
	reread        bool // to be emptied and read again once the change ends
	fitted        bool // its columns were widened to its text once
}

// LinkedRegion is a linked file as the UI shows it: where it is, what
// it reads, and how the reading goes.
type LinkedRegion struct {
	// Name names the region: in LiveOps, in formulas (nu.name) and to
	// commands ($name).
	Name   string
	Sheet  *Sheet
	Anchor Addr
	// Area is the cells it covers, the anchor alone while it has none.
	Area   Rect
	Source LinkSource
	// Rows counts the data rows it shows, under the header; Dropped the
	// older rows the window let go.
	Rows, Dropped int
	// Updated is when the file was last read, zero until it is.
	Updated time.Time
	// Err says why the file can't be read, or its rows can't be shown,
	// "" when all is well; Note says what was left out.
	Err, Note string
	// Paused is set while the region isn't following its file.
	Paused bool
	// Stale is set when its cells may no longer be the file's: the file
	// is to be read again, whole.
	Stale bool
}

// ErrLinkedEdit is what typing into a linked file's cell says.
var ErrLinkedEdit = errors.New("That cell shows part of a linked file: unlink it (Data > Linked file > Unlink) to edit it")

// linkedInfo describes the linked file r on s.
func (s *Sheet) linkedInfo(r Region) LinkedRegion {
	me := s.meta(nameKey(r.Name))
	err := me.err
	if me.why != "" {
		err = me.why
	}
	return LinkedRegion{Name: r.Name, Sheet: s, Anchor: r.At, Area: s.covered(r), Source: r.File,
		Rows: max(me.rows-1, 0), Dropped: me.dropped, Updated: me.updated, Err: err, Note: me.note,
		Paused: me.paused, Stale: me.stale || me.reread}
}

// LinkedAt returns the linked file covering the cell at a.
func (s *Sheet) LinkedAt(a Addr) (LinkedRegion, bool) {
	if r, ok := s.RegionAt(a); ok && r.Linked() {
		return s.linkedInfo(r), true
	}
	return LinkedRegion{}, false
}

// HasLinked reports whether the sheet holds a linked file.
func (s *Sheet) HasLinked() bool {
	for _, r := range s.regions.list {
		if r.Linked() {
			return true
		}
	}
	return false
}

// LinkedRegions returns the sheet's linked files, in the order they
// were made.
func (s *Sheet) LinkedRegions() []LinkedRegion {
	var out []LinkedRegion
	for _, r := range s.regions.list {
		if r.Linked() {
			out = append(out, s.linkedInfo(r))
		}
	}
	return out
}

// LinkedRegions returns every live sheet's linked files, sheet by sheet.
func (w *Workbook) LinkedRegions() []LinkedRegion {
	var out []LinkedRegion
	for _, s := range w.sheets {
		out = append(out, s.LinkedRegions()...)
	}
	return out
}

// LinkedRegion returns the linked file name names.
func (w *Workbook) LinkedRegion(name string) (LinkedRegion, bool) {
	if s, r, ok := w.Region(name); ok && r.Linked() {
		return s.linkedInfo(r), true
	}
	return LinkedRegion{}, false
}

// errLinkOver is why a linked file can't start where it's asked.
var errLinkOver = errors.New("A linked file needs an empty cell to start in")

// AddLinked makes a linked file at a reading src, named after the file,
// as one undo step, and returns its name. It starts empty and stale: the
// UI reads the file and sends its rows.
func (s *Sheet) AddLinked(a Addr, src LinkSource) (string, error) {
	name := s.wb.linkName(src.Path)
	if err := s.AddRegion(Region{Name: name, At: a, File: src}); err != nil {
		return "", err
	}
	return name, nil
}

// linkName is a free region name for the file at path: its name without
// the extension, made a nushell variable's name (app_log), numbered when
// taken (app_log_2).
func (w *Workbook) linkName(path string) string {
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	var b strings.Builder
	for i := range len(base) {
		if c := base[i]; isLetter(c) || isDigit(c) || c == '_' {
			b.WriteByte(c)
		} else if b.Len() > 0 && !strings.HasSuffix(b.String(), "_") {
			b.WriteByte('_')
		}
	}
	name := strings.Trim(b.String(), "_")
	if len(name) > 48 {
		name = name[:48]
	}
	if ValidRegionName(name) != nil {
		name = "file_" + name
	}
	for n := 2; w.checkRegionName(name, false) != nil; n++ {
		name = strings.TrimSuffix(name, "_"+strconv.Itoa(n-1)) + "_" + strconv.Itoa(n)
	}
	return name
}

// SetLinkSource changes what a linked file reads (its window, say), as
// one undo step; it is read again.
func (w *Workbook) SetLinkSource(name string, src LinkSource) error {
	s, r, ok := w.Region(name)
	if !ok || !r.Linked() {
		return ErrNoRegion
	}
	k := nameKey(r.Name)
	s.meta(k).reread = true
	s.change("change link "+r.Name, s.covered(r), func() {
		st := s.regionsCopy()
		st.list[s.regionIndex(k)].File = src
		s.putRegions(st)
	})
	return nil
}

// Unlink turns a linked file into the values it shows, as ordinary
// cells, as one undo step: FreezeRegion.
func (w *Workbook) Unlink(name string) error {
	s, r, ok := w.Region(name)
	if !ok || !r.Linked() {
		return ErrNoRegion
	}
	return s.FreezeRegion(r.Name)
}

// PauseLinked stops a linked file following its file, or has it follow
// again. It isn't an edit: what follows the file reads it.
func (w *Workbook) PauseLinked(name string, paused bool) {
	if s, r, ok := w.Region(name); ok && r.Linked() {
		s.meta(nameKey(r.Name)).paused = paused
	}
}

// ReloadLinked marks a linked file stale, so the UI reads it again,
// whole.
func (w *Workbook) ReloadLinked(name string) {
	if s, r, ok := w.Region(name); ok && r.Linked() {
		s.meta(nameKey(r.Name)).stale = true
	}
}

// RebaseLinks rewrites the path of every linked file with fn, as the UI
// does when the workbook is saved in another folder, in the undo
// history too, so undo puts back regions reading the same files. It
// isn't an edit: the regions read the same files.
func (w *Workbook) RebaseLinks(fn func(string) string) {
	seen := map[*Region]bool{}
	rebase := func(list []Region) {
		if len(list) == 0 || seen[&list[0]] {
			return // lists share their arrays with the steps that kept them
		}
		seen[&list[0]] = true
		for i, r := range list {
			if r.Linked() {
				list[i].File.Path = fn(r.File.Path)
			}
		}
	}
	for _, s := range w.sheets {
		rebase(s.regions.list)
	}
	for _, st := range append(w.hist.undo, w.hist.redo...) {
		for _, rs := range st.regions {
			rebase(rs.list)
		}
	}
}
