package diff

import "sort"

// minSimilar is how alike two sheets' cells must be, as the share of
// cells they hold the same, for a sheet gone under one name and a sheet
// come under another to count as one sheet renamed.
const minSimilar = 0.5

// pairing says which sheets of two workbooks are the same sheet: by
// index in a and in b.
type pairing struct {
	pairs [][2]int // matched, in a's order
	onlyA []int    // a's sheets without a match: removed
	onlyB []int    // b's sheets without a match: added
}

// matchSheets pairs a's sheets with b's: first by name, ignoring case,
// then each sheet left in a with the most alike left in b, when they
// share at least half their cells (a rename, perhaps with edits).
func matchSheets(a, b *rawBook) pairing {
	var p pairing
	usedB := make([]bool, len(b.sheets))
	var leftA []int
	for i, s := range a.sheets {
		j := b.sheetIndex(s.name)
		if j < 0 || usedB[j] {
			leftA = append(leftA, i)
			continue
		}
		usedB[j] = true
		p.pairs = append(p.pairs, [2]int{i, j})
	}
	type candidate struct {
		i, j  int
		score float64
	}
	var cands []candidate
	for _, i := range leftA {
		for j := range b.sheets {
			if !usedB[j] {
				if sc := similarity(a.sheets[i], b.sheets[j]); sc >= minSimilar {
					cands = append(cands, candidate{i, j, sc})
				}
			}
		}
	}
	sort.SliceStable(cands, func(x, y int) bool { return cands[x].score > cands[y].score })
	usedA := map[int]bool{}
	for _, c := range cands {
		if !usedA[c.i] && !usedB[c.j] {
			usedA[c.i], usedB[c.j] = true, true
			p.pairs = append(p.pairs, [2]int{c.i, c.j})
		}
	}
	sort.Slice(p.pairs, func(x, y int) bool { return p.pairs[x][0] < p.pairs[y][0] })
	for _, i := range leftA {
		if !usedA[i] {
			p.onlyA = append(p.onlyA, i)
		}
	}
	for j := range b.sheets {
		if !usedB[j] {
			p.onlyB = append(p.onlyB, j)
		}
	}
	return p
}

// similarity is the share of cells two sheets hold the same, 1 for two
// empty sheets.
func similarity(a, b *rawSheet) float64 {
	n := max(len(a.cells), len(b.cells))
	if n == 0 {
		return 1
	}
	same := 0
	for k, v := range a.cells {
		if equal(v, b.cells[k]) {
			same++
		}
	}
	return float64(same) / float64(n)
}

// sameSheet reports whether two sheets are alike in every field, their
// names included.
func sameSheet(a, b *rawSheet) bool {
	if a.name != b.name || len(a.cells) != len(b.cells) || len(a.fields) != len(b.fields) {
		return false
	}
	for k, v := range a.cells {
		if !equal(v, b.cells[k]) {
			return false
		}
	}
	for k, v := range a.fields {
		if !equal(v, b.fields[k]) {
			return false
		}
	}
	return true
}
