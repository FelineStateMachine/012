package jev

import (
	"testing"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// Every call must get its own key, and equal calls the same one, map
// order notwithstanding.
func TestKeysAreDistinct(t *testing.T) {
	calls := []sheet.RemoteCall{
		{Kind: "noul", State: "The box was crushed", Instructions: "Is this a complaint?"},
		{Kind: "noul", State: "The box was crushed", Instructions: "Is this a complaint?", Criteria: [2]string{"yes", ""}},
		{Kind: "noul", State: "The box was crushed", Instructions: "Is this a complaint?", Criteria: [2]string{"", "yes"}},
		{Kind: "noul", State: "The box was crushed ", Instructions: "Is this a complaint?"},
		{Kind: "noul", State: "The box", Instructions: " was crushedIs this a complaint?"},
		{Kind: "noul", State: 12.0, Instructions: "q"},
		{Kind: "noul", State: "12", Instructions: "q"},
		{Kind: "noul", State: true, Instructions: "q"},
		{Kind: "noul", State: "", Instructions: "q"},
		{Kind: "noul", State: nil, Instructions: "q"},
		{Kind: "noul", State: [][]any{{"a", 1.0}, {"b", 2.0}}, Instructions: "q"},
		{Kind: "noul", State: [][]any{{"a", 1.0, "b", 2.0}}, Instructions: "q"},
		{Kind: "choice", State: "x", Instructions: "q", Criteria: map[string]string{"neg": "bad", "pos": "good"}},
		{Kind: "choice", State: "x", Instructions: "q", Criteria: map[string]string{"neg": "good", "pos": "bad"}},
		{Kind: "score", State: "x", Instructions: "q", Criteria: []string{"low", "mid", "high"}},
		{Kind: "score", State: "x", Instructions: "q", Criteria: []string{"low", "midhigh"}},
		{Kind: "score", State: "x", Instructions: "q", Criteria: []int{1, 2}}, // JSON fallback
	}
	seen := map[string]int{}
	for i, c := range calls {
		k := Key(c)
		if j, dup := seen[k]; dup {
			t.Errorf("calls %d and %d share key %q", j, i, k)
		}
		seen[k] = i
	}
	m := map[string]string{}
	for _, l := range []string{"a", "b", "c", "d", "e", "f"} {
		m[l] = l + "!"
	}
	c := sheet.RemoteCall{Kind: "choice", State: "x", Instructions: "q", Criteria: m}
	for range 20 {
		if Key(c) != Key(c) {
			t.Fatal("map order changed the key")
		}
	}
}

func BenchmarkKey(b *testing.B) {
	c := sheet.RemoteCall{Kind: "choice", State: "The box was crushed", Instructions: "Sentiment",
		Criteria: map[string]string{"negative": "negative", "positive": "positive"}}
	for b.Loop() {
		Key(c)
	}
}
