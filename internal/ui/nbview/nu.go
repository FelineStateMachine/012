package nbview

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/FelineStateMachine/012/internal/nushell"
)

// NuSession is what a session's questions to nu share: whether nu may
// be asked, whether it answers, and how many questions are out. Each
// question is a process of its own, asked in the background once
// typing pauses, at most two at a time, stopped when the text it's
// about is stale and after Timeout. While nu can't be asked (SetOn said
// no, it isn't installed, it's older than nushell.MinVersion, or it
// timed out three times in a row) the notebook's Nu answers as the
// built-ins do, and nu isn't started.
type NuSession struct {
	Runner nushell.Runner
	// Timeout bounds each question; 0 is a second and a half.
	Timeout time.Duration

	on      atomic.Bool
	off     atomic.Bool  // nu can't be asked this session
	slow    atomic.Int32 // questions timed out in a row
	mu      sync.Mutex
	checked bool // nu's version was asked
	sem     chan struct{}
}

// NewNuSession returns a session asking nu through r, once SetOn allows
// it.
func NewNuSession(r nushell.Runner) *NuSession {
	return &NuSession{Runner: r, sem: make(chan struct{}, 2)}
}

// SetOn says whether nu may be asked: in 012 serve only as serve-shell
// allows, and only about a notebook whose cells may run here without
// asking (docs/nushell/notebooks.md#saving-and-trust).
func (s *NuSession) SetOn(on bool) { s.on.Store(on) }

// Asking reports whether nu is asked: allowed, and not found missing,
// old or slow.
func (s *NuSession) Asking() bool { return s.on.Load() && !s.off.Load() }

// slowAfter is how many questions in a row may time out before nu
// isn't asked again this session.
const slowAfter = 3

var errNotAsked = errors.New("nu isn't asked")

// ask asks nu with f, within the timeout, once nu is known to answer.
func (s *NuSession) ask(ctx context.Context, f func(context.Context) error) error {
	if !s.Asking() {
		return errNotAsked
	}
	timeout := s.Timeout
	if timeout <= 0 {
		timeout = 1500 * time.Millisecond
	}
	select {
	case s.sem <- struct{}{}:
		defer func() { <-s.sem }()
	case <-ctx.Done():
		return ctx.Err()
	}
	qctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if !s.versionOK(qctx) {
		return errNotAsked
	}
	err := f(qctx)
	switch {
	case err == nil:
		s.slow.Store(0)
	case ctx.Err() != nil: // stale: nobody waits for the answer
	case errors.Is(err, context.DeadlineExceeded):
		if s.slow.Add(1) >= slowAfter {
			s.off.Store(true)
		}
	case errors.Is(err, nushell.ErrMissing), errors.Is(err, nushell.ErrOld):
		s.off.Store(true)
	}
	return err
}

// versionOK asks nu's version the first time, turning nu off for the
// session if it's missing or too old.
func (s *NuSession) versionOK(ctx context.Context) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.checked {
		_, err := nushell.Version(ctx, s.Runner)
		if ctx.Err() != nil {
			return false // asked again next time
		}
		s.checked = true
		if err != nil {
			s.off.Store(true)
		}
	}
	return !s.off.Load()
}

// Nu is a notebook's language as nu itself knows it: highlighting from
// its --ide-ast shapes, completions from --ide-complete after the
// notebook's own words, and problems from --ide-check.
//
// nu doesn't know the variables a notebook binds ($sales, $selection,
// $sheet.A1:C9) nor a cell's `name =`, so it's asked about the source
// with those declared before it, in a prelude, and the cell's name and
// ranges of sheets masked to as many bytes, so offsets map back by the
// prelude's length alone (see prepare).
type Nu struct {
	s     *NuSession
	mu    sync.Mutex
	names []string
	words []Word
}

// For returns a notebook's Nu, asking through the session.
func (s *NuSession) For() *Nu { return &Nu{s: s} }

// SetWords gives the notebook's names: the variables it binds besides
// $selection (its cells' names and linked files'), and the built-in
// completer's words. It's called where the notebook changes, so no
// question reads the notebook itself.
func (n *Nu) SetWords(names []string, words []Word) {
	n.mu.Lock()
	n.names, n.words = names, words
	n.mu.Unlock()
}

func (n *Nu) known() ([]string, []Word) {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.names, n.words
}

// Instant implements Instant: while nu isn't asked, the tokenizer
// answers at once.
func (n *Nu) Instant() bool { return !n.s.Asking() }

// Draft implements Drafter: the tokenizer's answer, drawn until nu's
// comes.
func (n *Nu) Draft(src string) []Span { return Tokens{}.Highlight(context.Background(), src) }

// Highlight implements Highlighter: nu's shapes, or the tokenizer's
// when nu doesn't answer.
func (n *Nu) Highlight(ctx context.Context, src string) []Span {
	names, _ := n.known()
	p := prepare(src, names)
	var shapes []nushell.Shape
	err := n.s.ask(ctx, func(ctx context.Context) (err error) {
		shapes, err = nushell.AST(ctx, n.s.Runner, p.text)
		return err
	})
	if err != nil {
		return n.Draft(src)
	}
	return p.spans(src, shapes)
}

// maxProblems is the most problems nu's check reports.
const maxProblems = 20

// Check implements Checker: nu's problems with the pipeline, leaving
// out those that only say it's unfinished at its end, as it is while
// it's typed.
func (n *Nu) Check(ctx context.Context, src string) []Diagnostic {
	names, _ := n.known()
	p := prepare(src, names)
	var probs []nushell.Problem
	err := n.s.ask(ctx, func(ctx context.Context) (err error) {
		probs, err = nushell.Check(ctx, n.s.Runner, p.text, maxProblems)
		return err
	})
	if err != nil {
		return nil
	}
	end := len(strings.TrimRight(src, " \t\n"))
	var out []Diagnostic
	for _, pr := range probs {
		from, to := pr.From-p.at, pr.To-p.at
		if from < 0 || to > len(src) || from == to && from >= end {
			continue
		}
		out = append(out, Diagnostic{From: from, To: to, Msg: pr.Msg})
	}
	return out
}

// Complete implements Completer: the notebook's words, then nu's that
// they don't have.
func (n *Nu) Complete(ctx context.Context, src string, offset int) []Completion {
	offset = min(offset, len(src))
	names, words := n.known()
	out := Words(func() []Word { return words }).Complete(ctx, src, offset)
	p := prepare(src, names)
	var said []string
	if n.s.ask(ctx, func(ctx context.Context) (err error) {
		said, err = nushell.Complete(ctx, n.s.Runner, p.text, p.at+offset)
		return err
	}) != nil {
		return out
	}
	for _, w := range said {
		if strings.HasPrefix(w, "$__") || slices.ContainsFunc(out, func(c Completion) bool { return c.Text == w }) {
			continue // the prelude's own, or a word the notebook has
		}
		out = append(out, Completion{Text: w, From: replaced(src, offset, w), To: offset})
	}
	return out
}
