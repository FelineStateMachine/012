// Package sortbar is the bar that picks the columns and order to sort a
// range by, as Sheets' "Advanced range sorting options" dialog, on the
// context line. It shows each sort column as a chip; arrows change the
// focused one, and the model highlights the range being sorted with the
// sort column's header lit. It knows the UI only through Host.
package sortbar

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/ui/lineedit"
	"github.com/FelineStateMachine/012/internal/ui/overlay"
	"github.com/FelineStateMachine/012/internal/ui/theme"
)

// Host is what the sort bar needs of the UI.
type Host interface {
	Theme() *theme.Theme
	Size() (width, height int)
	// Sheet is the sheet sorted, for the headers' text.
	Sheet() *sheet.Sheet
	// Close closes the open overlay.
	Close()
	// Sort sorts data by keys, after the bar has closed. answer is the
	// bar's choices as a macro records them, the answer Answer takes.
	Sort(data sheet.Rect, keys []sheet.SortKey, answer string) tea.Cmd
}

// Bar is the sort bar over a range.
type Bar struct {
	h       Host
	rng     sheet.Rect // the whole range, header rows included
	headers int        // header rows at the top of rng; 0 when switched off
	guess   int        // header rows to use when switched on
	keys    []sheet.SortKey
	cur     int // the key being changed
}

// New returns a bar sorting rng, whose first headers rows are headers,
// by column col.
func New(h Host, rng sheet.Rect, headers, col int) *Bar {
	b := &Bar{h: h, rng: rng, headers: headers, guess: max(headers, 1)}
	b.keys = []sheet.SortKey{{Col: min(max(col, rng.From.Col), rng.To.Col)}}
	return b
}

// Data is the range the sort moves: the range without its headers.
func (b *Bar) Data() sheet.Rect {
	r := b.rng
	r.From.Row = min(r.From.Row+b.headers, r.To.Row)
	return r
}

// Active is the cell drawn as active while the bar is open: the top of
// the focused sort column, so its header lights up.
func (b *Bar) Active() sheet.Addr {
	return sheet.Addr{Col: b.keys[b.cur].Col, Row: b.Data().From.Row}
}

func (b *Bar) Indicator() string     { return "SORT" }
func (b *Bar) Layout() []overlay.Box { return nil }

// setCol points the focused key at column c, if it's in the range.
func (b *Bar) setCol(c int) {
	if c >= b.rng.From.Col && c <= b.rng.To.Col {
		b.keys[b.cur].Col = c
	}
}

// add adds a sort column after the focused one: the next column not yet
// sorted by.
func (b *Bar) add() {
	used := map[int]bool{}
	for _, k := range b.keys {
		used[k.Col] = true
	}
	for c := b.rng.From.Col; c <= b.rng.To.Col; c++ {
		if !used[c] {
			b.keys = append(b.keys[:b.cur+1], append([]sheet.SortKey{{Col: c}}, b.keys[b.cur+1:]...)...)
			b.cur++
			return
		}
	}
}

// toggleHeader switches the header rows off, or on at the guess.
func (b *Bar) toggleHeader() {
	if b.headers > 0 {
		b.headers = 0
	} else {
		b.headers = min(b.guess, b.rng.To.Row-b.rng.From.Row)
	}
}

// sort closes the bar and sorts.
func (b *Bar) sort() tea.Cmd {
	keys, r := b.keys, b.Data()
	b.h.Close()
	return b.h.Sort(r, keys, b.answer())
}

func (b *Bar) Key(k tea.KeyPressMsg) tea.Cmd {
	switch k.String() {
	case "esc":
		b.h.Close()
	case "enter":
		return b.sort()
	case "left":
		b.setCol(b.keys[b.cur].Col - 1)
	case "right":
		b.setCol(b.keys[b.cur].Col + 1)
	case "up", "down", "space":
		b.keys[b.cur].Desc = !b.keys[b.cur].Desc
	case "tab":
		b.cur = (b.cur + 1) % len(b.keys)
	case "shift+tab":
		b.cur = (b.cur + len(b.keys) - 1) % len(b.keys)
	case "alt+a":
		b.add()
	case "alt+h":
		b.toggleHeader()
	case "backspace", "delete":
		if len(b.keys) > 1 {
			b.keys = append(b.keys[:b.cur], b.keys[b.cur+1:]...)
			b.cur = min(b.cur, len(b.keys)-1)
		}
	default:
		// Typing a column's letter sorts by it.
		if t := lineedit.Typed(k); len(t) == 1 {
			if c, ok := sheet.ParseCol(t); ok {
				b.setCol(c)
			}
		}
	}
	return nil
}

// OrderName is a sort order as the bar and notes show it.
func OrderName(desc bool) string {
	if desc {
		return "Z→A"
	}
	return "A→Z"
}

// part is a piece of the bar: plain text, a sort key's chip, or the
// header row toggle.
type part struct {
	text   string
	key    int // index of the sort key, -1 for others
	toggle bool
}

