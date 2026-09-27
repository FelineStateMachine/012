// Package jev answers the spreadsheet's JEV functions with TypeSafe's
// hosted model. It caches answers by question, so recalculating never asks
// twice, and hands queued questions to the UI to send in the background.
// It is only used when an API key is configured.
package jev

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	typesafe "github.com/FelineStateMachine/typesafe-go"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// Config is how to reach the service.
type Config struct {
	APIKey, BaseURL, Model string
}

// LoadConfig reads TYPESAFE_API_KEY, TYPESAFE_BASE_URL and
// TYPESAFE_DEFAULT_MODEL from the environment, falling back to .env files
// in dirs, first found wins. It reports false when there is no key, which
// turns JEV functions off.
func LoadConfig(dirs ...string) (Config, bool) {
	vals := map[string]string{}
	for _, dir := range dirs {
		for k, v := range readDotEnv(filepath.Join(dir, ".env")) {
			if _, seen := vals[k]; !seen {
				vals[k] = v
			}
		}
	}
	get := func(k string) string {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			return v
		}
		return vals[k]
	}
	c := Config{APIKey: get("TYPESAFE_API_KEY"), BaseURL: get("TYPESAFE_BASE_URL"), Model: get("TYPESAFE_DEFAULT_MODEL")}
	return c, c.APIKey != ""
}

// readDotEnv reads the TYPESAFE_ settings from a .env file: KEY=value
// lines, optionally quoted, ignoring comments and anything else.
func readDotEnv(path string) map[string]string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	out := map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(sc.Text()), "export "))
		k, v, ok := strings.Cut(line, "=")
		if !ok || !strings.HasPrefix(k, "TYPESAFE_") {
			continue
		}
		v = strings.TrimSpace(v)
		if len(v) >= 2 && (v[0] == '"' || v[0] == '\'') && v[len(v)-1] == v[0] {
			v = v[1 : len(v)-1]
		}
		out[strings.TrimSpace(k)] = v
	}
	return out
}

// Client is the part of the TypeSafe SDK used here, so tests can fake it.
type Client interface {
	SystemOne(context.Context, typesafe.SystemOneRequest) (*typesafe.SystemOneResponse, error)
}

// NewClient connects to the service with c.
func NewClient(c Config) (Client, error) {
	opts := []typesafe.Option{typesafe.WithAPIKey(c.APIKey)}
	if c.BaseURL != "" {
		opts = append(opts, typesafe.WithBaseURL(c.BaseURL))
	}
	if c.Model != "" {
		opts = append(opts, typesafe.WithModel(c.Model))
	}
	return typesafe.NewClient(opts...)
}

// Cache holds answers and the questions still to ask. It implements
// sheet.RemoteSource.
type Cache struct {
	mu       sync.Mutex
	answers  map[string]sheet.RemoteAnswer
	queue    []sheet.RemoteCall
	waiting  map[string]bool // queued or in flight
	inFlight int
}

// NewCache returns an empty cache.
func NewCache() *Cache {
	return &Cache{answers: map[string]sheet.RemoteAnswer{}, waiting: map[string]bool{}}
}

// Lookup returns the answer to c, queuing c if it hasn't been asked.
func (m *Cache) Lookup(c sheet.RemoteCall) (sheet.RemoteAnswer, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := Key(c)
	if a, ok := m.answers[k]; ok {
		return a, true
	}
	if !m.waiting[k] {
		m.waiting[k] = true
		m.queue = append(m.queue, c)
	}
	return sheet.RemoteAnswer{}, false
}

// Answer returns the answer to c without queuing it.
func (m *Cache) Answer(c sheet.RemoteCall) (sheet.RemoteAnswer, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	a, ok := m.answers[Key(c)]
	return a, ok
}

// Take removes up to n queued questions, counting them as in flight.
func (m *Cache) Take(n int) []sheet.RemoteCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	n = min(n, len(m.queue))
	out := m.queue[:n:n]
	m.queue = m.queue[n:]
	m.inFlight += n
	return out
}

