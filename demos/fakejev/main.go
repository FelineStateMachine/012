// Command fakejev is a stand-in for the TypeSafe JEV service, for demo
// recordings: it answers every question about a value by looking for
// complaint words in the value, so a demo never needs a real key and
// always records the same answers. Point 012 at it with
//
//	TYPESAFE_API_KEY=demo TYPESAFE_BASE_URL=http://127.0.0.1:8799 ./bin/012
package main

import (
	"encoding/json"
	"flag"
	"io"
	"log"
	"net/http"
	"slices"
	"strings"
	"time"
)

// negative words mark a value as a complaint.
var negative = []string{"crushed", "broken", "late", "wrong", "refund", "never", "missing"}

func main() {
	addr := flag.String("addr", "127.0.0.1:8799", "listen address")
	delay := flag.Duration("delay", 600*time.Millisecond, "time to answer, so Loading… shows")
	flag.Parse()
	http.HandleFunc("/v1/systemone", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		time.Sleep(*delay)
		answer, err := answerFor(body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{
			"model": "jev-demo", "answers": map[string]any{"answer": answer},
			"usage": map[string]int{"input_tokens": 1, "output_tokens": 1},
		})
	})
	log.Printf("fake JEV service on http://%s", *addr)
	log.Fatal(http.ListenAndServe(*addr, nil))
}

// answerFor answers the request's question: yes-or-no, a choice of
// labels, or a score on levels.
func answerFor(body []byte) (map[string]any, error) {
	var req struct {
		State     json.RawMessage `json:"state"`
		Questions map[string]struct {
			Type     string          `json:"type"`
			Criteria json.RawMessage `json:"criteria"`
		} `json:"questions"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, err
	}
	text := strings.ToLower(string(req.State))
	bad := slices.ContainsFunc(negative, func(w string) bool { return strings.Contains(text, w) })
	q := req.Questions["answer"]
	switch q.Type {
	case "noul":
		p := 0.07
		if bad {
			p = 0.94
		}
		return map[string]any{"type": "noul", "noul": p}, nil
	case "choice":
		var labels map[string]any
		json.Unmarshal(q.Criteria, &labels)
		keys := make([]string, 0, len(labels))
		for l := range labels {
			keys = append(keys, l)
		}
		slices.Sort(keys)
		pick := keys[len(keys)-1]
		if bad {
			pick = keys[0]
		}
		probs := map[string]float64{}
		for _, l := range keys {
			probs[l] = 0.1 / float64(max(len(keys)-1, 1))
		}
		probs[pick] = 0.9
		return map[string]any{"type": "choice", "choice": pick, "confidence": 0.9, "probabilities": probs}, nil
	}
	score, probs := 0.2, map[string]float64{"0": 0.8, "1": 0.15, "2": 0.05}
	if bad {
		score, probs = 1.8, map[string]float64{"0": 0.05, "1": 0.15, "2": 0.8}
	}
	return map[string]any{"type": "score", "score": score, "confidence": 0.8,
		"legend": map[string]any{"0": "low", "1": "mid", "2": "high"}, "probabilities": probs}, nil
}
