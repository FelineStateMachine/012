// Package findbar is find and replace: a bar on the context line rather
// than a dialog, in the spirit of less and vim. Matches highlight in the
// grid as you type and the active cell follows the current one. Options
// are toggles shown as chips and switched with Alt keys, as in VS Code's
// find widget. The scope chip says where to search, as Sheets' "Search"
// choice: this sheet, all sheets, or the range selected when the bar
// opened; Alt+S goes through them. It knows the UI only through Host.
package findbar

import (
	"log/slog"
	"slices"
	"strconv"

	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/telemetry"
	"github.com/FelineStateMachine/012/internal/ui/lineedit"
	"github.com/FelineStateMachine/012/internal/ui/theme"
)

// Host is what the find bar needs of the UI.
type Host interface {
	Theme() *theme.Theme
	Size() (width, height int)
	// Line is the shared edit line the focused field is typed in.
	Line() *lineedit.Line
	Book() *sheet.Workbook
	// At is the sheet shown and its active cell.
	At() (*sheet.Sheet, sheet.Addr)
	// Show shows sheet s with a as the active cell.
	Show(s *sheet.Sheet, a sheet.Addr)
	// Note says how the last action went, on the context line.
	Note(msg string)
	// Edited marks the file modified.
	Edited()
	// Leave closes the bar, keeping its search for Ctrl+F and find next.
	Leave()
	// Press handles a mouse press outside the bar, as on the grid.
	Press(x, y int, button tea.MouseButton) tea.Cmd
}

// Bar is the find bar: the search, its options and its matches. It
// outlives being open, so the last search can be reopened or repeated.
type Bar struct {
	h       Host
	replace bool // Ctrl+H: a replacement field follows the query
	field   int  // 0 is the query, 1 the replacement
	fields  [2]string
	opts    sheet.FindOptions
	scope   *sheet.Rect  // the selection when the bar opened, if any
	home    *sheet.Sheet // the sheet scope is on
	where   scope

	matches []Match
	index   map[Match]int
	cur     int    // index of the current match, -1 when none
	err     string // e.g. an invalid regular expression
	from    Match  // where the search goes on from

	// Vim makes Enter close the bar on the match, as / does in vim.
	Vim bool
}

// scope is where the find bar searches.
type scope int

const (
	inSheet scope = iota // the sheet shown
	inAll                // every sheet, in tab order
	inRange              // the range selected when the bar opened
)

// Match is a cell on a particular sheet.
type Match struct {
	Sheet *sheet.Sheet
	At    sheet.Addr
}

// New returns a bar with no search yet.
func New(h Host) *Bar { return &Bar{h: h, cur: -1} }

// Matches are the matches of the search, in order.
func (f *Bar) Matches() []Match { return f.matches }

// Options are the search's options.
func (f *Bar) Options() sheet.FindOptions { return f.opts }

// Replacing reports whether the replacement field shows, and Field which
// field is focused: 0 the query, 1 the replacement.
func (f *Bar) Replacing() bool { return f.replace }
func (f *Bar) Field() int      { return f.field }

// Found reports whether a on s is a match.
func (f *Bar) Found(s *sheet.Sheet, a sheet.Addr) bool {
	_, hit := f.index[Match{s, a}]
	return hit
}

// Again goes to the next (d = 1) or previous (d = -1) match of the last
// search from the active cell, without opening the bar, as n and N do in
// vim and less, and says where it went. It does nothing without a search
// to repeat (see Searched).
func (f *Bar) Again(d int) {
	if f.fields[0] == "" {
		return
	}
	if f.where == inRange && !f.home.Live() {
		f.where = inSheet
	}
	s, a := f.h.At()
	here := Match{s, a}
	f.from = here
	f.search() // lands on the first match at or after here
	switch {
	case f.err != "":
		f.h.Note(f.err)
		return
	case len(f.matches) == 0:
		f.h.Note("No matches for " + f.fields[0])
		return
	case d > 0 && f.matches[f.cur] == here, d < 0:
		f.step(d)
	}
	f.h.Note("Match " + strconv.Itoa(f.cur+1) + " of " + strconv.Itoa(len(f.matches)) + " for " + f.fields[0])
}