func (b *Bar) parts() []part {
	th := b.h.Theme()
	parts := []part{{text: th.Key.Render("Sort "+b.Data().String()) + th.Muted.Render(" by "), key: -1}}
	for i, k := range b.keys {
		if i > 0 {
			parts = append(parts, part{text: th.Muted.Render(" then "), key: -1})
		}
		label := sheet.ColName(k.Col)
		if b.headers > 0 {
			if h := b.h.Sheet().ShownText(sheet.Addr{Col: k.Col, Row: b.rng.From.Row + b.headers - 1}); h != "" {
				label += " " + ansi.Truncate(h, 16, "…")
			}
		}
		style := th.KeyChip
		if i == b.cur {
			style = th.MenuSelected
		}
		parts = append(parts, part{text: style.Render(" " + label + "  " + OrderName(k.Desc) + " "), key: i})
	}
	style := th.Muted
	if b.headers > 0 {
		style = th.MenuSelected
	}
	return append(parts, part{text: "   ", key: -1}, part{text: style.Render(" Header row "), key: -1, toggle: true})
}

// ContextLine renders the bar for the context line.
func (b *Bar) ContextLine() (string, string) {
	var s strings.Builder
	for _, p := range b.parts() {
		s.WriteString(p.text)
	}
	return s.String(), ""
}

// Mouse focuses a key's chip (a second click flips its order) or flips
// the header row toggle; a click anywhere else cancels.
func (b *Bar) Mouse(e overlay.MouseEvent) tea.Cmd {
	if e.Kind != overlay.MousePress {
		return nil
	}
	if e.Y != overlay.ContextLine {
		b.h.Close()
		return nil
	}
	x := 0
	for _, p := range b.parts() {
		w := ansi.StringWidth(p.text)
		if e.X >= x && e.X < x+w {
			switch {
			case p.toggle:
				b.toggleHeader()
			case p.key == b.cur:
				b.keys[b.cur].Desc = !b.keys[b.cur].Desc
			case p.key >= 0:
				b.cur = p.key
			}
			return nil
		}
		x += w
	}
	return nil
}

// Status shows the keys, the most useful ones first on narrow screens.
func (b *Bar) Status() (string, string) {
	th := b.h.Theme()
	width, _ := b.h.Size()
	pairs := []string{"Left/Right", "column", "Space", "order", "Enter", "sort", "Esc", "cancel"}
	desc := "Alt+A add  Alt+H header  Tab next"
	for {
		keys := th.KeyHints(pairs...)
		switch {
		case ansi.StringWidth(desc)+3+ansi.StringWidth(keys) <= width:
			return th.Muted.Render(desc), keys
		case desc != "":
			desc = ""
		case len(pairs) > 4:
			pairs = pairs[2:]
		default:
			return "", keys
		}
	}
}

// answerJSON is the bar's choices as a macro answers them:
//
//	{"by": [{"column": "B"}, {"column": "C", "order": "desc"}], "header": true}
//
// the sort columns in order, as a pivot's rows are written, and whether
// the first rows are headers left in place. Without "header", the bar's
// own guess stands.
type answerJSON struct {
	By     []answerKey `json:"by"`
	Header *bool       `json:"header,omitempty"`
}

type answerKey struct {
	Column string `json:"column"`
	Order  string `json:"order,omitempty"` // "desc" for Z to A
}

// answer is the bar's choices as a macro records them.
func (b *Bar) answer() string {
	a := answerJSON{Header: new(bool)}
	*a.Header = b.headers > 0
	for _, k := range b.keys {
		ak := answerKey{Column: sheet.ColName(k.Col)}
		if k.Desc {
			ak.Order = "desc"
		}
		a.By = append(a.By, ak)
	}
	raw, _ := json.Marshal(a)
	return string(raw)
}

// Answer makes the choices a macro recorded (see answerJSON) and sorts,
// as if they were picked and Enter pressed.
func (b *Bar) Answer(text string) (tea.Cmd, error) {
	var a answerJSON
	if err := json.Unmarshal([]byte(text), &a); err != nil {
		return nil, fmt.Errorf("the sort's answer: %w", err)
	}
	if len(a.By) == 0 {
		return nil, errors.New(`the sort's answer names no columns: give "by"`)
	}
	var keys []sheet.SortKey
	for _, k := range a.By {
		c, ok := sheet.ParseCol(strings.ToUpper(k.Column))
		if !ok || c < b.rng.From.Col || c > b.rng.To.Col {
			return nil, fmt.Errorf("column %q isn't in %s", k.Column, b.rng)
		}
		if k.Order != "" && k.Order != "asc" && k.Order != "desc" {
			return nil, fmt.Errorf("the order of %s is %q: asc or desc", k.Column, k.Order)
		}
		keys = append(keys, sheet.SortKey{Col: c, Desc: k.Order == "desc"})
	}
	b.keys, b.cur = keys, 0
	if a.Header != nil && *a.Header != (b.headers > 0) {
		b.toggleHeader()
	}
	return b.sort(), nil
}
