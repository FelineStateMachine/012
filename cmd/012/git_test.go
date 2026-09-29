package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FelineStateMachine/012/internal/headless"
	"github.com/FelineStateMachine/012/internal/sheet"
)

// With O12_AS_MAIN set, the test binary is 012 itself, so git can run
// it as a diff and merge driver.
func TestMain(m *testing.M) {
	if os.Getenv("O12_AS_MAIN") == "1" {
		main()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// gitRepo is a repository in a temporary directory whose .012 files use
// 012 (this test binary) as their diff and merge drivers, as
// docs/files/git.md sets them up.
type gitRepo struct {
	t   *testing.T
	dir string
	env []string
}

func newGitRepo(t *testing.T) *gitRepo {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git isn't installed")
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	r := &gitRepo{t: t, dir: t.TempDir(), env: append(os.Environ(),
		"O12_AS_MAIN=1", "HOME="+home, "XDG_CONFIG_HOME="+home, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+filepath.Join(home, "gitconfig"),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com", "NO_COLOR=1")}
	r.git("init", "-q", "-b", "main")
	r.git("config", "diff.012.command", "'"+exe+"' diff")
	r.git("config", "diff.012.textconv", "'"+exe+"' diff --textconv")
	r.git("config", "merge.012.name", "012 cell by cell")
	r.git("config", "merge.012.driver", "'"+exe+"' merge-driver %O %A %B %P")
	r.write(".gitattributes", "*.012 diff=012 merge=012\n")
	return r
}

// git runs git in the repository and returns its output, failing the
// test when it fails.
func (r *gitRepo) git(args ...string) string {
	r.t.Helper()
	out, err := r.try(args...)
	if err != nil {
		r.t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return out
}

func (r *gitRepo) try(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir, cmd.Env = r.dir, r.env
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func (r *gitRepo) write(name, text string) {
	os.WriteFile(filepath.Join(r.dir, name), []byte(text), 0o644)
}

// set sets cells of book.012 with 012 set and commits.
func (r *gitRepo) set(msg string, pairs ...string) {
	r.t.Helper()
	e, _, _ := testEnv(r.t, nil)
	if err := run(append([]string{"set", filepath.Join(r.dir, "book.012")}, pairs...), e); err != nil {
		r.t.Fatal(err)
	}
	r.git("add", "-A")
	r.git("commit", "-q", "-m", msg)
}

func (r *gitRepo) cell(ref string, input bool) string {
	r.t.Helper()
	f, err := headless.Open(filepath.Join(r.dir, "book.012"), false)
	if err != nil {
		r.t.Fatal(err)
	}
	t, err := headless.Resolve(f.Book, ref)
	if err != nil {
		r.t.Fatal(err)
	}
	var b bytes.Buffer
	headless.Get(&b, t, headless.GetOptions{Input: input})
	return strings.TrimSuffix(b.String(), "\n")
}

func TestGitMerge(t *testing.T) {
	r := newGitRepo(t)
	r.set("base", "A1", "Item", "B1", "Price", "B2", "1", "B3", "2", "B4", "=SUM(B2:B3)")
	r.git("checkout", "-q", "-b", "theirs")
	r.set("theirs", "B2", "10", "C1", "Note")
	r.git("checkout", "-q", "main")
	r.set("ours", "B3", "20")
	out := r.git("merge", "-q", "--no-edit", "theirs")
	if r.cell("B2", true) != "10" || r.cell("B3", true) != "20" || r.cell("C1", true) != "Note" || r.cell("B4", false) != "30" {
		t.Errorf("merged: B2 %s, B3 %s, C1 %s, B4 %s\n%s", r.cell("B2", true), r.cell("B3", true), r.cell("C1", true), r.cell("B4", false), out)
	}

	// Both change B2: a conflict, ours kept, theirs in B2's note.
	r.git("checkout", "-q", "-b", "theirs2")
	r.set("theirs2", "B2", "11")
	r.git("checkout", "-q", "main")
	r.set("ours2", "B2", "12", "D1", "x")
	out, err := r.try("merge", "--no-edit", "theirs2")
	if err == nil || !strings.Contains(out, "012 merge-driver: 1 conflict in book.012") || !strings.Contains(out, "Sheet1!B2 input: ours 12, theirs 11, base 10") ||
		!strings.Contains(out, "CONFLICT") {
		t.Errorf("a conflict: %v\n%s", err, out)
	}
	f, _ := headless.Open(filepath.Join(r.dir, "book.012"), false)
	b2, _ := sheet.ParseAddr("B2")
	note := f.Book.Sheet(0).Note(b2)
	if r.cell("B2", true) != "12" || r.cell("D1", true) != "x" || note != "Merge conflict, kept ours; theirs had input 11" {
		t.Errorf("after the conflict: B2 %s, D1 %s, note %q", r.cell("B2", true), r.cell("D1", true), note)
	}
}

func TestGitDiff(t *testing.T) {
	r := newGitRepo(t)
	r.set("base", "A1", "Item", "B1", "1", "B2", "=B1*2")
	e, _, _ := testEnv(t, nil)
	run([]string{"set", filepath.Join(r.dir, "book.012"), "B1", "5", "A2", "new"}, e)
	out := r.git("diff")
	want := "diff --012 a/book.012 b/book.012\nSheet1!B1  input  1 → 5\nSheet1!A2  input  + new\nSheet1!B2  value  2 → 10\n"
	if out != want {
		t.Errorf("git diff:\n%s\nwant\n%s", out, want)
	}
	out = r.git("diff", "--no-ext-diff")
	for _, line := range []string{"-Sheet1!B1  1\n", "+Sheet1!B1  5\n", "+Sheet1!A2  new\n", "-Sheet1!B2  =B1*2  = 2\n", "+Sheet1!B2  =B1*2  = 10\n"} {
		if !strings.Contains(out, line) {
			t.Errorf("git diff with textconv lacks %q:\n%s", line, out)
		}
	}
	// A file added shows every cell added.
	r.git("add", "-A")
	r.git("commit", "-q", "-m", "more")
	os.WriteFile(filepath.Join(r.dir, "other.012"), []byte(`{"version": 2, "cells": {"A1": "hi"}}`), 0o644)
	r.git("add", "other.012")
	out = r.git("diff", "--cached")
	if !strings.Contains(out, "diff --012 a/other.012 b/other.012\nsheet Sheet1  added\nSheet1!A1  input  + hi\n") {
		t.Errorf("a file added:\n%s", out)
	}
}

func TestDiffCommand(t *testing.T) {
	e, out, _ := scriptEnv(t)
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a.012"), filepath.Join(dir, "b.012")
	status(e, "set", a, "A1", "1")
	status(e, "set", b, "A1", "2")
	out.Reset()
	if code, _ := status(e, "diff", a, a); code != 0 || out.Len() > 0 {
		t.Errorf("the same: %d %q", code, out)
	}
	if code, _ := status(e, "diff", a, b, "--format", "nuon"); code != 1 || out.String() != "[[kind, sheet, item, field, old, new]; [cell, Sheet1, A1, input, \"1\", \"2\"]]\n" {
		t.Errorf("different: %d %q", code, out)
	}
	out.Reset()
	e.stdoutTTY = true
	status(e, "diff", a, b)
	if !strings.Contains(out.String(), "\x1b[31m1\x1b[0m") {
		t.Errorf("no color on a terminal: %q", out)
	}
	for _, c := range []struct {
		args []string
		code int
		want string
	}{
		{[]string{"diff", a}, 2, "usage: 012 diff"},
		{[]string{"diff", a, filepath.Join(dir, "none.012")}, 2, "no such file"},
		{[]string{"diff", a, b, "--format", "xml"}, 2, "--format xml"},
		{[]string{"diff", a, b, "--color", "sometimes"}, 2, "--color sometimes"},
		{[]string{"merge-driver", a, b}, 2, "usage: 012 merge-driver"},
		{[]string{"merge-driver", a, b, filepath.Join(dir, "none.012")}, 2, "no such file"},
	} {
		if code, err := status(e, c.args...); code != c.code || err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%v: %d %v, want %d and %q", c.args, code, err, c.code, c.want)
		}
	}
	os.WriteFile(filepath.Join(dir, "broken.012"), []byte("{"), 0o644)
	before, _ := os.ReadFile(b)
	if code, err := status(e, "merge-driver", a, b, filepath.Join(dir, "broken.012")); code != 2 || !strings.Contains(err.Error(), "ours is left as it was") {
		t.Errorf("a broken side: %d %v", code, err)
	}
	if after, _ := os.ReadFile(b); !bytes.Equal(before, after) {
		t.Error("a failed merge changed ours")
	}
}
