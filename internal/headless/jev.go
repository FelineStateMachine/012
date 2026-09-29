package headless

import (
	"context"
	"sync"
	"time"

	"github.com/FelineStateMachine/012/internal/jev"
	"github.com/FelineStateMachine/012/internal/sheet"
)

// jevParallel is how many questions are asked at once, as on the
// screen.
const jevParallel = 8

// maxJEVRounds bounds the rounds of questions: answers can make new
// questions (a JEV function reading another's answer), and each round
// asks those the last one made.
const maxJEVRounds = 16

// AnswerJEV answers w's JEV functions with client, asking what the
// workbook's formulas ask, a few questions at a time, until none is
// waiting, each within timeout. It returns how many were asked.
func AnswerJEV(ctx context.Context, w *sheet.Workbook, client jev.Client, timeout time.Duration) int {
	return AnswerJEVFrom(ctx, w, client, jev.NewCache(), timeout)
}

// AnswerJEVFrom is AnswerJEV with the answers kept in cache, which a
// caller opening the workbook again and again (012 mcp) keeps, so each
// question is asked once.
func AnswerJEVFrom(ctx context.Context, w *sheet.Workbook, client jev.Client, cache *jev.Cache, timeout time.Duration) int {
	w.SetRemote(cache)
	asked := 0
	for range maxJEVRounds {
		calls := cache.Take(1 << 30)
		if len(calls) == 0 {
			break
		}
		asked += len(calls)
		answers := make([]sheet.RemoteAnswer, len(calls))
		var wg sync.WaitGroup
		sem := make(chan struct{}, jevParallel)
		for i, c := range calls {
			wg.Add(1)
			sem <- struct{}{}
			go func() {
				defer func() { <-sem; wg.Done() }()
				ctx, cancel := context.WithTimeout(ctx, timeout)
				defer cancel()
				answers[i] = jev.Ask(ctx, client, c)
			}()
		}
		wg.Wait()
		for i, c := range calls {
			cache.Store(c, answers[i])
		}
		w.RecalcAnswered(calls)
	}
	return asked
}
