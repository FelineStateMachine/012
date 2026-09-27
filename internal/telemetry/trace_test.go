package telemetry

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"
)

// byEvent indexes the logged events by name; names are unique in these
// tests.
func byEvent(t *testing.T, evs []map[string]any) map[string]map[string]any {
	t.Helper()
	m := map[string]map[string]any{}
	for _, ev := range evs {
		m[ev["event"].(string)] = ev
	}
	return m
}

// childOf checks that child is logged as a span under parent.
func childOf(t *testing.T, child, parent map[string]any) {
	t.Helper()
	if child["parent_id"] == nil || child["parent_id"] != parent["span_id"] || child["trace_id"] != parent["trace_id"] {
		t.Errorf("%v (parent %v, trace %v) is not under %v (span %v, trace %v)", child["event"], child["parent_id"], child["trace_id"],
			parent["event"], parent["span_id"], parent["trace_id"])
	}
}

func TestTraceNests(t *testing.T) {
	buf := record(t, slog.LevelInfo)
	tr := NewTrace(Parent{})
	cmd := tr.Start("command", slog.String("id", "format.bold"))
	tr.Begin("recalc")
	tr.Begin("pivot")
	tr.Event("jev.cached", time.Millisecond)
	tr.End(slog.Int("cells", 1))
	tr.End(slog.Int("cells", 2))
	p := tr.Parent()
	done := make(chan struct{})
	go func() { // work handed to another goroutine
		p.Start("jev").End()
		close(done)
	}()
	<-done
	cmd.End()
	Start("root").End()
	Event("event", time.Millisecond)
	if tr.Parent() != (Parent{}) || len(tr.begun) != 0 {
		t.Errorf("left open: %+v", tr)
	}

	evs := byEvent(t, events(t, buf))
	childOf(t, evs["recalc"], evs["command"])
	childOf(t, evs["pivot"], evs["recalc"])
	childOf(t, evs["jev.cached"], evs["pivot"])
	childOf(t, evs["jev"], evs["command"])
	for _, root := range []string{"command", "root", "event"} {
		ev := evs[root]
		if _, ok := ev["parent_id"]; ok || len(ev["trace_id"].(string)) != 32 || len(ev["span_id"].(string)) != 16 {
			t.Errorf("root %v", ev)
		}
	}
	if evs["root"]["trace_id"] == evs["command"]["trace_id"] || evs["event"]["trace_id"] == evs["root"]["trace_id"] {
		t.Error("roots share a trace")
	}
	if evs["recalc"]["cells"] != 2.0 || evs["pivot"]["cells"] != 1.0 {
		t.Errorf("End's attributes went to the wrong span: %v %v", evs["recalc"], evs["pivot"])
	}
}

// A session's span stays under everything its program starts; a macro's
// is entered only while its calls are served.
func TestTraceEnterLeave(t *testing.T) {
	buf := record(t, slog.LevelInfo)
	session := Start("serve.session")
	tr := NewTrace(session.Parent())
	macro := tr.Start("command", slog.String("id", "macro.run"))
	run := tr.Parent().Start("macro") // outlives the command
	macro.End()
	n := tr.Enter(run.Parent())
	tr.Start("command", slog.String("id", "set")).End()
	tr.Leave(n)
	run.End()
	tr.Start("after").End()
	session.End()

	evs := events(t, buf)
	got := map[string]map[string]any{}
	for _, ev := range evs {
		name := ev["event"].(string)
		if id, ok := ev["id"].(string); ok {
			name += " " + id
		}
		got[name] = ev
	}
	childOf(t, got["command macro.run"], got["serve.session"])
	childOf(t, got["macro"], got["command macro.run"])
	childOf(t, got["command set"], got["macro"])
	childOf(t, got["after"], got["serve.session"])
}

// A span ended out of order closes what was opened after it, so a lost
// End doesn't leave everything after it nested under a finished span.
func TestTraceOutOfOrder(t *testing.T) {
	record(t, slog.LevelInfo)
	tr := &Trace{}
	outer := tr.Start("outer")
	tr.Start("lost") // never ended
	outer.Fail(errors.New("failed"))
	if tr.Parent() != (Parent{}) {
		t.Errorf("still open: %+v", tr.open)
	}
	if tr.End() {
		t.Error("End with nothing begun")
	}
	var nilTrace *Trace
	nilTrace.Begin("recalc")
	nilTrace.Start("x").End()
	if nilTrace.End() || nilTrace.Enter(outer.Parent()) != 0 || nilTrace.Parent() != (Parent{}) {
		t.Error("nil Trace")
	}
}

func TestParentInContext(t *testing.T) {
	record(t, slog.LevelInfo)
	ctx := context.Background()
	if WithParent(ctx, Parent{}) != ctx || ParentFrom(ctx) != (Parent{}) {
		t.Error("zero Parent in a context")
	}
	s := Start("import")
	if ParentFrom(WithParent(ctx, s.Parent())) != s.Parent() {
		t.Error("Parent lost in a context")
	}
}

// Nested spans while off: one atomic load each, nothing kept open.
func TestTraceOff(t *testing.T) {
	tr := NewTrace(Parent{})
	s := tr.Start("command")
	tr.Begin("recalc")
	if s.name != "" || s.ids != (spanIDs{}) || len(tr.open) != 0 || len(tr.begun) != 0 {
		t.Errorf("span while off = %+v, trace %+v", s, tr)
	}
	n := testing.AllocsPerRun(100, func() {
		s := tr.Start("command", slog.String("id", "x"))
		tr.Begin("recalc")
		tr.Event("pivot", time.Millisecond, slog.Int("cells", 2))
		tr.End(slog.Int("cells", 2))
		tr.Parent().Start("jev").End()
		d := tr.Enter(s.Parent())
		tr.Leave(d)
		s.End()
	})
	if n != 0 {
		t.Errorf("%v allocations per call while off", n)
	}
}

// The cost of a span nested under an open one, while off and on.
func BenchmarkTraceOff(b *testing.B) {
	tr := &Trace{}
	for b.Loop() {
		s := tr.Start("recalc", slog.Int("n", 1))
		s.End(slog.Int("cells", 2))
	}
}

func BenchmarkTraceOn(b *testing.B) {
	start(io.Discard, nil, Config{})
	defer Close()
	tr := &Trace{}
	cmd := tr.Start("command")
	defer cmd.End()
	for b.Loop() {
		s := tr.Start("recalc", slog.Int("n", 1))
		s.End(slog.Int("cells", 2))
	}
}
