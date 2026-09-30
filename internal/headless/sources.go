package headless

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/FelineStateMachine/012/internal/paged"
	"github.com/FelineStateMachine/012/internal/sheet"
)

// ReadSources answers what the workbook's formulas and pivot tables ask
// of its linked sources, reading them to the end before it returns, as
// the screen does in the background. Only sources in the workbook's
// folder or below are read: a path out of it (absolute, or through ..)
// shows why it wasn't, as the screen asks first about such paths in a
// workbook from another computer.
func (f *File) ReadSources(ctx context.Context) {
	w := f.Book
	if len(w.Sources()) == 0 {
		return
	}
	dir := filepath.Dir(f.Path)
	h := paged.NewHost(sheet.MaxCells())
	defer h.Close()
	var inside []paged.Linked
	for _, l := range paged.Links(w, func(p string) string { return p }, "") {
		if p := l.Spec.Path; filepath.IsAbs(p) || p == ".." || strings.HasPrefix(p, ".."+string(filepath.Separator)) {
			w.SetSourceShape(l.Name, sheet.SourceShape{}, "Not read: outside the workbook's folder")
			continue
		}
		l.Spec.Path = filepath.Join(dir, l.Spec.Path)
		inside = append(inside, l)
	}
	h.Settle(ctx, w, inside)
}
