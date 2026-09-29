package nbview

import (
	"context"
	"slices"

	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/notebook"
)

// Highlighting: what the highlighter said of each source, kept as a
// role per rune. A slow highlighter (nu) is asked in the background:
// about the cells drawn, as they're drawn, and about the cell being
// edited once typing pauses, never per key. Until it answers, the code
// shows its draft (the tokenizer's roles), and the cell being edited
// keeps the answer about its last text where the text is unchanged, so
// nothing flickers between keys.

// syntaxCache keeps what the highlighter said of each source, as a role
// per rune, and which sources it has been asked about.
type syntaxCache struct {
	spans  map[string][]Span
	roles  map[string][]int8
	drafts map[string][]int8
	asked  map[string]bool
	want   []string // sources drawn without an answer yet
	last   string   // the edited text last answered about
}

// Instant is a highlighter quick enough to ask while drawing, as the
// built-in tokenizer is.
type Instant interface{ Instant() bool }

// Instant implements Instant: the tokenizer is.
func (Tokens) Instant() bool { return true }

// Drafter is a slow highlighter with a quick draft, drawn until its
// answer comes, and in place of one while it's Instant.
type Drafter interface{ Draft(src string) []Span }

// isInstant reports whether a highlighter or checker answers at once
// (or, a checker, not at all), so it needn't wait for typing to pause.
func isInstant(p any) bool {
	in, ok := p.(Instant)
	return ok && in.Instant()
}

// asksLater reports whether the language is asked once typing pauses:
// its highlighter or checker is slow.
func (v *View) asksLater() bool {
	hl, ck := v.Providers.Highlighter, v.Providers.Checker
	return hl != nil && !isInstant(hl) || ck != nil && !isInstant(ck)
}

// spansFor is src's roles, a role per rune, or nil until the
// highlighter has answered or drafted. The cell being edited (edited)
// isn't asked about here, but once typing pauses.
func (v *View) spansFor(src string, kind notebook.Kind, edited bool) []int8 {
	hl := v.Providers.Highlighter
	if kind != notebook.Code || hl == nil {
		return nil
	}
	c := &v.syntax
	if roles, ok := c.roles[src]; ok {
		return roles
	}
	d, drafts := hl.(Drafter)
	if isInstant(hl) && !drafts {
		v.keepSpans(src, hl.Highlight(context.Background(), src))
		return c.roles[src]
	}
	if !edited && !isInstant(hl) {
		c.ask(src)
	}
	if !drafts {
		return nil
	}
	draft := c.draft(src, d)
	if was, ok := c.roles[c.last]; ok && edited {
		return blend(c.last, src, was, draft)
	}
	return draft
}

// ask notes that src is drawn without an answer, for Fetch.
func (c *syntaxCache) ask(src string) {
	if c.asked == nil {
		c.asked = map[string]bool{}
	}
	if !c.asked[src] {
		c.asked[src] = true
		c.want = append(c.want, src)
	}
}

// draft is d's roles for src, kept.
func (c *syntaxCache) draft(src string, d Drafter) []int8 {
	if roles, ok := c.drafts[src]; ok {
		return roles
	}
	if c.drafts == nil || len(c.drafts) > 512 {
		c.drafts = map[string][]int8{}
	}
	roles := rolesOf(src, d.Draft(src))
	c.drafts[src] = roles
	return roles
}

// keepSpans keeps what the highlighter said of src.
func (v *View) keepSpans(src string, spans []Span) {
	c := &v.syntax
	if c.roles == nil || len(c.roles) > 512 {
		c.roles, c.spans = map[string][]int8{}, map[string][]Span{}
	}
	c.spans[src] = spans
	c.roles[src] = rolesOf(src, spans)
}

// rolesOf is a role per rune of src, -1 for none, from spans; a later
// span overrides an earlier one.
func rolesOf(src string, spans []Span) []int8 {
	roles := make([]int8, 0, len(src))
	for bi := range src {
		r := int8(-1)
		for _, s := range spans {
			if bi >= s.From && bi < s.To {
				r = int8(s.Kind)
			}
		}
		roles = append(roles, r)
	}
	return roles
}

// blend is src's values: was's (of old) where src keeps old's runes,
// its start and its end, and draft's in what changed between.
func blend[T any](old, src string, was, draft []T) []T {
	o, n := []rune(old), []rune(src)
	if len(was) != len(o) || len(draft) != len(n) {
		return draft
	}
	p := 0
	for p < len(o) && p < len(n) && o[p] == n[p] {
		p++
	}
	s := 0
	for s < len(o)-p && s < len(n)-p && o[len(o)-1-s] == n[len(n)-1-s] {
		s++
	}
	out := slices.Clone(draft)
	copy(out, was[:p])
	copy(out[len(n)-s:], was[len(o)-s:])
	return out
}

// Fetch asks the highlighter about the sources drawn without an answer,
// in the background.
func (v *View) Fetch() tea.Cmd {
	hl := v.Providers.Highlighter
	want := v.syntax.want
	v.syntax.want = nil
	if hl == nil || len(want) == 0 {
		return nil
	}
	cmds := make([]tea.Cmd, len(want))
	for i, src := range want {
		cmds[i] = func() tea.Msg {
			return highlightMsg{view: v, src: src, spans: hl.Highlight(context.Background(), src)}
		}
	}
	return tea.Batch(cmds...)
}
