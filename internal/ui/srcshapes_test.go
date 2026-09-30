package ui

import (
	"os"
	"path/filepath"
	"sync"

	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/stress"
)

// Linked sources for the speed gate (speed_test.go) and the stress
// benchmarks (srcstress_test.go): a model showing a source's tab, its
// jobs run as they come.

// sourceModel is a model w x h showing the tab of the Parquet file at
// path linked as a source, opened, its first rows read.
func sourceModel(path string, w, h int) *Model {
	m := sized(sheet.New(), w, h)
	m.addSource(sheet.LinkSource{Path: path})
	settleSources(m)
	return m
}

// settleSources runs what the sources have queued, and what that
// queues, until nothing is, reporting whether it settled.
func settleSources(m *Model) bool {
	for range 100 {
		run(m, m.syncSources())
		if r := m.sources(); r.host == nil || !r.host.Pending() && r.running == 0 && !m.pagesPending() {
			return true
		}
	}
	return false
}

// pagesPending reports whether a view is being built.
func (m *Model) pagesPending() bool {
	for _, p := range m.src.pages {
		if p.Building() {
			return true
		}
	}
	return false
}

// speedSources is the Parquet source of rows rows the speed gate
// links, made once in a folder of the system's temporary one.
var speedSources = sync.OnceValues(func() (string, error) {
	dir := filepath.Join(os.TempDir(), "012-speed-sources")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return stress.SalesParquet(dir, speedSourceRows)
})

// speedSourceRows is the rows of the speed gate's source.
const speedSourceRows = 200_000
