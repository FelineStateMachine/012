package sheet

import (
	"encoding/json"
	"fmt"
	"io"
)

// FileExt is the extension of the native worksheet format.
const FileExt = ".o23"

type fileFormat struct {
	Version int               `json:"version"`
	Widths  map[string]int    `json:"widths,omitempty"`
	Cells   map[string]string `json:"cells"`
}

// Write saves the worksheet as JSON, storing each cell's input as typed.
func (s *Sheet) Write(w io.Writer) error {
	f := fileFormat{
		Version: 1,
		Widths:  make(map[string]int, len(s.widths)),
		Cells:   make(map[string]string, len(s.cells)),
	}
	for c, width := range s.widths {
		f.Widths[ColName(c)] = width
	}
	for a, c := range s.cells {
		f.Cells[a.String()] = c.Input
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(f)
}

// Read loads a worksheet written by Write.
func Read(r io.Reader) (*Sheet, error) {
	var f fileFormat
	if err := json.NewDecoder(r).Decode(&f); err != nil {
		return nil, err
	}
	if f.Version != 1 {
		return nil, fmt.Errorf("unsupported file version %d", f.Version)
	}
	s := New()
	for name, width := range f.Widths {
		c, ok := ParseCol(name)
		if !ok {
			return nil, fmt.Errorf("invalid column %q", name)
		}
		s.SetColWidth(c, width)
	}
	for name, input := range f.Cells {
		a, ok := ParseAddr(name)
		if !ok {
			return nil, fmt.Errorf("invalid cell %q", name)
		}
		if err := s.put(a, input); err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
	}
	s.RecalcAll()
	return s, nil
}
