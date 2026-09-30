package serve

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/ssh"
	bm "charm.land/wish/v2/bubbletea"
	gossh "golang.org/x/crypto/ssh"

	"github.com/FelineStateMachine/012/internal/confine"
	"github.com/FelineStateMachine/012/internal/jev"
	"github.com/FelineStateMachine/012/internal/room"
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
	rooms   *room.Registry // the shared workbooks; nil with sharing off

	// stop is closed when the server shuts down, ending every session,
	// which keeps its unsaved work first; running counts the sessions
	// still doing so. mu orders sessions starting against stopping.
	mu      sync.Mutex
	stopped bool
	stop    chan struct{}
	running sync.WaitGroup
}

// halt tells every session to end; no new one starts after it.
func (s *Server) halt() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.stopped {
		s.stopped = true
		close(s.stop)
	}
}

// begin counts a session as running, or reports false when the server
// is stopping. A session that began calls s.running.Done when it ends.
func (s *Server) begin() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopped {
		return false
	}
	s.running.Add(1)
	return true
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
	s := &Server{opts: o, root: root, log: d.Log, jev: d.JEV, hostPub: signer.PublicKey(), slots: make(chan struct{}, o.MaxSessions),
		stop: make(chan struct{})}
	switch o.Share {
	case "edit":
		s.rooms = room.NewRegistry(room.Edit)
	case "view":
		s.rooms = room.NewRegistry(room.View)
	}
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
		SessionRequestCallback: allowSession,
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

// allowSession lets a session start 012 and nothing else. A shell
// request starts it on a new sheet. An exec request is taken only as the
// name of a file to open, with a terminal and in one word (ssh -t host
// file.012); it is never run. Other exec requests (ssh host command) and
// subsystems (sftp) are refused.
func allowSession(sess ssh.Session, requestType string) bool {
	switch requestType {
	case "shell":
		return true
	case "exec":
		_, _, pty := sess.Pty()
		return pty && len(sess.Command()) == 1
	}
	return false
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

// Shutdown ends every session, which keeps its unsaved work in a
// recovery file and tells the client, waits for them to end until ctx is
// done, then closes the listener and the connections left. Those hold no
// session, and a connection can hold nothing else.
func (s *Server) Shutdown(ctx context.Context) error {
	s.halt()
	err := s.waitSessions(ctx)
	if cerr := s.ssh.Close(); err == nil {
		err = cerr
	}
	return err
}

// closeWait bounds how long Close waits for sessions to keep their
// unsaved work.
const closeWait = 5 * time.Second

// Close stops the server and drops every connection at once. Sessions
// still keep their unsaved work, for up to closeWait.
func (s *Server) Close() error {
	s.halt()
	err := s.ssh.Close()
	ctx, cancel := context.WithTimeout(context.Background(), closeWait)
	defer cancel()
	s.waitSessions(ctx)
	return err
}

// waitSessions waits for running sessions to end, until ctx is done.
func (s *Server) waitSessions(ctx context.Context) error {
	done := make(chan struct{})
	go func() {
		s.running.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

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
	name, err := s.fileArg(sess)
	if err != nil {
		fmt.Fprintf(sess.Stderr(), "012: %v\r\n", err)
		sess.Exit(1)
		return
	}
	if !s.begin() {
		fmt.Fprint(sess.Stderr(), "012: the server is stopping\r\n")
		sess.Exit(1)
		return
	}
	defer s.running.Done()
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
	s.event("ssh.session", append(who, slog.String("term", pty.Term), slog.Int("width", pty.Window.Width), slog.Int("height", pty.Window.Height),
		slog.Bool("file", name != ""))...)
	start := time.Now()
	defer func() {
		telemetry.Set("ssh_sessions", s.active.Add(-1))
		<-s.slots
	}()

	// The session's span holds what its program does, in one trace.
	span := telemetry.Start("serve.session", slog.String("term", pty.Term))
	end := s.run(sess, pty, winch, name, span.Parent())
	span.End(slog.Bool("idle", end.idle))
	d := time.Since(start)
	s.event("ssh.session_end", append(who, slog.Duration("duration", d), slog.Bool("idle", end.idle),
		slog.Bool("stopped", end.stopped), slog.Bool("recovered", end.kept != ""))...)
	end.tell(sess, s.opts.IdleTimeout, s.root)
	sess.Exit(0)
}

// run runs the program, on the file name when there's one, until it
// quits, the client goes away, the session goes idle or the server
// stops, and keeps unsaved work for the last two.
func (s *Server) run(sess ssh.Session, pty ssh.Pty, winch <-chan ssh.Window, name string, parent telemetry.Parent) ending {
	m := ui.New(sheet.New(), "")
	m.TraceUnder(parent)
	env := append(sess.Environ(), "TERM="+pty.Term)
	m.Serve(s.root, env)
	if s.rooms != nil {
		m.ShareRooms(s.rooms, sess.User())
	}
	m.OpenOnStart(name)
	if s.jev != nil {
		// Each session has its own cache: it queues that session's
		// questions, and its answers recalculate that session's sheets.
		m.EnableJEV(s.jev, jev.NewCache())
	}
	in := newActivity(sess)
	g := ui.Guard(m)
	var model tea.Model = g
	var shared *ui.Shared
	if s.rooms != nil {
		shared = ui.InRooms(g)
		model = shared
	}
	p := tea.NewProgram(model, append(bm.MakeOptions(sess), tea.WithInput(in), tea.WithFPS(ui.FrameRate))...)

	// The connection's context ends when the client goes away; cancel
	// stops the goroutines below when the program quits first.
	ctx, cancel := context.WithCancel(sess.Context())
	defer cancel()
	if shared != nil {
		shared.Attach(ctx, p)
	}
	go resize(ctx, p, winch)
	go func() {
		select {
		case <-s.stop:
			p.Quit()
		case <-ctx.Done():
		}
	}()
	var idled atomic.Bool
	if s.opts.IdleTimeout > 0 {
		go func() {
			if in.waitIdle(ctx, s.opts.IdleTimeout) {
				idled.Store(true)
				p.Quit()
			}
		}()
	}
	_, err := p.Run()
	if err != nil && !errors.Is(err, tea.ErrProgramKilled) {
		s.log.Error("session", "error", err.Error())
	}
	p.Kill()
	end := ending{idle: idled.Load(), stopped: s.stopping(), crash: g.Finish(err)}
	if c := end.crash; c != nil {
		// The report is the server's log: the operator's to read.
		s.log.Error("session panic", "where", c.Where, "panic", fmt.Sprint(c.Value), "stack", string(c.Stack))
	}
	// A shared workbook stays with those still in its room; the last
	// one out keeps its unsaved work.
	_, end.others = g.Model().Others()
	if g.Model().LeaveRoom() {
		end.others = ""
		end.keep(g)
	}
	return end
}

// stopping reports whether the server is shutting down.
func (s *Server) stopping() bool {
	select {
	case <-s.stop:
		return true
	default:
		return false
	}
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
