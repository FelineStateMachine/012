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
	// Duration includes the pivot tables the recalculation refreshed,
	// and the recalculation of what reads their results.
	Duration time.Duration
}

// The engine knows nothing of logging; cmd/012 points these hooks at
// telemetry. They run on the goroutine that changed the workbook and
// must be cheap. trace is the workbook's (see SetTrace), handed back so
// the engine's work is timed as spans nested in whatever the workbook's
// owner has open: a command, an import, a macro run.
var (
	// OnBegin, when set, is called as a recalculation ("recalc") or a
	// pivot table refresh ("pivot") begins; OnRecalc or OnPivot as it
	// ends. They pair up like parentheses: a recalculation holds the
	// pivot refreshes it causes, which follow its own evaluation.
	OnBegin func(trace any, op string)
	// OnRecalc, when set, is called after every recalculation of any
	// workbook.
	OnRecalc func(trace any, i RecalcInfo)
)

// SetTrace gives the workbook its owner's trace, opaque to the engine:
// a *telemetry.Trace from the program or import that holds the workbook,
// which only its goroutine uses, as with the workbook itself.
func (w *Workbook) SetTrace(trace any) { w.trace = trace }

// recalcStart is when a recalculation began, or zero when nobody is
// watching, so an unobserved recalculation doesn't read the clock.
func (w *Workbook) recalcStart() time.Time {
	if OnRecalc == nil {
		return time.Time{}
	}
	if OnBegin != nil {
		OnBegin(w.trace, "recalc")
	}
	return time.Now()
}

func (w *Workbook) observe(full bool, start time.Time, evaluated int) {
	if OnRecalc == nil || start.IsZero() {
		return
	}
	info := RecalcInfo{Full: full, Evaluated: evaluated, Circular: w.Circular}
	for _, s := range w.sheets {
		info.Cells += s.cells.len()
		info.Volatile += len(s.volatile)
	}
	info.Duration = time.Since(start)
	OnRecalc(w.trace, info)
}
