package nushell

import (
	"context"
	"errors"
	"io"
	"os/exec"
	"strings"
	"testing"
	"time"
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

func TestScript(t *testing.T) {
	got := Script(Job{Command: "$r1 | where size > 1kb", Tables: map[string][]byte{"r1": nil}}, []string{"big", "r1"})
	want := "let big = (open --raw $env.NU012_TABLE_0 | from nuon)\n" +
		"let r1 = (open --raw $env.NU012_TABLE_1 | from nuon)\n" +
		"do {\n$r1 | where size > 1kb\n} | to nuon\n"
	if got != want {
		t.Errorf("script:\n%s\nwant:\n%s", got, want)
	}
}

func TestExec(t *testing.T) {
	out, err := Exec(context.Background(), &fake{out: " [[name, size]; [a, 1kb]]\n"}, Job{Command: "x"}, time.Second, 0)
	if err != nil || string(out) != "[[name, size]; [a, 1kb]]" {
		t.Errorf("%q %v", out, err)
	}
	_, err = Exec(context.Background(), &fake{err: &Error{Msg: "Cannot find column 'x'", Stderr: "  × Cannot find column 'x'\n  help: try get\n"}}, Job{Command: "x"}, time.Second, 0)
	var e *Error
	if !errors.As(err, &e) || e.Msg != "Cannot find column 'x'" || e.Help() != "help: try get" {
		t.Errorf("error %v", err)
	}
	_, err = Exec(context.Background(), &fake{out: strings.Repeat("x", 100)}, Job{Command: "x"}, time.Second, 10)
	if !errors.Is(err, ErrTooLarge) {
		t.Errorf("too large: %v", err)
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

// The real nu, when it's installed: tables go in as variables and come
// back as NUON.
func TestNu(t *testing.T) {
	if _, err := exec.LookPath("nu"); err != nil {
		t.Skip("nu isn't installed")
	}
	job := Job{
		Command: "$r1 | where size > 1kb | append $__sheet1",
		Tables:  map[string][]byte{"r1": []byte("[[name, size]; [a, 2kb], [b, 10b]]"), "__sheet1": []byte("[[name, size]; [c, 5mb]]")},
	}
	out, err := Exec(context.Background(), Nu{}, job, 30*time.Second, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(out); got != "[[name, size]; [a, 2000b], [c, 5000000b]]" && got != "[[name, size]; [a, 2kB], [c, 5MB]]" {
		t.Logf("output %s", got) // nu's own spelling of sizes varies by version
		if !strings.Contains(got, "a") || !strings.Contains(got, "c") || strings.Contains(got, "b,") {
			t.Errorf("output %s", got)
		}
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
