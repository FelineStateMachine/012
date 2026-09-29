package sheet

import (
	"fmt"
	"slices"
	"strings"
)

// The regions' dependency graph: a command reading $r1 depends on region
// r1, so refreshing r1 runs it again and then every region that reads
// it, each after what it reads. A command can't read itself, directly
// or through others.

// RegionRefs returns the names a command reads as $name, in the order it
// first names them, leaving out nushell's own variables.
func RegionRefs(command string) []string {
	var out []string
	for i := 0; i < len(command); i++ {
		if command[i] != '$' {
			continue
		}
		j := i + 1
		for j < len(command) && (isLetter(command[j]) || isDigit(command[j]) || command[j] == '_') {
			j++
		}
		name := command[i+1 : j]
		if name != "" && ValidRegionName(name) == nil && !slices.Contains(out, name) {
			out = append(out, name)
		}
		i = j - 1
	}
	return out
}

// RegionDeps returns the regions a command reads: the names in it that
// regions of the workbook have, as the regions spell them.
func (w *Workbook) RegionDeps(command string) []string {
	var out []string
	for _, name := range RegionRefs(command) {
		if _, r, ok := w.Region(name); ok && r.Name == name {
			out = append(out, r.Name)
		}
	}
	return out
}

// allRegions returns every region of the workbook's sheets, sheet by
// sheet in run order.
func (w *Workbook) allRegions() []Region {
	var out []Region
	for _, s := range w.sheets {
		out = append(out, s.regions.list...)
	}
	return out
}

// checkDeps refuses deps for the region name when one of them reads it,
// directly or through others.
func (w *Workbook) checkDeps(name string, deps []string) error {
	k := nameKey(name)
	byKey := map[string]Region{}
	for _, r := range w.allRegions() {
		byKey[nameKey(r.Name)] = r
	}
	seen := map[string]bool{}
	var reads func(d string) bool
	reads = func(d string) bool {
		dk := nameKey(d)
		if dk == k {
			return true
		}
		if seen[dk] {
			return false
		}
		seen[dk] = true
		return slices.ContainsFunc(byKey[dk].Deps, reads)
	}
	for _, d := range deps {
		if reads(d) {
			return fmt.Errorf("%s can't read $%s, which reads %s: that would be a cycle", name, d, name)
		}
	}
	return nil
}

// RefreshOrder returns the regions to run when name is refreshed: name
// (unless it's a linked file, which doesn't run),
// then every region that reads it, directly or through others, each
// after the regions it reads.
func (w *Workbook) RefreshOrder(name string) []string {
	regions := w.allRegions()
	want := map[string]bool{nameKey(name): true}
	for changed := true; changed; {
		changed = false
		for _, r := range regions {
			k := nameKey(r.Name)
			if !want[k] && slices.ContainsFunc(r.Deps, func(d string) bool { return want[nameKey(d)] }) {
				want[k], changed = true, true
			}
		}
	}
	commands(regions, want)
	return w.ordered(regions, want)
}

// commands leaves out of want the linked files, which don't run.
func commands(regions []Region, want map[string]bool) {
	for _, r := range regions {
		if r.Linked() {
			delete(want, nameKey(r.Name))
		}
	}
}

// RunOrder returns every region, each after the regions it reads, in run
// order otherwise: what Run all runs.
func (w *Workbook) RunOrder() []string {
	regions := w.allRegions()
	want := map[string]bool{}
	for _, r := range regions {
		want[nameKey(r.Name)] = true
	}
	commands(regions, want)
	return w.ordered(regions, want)
}

// ordered lists the regions want names, each after those it reads among
// them, otherwise in run order.
func (w *Workbook) ordered(regions []Region, want map[string]bool) []string {
	var out []string
	done := map[string]bool{}
	for len(out) < len(want) {
		progress := false
		for _, r := range regions {
			k := nameKey(r.Name)
			if !want[k] || done[k] {
				continue
			}
			ready := !slices.ContainsFunc(r.Deps, func(d string) bool { dk := nameKey(d); return want[dk] && !done[dk] })
			if ready {
				out, done[k], progress = append(out, r.Name), true, true
			}
		}
		if !progress {
			break // a cycle a file made: what's left doesn't run
		}
	}
	return out
}

// DependsOn returns the regions that read name directly, for saying what
// deleting or freezing it leaves without input.
func (w *Workbook) DependsOn(name string) []string {
	var out []string
	for _, r := range w.allRegions() {
		if slices.ContainsFunc(r.Deps, func(d string) bool { return strings.EqualFold(d, name) }) {
			out = append(out, r.Name)
		}
	}
	return out
}
