package sheet

import (
	"fmt"
	"slices"

	"github.com/FelineStateMachine/012/internal/formula"
)

// Protected ranges and sheets follow Sheets' Data > Protect sheets and
// ranges in its "Show a warning when editing this range" mode: nothing is
// locked, but an edit that touches one asks first. The engine only keeps
// them (saved with the sheet, following inserted and deleted rows and
// columns, every change an undo step); the UI asks.

// Protection is a protected range, or the whole sheet.
type Protection struct {
	Range Rect   // the whole grid when Sheet is set
	Sheet bool   // the whole sheet is protected
	Desc  string // what the user wrote to describe it, may be ""
}

// Label names the protection as the UI shows it: its description, or its
// range, or "the sheet".
func (p Protection) Label() string {
	switch {
	case p.Desc != "":
		return p.Desc
	case p.Sheet:
		return "the sheet"
	}
	return p.Range.String()
}

// Protections returns the sheet's protected ranges, in the order added.
func (s *Sheet) Protections() []Protection { return slices.Clone(s.view.protected) }

// Protecting returns the first protection r overlaps.
func (s *Sheet) Protecting(r Rect) (Protection, bool) {
	for _, p := range s.view.protected {
		if p.Sheet || overlaps(p.Range, r) {
			return p, true
		}
	}
	return Protection{}, false
}

// Protect adds a protected range, or protects the whole sheet, as one
// undo step. Protecting the sheet again replaces its description.
func (s *Sheet) Protect(p Protection) {
	if p.Sheet {
		p.Range = Rect{To: Addr{Col: MaxCols - 1, Row: MaxRows - 1}}
	}
	next := slices.Clone(s.view.protected)
	label := "protect " + p.Range.String()
	if p.Sheet {
		label = "protect sheet " + s.name
		next = slices.DeleteFunc(next, func(q Protection) bool { return q.Sheet })
	}
	s.setProtected(label, p.Range, append(next, p))
}

// Unprotect removes protection i (in Protections' order).
func (s *Sheet) Unprotect(i int) {
	if i < 0 || i >= len(s.view.protected) {
		return
	}
	p := s.view.protected[i]
	next := slices.Delete(slices.Clone(s.view.protected), i, i+1)
	s.setProtected("remove protection of "+p.Label(), p.Range, next)
}

// UnprotectRange removes every protection r overlaps, the sheet's
// included, as one undo step, and returns how many there were.
func (s *Sheet) UnprotectRange(r Rect) int {
	next := slices.DeleteFunc(slices.Clone(s.view.protected), func(p Protection) bool {
		return p.Sheet || overlaps(p.Range, r)
	})
	n := len(s.view.protected) - len(next)
	if n > 0 {
		s.setProtected("remove protection", r, next)
	}
	return n
}

func (s *Sheet) setProtected(label string, focus Rect, next []Protection) {
	if len(next) == 0 {
		next = nil
	}
	s.change(label, focus, func() {
		s.recordView()
		s.view.protected = next
	})
}

// LoadProtection adds a protection as a loader does: without recording
// undo.
func (s *Sheet) LoadProtection(p Protection) {
	if p.Sheet {
		p.Range = Rect{To: Addr{Col: MaxCols - 1, Row: MaxRows - 1}}
	}
	s.view.protected = append(slices.Clone(s.view.protected), p)
}

// shiftProtected moves protected ranges with inserted or deleted rows or
// columns, as a range reference moves; one whose cells are all deleted
// goes.
func shiftProtected(ps []Protection, rows bool, sp formula.Span) []Protection {
	if len(ps) == 0 {
		return ps
	}
	_, rng := formula.AxisMaps(rows, sp)
	var out []Protection
	for _, p := range ps {
		if !p.Sheet {
			r, ok := rng(p.Range)
			if !ok {
				continue
			}
			p.Range = r
		}
		out = append(out, p)
	}
	return out
}

// fileProtection is a protection in the file: {"range": "B2:C9",
// "description": "Totals"}, or {"sheet": true}. Earlier builds ignore
// the field, so it needs no version bump; they simply don't warn.
type fileProtection struct {
	Range string `json:"range,omitempty"`
	Sheet bool   `json:"sheet,omitempty"`
	Desc  string `json:"description,omitempty"`
}

func encodeProtections(ps []Protection) []fileProtection {
	var out []fileProtection
	for _, p := range ps {
		fp := fileProtection{Sheet: p.Sheet, Desc: p.Desc}
		if !p.Sheet {
			fp.Range = p.Range.String()
		}
		out = append(out, fp)
	}
	return out
}

func decodeProtections(fps []fileProtection) ([]Protection, error) {
	var out []Protection
	for _, fp := range fps {
		p := Protection{Sheet: fp.Sheet, Desc: fp.Desc}
		if p.Sheet {
			p.Range = Rect{To: Addr{Col: MaxCols - 1, Row: MaxRows - 1}}
		} else {
			r, ok := ParseRange(fp.Range)
			if !ok {
				return nil, fmt.Errorf("invalid protected range %q", fp.Range)
			}
			p.Range = r
		}
		out = append(out, p)
	}
	return out, nil
}
