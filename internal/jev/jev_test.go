package jev

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	typesafe "github.com/FelineStateMachine/typesafe-go"

	"012/internal/sheet"
)

func TestLoadConfig(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, ".env"), []byte("# settings\nOTHER=x\nexport TYPESAFE_API_KEY=\"from-file\"\nTYPESAFE_DEFAULT_MODEL=jev-small\n"), 0o600)
	t.Setenv("TYPESAFE_API_KEY", "")
	t.Setenv("TYPESAFE_BASE_URL", "")
	t.Setenv("TYPESAFE_DEFAULT_MODEL", "")

	c, ok := LoadConfig(dir)
	if !ok || c.APIKey != "from-file" || c.Model != "jev-small" {
		t.Errorf("from .env: %+v %v", c, ok)
	}
	t.Setenv("TYPESAFE_API_KEY", "from-env")
	if c, _ := LoadConfig(dir); c.APIKey != "from-env" {
		t.Errorf("environment should win: %q", c.APIKey)
	}
	t.Setenv("TYPESAFE_API_KEY", "")
	if _, ok := LoadConfig(t.TempDir()); ok {
		t.Error("no key anywhere, but JEV is on")
	}
}

func TestCacheQueuesOnce(t *testing.T) {
	m := NewCache()
	q := sheet.RemoteCall{Kind: "noul", State: "x", Instructions: "Q", Criteria: [2]string{}}
	for range 3 {
		if _, ok := m.Lookup(q); ok {
			t.Fatal("answered before asking")
		}
	}
	if in, queued := m.Busy(); in != 0 || queued != 1 {
		t.Fatalf("busy %d %d: duplicate questions queued", in, queued)
	}
	taken := m.Take(4)
	if len(taken) != 1 {
		t.Fatalf("took %d", len(taken))
	}
	m.Lookup(q) // asking again while in flight doesn't re-queue
	if in, queued := m.Busy(); in != 1 || queued != 0 {
		t.Fatalf("busy %d %d", in, queued)
	}
	m.Store(q, sheet.RemoteAnswer{Noul: 0.7})
	if a, ok := m.Lookup(q); !ok || a.Noul != 0.7 {
		t.Errorf("after store: %+v %v", a, ok)
	}
	m.Forget([]sheet.RemoteCall{q})
	if _, ok := m.Lookup(q); ok {
		t.Error("forgotten answer still there")
	}
	if _, queued := m.Busy(); queued != 1 {
		t.Error("forgotten question not re-queued")
	}
}

type fakeClient struct {
	resp *typesafe.SystemOneResponse
	err  error
	got  typesafe.SystemOneRequest
}

func (f *fakeClient) SystemOne(_ context.Context, r typesafe.SystemOneRequest) (*typesafe.SystemOneResponse, error) {
	f.got = r
	return f.resp, f.err
}

func TestAsk(t *testing.T) {
	answers := func(a typesafe.Answer) *typesafe.SystemOneResponse {
		return &typesafe.SystemOneResponse{Answers: map[string]typesafe.Answer{"answer": a}}
	}
	tests := []struct {
		call sheet.RemoteCall
		resp *typesafe.SystemOneResponse
		want sheet.RemoteAnswer
		desc string
	}{
		{sheet.RemoteCall{Kind: "noul", State: 42.0, Instructions: "Q", Criteria: [2]string{"big", ""}},
			answers(typesafe.NoulAnswer{Noul: 0.25}), sheet.RemoteAnswer{Noul: 0.25}, "JEV: 25% likely yes"},
		{sheet.RemoteCall{Kind: "choice", State: "x", Instructions: "Q", Criteria: map[string]string{"a": "A", "b": "B"}},
			answers(typesafe.ChoiceAnswer{Choice: "b", Confidence: 0.9}), sheet.RemoteAnswer{Choice: "b", Confidence: 0.9}, "JEV: b, 90% confident"},
		{sheet.RemoteCall{Kind: "score", State: "x", Instructions: "Q", Criteria: []string{"low", "mid", "high"}},
			answers(typesafe.ScoreAnswer{Score: 1.5, Confidence: 0.6}), sheet.RemoteAnswer{Score: 1.5, Confidence: 0.6}, "JEV: score 1.5 on 0 to 2, 60% confident"},
		{sheet.RemoteCall{Kind: "score", State: "x", Instructions: "Q", Criteria: []string{"low", "high"}},
			answers(typesafe.NoulAnswer{Noul: 1}), sheet.RemoteAnswer{Failed: "x"}, ""}, // wrong answer type
	}
	for _, tt := range tests {
		c := &fakeClient{resp: tt.resp}
		got := Ask(context.Background(), c, tt.call)
		if tt.want.Failed != "" {
			if got.Failed == "" {
				t.Errorf("%s: expected a failure, got %+v", tt.call.Kind, got)
			}
			continue
		}
		if got != tt.want {
			t.Errorf("%s: %+v, want %+v", tt.call.Kind, got, tt.want)
		}
		if d := Describe(tt.call, got); d != tt.desc {
			t.Errorf("%s: describe %q, want %q", tt.call.Kind, d, tt.desc)
		}
		// State always travels as an object, which the API requires.
		if st, ok := c.got.State.(map[string]any); !ok || st["value"] != tt.call.State {
			t.Errorf("%s: state %#v", tt.call.Kind, c.got.State)
		}
	}
	// Noul criteria fill in the missing side.
	c := &fakeClient{resp: answers(typesafe.NoulAnswer{})}
	Ask(context.Background(), c, tests[0].call)
	if q := c.got.Questions["answer"].(typesafe.NoulQuestion); q.Criteria.True != "big" || q.Criteria.False != "No." {
		t.Errorf("noul criteria %+v", q.Criteria)
	}
	// Network failures become answers with Failed set.
	if a := Ask(context.Background(), &fakeClient{err: errors.New("offline")}, tests[0].call); a.Failed != "offline" {
		t.Errorf("failure %+v", a)
	}
}

// The engine rejects oversized questions itself; its limits must match
// the SDK's, which match the service.
func TestLimitsMatchSDK(t *testing.T) {
	s := sheet.New()
	sheet.Remote = NewCache()
	defer func() { sheet.Remote = nil }()
	labels := func(n int) string {
		out := make([]string, n)
		for i := range out {
			out[i] = fmt.Sprint("l", i)
		}
		return strings.Join(out, ",")
	}
	for _, tt := range []struct {
		fn  string
		max int
	}{{"JEV.CLASSIFY", typesafe.MaxChoiceOptions}, {"JEV.SCORE", typesafe.MaxScoreLevels}} {
		at := sheet.Addr{}
		s.Set(at, fmt.Sprintf(`=%s("x", "q", "%s")`, tt.fn, labels(tt.max)))
		if !sheet.IsPending(s.Value(at)) {
			t.Errorf("%s with %d options rejected: %+v", tt.fn, tt.max, s.Value(at))
		}
		s.Set(at, fmt.Sprintf(`=%s("x", "q", "%s")`, tt.fn, labels(tt.max+1)))
		if v := s.Value(at); v != sheet.ErrValue {
			t.Errorf("%s with %d options = %+v, want #VALUE!", tt.fn, tt.max+1, v)
		}
	}
}
