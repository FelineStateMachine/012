package sheet

import "time"

// RecalcInfo describes one recalculation, for telemetry. It holds counts
// only, never contents.
type RecalcInfo struct {
	Full      bool // every formula, as after loading
	Evaluated int  // cells marked dirty and recomputed
	Cells     int  // cells stored on every sheet, including formatting-only ones
	Volatile  int  // volatile formulas on every sheet, recomputed every time
	Circular  bool
	Duration  time.Duration
}

// OnRecalc, when set, is called after every recalculation of any workbook.
// The engine knows nothing of logging; cmd/012 points this at telemetry.
// It runs on the goroutine that changed the sheet and must be cheap.
var OnRecalc func(RecalcInfo)

// recalcStart is when a recalculation began, or zero when nobody is
// watching, so an unobserved recalculation doesn't read the clock.
func recalcStart() time.Time {
	if OnRecalc == nil {
		return time.Time{}
	}
	return time.Now()
}

func (w *Workbook) observe(full bool, start time.Time, evaluated int) {
	if OnRecalc == nil || start.IsZero() {
		return
	}
	info := RecalcInfo{Full: full, Evaluated: evaluated, Circular: w.Circular}
	for _, s := range w.sheets {
		info.Cells += len(s.cells)
		info.Volatile += len(s.volatile)
	}
	info.Duration = time.Since(start)
	OnRecalc(info)
}
