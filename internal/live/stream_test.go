package live

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"
)

// A stream's rows come as the program prints them, a line cut in two
// waiting for its end; Reload gives them all again, and the end is
// reported once, with the error it ended on.
func TestStreamRowsAsPrinted(t *testing.T) {
	next := make(chan string)
	s := NewStream(func(ctx context.Context, w io.Writer) error {
		for text := range next {
			io.WriteString(w, text)
		}
		return errors.New("tail: app.log: no such file")
	})
	defer s.Close()
	s.Wait = time.Second
	ctx := context.Background()

	next <- "{x: a, y: 1}\n{x: b"
	u, ok := s.Poll(ctx)
	if !ok || len(u.Rows) != 1 || u.Header == nil || u.Header[0].V.Str != "x" || u.Rows[0][1].V.Num != 1 {
		t.Fatalf("first poll: %v %+v", ok, u)
	}
	next <- ", y: 2}\n"
	if u, ok = s.Poll(ctx); !ok || len(u.Rows) != 1 || u.Rows[0][0].V.Str != "b" || u.Header != nil {
		t.Fatalf("second poll: %v %+v", ok, u)
	}
	if got := string(s.NUON()); got != "[{x: a, y: 1}, {x: b, y: 2}]" {
		t.Errorf("NUON %s", got)
	}
	s.Reload()
	next <- "{x: c, y: 3}\n"
	if u, ok = s.Poll(ctx); !ok || !u.Reset || len(u.Rows) != 3 || u.Header == nil {
		t.Fatalf("after Reload: %v %+v", ok, u)
	}
	if done, _ := s.Ended(); done {
		t.Fatal("ended too soon")
	}
	close(next)
	u, ok = s.Poll(ctx)
	if !ok || u.Err != "tail: app.log: no such file" {
		t.Fatalf("the end: %v %+v", ok, u)
	}
	if done, err := s.Ended(); !done || err == nil || s.Rows() != 3 {
		t.Errorf("ended %v %v, %d rows", done, err, s.Rows())
	}
	if _, ok := s.Poll(ctx); ok {
		t.Error("polled after the end")
	}
}

// Closing a stream stops its program, which ends without an error.
func TestStreamClose(t *testing.T) {
	s := NewStream(func(ctx context.Context, w io.Writer) error {
		io.WriteString(w, "1\n")
		<-ctx.Done()
		return ctx.Err()
	})
	s.Wait = time.Second
	if u, ok := s.Poll(context.Background()); !ok || len(u.Rows) != 1 {
		t.Fatalf("%v %+v", ok, u)
	}
	s.Close()
	u, ok := s.Poll(context.Background())
	if !ok || u.Err != "" {
		t.Fatalf("after Close: %v %+v", ok, u)
	}
	if done, err := s.Ended(); !done || err != nil {
		t.Errorf("ended %v %v", done, err)
	}
}

// Only the last Keep values are kept.
func TestStreamKeepsTheLast(t *testing.T) {
	s := NewStream(func(ctx context.Context, w io.Writer) error {
		io.WriteString(w, "1\n2\n3\n")
		return nil
	})
	s.Keep = 2
	for done := false; !done; done, _ = s.Ended() {
		s.Poll(context.Background())
	}
	if got := string(s.NUON()); got != "[2, 3]" {
		t.Errorf("NUON %s", got)
	}
}
