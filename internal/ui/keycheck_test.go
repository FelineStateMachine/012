package ui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/config"
	"github.com/FelineStateMachine/012/internal/jev"
	"github.com/FelineStateMachine/012/internal/keyring"
)

// fakeTypeSafe stands in for the TypeSafe API on this machine: it
// accepts only goodKey and answers yes/no questions with 0.9.
func fakeTypeSafe(t *testing.T, goodKey string) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n.Add(1)
		if r.URL.Path != "/v1/systemone" || r.Header.Get("Authorization") != "Bearer "+goodKey {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":{"message":"invalid api key"}}`))
			return
		}
		json.NewEncoder(w).Encode(map[string]any{
			"model":   "jev-latest",
			"answers": map[string]any{"answer": map[string]any{"type": "noul", "noul": 0.9}},
			"usage":   map[string]int{"input_tokens": 1, "output_tokens": 1},
		})
	}))
	t.Cleanup(srv.Close)
	return srv, &n
}

// Storing a key makes one test call with it and says how it went,
// without ever showing the key; a refused key is still stored.
func TestAPIKeyChecked(t *testing.T) {
	const good, bad = "ts-good-4242", "ts-bad-1313"
	srv, requests := fakeTypeSafe(t, good)
	path := filepath.Join(t.TempDir(), "config")
	os.WriteFile(path, nil, 0o600)
	store := &keyring.Memory{}
	m := newModel()
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 20})
	m.Configure(Settings{Config: config.Load(path, func(string) string { return "" }, nil), Keys: store,
		Connect: func(key string) (jev.Client, error) {
			return jev.NewClient(jev.Config{APIKey: key, BaseURL: srv.URL})
		}})

	m.runCommand("settings.jev_key")
	press(t, m, good, "<enter>")
	if got := line(m, contextLine); !strings.Contains(got, "Key saved and checked") || requests.Load() != 1 {
		t.Fatalf("good key: %q after %d requests", got, requests.Load())
	}

	m.runCommand("settings.jev_key")
	press(t, m, bad, "<enter>")
	got := line(m, contextLine)
	if !strings.Contains(got, "Key saved, but the check failed: the service refused the key (HTTP 401)") {
		t.Errorf("bad key: %q", got)
	}
	if strings.Contains(screen(m), bad) || strings.Contains(screen(m), good) {
		t.Error("a key is on the screen")
	}
	if k, _ := store.Get(t.Context()); k != bad || m.jev == nil {
		t.Errorf("the refused key wasn't kept: %q, JEV on %v", k, m.jev != nil)
	}
	if n := requests.Load(); n != 2 {
		t.Errorf("%d requests; want one per key", n)
	}
}
