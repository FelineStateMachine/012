package sheet

import (
	"bufio"
	"encoding/json"
	"fmt"
)

// fileLink is a linked region in the file: its anchor and its source,
// one per line after the charts, never its rows, which are read from the
// source again on opening. Older builds ignore the field and show the
// region's cells empty, so it needs no version:
//
//	"links": [
//	  {"at": "A1", "path": "logs/app.csv", "window": 500}
//	]
type fileLink struct {
	At     string `json:"at"`
	Path   string `json:"path"`
	Format string `json:"format,omitempty"`
	Table  string `json:"table,omitempty"`
	Query  string `json:"query,omitempty"`
	Window int    `json:"window,omitempty"`
}

// writeLinks writes the sheet's linked regions.
func (s *Sheet) writeLinks(b *bufio.Writer, indent string) error {
	if len(s.links) == 0 {
		return nil
	}
	b.WriteString(",\n" + indent + `"links": [`)
	for i, l := range s.links {
		raw, err := json.Marshal(fileLink{At: l.anchor.String(), Path: l.src.Path, Format: l.src.Format,
			Table: l.src.Table, Query: l.src.Query, Window: l.src.Window})
		if err != nil {
			return err
		}
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(b, "\n%s  %s", indent, raw)
	}
	b.WriteString("\n" + indent + "]")
	return nil
}

// readLinks makes the file's linked regions, empty and stale for the UI
// to read.
func (s *Sheet) readLinks(links []fileLink) error {
	for i, fl := range links {
		a, ok := ParseAddr(fl.At)
		switch {
		case !ok:
			return fmt.Errorf("link %d: invalid cell %q", i+1, fl.At)
		case fl.Path == "":
			return fmt.Errorf("link %d: no path", i+1)
		case fl.Window < 0:
			return fmt.Errorf("link %d: invalid window %d", i+1, fl.Window)
		case s.linkedAt(a) != nil:
			return fmt.Errorf("link %d: %s is linked twice", i+1, fl.At)
		}
		s.wb.linkSeq++
		s.links = append(s.links, &linked{id: s.wb.linkSeq, anchor: a, liveState: &liveState{stale: true},
			src: LinkSource{Path: fl.Path, Format: fl.Format, Table: fl.Table, Query: fl.Query, Window: fl.Window}})
	}
	return nil
}

// LinkOrigin identifies the computer the workbook's linked regions were
// made or trusted on, as saved in the file; "" when unknown. The UI asks
// before following files outside the workbook's folder for a workbook
// made elsewhere, as it does before running its macros.
func (w *Workbook) LinkOrigin() string { return w.linkOrigin }

// SetLinkOrigin records where the linked regions were made or trusted.
// It isn't an edit, and is saved with the next save.
func (w *Workbook) SetLinkOrigin(origin string) { w.linkOrigin = origin }
