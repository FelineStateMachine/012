package sheet

import (
	"bufio"
	"encoding/json"
	"fmt"
)

// fileRegion is a region in the file: its definition, one line per
// region after the cells of its sheet, never its table, which its
// command makes again when it runs or its file gives again when it's
// read. They need no version: earlier builds ignore them and open the
// sheet without its regions.
//
//	"notebook": true,
//	"regions": [
//	  {"name": "r1", "command": "ls", "at": "A1", "rows": 12, "cols": 4},
//	  {"name": "big", "command": "$r1 | where size > 1kb", "at": "A16", "rows": 3, "cols": 4, "reads": ["r1"]},
//	  {"name": "app", "at": "F1", "path": "logs/app.csv", "window": 500}
//	]
type fileRegion struct {
	Name    string           `json:"name"`
	Command string           `json:"command,omitempty"`
	At      string           `json:"at"`
	Rows    int              `json:"rows,omitempty"`
	Cols    int              `json:"cols,omitempty"`
	Reads   []string         `json:"reads,omitempty"`
	Input   string           `json:"input,omitempty"`
	Sort    []fileRegionSort `json:"sort,omitempty"`
	// A linked file's: see linked.go.
	Path   string `json:"path,omitempty"`
	Format string `json:"format,omitempty"`
	Table  string `json:"table,omitempty"`
	Query  string `json:"query,omitempty"`
	Window int    `json:"window,omitempty"`
}

// fileRegionSort is a column a region's table is sorted by, counted from
// 1 at the region's first column.
type fileRegionSort struct {
	Column int  `json:"column"`
	Desc   bool `json:"desc,omitempty"`
}

// writeRegionDefs writes the notebook flag and the regions, one per line.
func (s *Sheet) writeRegionDefs(b *bufio.Writer, indent string) error {
	if s.regions.notebook {
		b.WriteString(",\n" + indent + `"notebook": true`)
	}
	if len(s.regions.list) == 0 {
		return nil
	}
	b.WriteString(",\n" + indent + `"regions": [`)
	for i, r := range s.regions.list {
		fr := fileRegion{Name: r.Name, Command: r.Command, At: r.At.String(), Rows: r.Rows, Cols: r.Cols, Reads: r.Deps, Input: r.Input,
			Path: r.File.Path, Format: r.File.Format, Table: r.File.Table, Query: r.File.Query, Window: r.File.Window}
		for _, k := range r.Sort {
			fr.Sort = append(fr.Sort, fileRegionSort{Column: k.Col + 1, Desc: k.Desc})
		}
		raw, err := json.Marshal(fr)
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

// readRegions reads a sheet's regions, not yet shown.
func (s *Sheet) readRegions(notebook bool, frs []fileRegion) error {
	s.regions.notebook = notebook
	for _, fr := range frs {
		r, err := decodeRegion(fr)
		if err != nil {
			return err
		}
		if _, _, dup := s.wb.Region(r.Name); dup || s.regionIndex(nameKey(r.Name)) >= 0 {
			return fmt.Errorf("region %q is defined twice", r.Name)
		}
		s.regions.list = append(s.regions.list, r)
	}
	if len(s.regions.list) > 0 {
		s.regions.data = map[string]*RegionData{}
		s.regionsStale = true
	}
	for _, r := range s.regions.list {
		if r.Linked() {
			s.meta(nameKey(r.Name)).stale = true
		}
	}
	return nil
}

func decodeRegion(fr fileRegion) (Region, error) {
	if err := ValidRegionName(fr.Name); err != nil {
		return Region{}, fmt.Errorf("region %q: %w", fr.Name, err)
	}
	at, ok := ParseAddr(fr.At)
	if !ok {
		return Region{}, fmt.Errorf("region %s: invalid cell %q", fr.Name, fr.At)
	}
	if fr.Rows < 0 || fr.Cols < 0 || at.Row+fr.Rows >= MaxRows || at.Col+fr.Cols > MaxCols {
		return Region{}, fmt.Errorf("region %s: invalid size %d by %d", fr.Name, fr.Rows, fr.Cols)
	}
	r := Region{Name: fr.Name, Command: fr.Command, At: at, Rows: fr.Rows, Cols: fr.Cols, Input: fr.Input,
		File: LinkSource{Path: fr.Path, Format: fr.Format, Table: fr.Table, Query: fr.Query, Window: fr.Window}}
	switch {
	case fr.Window < 0:
		return Region{}, fmt.Errorf("region %s: invalid window %d", fr.Name, fr.Window)
	case r.Linked() && (r.Command != "" || len(fr.Reads) > 0 || len(fr.Sort) > 0 || r.Input != ""):
		return Region{}, fmt.Errorf("region %s: a linked file has no command", fr.Name)
	case r.Linked():
		r.Rows, r.Cols = 0, 0 // its size is the file's
	}
	for _, d := range fr.Reads {
		if err := ValidRegionName(d); err != nil {
			return Region{}, fmt.Errorf("region %s reads %q: %w", fr.Name, d, err)
		}
		r.Deps = append(r.Deps, d)
	}
	for _, k := range fr.Sort {
		if k.Column < 1 || k.Column > MaxCols {
			return Region{}, fmt.Errorf("region %s: invalid sort column %d", fr.Name, k.Column)
		}
		r.Sort = append(r.Sort, SortKey{Col: k.Column - 1, Desc: k.Desc})
	}
	return r, nil
}
