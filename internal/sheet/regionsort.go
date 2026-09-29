package sheet

import "slices"

// Sorting a command region's table: the order is the region's, kept
// when it runs again (see region.go).

// sortedTable is the order of d's rows: the header first, then the rest
// by keys.
func sortedTable(d *RegionData, keys []SortKey) []int {
	order := make([]int, d.Rows)
	for i := range order {
		order[i] = i
	}
	if len(keys) == 0 || d.Rows < 3 {
		return order
	}
	slices.SortStableFunc(order[1:], func(i, j int) int {
		for _, k := range keys {
			a, _ := d.At(i, k.Col)
			b, _ := d.At(j, k.Col)
			if c := compareKey(a, b, k.Desc); c != 0 {
				return c
			}
		}
		return 0
	})
	return order
}

// compareKey orders two values of a sort key, blanks last either way.
func compareKey(a, b Value, desc bool) int {
	switch {
	case a.Kind == Empty && b.Kind == Empty:
		return 0
	case a.Kind == Empty:
		return 1
	case b.Kind == Empty:
		return -1
	case desc:
		return -sortCompare(a, b)
	}
	return sortCompare(a, b)
}

// SortRegion sorts a region's table below its header by keys, columns of
// the sheet, as one undo step: the order is the region's, so the table
// stays sorted when it runs again.
func (s *Sheet) SortRegion(name string, keys []SortKey) error {
	i := s.regionIndex(nameKey(name))
	if i < 0 {
		return ErrNoRegion
	}
	r := s.regions.list[i]
	if r.Linked() {
		return errFileRegion
	}
	rel := make([]SortKey, len(keys))
	for j, key := range keys {
		rel[j] = SortKey{Col: key.Col - r.At.Col, Desc: key.Desc}
	}
	s.change("sort "+r.Name, s.regionArea(r), func() {
		st := s.regionsCopy()
		st.list[i].Sort = rel
		s.putRegions(st)
	})
	return nil
}
