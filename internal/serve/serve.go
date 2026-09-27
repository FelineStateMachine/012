package serve

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync/atomic"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/ssh"
	bm "charm.land/wish/v2/bubbletea"
	gossh "golang.org/x/crypto/ssh"

	"github.com/FelineStateMachine/012/internal/confine"
	"github.com/FelineStateMachine/012/internal/jev"
	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/telemetry"
	"github.com/FelineStateMachine/012/internal/ui"
)

// handshakeTimeout bounds the time from connecting to logging in.
const handshakeTimeout = 30 * time.Second

// Server serves 012 sessions over SSH.
type Server struct {
	opts    Options
	root    confine.Root
	log     *slog.Logger
	jev     jev.Client // nil: JEV functions are off
	ssh     *ssh.Server
	hostPub gossh.PublicKey
	slots   chan struct{} // one per running session
	active  atomic.Int64
}

// Deps are what a Server takes from its surroundings rather than its
// options.
type Deps struct {
	// Log gets a line per login, rejection and session, besides the
	// telemetry events; nil discards them.
	Log *slog.Logger
	// JEV answers JEV functions, with a cache per session; nil turns them
	// off.
	JEV jev.Client
}

// New checks o and prepares a server: the served directory, the host
// key (generated if missing) and the authorized keys must be usable.
func New(o Options, d Deps) (*Server, error) {
	if err := o.validate(); err != nil {
		return nil, err
	}
	root, err := confine.New(o.Dir)
	if err != nil {
		return nil, fmt.Errorf("served directory: %w", err)
	}
	keys, _, err := authorizedKeys(o.AuthorizedKeys)
	if err != nil {
		return nil, fmt.Errorf("authorized keys: %w", err)
	}
	if len(keys) == 0 {
		return nil, fmt.Errorf("authorized keys: no usable keys in %s", o.AuthorizedKeys)
	}
	signer, err := hostKey(o.HostKey)
	if err != nil {
		return nil, err
	}
	if d.Log == nil {
		d.Log = slog.New(slog.DiscardHandler)
	}
	s := &Server{opts: o, root: root, log: d.Log, jev: d.JEV, hostPub: signer.PublicKey(), slots: make(chan struct{}, o.MaxSessions)}
	s.ssh = &ssh.Server{
		Addr:             o.Listen,
		Handler:          s.session,
		HostSigners:      []ssh.Signer{signer},
		Version:          "012",
		PublicKeyHandler: s.authorize,
		// No password or keyboard-interactive auth, no forwarding of any
		// kind, no global requests and no subsystems: only sessions.
		ChannelHandlers:        map[string]ssh.ChannelHandler{"session": ssh.DefaultSessionHandler},
		RequestHandlers:        map[string]ssh.RequestHandler{},
		SubsystemHandlers:      map[string]ssh.SubsystemHandler{},
		SessionRequestCallback: allowShell,
		ConnectionFailedCallback: func(c net.Conn, err error) {
			s.event("ssh.failed", slog.String("remote", c.RemoteAddr().String()), slog.String("error", err.Error()))
		},
		HandshakeTimeout: handshakeTimeout,
	}
	if o.IdleTimeout > 0 {
		// A backstop for connections without a session. Sessions end
		// on their own after IdleTimeout without input (see run), which
		// keepalives don't reset.
		s.ssh.IdleTimeout = o.IdleTimeout + time.Minute
	}
	return s, nil
}

// allowShell lets a session start 012 and nothing else: exec requests
// (ssh host command) and subsystems (sftp) are refused.
func allowShell(sess ssh.Session, requestType string) bool {
	return requestType == "shell"
}

// HostKey is the server's public key, for known_hosts.
func (s *Server) HostKey() gossh.PublicKey { return s.hostPub }

// Dir is the served directory, with links resolved.
func (s *Server) Dir() string { return s.root.Dir() }

// Sessions is how many sessions are running.
func (s *Server) Sessions() int { return int(s.active.Load()) }

// ListenAndServe listens on the options' address and serves until
// Shutdown or Close.
func (s *Server) ListenAndServe() error {
	l, err := net.Listen("tcp", s.opts.Listen)
	if err != nil {
		return err
	}
	return s.Serve(l)
}

// Serve serves sessions arriving on l.
func (s *Server) Serve(l net.Listener) error {
	s.log.Info("serving", "addr", l.Addr().String(), "dir", s.root.Dir(), "host_key", fingerprint(s.hostPub))
	err := s.ssh.Serve(l)
	if errors.Is(err, ssh.ErrServerClosed) {
		return nil
	}
	return err
}

// Shutdown stops listening and waits for sessions to end, until ctx is
// done.
func (s *Server) Shutdown(ctx context.Context) error { return s.ssh.Shutdown(ctx) }

// Close stops the server and drops every connection at once.
func (s *Server) Close() error { return s.ssh.Close() }

