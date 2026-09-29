package ui

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"runtime"
	"slices"
	"strconv"
	"testing"
	"time"
)

// speedBaselinePath holds the speed gate's baseline, rewritten by
// make speed-update.
const speedBaselinePath = "testdata/speed.json"

// The margins a case may exceed its baseline by before it fails: time
// relative to the calibration may double (the ratio still moves with
// the CPU, the Go version and the machine's load, and a clear
// regression is a multiple, not a percentage), and allocations, which
// are counted, not timed, may grow by a quarter plus a few.
const (
	ratioMargin  = 2.0
	allocsMargin = 1.25
	allocsSlack  = 16
)

// speedRounds is how many times each case is timed, beside the
// calibration, keeping the fastest of each; speedBatch is about how long
// one timing of a case runs.
const (
	speedRounds = 7
	speedBatch  = 2 * time.Millisecond
)

// speedResult is one case measured: nanoseconds per operation (for
// reading), the same relative to the calibration workload, and
// allocations per operation.
type speedResult struct {
	NS     float64 `json:"ns"`
	Ratio  float64 `json:"ratio"`
	Allocs float64 `json:"allocs"`
}

// speedBaseline is testdata/speed.json: the machine it was measured on
// and each case's result. Ratios are compared only on the same OS and
// architecture; allocations everywhere.
type speedBaseline struct {
	GOOS   string                 `json:"goos"`
	GOARCH string                 `json:"goarch"`
	Cases  map[string]speedResult `json:"cases"`
}

func readSpeedBaseline() (speedBaseline, error) {
	var b speedBaseline
	data, err := os.ReadFile(speedBaselinePath)
	if err != nil {
		return b, fmt.Errorf("%w (make speed-update writes it)", err)
	}
	return b, json.Unmarshal(data, &b)
}

func (b speedBaseline) write() error {
	for name, r := range b.Cases {
		r.NS = math.Round(r.NS)
		r.Ratio = math.Round(r.Ratio*1000) / 1000
		r.Allocs = math.Round(r.Allocs)
		b.Cases[name] = r
	}
	data, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(speedBaselinePath, append(data, '\n'), 0o644)
}

// judge says how got is over the baseline's margins, or "" when it
// isn't.
func (b speedBaseline) judge(name string, got speedResult) string {
	want, ok := b.Cases[name]
	if !ok {
		return "not in " + speedBaselinePath + " (make speed-update)"
	}
	if limit := want.Allocs*allocsMargin + allocsSlack; got.Allocs > limit {
		return fmt.Sprintf("%.0f allocations an operation, over %.0f (baseline %.0f)", got.Allocs, limit, want.Allocs)
	}
	sameMachine := b.GOOS == runtime.GOOS && b.GOARCH == runtime.GOARCH
	if limit := want.Ratio * ratioMargin; sameMachine && got.Ratio > limit {
		return fmt.Sprintf("%.2fx the calibration, over %.2fx (baseline %.2fx, %.3f ms now)", got.Ratio, limit, want.Ratio, got.NS/1e6)
	}
	return ""
}

// measureSpeed times op against the calibration and counts its allocations.
// Calls come in even numbers, so an operation alternating two states
// ends where it started.
func measureSpeed(op func()) speedResult {
	op()
	op()
	n := 2
	for start := time.Now(); time.Since(start) < speedBatch/2; n += 2 {
		op()
		op()
	}
	allocs := testing.AllocsPerRun(n, op)
	op() // AllocsPerRun calls it once more than n
	best, calib := time.Duration(math.MaxInt64), time.Duration(math.MaxInt64)
	for range speedRounds {
		calib = min(calib, calibrate())
		start := time.Now()
		for range n {
			op()
		}
		best = min(best, time.Since(start))
	}
	per := float64(best) / float64(n)
	return speedResult{NS: per, Ratio: per / float64(calib), Allocs: allocs}
}

// calibrationSink keeps the calibration's result alive.
var calibrationSink float64

// calibrate times a fixed workload of the kinds of work the cases do
// (formatting numbers, map lookups by string, sorting, allocating) and
// returns how long it took: the unit the cases' times are measured in.
func calibrate() time.Duration {
	start := time.Now()
	for range 4 {
		m := make(map[string]float64)
		xs := make([]float64, 0, 2048)
		for i := range 2048 {
			k := strconv.Itoa(i * 7919 % 4099)
			m[k] += float64(i)
			xs = append(xs, float64(i*7919%4099))
		}
		slices.Sort(xs)
		for i := range 2048 {
			calibrationSink += m[strconv.Itoa(i)] + xs[i]
		}
	}
	return time.Since(start)
}
