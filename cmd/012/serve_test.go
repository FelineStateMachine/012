package main

import (
	"io"
	"testing"
	"time"

	"github.com/FelineStateMachine/012/internal/serve"
)

func TestServeFlags(t *testing.T) {
	o, err := serveFlags(nil, io.Discard)
	if err != nil || o != serve.Defaults() {
		t.Errorf("no flags: %+v, %v; want the defaults", o, err)
	}
	o, err = serveFlags([]string{"--listen", "127.0.0.1:9000", "sheets", "-idle-timeout=5m", "--max-sessions", "3",
		"--authorized-keys", "/k", "--host-key=/h"}, io.Discard)
	want := serve.Options{Dir: "sheets", Listen: "127.0.0.1:9000", AuthorizedKeys: "/k", HostKey: "/h", IdleTimeout: 5 * time.Minute, MaxSessions: 3}
	if err != nil || o != want {
		t.Errorf("got %+v, %v; want %+v", o, err, want)
	}
	for _, args := range [][]string{{"a", "b"}, {"--nope"}, {"--max-sessions", "x"}} {
		if _, err := serveFlags(args, io.Discard); err == nil {
			t.Errorf("%q: no error", args)
		}
	}
}