// Searched reports whether there's a search to repeat.
func (f *Bar) Searched() bool { return f.fields[0] != "" }

// Reset gets the bar ready to open over the sheet shown, keeping the last
// search. A multi-cell selection sel becomes the search scope, as Sheets'
// "specific range". The kept query is loaded into the edit line.
func (f *Bar) Reset(sel *sheet.Rect) {
	s, a := f.h.At()
	f.scope, f.home = nil, s
	if f.where == inRange || f.where == inAll && len(f.h.Book().Visible()) == 1 {
		f.where = inSheet
	}
	if sel != nil {
		f.scope, f.where = sel, inRange
	}
	f.from = Match{s, a}
	// Load the kept query so focus doesn't overwrite it.
	f.h.Line().Buf = []rune(f.fields[f.field])
}

// Open focuses the query, or with replace the replacement once there's
// a query, and searches.
func (f *Bar) Open(replace bool) {
	f.replace = f.replace || replace
	if replace && f.fields[0] != "" {
		f.focus(1)
	} else {
		f.focus(0)
	}
	f.search()
}

func (f *Bar) Indicator() string { return "FIND" }

// focus moves editing to field i, keeping the other field's text.
func (f *Bar) focus(i int) {
	line := f.h.Line()
	f.fields[f.field] = line.Text()
	f.field = i
	line.Set(f.fields[i])
}

// Changed re-runs the search as the query is typed.
func (f *Bar) Changed() {
	f.fields[f.field] = f.h.Line().Text()
	if f.field == 0 {
		f.search()
	}
}

// options returns the search options, with the scope applied.
func (f *Bar) options() sheet.FindOptions {
	o := f.opts
	o.Within = nil
	if f.where == inRange {
		o.Within = f.scope
	}
	return o
}

// sheets are the sheets searched, in order.
func (f *Bar) sheets() []*sheet.Sheet {
	switch f.where {
	case inAll:
		return f.h.Book().Visible() // a match on a hidden sheet couldn't be shown
	case inRange:
		if f.home.Live() {
			return []*sheet.Sheet{f.home}
		}
	}
	s, _ := f.h.At()
	return []*sheet.Sheet{s}
}

// search finds all matches and makes the first one at or after where the
// search started the current one, so the view doesn't jump backwards
// while typing.
func (f *Bar) search() {
	f.err, f.matches, f.index, f.cur = "", nil, nil, -1
	span := telemetry.Start("find")
	sheets := f.sheets()
	for _, s := range sheets {
		found, err := s.Find(f.fields[0], f.options())
		if err != nil {
			span.End(slog.Int("matches", 0))
			f.err = "Invalid regular expression"
			return
		}
		for _, a := range found {
			f.matches = append(f.matches, Match{s, a})
		}
	}
	span.End(slog.Int("matches", len(f.matches)), slog.Int("sheets", len(sheets)))
	f.index = make(map[Match]int, len(f.matches))
	for i, c := range f.matches {
		f.index[c] = i
	}
	if len(f.matches) == 0 {
		return
	}
	order := func(s *sheet.Sheet) int { return slices.Index(sheets, s) }
	f.cur = 0
	for i, c := range f.matches {
		if d := order(c.Sheet) - order(f.from.Sheet); d > 0 || d == 0 && (c.At.Row > f.from.At.Row || c.At.Row == f.from.At.Row && c.At.Col >= f.from.At.Col) {
			f.cur = i
			break
		}
	}
	f.show()
}

// show makes the current match the active cell, on its sheet.
func (f *Bar) show() {
	c := f.matches[f.cur]
	f.h.Show(c.Sheet, c.At)
}

// step moves to the next (+1) or previous (-1) match, wrapping around.
func (f *Bar) step(d int) {
	if len(f.matches) == 0 {
		return
	}
	f.cur = (f.cur + d + len(f.matches)) % len(f.matches)
	f.show()
	f.from = f.matches[f.cur]
}

