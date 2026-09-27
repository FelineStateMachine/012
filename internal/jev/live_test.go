package jev

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// TestLive asks the real service one question of each kind. It only runs
// with JEV_LIVE_TEST=1 and a key in the environment or the repo's .env.
func TestLive(t *testing.T) {
	if os.Getenv("JEV_LIVE_TEST") != "1" {
		t.Skip("set JEV_LIVE_TEST=1 to call the live JEV service")
	}
	cfg, ok := LoadConfig("../..")
	if !ok {
		t.Skip("no TYPESAFE_API_KEY")
	}
	client, err := NewClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	text := "The package arrived crushed and support hasn't replied in a week."
	for _, c := range []sheet.RemoteCall{
		{Kind: "noul", State: text, Instructions: "Is this a complaint?", Criteria: [2]string{}},
		{Kind: "choice", State: text, Instructions: "Sentiment", Criteria: map[string]string{"positive": "positive", "negative": "negative", "neutral": "neutral"}},
		{Kind: "score", State: text, Instructions: "How urgent is this?", Criteria: []string{"low", "medium", "high"}},
	} {
		a := Ask(ctx, client, c)
		if a.Failed != "" {
			t.Fatalf("%s: %s", c.Kind, a.Failed)
		}
		t.Logf("%s: %s", c.Kind, Describe(c, a))
	}
}
