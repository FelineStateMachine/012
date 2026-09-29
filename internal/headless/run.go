package headless

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/FelineStateMachine/012/internal/fileio"
	"github.com/FelineStateMachine/012/internal/jev"
	"github.com/FelineStateMachine/012/internal/nushell"
	"github.com/FelineStateMachine/012/internal/sheet"
)

// RegionOptions say how RunRegions runs commands.
type RegionOptions struct {
	Runner   nushell.Runner // nu, or a fake in tests
	Timeout  time.Duration  // for each command
	NuConfig bool           // run nu with the user's config files
}

// RunRegions runs every notebook region's command with nu, each after
// the regions it reads, as Run all regions does on the screen, and
// shows each table. It returns what failed, one line per region; a
// region that fails leaves the table it had, which in a file just read
// is none. Linked files' regions are left empty: following a file is
// the screen's.
func RunRegions(ctx context.Context, w *sheet.Workbook, o RegionOptions) []string {
	var failed []string
	for _, name := range w.RunOrder() {
		s, r, ok := w.Region(name)
		if !ok || r.Linked() {
			continue
		}
		if err := runRegion(ctx, s, r, o); err != nil {
			failed = append(failed, fmt.Sprintf("region %s (%s): %s", r.Name, r.Command, err))
		}
	}
	return failed
}

func runRegion(ctx context.Context, s *sheet.Sheet, r sheet.Region, o RegionOptions) error {
	job, err := jobFor(s.Book(), r, o.NuConfig)
	if err != nil {
		return err
	}
	data, err := nushell.Exec(ctx, o.Runner, job, o.Timeout, 0)
	if err != nil {
		return err
	}
	return s.ShowRegion(r.Name, data)
}

// jobFor is what running r takes: the tables it reads, as NUON.
func jobFor(w *sheet.Workbook, r sheet.Region, config bool) (nushell.Job, error) {
	job := nushell.Job{Command: r.Command, Tables: map[string][]byte{}, Config: config}
	for _, d := range r.Deps {
		s, dr, ok := w.Region(d)
		if !ok {
			return job, fmt.Errorf("it reads $%s, which isn't a region any more", d)
		}
		t, ok := s.RegionTable(dr.Name)
		if !ok {
			t = sheet.Rect{From: dr.At, To: dr.At}
		}
		snap := fileio.Snap(s, t, dr.Name)
		snap.HiddenRows = nil
		if !ok {
			snap.Cells = nil
		}
		data, err := encodeNUON(snap)
		if err != nil {
			return job, err
		}
		job.Tables[d] = data
	}
	if r.Input != "" {
		t, err := Resolve(w, r.Input)
		if err != nil {
			return job, fmt.Errorf("its input %s is gone", r.Input)
		}
		if job.Input, err = encodeNUON(fileio.Snap(t.Sheet, t.Range, t.Sheet.Name())); err != nil {
			return job, err
		}
	}
	return job, nil
}

func encodeNUON(snap *fileio.Snapshot) ([]byte, error) {
	var b bytes.Buffer
	_, err := fileio.Encode(&b, fileio.NUON, snap)
	return b.Bytes(), err
}

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
	cache := jev.NewCache()
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
