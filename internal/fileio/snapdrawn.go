package fileio

import "github.com/FelineStateMachine/012/internal/sheet"

// SnapChart is a chart with the values it draws, read when the
// snapshot is taken.
type SnapChart struct {
	sheet.Chart
	Values sheet.ChartData
}

// snapDrawn adds what the screen draws over the cells of snap's range:
// the looks of the cells the sheet's rules touch, and the charts
// anchored in the range, or all of them for a whole sheet. Looks cost a cell's rules, and only sheets
// with rules have any.
func snapDrawn(snap *Snapshot, s *sheet.Sheet, whole bool) {
	if s.HasRules() {
		for a := range snap.Cells {
			if l := s.Look(a); l != (sheet.Look{}) {
				if snap.Looks == nil {
					snap.Looks = map[sheet.Addr]sheet.Look{}
				}
				snap.Looks[a] = l
			}
		}
	}
	for _, c := range s.Charts() {
		if whole || snap.Range.Contains(c.At) {
			snap.Charts = append(snap.Charts, SnapChart{Chart: c, Values: s.ChartData(c)})
		}
	}
}
