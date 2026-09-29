package nuon

import (
	"os/exec"
	"strings"
	"testing"
)

// nu runs a nushell pipeline with input on its standard input, skipping
// the test when nushell isn't installed.
func nu(t *testing.T, script, input string) string {
	t.Helper()
	path, err := exec.LookPath("nu")
	if err != nil {
		t.Skip("nu isn't installed")
	}
	cmd := exec.Command(path, "--no-config-file", "--stdin", "-c", script)
	cmd.Stdin = strings.NewReader(input)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("nu -c %q: %v\n%s", script, err, out)
	}
	return strings.TrimSpace(string(out))
}

// nuTables are pipelines whose output covers nushell's types.
var nuTables = []string{
	`[[name, size, modified]; ["CLAUDE.md", 1646b, 2026-09-27T11:27:31.351890163-06:00], [go.mod, 1.5kb, 2026-01-02]]`,
	`[{a: 1, b: 1.5, c: true, d: null, e: "x y", f: 90sec, g: 0x[DEAD]}, {a: -2, h: [1, 2], i: {j: k}}]`,
	`[[s]; ["true"], ["12"], [""], ["a\"b\nc"], ["-"], [TRUE], ["2026-01-01"], ["1kb"], ["#x"], ["ü"]]`,
	`{name: 012, when: (1day + 2hr), inf: inf, big: 1e20, small: 1e-7}`,
	`seq 1 5 | each {|i| {i: $i, sq: ($i * $i), half: ($i / 2)}}`,
	`[]`,
}

// TestRoundTripThroughNu reads what nushell writes and writes it back:
// nushell must read the same values from our text as from its own.
func TestRoundTripThroughNu(t *testing.T) {
	for _, pipeline := range nuTables {
		theirs := nu(t, pipeline+" | to nuon", "")
		v, err := Parse([]byte(theirs))
		if err != nil {
			t.Errorf("%s: parsing nu's %s: %v", pipeline, theirs, err)
			continue
		}
		ours := v.String()
		if back := nu(t, "from nuon | to nuon", ours); back != theirs {
			t.Errorf("%s:\nnu wrote  %s\nwe wrote  %s\nnu reread %s", pipeline, theirs, ours, back)
		}
	}
}

// TestReaderReadsNuTables reads nushell's tables row by row and writes
// them with a TableWriter, which nushell reads back as the same table.
func TestReaderReadsNuTables(t *testing.T) {
	for _, pipeline := range nuTables[:3] {
		theirs := nu(t, pipeline+" | to nuon", "")
		rows, _ := rows(t, theirs)
		cols := []string{}
		for _, f := range rows[0] {
			cols = append(cols, f.Key)
		}
		if len(rows[0]) != len(rows[len(rows)-1]) {
			continue // a table whose rows differ is written as records
		}
		var b strings.Builder
		w := NewTableWriter(&b, cols)
		for _, r := range rows {
			vs := make([]Value, len(r))
			for i, f := range r {
				vs[i] = f.Value
			}
			w.Write(vs)
		}
		w.Close()
		if back := nu(t, "from nuon | to nuon", b.String()); back != theirs {
			t.Errorf("%s:\nnu wrote  %s\nwe wrote  %s\nnu reread %s", pipeline, theirs, b.String(), back)
		}
	}
}

// TestJSONThroughNu writes JSON that nushell reads as it reads its own.
func TestJSONThroughNu(t *testing.T) {
	theirs := nu(t, nuTables[1]+" | to json -r", "")
	v, err := Parse([]byte(nu(t, nuTables[1]+" | to nuon", "")))
	if err != nil {
		t.Fatal(err)
	}
	if ours := string(AppendJSON(nil, v)); ours != theirs {
		t.Errorf("nu wrote %s\nwe wrote %s", theirs, ours)
	}
}
