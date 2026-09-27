package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	gossh "golang.org/x/crypto/ssh"

	"github.com/FelineStateMachine/012/internal/jev"
	"github.com/FelineStateMachine/012/internal/serve"
)

const serveUsage = "usage: 012 serve [--listen addr] [--authorized-keys file] [--host-key file] [--idle-timeout 30m] [--max-sessions 8] [dir]"

// serveFlags parses 012 serve's arguments into options, starting from
// the defaults. The served directory may come before or after the flags.
func serveFlags(args []string, stderr io.Writer) (serve.Options, error) {
	o := serve.Defaults()
	fs := flag.NewFlagSet("012 serve", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprintln(stderr, serveUsage)
		fs.PrintDefaults()
	}
	fs.StringVar(&o.Listen, "listen", o.Listen, "address to listen on")
	fs.StringVar(&o.AuthorizedKeys, "authorized-keys", o.AuthorizedKeys, "public keys allowed to log in, in authorized_keys format")
	fs.StringVar(&o.HostKey, "host-key", o.HostKey, "the server's private key, generated if missing")
	fs.DurationVar(&o.IdleTimeout, "idle-timeout", o.IdleTimeout, "end sessions without input for this long (0: never)")
	fs.IntVar(&o.MaxSessions, "max-sessions", o.MaxSessions, "most sessions at once")
	var dirs []string
	for {
		if err := fs.Parse(args); err != nil {
			return o, err
		}
		if fs.NArg() == 0 {
			break
		}
		dirs, args = append(dirs, fs.Arg(0)), fs.Args()[1:]
	}
	switch len(dirs) {
	case 0:
	case 1:
		o.Dir = dirs[0]
	default:
		return o, errors.New(serveUsage)
	}
	return o, nil
}

// runServe is 012 serve: sessions over SSH until interrupted.
func runServe(args []string) error {
	tc, args, err := telemetryFlags(args)
	if err != nil {
		return err
	}
	o, err := serveFlags(args, os.Stderr)
	if errors.Is(err, flag.ErrHelp) {
		return nil
	}
	if err != nil {
		return err
	}
	stopTelemetry, err := startTelemetry(tc)
	if err != nil {
		return err
	}
	defer stopTelemetry()
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	d := serve.Deps{Log: log}
	// JEV functions run when the server's own environment has an API
	// key; sessions never read .env files.
	if cfg, ok := jev.LoadConfig(); ok {
		if d.JEV, err = jev.NewClient(cfg); err != nil {
			return fmt.Errorf("JEV: %w", err)
		}
	}
	srv, err := serve.New(o, d)
	if err != nil {
		return err
	}
	l, err := net.Listen("tcp", o.Listen)
	if err != nil {
		return err
	}
	if host, _, err := net.SplitHostPort(o.Listen); err == nil {
		if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
			log.Warn("listening beyond this machine: anyone holding an authorized key who can reach " + o.Listen + " can read and write " + srv.Dir())
		}
	}
	fmt.Fprintf(os.Stderr, "012 serving %s on %s\nhost key %s\nconnect: ssh -p %s %s\n",
		srv.Dir(), l.Addr(), gossh.FingerprintSHA256(srv.HostKey()), portOf(l.Addr()), hostOf(l.Addr()))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	done := make(chan error, 1)
	go func() { done <- srv.Serve(l) }()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
	}
	// Stop listening, give sessions a moment, then drop what's left.
	log.Info("stopping", "sessions", srv.Sessions())
	sctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := srv.Shutdown(sctx); err != nil {
		srv.Close()
	}
	return <-done
}

func portOf(a net.Addr) string {
	_, port, _ := net.SplitHostPort(a.String())
	return port
}

func hostOf(a net.Addr) string {
	host, _, _ := net.SplitHostPort(a.String())
	return host
}
