package paged

import (
	"context"

	"github.com/FelineStateMachine/012/internal/fileio"
	"github.com/FelineStateMachine/012/internal/sheet"
)

// Links are the workbook's linked sources as the host reads them: each
// one's file through abs (which resolves it from the workbook's folder),
// what they build kept in tempDir ("" for the system's).
func Links(w *sheet.Workbook, abs func(string) string, tempDir string) []Linked {
	var out []Linked
	for _, info := range w.Sources() {
		src := info.Source
		out = append(out, Linked{Name: info.Name, Spec: fileio.SourceSpec{
			Path: abs(src.Path), Format: src.Format, Table: src.Table, Query: src.Query, TempDir: tempDir,
		}})
	}
	return out
}

// Apply tells the workbook what storing a job found: a source opened,
// or questions answered.
func Apply(w *sheet.Workbook, d Done) {
	if d.Opened != "" {
		w.SetSourceShape(d.Opened, d.Shape, d.Err)
	}
	if len(d.Keys) > 0 {
		w.RecalcSourced(d.Keys)
	}
}

// Settle links the sources links names (Links), unless links is nil,
// and runs every job the host queues, one after another, applying each,
// until none is left: for what has no screen to keep live while they
// run (012 get, tests).
func (h *Host) Settle(ctx context.Context, w *sheet.Workbook, links []Linked) {
	if links != nil {
		h.Link(links)
	}
	w.SetSources(h)
	for jobs := h.Jobs(); len(jobs) > 0; jobs = h.Jobs() {
		for _, j := range jobs {
			j.Run(ctx)
			Apply(w, h.Store(j))
		}
		if ctx.Err() != nil {
			return
		}
	}
}