// authorize accepts key if the authorized_keys file lists it, reading
// the file again so edits apply without a restart.
func (s *Server) authorize(ctx ssh.Context, key ssh.PublicKey) bool {
	keys, _, err := authorizedKeys(s.opts.AuthorizedKeys)
	if err != nil {
		s.event("ssh.auth_error", slog.String("error", err.Error()))
		return false
	}
	for _, k := range keys {
		if ssh.KeysEqual(key, k) {
			return true
		}
	}
	s.event("ssh.rejected", slog.String("user", ctx.User()), slog.String("key", fingerprint(key)),
		slog.String("remote", ctx.RemoteAddr().String()))
	return false
}

// event logs name to telemetry and the server log. Events name users,
// keys and addresses, never what's in files.
func (s *Server) event(name string, attrs ...slog.Attr) {
	telemetry.Event(name, 0, attrs...)
	s.log.LogAttrs(context.Background(), slog.LevelInfo, name, attrs...)
}

// session runs one 012 over one SSH session.
func (s *Server) session(sess ssh.Session) {
	who := []slog.Attr{slog.String("user", sess.User()), slog.String("key", fingerprint(sess.PublicKey())),
		slog.String("remote", sess.RemoteAddr().String())}
	pty, winch, ok := sess.Pty()
	if !ok {
		fmt.Fprint(sess.Stderr(), "012 needs a terminal: connect with ssh -t\r\n")
		sess.Exit(1)
		return
	}
	select {
	case s.slots <- struct{}{}:
	default:
		s.event("ssh.full", who...)
		fmt.Fprintf(sess.Stderr(), "012: %d sessions are running, the most this server allows; try again later\r\n", s.opts.MaxSessions)
		sess.Exit(1)
		return
	}
	n := s.active.Add(1)
	telemetry.Set("ssh_sessions", n)
	s.event("ssh.session", append(who, slog.String("term", pty.Term), slog.Int("width", pty.Window.Width), slog.Int("height", pty.Window.Height))...)
	start := time.Now()
	defer func() {
		telemetry.Set("ssh_sessions", s.active.Add(-1))
		<-s.slots
	}()

	// The session's span holds what its program does, in one trace.
	span := telemetry.Start("serve.session", slog.String("term", pty.Term))
	idle := s.run(sess, pty, winch, span.Parent())
	span.End(slog.Bool("idle", idle))
	d := time.Since(start)
	s.event("ssh.session_end", append(who, slog.Duration("duration", d), slog.Bool("idle", idle))...)
	if idle {
		fmt.Fprintf(sess, "012: closed after %v without input\r\n", s.opts.IdleTimeout)
	}
	sess.Exit(0)
}

// run runs the program until it quits, the client goes away or the
// session goes idle, which it reports.
func (s *Server) run(sess ssh.Session, pty ssh.Pty, winch <-chan ssh.Window, parent telemetry.Parent) (idle bool) {
	m := ui.New(sheet.New(), "")
	m.TraceUnder(parent)
	env := append(sess.Environ(), "TERM="+pty.Term)
	m.Serve(s.root, env)
	if s.jev != nil {
		// Each session has its own cache: it queues that session's
		// questions, and its answers recalculate that session's sheets.
		m.EnableJEV(s.jev, jev.NewCache())
	}
	in := newActivity(sess)
	p := tea.NewProgram(m, append(bm.MakeOptions(sess), tea.WithInput(in), tea.WithFPS(ui.FrameRate))...)

	// The connection's context ends when the client goes away; cancel
	// stops the goroutines below when the program quits first.
	ctx, cancel := context.WithCancel(sess.Context())
	defer cancel()
	go resize(ctx, p, winch)
	var idled atomic.Bool
	if s.opts.IdleTimeout > 0 {
		go func() {
			if in.waitIdle(ctx, s.opts.IdleTimeout) {
				idled.Store(true)
				p.Quit()
			}
		}()
	}
	if _, err := p.Run(); err != nil && !errors.Is(err, tea.ErrProgramKilled) {
		s.log.Error("session", "error", err.Error())
	}
	p.Kill()
	return idled.Load()
}

// resize tells p about the client's window changes, and quits it when
// ctx ends.
func resize(ctx context.Context, p *tea.Program, winch <-chan ssh.Window) {
	for {
		select {
		case <-ctx.Done():
			p.Quit()
			return
		case w, ok := <-winch:
			if !ok {
				<-ctx.Done()
				p.Quit()
				return
			}
			p.Send(tea.WindowSizeMsg{Width: w.Width, Height: w.Height})
		}
	}
}

// activity is a session's input, noting when it last had any.
type activity struct {
	r    io.Reader
	last atomic.Int64 // UnixNano of the last read that returned bytes
}

func newActivity(r io.Reader) *activity {
	a := &activity{r: r}
	a.last.Store(time.Now().UnixNano())
	return a
}

func (a *activity) Read(p []byte) (int, error) {
	n, err := a.r.Read(p)
	if n > 0 {
		a.last.Store(time.Now().UnixNano())
	}
	return n, err
}

// waitIdle waits until there's been no input for d, reporting true, or
// until ctx ends, reporting false.
func (a *activity) waitIdle(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return false
		case now := <-t.C:
			left := d - now.Sub(time.Unix(0, a.last.Load()))
			if left <= 0 {
				return true
			}
			t.Reset(left)
		}
	}
}
