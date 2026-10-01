package e2e

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// fakeTypeSafe stands in for the TypeSafe API: yes/no questions get 0.9,
// choices pick the last label, scores get 1.5.
func fakeTypeSafe(t *testing.T) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/systemone" || r.Header.Get("Authorization") != "Bearer test-key" {
			http.Error(w, "unexpected request", http.StatusBadRequest)
			return
		}
		requests.Add(1)
		var req struct {
			Questions map[string]struct {
				Type     string          `json:"type"`
				Criteria json.RawMessage `json:"criteria"`
			} `json:"questions"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		q := req.Questions["answer"]
		var answer map[string]any
		switch q.Type {
		case "noul":
			answer = map[string]any{"type": "noul", "noul": 0.9}
		case "choice":
			var labels map[string]any
			json.Unmarshal(q.Criteria, &labels)
			probs, last := map[string]float64{}, ""
			for l := range labels {
				probs[l] = 0
				if l > last {
					last = l
				}
			}
			probs[last] = 1
			answer = map[string]any{"type": "choice", "choice": last, "confidence": 0.8, "probabilities": probs}
		default:
			answer = map[string]any{"type": "score", "score": 1.5, "confidence": 0.6,
				"legend":        map[string]any{"0": "low", "1": "mid", "2": "high"},
				"probabilities": map[string]float64{"0": 0.2, "1": 0.3, "2": 0.5}}
		}
		json.NewEncoder(w).Encode(map[string]any{
			"model": "jev-latest", "answers": map[string]any{"answer": answer},
			"usage": map[string]int{"input_tokens": 1, "output_tokens": 1},
		})
	}))
	t.Cleanup(srv.Close)
	return srv, &requests
}

func TestJEVFunctionsAgainstFakeService(t *testing.T) {
	t.Parallel()
	srv, requests := fakeTypeSafe(t)
	s := startWith(t, options{env: []string{"TYPESAFE_API_KEY=test-key", "TYPESAFE_BASE_URL=" + srv.URL}})
	s.keys("The box arrived crushed", "<enter>")
	s.keys(`=JEV.TEST(A1, "Is this a complaint?")`, "<enter>")
	s.keys(`=JEV.PROB(A1, "Is this a complaint?")`, "<enter>")
	s.keys(`=JEV.CLASSIFY(A1, "Sentiment", "negative, positive")`, "<enter>")
	s.keys(`=JEV.SCORE(A1, "Urgency", "low, mid, high")`, "<enter>")
	s.waitForLine(gridRow1+1, "    2    TRUE")
	s.waitForLine(gridRow1+2, numRow(3, "90%"))
	s.waitForLine(gridRow1+3, "    4  positive")
	s.waitForLine(gridRow1+4, numRow(5, "1.5"))
	// TEST and PROB asked the same question: three requests, not four.
	if n := requests.Load(); n != 3 {
		t.Errorf("%d requests", n)
	}
	s.keys("<up>")
	s.waitFor("JEV: score 1.5 on 0 to 2, 60% confident")
}

func TestJEVOffWithoutKey(t *testing.T) {
	t.Parallel()
	s := start(t, "")
	s.keys(`=JEV.TEST("x", "Q")`, "<enter>", "<up>")
	s.waitFor("JEV functions need an API key")
	if !strings.Contains(s.line(gridRow1), "#N/A") {
		t.Errorf("A1 shows %q", s.line(gridRow1))
	}
}
