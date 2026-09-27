package jev

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// fakeTypeSafe stands in for the TypeSafe API, accepting only key and
// answering yes/no questions with 0.9. It counts requests and remembers
// the last question.
func fakeTypeSafe(t *testing.T, key string) (*httptest.Server, *atomic.Int32, *atomic.Value) {
	t.Helper()
	var n atomic.Int32
	var last atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n.Add(1)
		if r.Header.Get("Authorization") != "Bearer "+key {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":{"message":"invalid api key"}}`))
			return
		}
		var req struct {
			Questions map[string]struct {
				Type         string `json:"type"`
				Instructions string `json:"instructions"`
			} `json:"questions"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		last.Store(req.Questions["answer"].Instructions)
		json.NewEncoder(w).Encode(map[string]any{
			"model":   "jev-latest",
			"answers": map[string]any{"answer": map[string]any{"type": "noul", "noul": 0.9}},
			"usage":   map[string]int{"input_tokens": 1, "output_tokens": 1},
		})
	}))
	t.Cleanup(srv.Close)
	return srv, &n, &last
}

// Check asks one fixed question; a key the service refuses fails with
// a reason that doesn't hold the key.
func TestCheck(t *testing.T) {
	srv, n, last := fakeTypeSafe(t, "good-key")
	c, err := NewClient(Config{APIKey: "good-key", BaseURL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	if err := Check(context.Background(), c); err != nil {
		t.Fatalf("good key: %v", err)
	}
	if n.Load() != 1 || last.Load() != CheckQuestion {
		t.Errorf("%d requests, question %q", n.Load(), last.Load())
	}

	const bad = "bad-key-12345"
	c, _ = NewClient(Config{APIKey: bad, BaseURL: srv.URL})
	err = Check(context.Background(), c)
	if err == nil || !strings.Contains(err.Error(), "refused the key (HTTP 401)") || strings.Contains(err.Error(), bad) {
		t.Errorf("bad key: %v", err)
	}
}

// A service that can't be reached is said in few words.
func TestCheckUnreachable(t *testing.T) {
	c, err := NewClient(Config{APIKey: "k", BaseURL: "http://127.0.0.1:1"})
	if err != nil {
		t.Fatal(err)
	}
	if err := Check(context.Background(), c); err == nil || err.Error() != "couldn't reach the service: connection refused" {
		t.Errorf("unreachable: %v", err)
	}
}
