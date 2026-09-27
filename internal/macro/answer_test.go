package macro

import (
	"strings"
	"testing"
)

func TestJSONAnswersAreWrittenAsDicts(t *testing.T) {
	a := Call("run", "data.sort_range").With("answer", JSON(`{"by":[{"column":"B","order":"desc"},{"column":"A"}],"header":true,"min":-1.5,"x":null,"q":"it's \"so\""}`))
	want := `run("data.sort_range", answer={"by": [{"column": "B", "order": "desc"}, {"column": "A"}], "header": True, "min": -1.5, "x": None, "q": "it's \"so\""})`
	if got := a.String(); got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
	// The script hands the answer back as the JSON it came from, keys in
	// the order written.
	f := newFake()
	if _, err := New("m", Source("x", []Action{a}), Env{Host: f}).Exec(); err != nil {
		t.Fatal(err)
	}
	wantCall := `run data.sort_range "{\"by\":[{\"column\":\"B\",\"order\":\"desc\"},{\"column\":\"A\"}],\"header\":true,\"min\":-1.5,\"x\":null,\"q\":\"it's \\\"so\\\"\"}"`
	if len(f.calls) != 1 || f.calls[0] != wantCall {
		t.Fatalf("calls %q\nwant  %q", f.calls, wantCall)
	}
}

func TestListAnswersAndBadOnes(t *testing.T) {
	f := newFake()
	if _, err := New("m", `run("x", answer=[1, (2, 3), {"k": [True]}])`, Env{Host: f}).Exec(); err != nil {
		t.Fatal(err)
	}
	if f.calls[0] != `run x "[1,[2,3],{\"k\":[true]}]"` {
		t.Fatalf("calls %q", f.calls)
	}
	for src, want := range map[string]string{
		`run("x", answer={1: 2})`:         "keys are strings, not int",
		`run("x", answer={"f": print})`:   "can't hold a builtin_function_or_method",
		`run("x", answer=[float("nan")])`: "unsupported value",
	} {
		_, err := New("m", src, Env{Host: newFake()}).Exec()
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: %v, want %q", src, err, want)
		}
	}
}