// nextScope is where Alt+S goes from where: the range (if there is one),
// the sheet, then all sheets (if there are several).
func (f *Bar) nextScope() scope {
	order := []scope{inSheet}
	if len(f.h.Book().Visible()) > 1 {
		order = append(order, inAll)
	}
	if f.scope != nil {
		order = append([]scope{inRange}, order...)
	}
	return order[(slices.Index(order, f.where)+1)%len(order)]
}

func (f *Bar) Key(k tea.KeyPressMsg) tea.Cmd {
	switch k.String() {
	case "esc":
		f.h.Leave()
	case "enter":
		if f.Vim && !f.replace {
			// As / in vim: Enter stays on the match; n and N go on.
			f.h.Leave()
			return nil
		}
		if f.replace && f.field == 1 {
			f.replaceOne()
		} else {
			f.step(1)
		}
	case "shift+enter", "up":
		f.step(-1)
	case "down":
		f.step(1)
	case "ctrl+enter", "alt+a":
		if f.replace {
			f.replaceAll()
		}
	case "tab", "shift+tab":
		if f.replace {
			f.focus(1 - f.field)
		}
	case "ctrl+f":
		f.focus(0)
	case "ctrl+h":
		f.replace = true
		f.focus(1)
	default:
		f.optionKey(k)
	}
	return nil
}

// optionKey flips an option or the scope, or else edits the focused
// field.
func (f *Bar) optionKey(k tea.KeyPressMsg) {
	switch k.String() {
	case "alt+c":
		f.opts.MatchCase = !f.opts.MatchCase
	case "alt+w":
		f.opts.WholeCell = !f.opts.WholeCell
	case "alt+r":
		f.opts.Regex = !f.opts.Regex
	case "alt+=":
		f.opts.InFormulas = !f.opts.InFormulas
	case "alt+s":
		next := f.nextScope()
		if next == f.where {
			return
		}
		f.where = next
		s, a := f.h.At()
		f.from = Match{s, a}
	default:
		line := f.h.Line()
		before := line.Text()
		line.Key(k)
		if line.Text() != before {
			f.Changed()
		}
		return
	}
	f.search()
}

// replaceOne replaces the current match and moves to the next one.
func (f *Bar) replaceOne() {
	if f.cur < 0 {
		return
	}
	c := f.matches[f.cur]
	a := c.At
	changed, err := c.Sheet.Replace(a, f.fields[0], f.fields[1], f.options())
	switch {
	case err != nil:
		f.h.Note("Can't replace in " + a.String() + ": " + err.Error())
		return
	case !changed:
		f.h.Note("Formulas in " + a.String() + " change only when searching within formulas")
	default:
		f.h.Edited()
	}
	f.from = c
	f.search()
	if changed && f.cur >= 0 && f.matches[f.cur] == c {
		f.step(1) // the cell still matches; move past it
	}
}

func (f *Bar) replaceAll() {
	// Across sheets, still one undo step.
	span := telemetry.Start("replace")
	n := 0
	s, _ := f.h.At()
	err := f.h.Book().Batch(sheet.Change{Label: "replace all", Sheet: s}, func() error {
		for _, s := range f.sheets() {
			k, err := s.ReplaceAll(f.fields[0], f.fields[1], f.options())
			n += k
			if err != nil {
				return err
			}
		}
		return nil
	})
	span.End(slog.Int("replaced", n))
	if n > 0 {
		f.h.Edited()
	}
	switch {
	case err != nil:
		f.h.Note("Replaced " + cellCount(n) + ", then stopped: " + err.Error())
	case n == 0:
		f.h.Note("Nothing to replace")
	default:
		f.h.Note("Replaced " + cellCount(n))
	}
	f.search()
}

// cellCount is n cells in words: "1 cell", "3 cells".
func cellCount(n int) string {
	if n == 1 {
		return "1 cell"
	}
	return strconv.Itoa(n) + " cells"
}
