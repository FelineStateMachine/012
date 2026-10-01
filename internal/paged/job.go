package paged

import (
	"context"
	"fmt"
	"os"

	"github.com/FelineStateMachine/012/internal/fileio"
	"github.com/FelineStateMachine/012/internal/functions"
	"github.com/FelineStateMachine/012/internal/sheet"
)

type jobKind uint8

const (
	jobOpen jobKind = iota
	jobAnswer
)

// A Job is work the host queued: opening a source, or answering a
// question by reading the sources it names. Run it on any goroutine,
// once, then give it back to Host.Store on the owner's.
type Job struct {
	kind jobKind

	// Opening a source.
	name  string
	spec  fileio.SourceSpec
	gen   int
	open  Opener
	h     fileio.Source
	shape sheet.SourceShape
	stamp stamp
	err   error

	// Answering a question.
	q      sheet.SourceQuestion
	key    string
	srcs   map[string]snapshot // the sources it reads, by name key
	budget int
	ans    sheet.SourceAnswer
}

// snapshot is a source as a job reads it: opened, with what it is.
type snapshot struct {
	name  string
	h     fileio.Source
	shape sheet.SourceShape
	gen   int
	dir   string // where sorting spills, "" for the system's temporary files
}

// String says what the job does, for telemetry.
func (j *Job) String() string {
	if j.kind == jobOpen {
		return "open " + j.name
	}
	return j.key
}

// Run does the job.
func (j *Job) Run(ctx context.Context) {
	if j.kind == jobOpen {
		j.runOpen(ctx)
		return
	}
	j.ans = j.answer(ctx)
}

func (j *Job) runOpen(ctx context.Context) {
	if st, err := os.Stat(j.spec.Path); err == nil {
		j.stamp = stamp{st.Size(), st.ModTime()}
	}
	h, err := j.open(ctx, j.spec)
	if err != nil {
		j.err = err
		return
	}
	j.h, j.shape = h, ShapeOf(h)
}

// ShapeOf is what the engine keeps of a source.
func ShapeOf(h fileio.Source) sheet.SourceShape {
	cols := h.Columns()
	sh := sheet.SourceShape{Rows: int(h.Rows())}
	for _, c := range cols {
		sh.Cols = append(sh.Cols, c.Name)
		sh.Formats = append(sh.Formats, c.Format)
		sh.Numeric = append(sh.Numeric, c.Numeric)
	}
	return sh
}

// answer works out the job's question.
func (j *Job) answer(ctx context.Context) sheet.SourceAnswer {
	b := &book{ctx: ctx, srcs: j.srcs}
	defer b.close()
	var ans sheet.SourceAnswer
	switch j.q.Kind {
	case sheet.AskCell:
		ans.V = b.Cell(j.q.Source, j.q.At)
	case sheet.AskRange:
		ans = b.rangeValues(j.q.Source, j.q.R, j.budget)
	case sheet.AskCall:
		sa := functions.EvalStream(j.q.Call, b, j.budget)
		ans = sheet.SourceAnswer{V: sa.V, A: sa.A, Why: sa.Why}
	case sheet.AskPivot:
		ans = b.pivot(j.q.Source, j.q.Pivot)
	}
	if b.err != nil {
		return sheet.SourceAnswer{V: sheet.ErrRef, Why: fmt.Sprintf("%s: %s", j.q.Source, describe(b.err))}
	}
	return ans
}
