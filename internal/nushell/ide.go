package nushell

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// What nu says of a script without running it, through its IDE flags,
// each reading the script from a file:
//
//	nu --ide-ast file           [{"type":"ast","span":{"start":0,"end":2},"shape":"shape_internalcall","content":"ls"}, ...]
//	nu --ide-check 20 file      {"type":"diagnostic","severity":"Error","message":"Variable not found.","span":{"start":31,"end":35}}
//	                            one object a line, "hint" lines among them
//	nu --ide-complete 7 file    {"completions": ["sort", "sort-by"]}
//
// Spans and offsets are bytes of the file. These are the shapes of nu
// 0.116 (testdata/ide holds its answers); an older nu whose answers
// don't read as these is reported as ErrOld, and a nu older than
// MinVersion isn't asked at all.

// MinVersion is the oldest nu asked about scripts.
const MinVersion = "0.100.0"

// ErrOld is a nu too old to ask, or one whose answers don't read.
var ErrOld = errors.New("nu is too old to ask about pipelines")

// Shape is a stretch of a script, by byte offsets, and nu's name for
// what it is: shape_internalcall, shape_string.
type Shape struct {
	From, To int
	Shape    string
}

// Problem is what nu's check found wrong with a stretch of a script.
type Problem struct {
	From, To int
	Severity string // Error or Warning
	Msg      string
}

// maxAnswer is the most of an answer kept: a script's shapes or
// completions are far less.
const maxAnswer = 4 << 20

// ask runs job, asking nu about script, and returns what it printed.
func ask(ctx context.Context, r Runner, script string, flags ...string) ([]byte, error) {
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	out := &capped{max: maxAnswer, stop: func() { cancel(ErrTooLarge) }}
	err := r.Run(ctx, Job{Command: script, IDE: flags}, script, out)
	switch {
	case ctx.Err() != nil:
		return nil, context.Cause(ctx)
	case err != nil:
		return nil, err
	}
	return bytes.TrimSpace(out.b.Bytes()), nil
}

// Version is nu's version, checked against MinVersion: ErrOld when it's
// older or doesn't read as one.
func Version(ctx context.Context, r Runner) (string, error) {
	out, err := ask(ctx, r, "", "--version")
	if err != nil {
		return "", err
	}
	v := string(out)
	if !AtLeast(v, MinVersion) {
		return v, fmt.Errorf("%w: %s", ErrOld, v)
	}
	return v, nil
}

// AtLeast reports whether version v is oldest or later; one that doesn't
// read as major.minor.patch isn't.
func AtLeast(v, oldest string) bool {
	a, okA := semver(v)
	b, okB := semver(oldest)
	if !okA || !okB {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return a[i] > b[i]
		}
	}
	return true
}

func semver(v string) ([3]int, bool) {
	var out [3]int
	v, _, _ = strings.Cut(strings.TrimSpace(v), "-") // 0.117.0-nightly
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return out, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return out, false
		}
		out[i] = n
	}
	return out, true
}

// AST is nu's shapes of script's tokens, in order.
func AST(ctx context.Context, r Runner, script string) ([]Shape, error) {
	out, err := ask(ctx, r, script, "--ide-ast")
	if err != nil {
		return nil, err
	}
	return ParseAST(out)
}

type span struct{ Start, End int }

// ParseAST reads what --ide-ast printed.
func ParseAST(out []byte) ([]Shape, error) {
	var items []struct {
		Type  string
		Span  *span
		Shape string
	}
	if err := json.Unmarshal(out, &items); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrOld, err)
	}
	shapes := make([]Shape, 0, len(items))
	for _, it := range items {
		if it.Type != "ast" || it.Span == nil || !strings.HasPrefix(it.Shape, "shape_") {
			continue
		}
		shapes = append(shapes, Shape{From: it.Span.Start, To: it.Span.End, Shape: it.Shape})
	}
	return shapes, nil
}

// Check is what nu's check finds wrong with script, at most limit
// problems, each once.
func Check(ctx context.Context, r Runner, script string, limit int) ([]Problem, error) {
	out, err := ask(ctx, r, script, "--ide-check", strconv.Itoa(limit))
	if err != nil {
		return nil, err
	}
	return ParseCheck(out)
}

// ParseCheck reads what --ide-check printed: its diagnostics, each once,
// leaving out its hints (the types it inferred).
func ParseCheck(out []byte) ([]Problem, error) {
	var probs []Problem
	sc := bufio.NewScanner(bytes.NewReader(out))
	sc.Buffer(nil, maxAnswer)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var d struct {
			Type, Severity, Message string
			Span                    *span
		}
		if err := json.Unmarshal(line, &d); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrOld, err)
		}
		if d.Type != "diagnostic" || d.Span == nil {
			continue
		}
		p := Problem{From: d.Span.Start, To: d.Span.End, Severity: d.Severity, Msg: d.Message}
		if !containsProblem(probs, p) {
			probs = append(probs, p)
		}
	}
	return probs, sc.Err()
}

func containsProblem(probs []Problem, p Problem) bool {
	for _, q := range probs {
		if q == p {
			return true
		}
	}
	return false
}

// Complete is what nu would complete at offset, a byte of script, in
// its order. nu says what goes there, not what it replaces.
func Complete(ctx context.Context, r Runner, script string, offset int) ([]string, error) {
	out, err := ask(ctx, r, script, "--ide-complete", strconv.Itoa(offset))
	if err != nil {
		return nil, err
	}
	return ParseComplete(out)
}

// ParseComplete reads what --ide-complete printed.
func ParseComplete(out []byte) ([]string, error) {
	if len(out) == 0 {
		return nil, nil
	}
	var c struct {
		Completions *[]string
	}
	if err := json.Unmarshal(out, &c); err != nil || c.Completions == nil {
		return nil, fmt.Errorf("%w: %s", ErrOld, firstLine(out))
	}
	return *c.Completions, nil
}

func firstLine(b []byte) string {
	line, _, _ := bytes.Cut(b, []byte("\n"))
	return string(line)
}
