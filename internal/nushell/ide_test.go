package nushell

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// testdata/ide holds nu 0.116's answers about the scripts beside them
// (ast.nu, check.nu, complete.nu at its end): what 012 relies on nu
// saying.
func recorded(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "ide", name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestParseAST(t *testing.T) {
	shapes, err := ParseAST(recorded(t, "ast.json"))
	if err != nil {
		t.Fatal(err)
	}
	src := string(recorded(t, "ast.nu"))
	var got []string
	for _, s := range shapes {
		got = append(got, src[s.From:s.To]+"="+strings.TrimPrefix(s.Shape, "shape_"))
	}
	want := []string{"ls=internalcall", "|=pipe", "where=internalcall", "size=string", "size=variable", ">=operator",
		"1=int", "kb=string", "|=pipe", "sort-by=internalcall", "name=string", "--reverse=flag"}
	if !slices.Equal(got, want) {
		t.Errorf("shapes\n got %q\nwant %q", got, want)
	}
	if _, err := ParseAST([]byte("Error: unknown flag")); !errors.Is(err, ErrOld) {
		t.Errorf("unreadable answer: %v", err)
	}
}

func TestParseCheck(t *testing.T) {
	probs, err := ParseCheck(recorded(t, "check.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	src := string(recorded(t, "check.nu"))
	var got []string
	for _, p := range probs {
		got = append(got, src[p.From:p.To]+": "+p.Msg)
	}
	want := []string{"(: Unclosed delimiter.", "--foo: The `math sum` command doesn't have flag `foo`.", ": Unclosed delimiter.",
		"(: Type mismatch.", "where: Command does not support number input."}
	if !slices.Equal(got, want) {
		t.Errorf("problems\n got %q\nwant %q", got, want)
	}
	probs, err = ParseCheck([]byte(`{"type":"hint","typename":"any","position":{"start":4,"end":6}}` + "\n"))
	if err != nil || len(probs) != 0 {
		t.Errorf("hints are left out: %v %v", probs, err)
	}
}

func TestParseComplete(t *testing.T) {
	got, err := ParseComplete(recorded(t, "complete.json"))
	if err != nil || !slices.Equal(got, []string{"sort", "sort-by", "source", "source-env"}) {
		t.Errorf("completions %q %v", got, err)
	}
	if got, err := ParseComplete([]byte(`{"completions": []}`)); err != nil || len(got) != 0 {
		t.Errorf("none %q %v", got, err)
	}
	if _, err := ParseComplete([]byte(`{"other": 1}`)); !errors.Is(err, ErrOld) {
		t.Errorf("unreadable answer: %v", err)
	}
}

func TestVersion(t *testing.T) {
	f := &fake{out: string(recorded(t, "version.txt"))}
	if v, err := Version(context.Background(), f); err != nil || v != "0.116.0" || !slices.Equal(f.job.IDE, []string{"--version"}) {
		t.Errorf("version %q %v %q", v, err, f.job.IDE)
	}
	if _, err := Version(context.Background(), &fake{out: "0.99.1\n"}); !errors.Is(err, ErrOld) {
		t.Errorf("an old nu: %v", err)
	}
	for v, want := range map[string]bool{"0.100.0": true, "0.117.0-nightly.3": true, "1.0.0": true, "0.99.9": false, "nu": false} {
		if AtLeast(v, MinVersion) != want {
			t.Errorf("AtLeast(%q) = %v", v, !want)
		}
	}
}

// The script asked about reaches the runner as it is, with the flags.
func TestAskHandsTheScript(t *testing.T) {
	f := &fake{out: `{"completions": ["sort"]}`}
	if _, err := Complete(context.Background(), f, "ls | so", 7); err != nil {
		t.Fatal(err)
	}
	if f.script != "ls | so" || !slices.Equal(f.job.IDE, []string{"--ide-complete", "7"}) {
		t.Errorf("script %q flags %q", f.script, f.job.IDE)
	}
}

// The real nu, when it's installed, says what the recorded answers do.
func TestNuIDE(t *testing.T) {
	if _, err := exec.LookPath("nu"); err != nil {
		t.Skip("nu isn't installed")
	}
	ctx := context.Background()
	if _, err := Version(ctx, Nu{}); err != nil {
		t.Skipf("nu: %v", err)
	}
	shapes, err := AST(ctx, Nu{}, string(recorded(t, "ast.nu")))
	if err != nil || len(shapes) < 5 || shapes[0] != (Shape{0, 2, "shape_internalcall"}) {
		t.Errorf("ast %v %v", shapes, err)
	}
	probs, err := Check(ctx, Nu{}, string(recorded(t, "check.nu")), 20)
	if err != nil || !slices.ContainsFunc(probs, func(p Problem) bool { return strings.Contains(p.Msg, "flag `foo`") }) {
		t.Errorf("check %v %v", probs, err)
	}
	comp, err := Complete(ctx, Nu{}, "ls | so", 7)
	if err != nil || !slices.Contains(comp, "sort-by") {
		t.Errorf("complete %q %v", comp, err)
	}
}
