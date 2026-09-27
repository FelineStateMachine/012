// Command benchrec turns `go test -bench` output into run records, one
// JSON line per benchmark, appended to a results file that DuckDB (and
// ClickHouse) read; see scripts/stress and docs/observability.md. It also
// prints a compact table of the run.
//
//	go test -bench . -benchmem ./... | go run ./internal/stress/benchrec -out runs.jsonl
//
// GIT_SHA and GIT_DIRTY describe the code measured; the rest is read
// from the output and the machine.
package main

import (
	"bufio"
	"cmp"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"maps"
	"os"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"
)

// Record is one benchmark result of one run. The same columns are the
// stress_results table in ClickHouse (deploy/observability).
type Record struct {
	RunID     string             `json:"run_id"`
	TS        string             `json:"ts"`
	GitSHA    string             `json:"git_sha"`
	GitDirty  bool               `json:"git_dirty"`
	GoVersion string             `json:"go_version"`
	GOOS      string             `json:"goos"`
	GOARCH    string             `json:"goarch"`
	CPU       string             `json:"cpu"`
	Host      string             `json:"host"`
	Pkg       string             `json:"pkg"`
	Name      string             `json:"name"` // without the -GOMAXPROCS suffix
	Procs     int                `json:"procs"`
	N         int64              `json:"n"`
	NsOp      float64            `json:"ns_op"`
	BOp       float64            `json:"b_op"`
	AllocsOp  float64            `json:"allocs_op"`
	MBs       float64            `json:"mb_s"`
	Metrics   map[string]float64 `json:"metrics"` // custom b.ReportMetric units
}

func main() {
	out := flag.String("out", "", "append records to this JSONL file")
	flag.Parse()
	recs, err := parse(os.Stdin, meta())
	if err != nil {
		fmt.Fprintln(os.Stderr, "benchrec:", err)
		os.Exit(1)
	}
	if len(recs) == 0 {
		fmt.Fprintln(os.Stderr, "benchrec: no benchmark results in the input")
		os.Exit(1)
	}
	if *out != "" {
		if err := appendJSONL(*out, recs); err != nil {
			fmt.Fprintln(os.Stderr, "benchrec:", err)
			os.Exit(1)
		}
	}
	summary(os.Stdout, recs)
	if *out != "" {
		fmt.Printf("\n%d results of run %s appended to %s\n", len(recs), recs[0].RunID, *out)
	}
}

// meta describes the run: when, which code, which machine.
func meta() Record {
	now := time.Now().UTC()
	host, _ := os.Hostname()
	sha := os.Getenv("GIT_SHA")
	r := Record{
		TS: now.Format(time.RFC3339), GitSHA: sha, GitDirty: os.Getenv("GIT_DIRTY") == "1",
		GoVersion: runtime.Version(), GOOS: runtime.GOOS, GOARCH: runtime.GOARCH, Host: host,
	}
	r.RunID = now.Format("20060102T150405Z") + "-" + cmp.Or(sha, "unknown")
	return r
}

var benchLine = regexp.MustCompile(`^(Benchmark\S+?)(?:-(\d+))?\s+(\d+)\s+(.*)$`)

// parse reads benchmark output: "pkg:" and "cpu:" headers, then result
// lines of an iteration count and value-unit pairs.
func parse(r io.Reader, base Record) ([]Record, error) {
	var out []Record
	pkg, cpu := "", ""
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "pkg: "):
			pkg = strings.TrimPrefix(line, "pkg: ")
			continue
		case strings.HasPrefix(line, "cpu: "):
			cpu = strings.TrimPrefix(line, "cpu: ")
			continue
		}
		m := benchLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		rec := base
		rec.Pkg, rec.CPU, rec.Name = pkg, cpu, strings.TrimPrefix(m[1], "Benchmark")
		rec.Procs, _ = strconv.Atoi(m[2])
		rec.N, _ = strconv.ParseInt(m[3], 10, 64)
		rec.Metrics = map[string]float64{}
		f := strings.Fields(m[4])
		for i := 0; i+1 < len(f); i += 2 {
			v, err := strconv.ParseFloat(f[i], 64)
			if err != nil {
				return nil, fmt.Errorf("%s: value %q", line, f[i])
			}
			switch unit := f[i+1]; unit {
			case "ns/op":
				rec.NsOp = v
			case "B/op":
				rec.BOp = v
			case "allocs/op":
				rec.AllocsOp = v
			case "MB/s":
				rec.MBs = v
			default:
				rec.Metrics[unit] = v
			}
		}
		out = append(out, rec)
	}
	return out, sc.Err()
}

func appendJSONL(path string, recs []Record) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(f)
	for _, r := range recs {
		if err := enc.Encode(r); err != nil {
			f.Close()
			return err
		}
	}
	return f.Close()
}

// summary prints one line per benchmark: time per op in readable units,
// allocations, and custom metrics.
func summary(w io.Writer, recs []Record) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "benchmark\ttime/op\tallocs/op\tmem/op\tother\t\n")
	pkg := ""
	for _, r := range recs {
		if r.Pkg != pkg {
			pkg = r.Pkg
			fmt.Fprintf(tw, "%s\t\t\t\t\t\n", pkg)
		}
		var other []string
		if r.MBs > 0 {
			other = append(other, fmt.Sprintf("%.1f MB/s", r.MBs))
		}
		for _, k := range slices.Sorted(maps.Keys(r.Metrics)) {
			other = append(other, fmt.Sprintf("%s %s", num(r.Metrics[k]), k))
		}
		fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\t%s\t\n", r.Name, duration(r.NsOp), num(r.AllocsOp), bytes(r.BOp), strings.Join(other, ", "))
	}
	tw.Flush()
}

func duration(ns float64) string {
	switch {
	case ns >= 1e9:
		return fmt.Sprintf("%.2f s", ns/1e9)
	case ns >= 1e6:
		return fmt.Sprintf("%.2f ms", ns/1e6)
	case ns >= 1e3:
		return fmt.Sprintf("%.1f us", ns/1e3)
	}
	return fmt.Sprintf("%.0f ns", ns)
}

func bytes(b float64) string {
	switch {
	case b >= 1<<30:
		return fmt.Sprintf("%.1f GB", b/(1<<30))
	case b >= 1<<20:
		return fmt.Sprintf("%.1f MB", b/(1<<20))
	case b >= 1<<10:
		return fmt.Sprintf("%.1f KB", b/(1<<10))
	}
	return fmt.Sprintf("%.0f B", b)
}

func num(v float64) string {
	switch {
	case v >= 1e6:
		return fmt.Sprintf("%.1fM", v/1e6)
	case v >= 1e4:
		return fmt.Sprintf("%.0fk", v/1e3)
	case v == float64(int64(v)):
		return strconv.FormatInt(int64(v), 10)
	}
	return strconv.FormatFloat(v, 'f', 1, 64)
}
