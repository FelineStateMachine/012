package ui

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/telemetry"
)

// logSpans turns telemetry on into a JSON log with the engine's hooks
// set as cmd/012 sets them, and returns what reads the events logged.
func logSpans(t *testing.T) func() []map[string]any {
	t.Helper()
	path := filepath.Join(t.TempDir(), "log.jsonl")
	stop, err := telemetry.Setup(telemetry.Config{LogPath: path})
	if err != nil {
		t.Fatal(err)
	}
	end := func(trace any, name string, d time.Duration, attrs ...slog.Attr) {
		if tr, ok := trace.(*telemetry.Trace); ok && tr.End(attrs...) {
			return
		}
		telemetry.Event(name, d, attrs...)
	}
	sheet.OnBegin = func(trace any, op string) {
		if tr, ok := trace.(*telemetry.Trace); ok {
			tr.Begin(op)
		}
	}
	sheet.OnRecalc = func(trace any, i sheet.RecalcInfo) { end(trace, "recalc", i.Duration) }
	sheet.OnPivot = func(trace any, i sheet.PivotInfo) { end(trace, "pivot", i.Duration) }
	t.Cleanup(func() {
		sheet.OnBegin, sheet.OnRecalc, sheet.OnPivot = nil, nil, nil
		stop()
	})
	return func() []map[string]any {
		stop()
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var evs []map[string]any
		for _, l := range strings.Split(strings.TrimSpace(string(b)), "\n") {
			var ev map[string]any
			if err := json.Unmarshal([]byte(l), &ev); err != nil {
				t.Fatal(err)
			}
			evs = append(evs, ev)
		}
		return evs
	}
}

// under reports whether some event named child is logged under the one
// that matches parent.
func under(evs []map[string]any, child string, parent func(map[string]any) bool) bool {
	for _, p := range evs {
		if !parent(p) {
			continue
		}
		for _, c := range evs {
			if c["event"] == child && c["parent_id"] == p["span_id"] && c["trace_id"] == p["trace_id"] {
				return true
			}
		}
	}
	return false
}

func spanNamed(event, id string) func(map[string]any) bool {
	return func(ev map[string]any) bool { return ev["event"] == event && (id == "" || ev["id"] == id) }
}

// A trace shows what a command caused, and what a macro ran.
func TestSpansNest(t *testing.T) {
	read := logSpans(t)
	m := newModel()
	m.sheet.Set(addr("A1"), "1")
	m.sheet.Set(addr("B1"), "=A1*2")
	m.runCommand("clear")
	script(t, m, `
set("A2", 5)
run("format.bold")
`)
	evs := read()
	if !under(evs, "recalc", spanNamed("command", "clear")) {
		t.Error("clear's recalculation is not under its command")
	}
	if !under(evs, "command", spanNamed("macro", "")) {
		t.Error("the macro's command is not under the macro")
	}
	if !under(evs, "recalc", spanNamed("macro", "")) {
		t.Error("the macro's recalculation is not under the macro")
	}
	for _, ev := range evs {
		if ev["event"] == "command" && ev["id"] == "clear" && ev["parent_id"] != nil {
			t.Errorf("a command from a key is a root: %v", ev)
		}
	}
}
