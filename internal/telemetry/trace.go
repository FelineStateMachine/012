package telemetry

import (
	"context"
	"encoding/hex"
	"log/slog"
	"time"
)

// Spans nest. Every span has a trace id, a span id and, unless it is a
// root, its parent's span id; a child shares its parent's trace id. No
// context.Context goes through the engine for it: whoever owns a thread
// of work (one UI program, one import) keeps a Trace, the spans it has
// open, and what starts through the Trace nests under the innermost.
// Work handed to another goroutine (a background tea.Cmd) takes a Parent
// instead, an explicit handle on the span to nest under. There is no
// goroutine-global "current span": 012 serve runs many programs in one
// process, and their commands run on other goroutines.

// Parent identifies a span that others can nest under, from any
// goroutine. The zero Parent is none: spans under it are roots, each in
// a trace of its own.
type Parent struct {
	trace traceID
	span  spanID
}

// spanIDs are a span's trace and span ids, and its parent's span id
// (zero for a root).
type spanIDs struct {
	trace  traceID
	span   spanID
	parent spanID
}

// child makes the ids of a new span under p.
func (p Parent) child() spanIDs {
	ids := spanIDs{trace: p.trace, parent: p.span}
	for ids.trace.IsZero() {
		randomBytes(ids.trace[:])
	}
	for ids.span.IsZero() {
		randomBytes(ids.span[:])
	}
	return ids
}

// Span times one operation. The zero Span, returned while off, does
// nothing. With OTLP on it is sent as a span, and the log record of the
// same event carries its trace, span and parent ids.
type Span struct {
	name  string
	start time.Time
	// attrs holds up to four attributes given to Start, in the Span
	// itself: keeping the variadic slice would move it to the heap at
	// every call site, telemetry on or off.
	attrs [4]slog.Attr
	n     int
	ids   spanIDs
	tr    *Trace // the Trace it is open in, if it was started through one
}

// Start begins timing an operation named like "recalc" or "import", with
// up to four attributes known at the start, as a root span.
func Start(name string, attrs ...slog.Attr) Span {
	if !on.Load() {
		return Span{}
	}
	return newSpan(Parent{}, name, attrs)
}

// Start begins a span under p, like the package's Start.
func (p Parent) Start(name string, attrs ...slog.Attr) Span {
	if !on.Load() {
		return Span{}
	}
	return newSpan(p, name, attrs)
}

func newSpan(p Parent, name string, attrs []slog.Attr) Span {
	s := Span{name: name, start: time.Now(), ids: p.child()}
	s.n = copy(s.attrs[:], attrs)
	return s
}

// Parent is the handle for nesting spans under s; zero for the zero Span.
func (s Span) Parent() Parent { return Parent{s.ids.trace, s.ids.span} }

// End logs the operation with its duration in milliseconds and any
// attributes learned along the way, such as rows read. A span started
// through a Trace is no longer open in it.
func (s Span) End(attrs ...slog.Attr) {
	if s.name == "" {
		return
	}
	s.tr.close(s.ids.span)
	if !on.Load() {
		return
	}
	emit(slog.LevelInfo, s.name, time.Since(s.start), append(s.attrs[:s.n:s.n], attrs...), s.ids)
}

// Fail ends the span with an error, logged at Warn with its message.
func (s Span) Fail(err error, attrs ...slog.Attr) {
	if err == nil {
		s.End(attrs...)
		return
	}
	if s.name == "" {
		return
	}
	s.tr.close(s.ids.span)
	if !on.Load() {
		return
	}
	attrs = append(append(s.attrs[:s.n:s.n], attrs...), slog.String("error", err.Error()))
	emit(slog.LevelWarn, s.name, time.Since(s.start), attrs, s.ids)
}

// Event logs a finished operation that took d, as a root span.
func Event(name string, d time.Duration, attrs ...slog.Attr) {
	if !on.Load() {
		return
	}
	emit(slog.LevelInfo, name, d, attrs, Parent{}.child())
}

// Event logs a finished operation that took d, as a span under p.
func (p Parent) Event(name string, d time.Duration, attrs ...slog.Attr) {
	if !on.Load() {
		return
	}
	emit(slog.LevelInfo, name, d, attrs, p.child())
}

// Trace is the spans open in one thread of work, innermost last: one UI
// program's, or one import's. Spans started through it nest under the
// innermost open span, and stay open in it until they end. It belongs
// to one goroutine at a time, like the Model or workbook that keeps it.
// A nil *Trace works too, with nothing open.
type Trace struct {
	open  []Parent
	begun []Span // spans started by Begin, for End
}

