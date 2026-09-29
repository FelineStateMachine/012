package sheet

import (
	"bufio"
	"encoding/json"
	"fmt"
)

// fileRegion is a region in the file: its definition, one line per
// region after the cells of its sheet, never its table, which its
// source sends again when the file is opened. They need no version:
// earlier builds ignore them and open the sheet without its regions.
//
//	"regions": [
//	  {"name": "sales", "at": "A1", "output": true},
//	  {"name": "app", "at": "F1", "path": "logs/app.csv", "window": 500}
//	]
//
// A region with a command is a notebook sheet's region as earlier
// builds wrote them, with its label line at "at" and its table under
// it; opening the file converts it (convertOld).
type fileRegion struct {
	Name string `json:"name"`
	At   string `json:"at"`
	// A sent output's.
	Output bool `json:"output,omitempty"`
	// A linked file's: see linked.go.
	Path   string `json:"path,omitempty"`
	Format string `json:"format,omitempty"`
	Table  string `json:"table,omitempty"`
	Query  string `json:"query,omitempty"`
	Window int    `json:"window,omitempty"`
	// A command region's, read to convert it.
	Command string           `json:"command,omitempty"`
	Input   string           `json:"input,omitempty"`
	Rows    int              `json:"rows,omitempty"`
	Cols    int              `json:"cols,omitempty"`
	Reads   []string         `json:"reads,omitempty"`
	Sort    []fileRegionSort `json:"sort,omitempty"`
}

// fileRegionSort is a column a command region's table was sorted by.
type fileRegionSort struct {
	Column int  `json:"column"`
	Desc   bool `json:"desc,omitempty"`
}

// writeRegionDefs writes the regions, one per line.
func (s *Sheet) writeRegionDefs(b *bufio.Writer, indent string) error {
	if len(s.regions.list) == 0 {
		return nil
	}
	b.WriteString(",\n" + indent + `"regions": [`)
	for i, r := range s.regions.list {
		fr := fileRegion{Name: r.Name, At: r.At.String(), Output: r.Output,
			Path: r.File.Path, Format: r.File.Format, Table: r.File.Table, Query: r.File.Query, Window: r.File.Window}
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

// readRegions reads a sheet's regions, stale until their sources send
// their rows, adding the command regions of earlier builds to old.
func (s *Sheet) readRegions(frs []fileRegion, old *[]oldRegion) error {
	for _, fr := range frs {
		if err := ValidRegionName(fr.Name); err != nil {
			return fmt.Errorf("region %q: %w", fr.Name, err)
		}
		if _, _, dup := s.wb.Region(fr.Name); dup || s.regionIndex(nameKey(fr.Name)) >= 0 || s.wb.converting(*old, fr.Name) {
			return fmt.Errorf("region %q is defined twice", fr.Name)
		}
		at, ok := ParseAddr(fr.At)
		if !ok {
			return fmt.Errorf("region %s: invalid cell %q", fr.Name, fr.At)
		}
		r := Region{Name: fr.Name, At: at, Output: fr.Output,
			File: LinkSource{Path: fr.Path, Format: fr.Format, Table: fr.Table, Query: fr.Query, Window: fr.Window}}
		switch {
		case fr.Window < 0:
			return fmt.Errorf("region %s: invalid window %d", fr.Name, fr.Window)
		case r.Linked() && (r.Output || fr.Command != ""):
			return fmt.Errorf("region %s: a linked file has no command", fr.Name)
		case r.Output && fr.Command != "":
			return fmt.Errorf("region %s: a sent output has no command", fr.Name)
		case !r.Linked() && !r.Output:
			*old = append(*old, oldRegion{s, fr})
			continue
		}
		s.regions.list = append(s.regions.list, r)
		s.meta(nameKey(r.Name)).stale = true
		s.regionsStale = true
	}
	return nil
}

// converting reports whether name is among the command regions being
// converted.
func (w *Workbook) converting(old []oldRegion, name string) bool {
	for _, o := range old {
		if nameKey(o.fr.Name) == nameKey(name) {
			return true
		}
	}
	return false
}
