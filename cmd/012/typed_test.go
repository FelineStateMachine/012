package main

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// 012 set --table writes a NUON table from standard input with its
// types, --value types values, and 012 get writes them back: as typed
// JSON, and as NUON nushell reads with its types.
func TestTypedSetAndGet(t *testing.T) {
	e, out, _ := scriptEnv(t)
	path := filepath.Join(t.TempDir(), "book.012")
	e.stdin = strings.NewReader(`[[name, size, took, when]; [a.txt, 1.5kb, 90sec, 2026-09-29T14:30:00+00:00], [b.txt, 2mb, 2hr, 2026-09-28T09:00:00+00:00]]`)
	if code, err := status(e, "set", path, "--table", "A1", "--value", "E1", `"price"`, "E2", `{"currency": 3.5}`, "E3", `{currency: 12, decimals: 0}`); code != 0 {
		t.Fatal(err)
	}
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"E2"}, "$3.50\n"},
		{[]string{"E3", "--format", "json"}, `{"currency":12,"decimals":0}` + "\n"},
		{[]string{"B2", "--format", "json"}, `{"size":1500}` + "\n"},
		{[]string{"C3", "--format", "json"}, `{"duration":"2hr"}` + "\n"},
		{[]string{"A1:C2", "--format", "json"}, "[\n" + `{"name":"a.txt","size":{"size":1500},"took":{"duration":"90sec"}}` + "\n]\n"},
		{[]string{"A1:C3", "--format", "nuon"}, "[[name, size, took]; [a.txt, 1500b, 90000000000ns],\n[b.txt, 2000000b, 7200000000000ns]]\n"},
	} {
		out.Reset()
		if code, err := status(e, append([]string{"get", path}, c.args...)...); code != 0 || out.String() != c.want {
			t.Errorf("get %v: %d %v %q, want %q", c.args, code, err, out, c.want)
		}
	}
	nu, err := exec.LookPath("nu")
	if err != nil {
		t.Skip("nu isn't installed")
	}
	out.Reset()
	status(e, "get", path, "A1:E3", "--format", "nuon")
	cmd := exec.Command(nu, "--no-config-file", "--stdin", "-c", `from nuon | each {|r| [($r.size | describe), ($r.took | describe), ($r.when | describe), ($r.price | describe)] | str join " "} | str join "\n"`)
	cmd.Stdin = strings.NewReader(out.String())
	got, err := cmd.Output()
	if err != nil {
		t.Fatalf("nu: %v %s", err, got)
	}
	if want := "filesize duration datetime float\nfilesize duration datetime int"; strings.TrimSpace(string(got)) != want {
		t.Errorf("nushell reads %q, want %q", got, want)
	}
}
