package nushell

import (
	"context"
	"errors"
	"io"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// fake prints out, or fails with err, and keeps the script it was given.
type fake struct {
	out    string
	err    error
	script string
	job    Job
}

func (f *fake) Run(ctx context.Context, job Job, script string, stdout io.Writer) error {
	f.script, f.job = script, job
	if f.err != nil {
		return f.err
	}
	_, err := io.WriteString(stdout, f.out)
	return err
}

func cells(d *sheet.RegionData) [][]string {
	var out [][]string
	for r := range d.Rows {
		var row []string
		for c := range d.Cols {
			v, f := d.At(r, c)
			row = append(row, sheet.FormatText(v, f))
		}
		out = append(out, row)
	}
	return out
}

func TestScript(t *testing.T) {
	got := Script(Job{Command: "$r1 | where size > 1kb", Tables: map[string][]byte{"r1": nil}, Input: []byte("[]")}, []string{"big", "r1"})
	want := "let big = (open --raw $env.NU012_TABLE_0 | from nuon)\n" +
		"let r1 = (open --raw $env.NU012_TABLE_1 | from nuon)\n" +
		"$in | from nuon | do {\n$r1 | where size > 1kb\n} | to nuon\n"
	if got != want {
		t.Errorf("script:\n%s\nwant:\n%s", got, want)
	}
}

func TestExecReadsTables(t *testing.T) {
	for _, c := range []struct {
		out  string
		want string
	}{
		{"[[name, size]; [a, 1kb], [b, 2b]]", "name size|a 1.0 kB|b 2 B"},
		{"{a: 1, b: true}", "a b|1 TRUE"},
		{"3", "value|3"},
		{`"hello"`, "value|hello"},
		{"null", "value"},
		{"[1, 2]", "value|1|2"},
		{"", ""},
	} {
		d, err := Exec(context.Background(), &fake{out: c.out}, Job{Command: "x"}, time.Second, 0)
		if err != nil {
			t.Errorf("%q: %v", c.out, err)
			continue
		}
		var rows []string
		for _, r := range cells(d) {
			rows = append(rows, strings.TrimSpace(strings.Join(r, " ")))
		}
		if got := strings.Join(rows, "|"); got != c.want {
			t.Errorf("%q read as %q, want %q", c.out, got, c.want)
		}
	}
}

func TestExecFails(t *testing.T) {
	_, err := Exec(context.Background(), &fake{err: &Error{Msg: "Cannot find column 'x'"}}, Job{Command: "x"}, time.Second, 0)
	var e *Error
	if !errors.As(err, &e) || e.Msg != "Cannot find column 'x'" {
		t.Errorf("error %v", err)
	}
	_, err = Exec(context.Background(), &fake{out: "[[a]; [1"}, Job{Command: "x"}, time.Second, 0)
	if err == nil || !strings.Contains(err.Error(), "isn't NUON") {
		t.Errorf("bad output: %v", err)
	}
	// Past max-cells, whole rows are kept and the note says so.
	d, err := Exec(context.Background(), &fake{out: "[[a, b]; [1, 2], [3, 4], [5, 6]]"}, Job{Command: "x"}, time.Second, 4)
	if err != nil || d.Rows != 2 || d.Note == "" {
		t.Errorf("capped: %+v %v", d, err)
	}
}

func TestMessage(t *testing.T) {
	stderr := "Error: nu::shell::column_not_found\n\n  × Cannot find column 'xyz'\n   ╭─[source:1:6]\n"
	if got := Message(stderr, errors.New("exit status 1")); got != "Cannot find column 'xyz'" {
		t.Errorf("message %q", got)
	}
	if got := Message("", errors.New("exit status 1")); got != "exit status 1" {
		t.Errorf("message %q", got)
	}
}

// The real nu, when it's installed: tables go in as variables and $in,
// and come back typed.
func TestNu(t *testing.T) {
	if _, err := exec.LookPath("nu"); err != nil {
		t.Skip("nu isn't installed")
	}
	job := Job{
		Command: "$in | append ($r1 | where size > 1kb)",
		Tables:  map[string][]byte{"r1": []byte("[[name, size]; [a, 2kb], [b, 10b]]")},
		Input:   []byte("[[name, size]; [c, 5mb]]"),
	}
	d, err := Exec(context.Background(), Nu{}, job, 30*time.Second, 0)
	if err != nil {
		t.Fatal(err)
	}
	got := cells(d)
	if len(got) != 3 || got[1][0] != "c" || got[1][1] != "5.0 MB" || got[2][0] != "a" || got[2][1] != "2.0 kB" {
		t.Errorf("table %v", got)
	}
	_, err = Exec(context.Background(), Nu{}, Job{Command: "ls | get nosuchcolumn"}, 30*time.Second, 0)
	var e *Error
	if !errors.As(err, &e) || !strings.Contains(e.Msg, "nosuchcolumn") {
		t.Errorf("failure: %v", err)
	}
	_, err = Exec(context.Background(), Nu{}, Job{Command: "sleep 10sec"}, 200*time.Millisecond, 0)
	if err == nil || !strings.Contains(err.Error(), "stopped after") {
		t.Errorf("timeout: %v", err)
	}
	names, err := Commands(context.Background(), Nu{}, 30*time.Second)
	if err != nil || len(names) < 100 {
		t.Errorf("commands: %d %v", len(names), err)
	}
	if _, err := Exec(context.Background(), Nu{Path: "/nonexistent/nu"}, Job{Command: "1"}, time.Second, 0); err == nil {
		t.Error("a missing nu ran")
	}
}