// NewTrace returns a Trace with base open, if it isn't zero, beneath
// everything started through it.
func NewTrace(base Parent) *Trace {
	t := &Trace{}
	if base != (Parent{}) {
		t.open = append(t.open, base)
	}
	return t
}

// Parent is the innermost span open, zero if none is: the handle to give
// work done on another goroutine.
func (t *Trace) Parent() Parent {
	if t == nil || len(t.open) == 0 {
		return Parent{}
	}
	return t.open[len(t.open)-1]
}

// Start begins a span under the innermost open one; it is the innermost
// itself until it ends.
func (t *Trace) Start(name string, attrs ...slog.Attr) Span {
	if !on.Load() {
		return Span{}
	}
	s := newSpan(t.Parent(), name, attrs)
	if t != nil {
		s.tr = t
		t.open = append(t.open, s.Parent())
	}
	return s
}

// Event logs a finished operation that took d, under the innermost open
// span.
func (t *Trace) Event(name string, d time.Duration, attrs ...slog.Attr) {
	if !on.Load() {
		return
	}
	emit(slog.LevelInfo, name, d, attrs, t.Parent().child())
}

// Begin starts a span like Start and keeps it for End, for code that
// has nowhere to keep a Span, such as the engine's hooks (see cmd/012).
// On a nil Trace it does nothing.
func (t *Trace) Begin(name string) {
	if t == nil || !on.Load() {
		return
	}
	t.begun = append(t.begun, t.Start(name))
}

// End ends the span last begun by Begin, reporting whether there was one.
func (t *Trace) End(attrs ...slog.Attr) bool {
	if t == nil || len(t.begun) == 0 {
		return false
	}
	s := t.begun[len(t.begun)-1]
	t.begun = t.begun[:len(t.begun)-1]
	s.End(attrs...)
	return true
}

// Enter makes p the innermost open span, until Leave with the depth it
// returns: for a span that lives across several updates (a macro run, a
// session) to hold what each of them starts.
func (t *Trace) Enter(p Parent) int {
	if t == nil {
		return 0
	}
	n := len(t.open)
	if p != (Parent{}) {
		t.open = append(t.open, p)
	}
	return n
}

// Leave closes what was opened since the Enter that returned depth.
func (t *Trace) Leave(depth int) {
	if t != nil && depth < len(t.open) {
		t.open = t.open[:depth]
	}
}

// close closes span and anything opened after it that is still open (a
// span ended out of order, or not at all).
func (t *Trace) close(span spanID) {
	if t == nil {
		return
	}
	for i := len(t.open) - 1; i >= 0; i-- {
		if t.open[i].span == span {
			t.open = t.open[:i]
			return
		}
	}
}

// parentKey carries a Parent in a context.Context, where one already
// goes by, as into an import.
type parentKey struct{}

// WithParent returns ctx carrying p, for ParentFrom.
func WithParent(ctx context.Context, p Parent) context.Context {
	if p == (Parent{}) {
		return ctx
	}
	return context.WithValue(ctx, parentKey{}, p)
}

// ParentFrom is the Parent ctx carries, zero if none.
func ParentFrom(ctx context.Context) Parent {
	p, _ := ctx.Value(parentKey{}).(Parent)
	return p
}

// idsHandler adds a span's ids to its record in the JSON log, as
// trace_id, span_id and, below a root, parent_id, so the file log holds
// the same tree as the traces.
type idsHandler struct{ slog.Handler }

func (h idsHandler) Handle(ctx context.Context, r slog.Record) error {
	if ids, ok := ctx.Value(spanKey{}).(spanIDs); ok {
		r = r.Clone()
		r.AddAttrs(slog.String("trace_id", hex.EncodeToString(ids.trace[:])), slog.String("span_id", hex.EncodeToString(ids.span[:])))
		if !ids.parent.IsZero() {
			r.AddAttrs(slog.String("parent_id", hex.EncodeToString(ids.parent[:])))
		}
	}
	return h.Handler.Handle(ctx, r)
}

func (h idsHandler) WithAttrs(as []slog.Attr) slog.Handler {
	return idsHandler{h.Handler.WithAttrs(as)}
}
func (h idsHandler) WithGroup(name string) slog.Handler { return idsHandler{h.Handler.WithGroup(name)} }