// Store records the answer to a question that was in flight.
func (m *Cache) Store(c sheet.RemoteCall, a sheet.RemoteAnswer) {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := Key(c)
	m.answers[k] = a
	delete(m.waiting, k)
	m.inFlight--
}

// Forget drops answers so the questions are asked again.
func (m *Cache) Forget(calls []sheet.RemoteCall) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, c := range calls {
		delete(m.answers, Key(c))
	}
}

// Busy returns how many questions are in flight and queued.
func (m *Cache) Busy() (inFlight, queued int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.inFlight, len(m.queue)
}

// Ask sends one question and converts the reply. Failures are answers
// too, with Failed set, so they're shown rather than retried in a loop.
func Ask(ctx context.Context, client Client, c sheet.RemoteCall) sheet.RemoteAnswer {
	q, err := question(c)
	if err != nil {
		return sheet.RemoteAnswer{Failed: err.Error()}
	}
	resp, err := client.SystemOne(ctx, typesafe.SystemOneRequest{
		State:     map[string]any{"value": c.State},
		Questions: map[string]typesafe.Question{"answer": q},
	})
	if err != nil {
		return sheet.RemoteAnswer{Failed: err.Error()}
	}
	switch c.Kind {
	case "noul":
		a, err := resp.Noul("answer")
		if err != nil {
			return sheet.RemoteAnswer{Failed: err.Error()}
		}
		return sheet.RemoteAnswer{Noul: a.Noul}
	case "choice":
		a, err := resp.Choice("answer")
		if err != nil {
			return sheet.RemoteAnswer{Failed: err.Error()}
		}
		return sheet.RemoteAnswer{Choice: a.Choice, Confidence: a.Confidence}
	default:
		a, err := resp.Score("answer")
		if err != nil {
			return sheet.RemoteAnswer{Failed: err.Error()}
		}
		return sheet.RemoteAnswer{Score: a.Score, Confidence: a.Confidence}
	}
}

// question builds the SDK question for a call.
func question(c sheet.RemoteCall) (typesafe.Question, error) {
	switch c.Kind {
	case "noul":
		q := typesafe.NoulQuestion{Instructions: c.Instructions}
		if means, _ := c.Criteria.([2]string); means[0] != "" || means[1] != "" {
			q.Criteria = &typesafe.NoulCriteria{
				True:  orDefault(means[0], "Yes."),
				False: orDefault(means[1], "No."),
			}
		}
		return q, nil
	case "choice":
		labels, _ := c.Criteria.(map[string]string)
		criteria := make(map[string]any, len(labels))
		for l, d := range labels {
			criteria[l] = d
		}
		return typesafe.ChoiceQuestion{Instructions: c.Instructions, Criteria: criteria}, nil
	case "score":
		levels, _ := c.Criteria.([]string)
		criteria := make([]any, len(levels))
		for i, l := range levels {
			criteria[i] = l
		}
		return typesafe.ScoreQuestion{Instructions: c.Instructions, Criteria: criteria}, nil
	}
	return nil, fmt.Errorf("unknown question kind %q", c.Kind)
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// Describe summarizes an answer for the context line.
func Describe(c sheet.RemoteCall, a sheet.RemoteAnswer) string {
	pct := func(f float64) string { return fmt.Sprintf("%.0f%%", f*100) }
	switch {
	case a.Failed != "":
		return "JEV couldn't answer: " + a.Failed
	case c.Kind == "noul":
		return "JEV: " + pct(a.Noul) + " likely yes"
	case c.Kind == "choice":
		return "JEV: " + a.Choice + ", " + pct(a.Confidence) + " confident"
	}
	levels, _ := c.Criteria.([]string)
	return fmt.Sprintf("JEV: score %.2g on 0 to %d, %s confident", a.Score, len(levels)-1, pct(a.Confidence))
}
