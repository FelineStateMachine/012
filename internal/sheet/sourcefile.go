package sheet

import (
	"errors"
	"fmt"
)

// A linked source in the file (source.go): its tab is a sheet whose one
// region is paged, at A1, with no cells. The region keeps what the
// source reads and how its tab sorts and filters it, never its rows:
//
//	"regions": [
//	  {"name":"sales","at":"A1","path":"sales.parquet","paged":true,
//	   "order":{"sort":[{"column":"C","desc":true}],"filter":[{"column":"B","condition":"eq","value":"North"}]}}
//	]
//
// Sources need version 7: a build without them would read the region
// as a linked file and load the source into cells.

// fileSourceOrder is a source's SourceOrder, columns by letter.
type fileSourceOrder struct {
	Sort   []fileSourceSort   `json:"sort,omitempty"`
	Filter []fileSourceFilter `json:"filter,omitempty"`
}

type fileSourceSort struct {
	Column string `json:"column"`
	Desc   bool   `json:"desc,omitempty"`
}

type fileSourceFilter struct {
	Column    string `json:"column"`
	Condition string `json:"condition"`
	Value     string `json:"value,omitempty"`
}

// encodeOrder is o as the file keeps it, nil for the source's own.
func encodeOrder(o *SourceOrder) *fileSourceOrder {
	if o.IsZero() {
		return nil
	}
	fo := &fileSourceOrder{}
	for _, s := range o.Sort {
		fo.Sort = append(fo.Sort, fileSourceSort{Column: ColName(s.Col), Desc: s.Desc})
	}
	for _, f := range o.Filter {
		fo.Filter = append(fo.Filter, fileSourceFilter{Column: ColName(f.Col), Condition: f.Cond.Op.String(), Value: f.Cond.Arg})
	}
	return fo
}

// decodeOrder reads what encodeOrder wrote.
func decodeOrder(fo *fileSourceOrder) (*SourceOrder, error) {
	if fo == nil {
		return nil, nil
	}
	o := &SourceOrder{}
	for _, s := range fo.Sort {
		c, ok := ParseCol(s.Column)
		if !ok {
			return nil, fmt.Errorf("invalid sort column %q", s.Column)
		}
		o.Sort = append(o.Sort, SourceSort{Col: c, Desc: s.Desc})
	}
	for _, f := range fo.Filter {
		c, ok := ParseCol(f.Column)
		if !ok {
			return nil, fmt.Errorf("invalid filter column %q", f.Column)
		}
		op, ok := ParseCondOp(f.Condition)
		if !ok || op == CondNone {
			return nil, fmt.Errorf("invalid filter condition %q", f.Condition)
		}
		o.Filter = append(o.Filter, SourceFilter{Col: c, Cond: Condition{Op: op, Arg: f.Value}})
	}
	if o.IsZero() {
		return nil, nil
	}
	return o, nil
}

// readSource reads a linked source's region, r as far as it's read.
func (s *Sheet) readSource(fr fileRegion, r Region) error {
	switch {
	case !fr.Paged:
		return errors.New("only a linked source has an order")
	case fr.Path == "" || fr.Output || fr.Command != "" || fr.Window != 0:
		return errors.New("a linked source reads a file, all of it")
	}
	o, err := decodeOrder(fr.Order)
	if err != nil {
		return err
	}
	r.File.Paged, r.File.Order = true, o
	s.regions.list = append(s.regions.list, r)
	return nil
}

// errSourceTab is why a sheet can't be a source's tab as the file has
// it.
var errSourceTab = errors.New("a linked source's tab holds its source alone, at A1")

// checkSourceTab checks a sheet read from a file with a paged region:
// the region is its only one, at A1, and it has no cells.
func (s *Sheet) checkSourceTab() error {
	if !s.IsSource() {
		return nil
	}
	if len(s.regions.list) != 1 || s.regions.list[0].At != (Addr{}) || s.cells.len() > 0 || s.regions.notebook {
		return errSourceTab
	}
	return nil
}
