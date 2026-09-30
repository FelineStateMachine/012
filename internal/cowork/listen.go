package cowork

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/FelineStateMachine/012/internal/mcp"
	"github.com/FelineStateMachine/012/internal/room"
	"github.com/FelineStateMachine/012/internal/sheet"
)

// An attach starts with one line each way before the MCP messages: the
// agent's hello (which room, and the MCP client's name, read from its
// initialize request), and the session's welcome (the workbook and the
// scope, or why not). After that the connection carries MCP's
// newline-delimited JSON-RPC, as standard input and output would.

// hello is what 012 mcp --attach sends first.
type hello struct {
	Version int    `json:"version"`
	Room    string `json:"room,omitempty"`
	Client  string `json:"client,omitempty"`
}

// welcome is the session's answer.
type welcome struct {
	Error    string `json:"error,omitempty"`
	Workbook string `json:"workbook,omitempty"`
	Scope    string `json:"scope,omitempty"`
}

// Options say what a listener serves.
type Options struct {
	Registry *room.Registry
	// Room is the key of the room the agent joins, given the name its
	// attach gave ("" for none): a session's own, whatever the name, or
	// in 012 serve the file named, which someone must have open.
	Room func(name string) (string, error)
	// Kind and Workbook are what the endpoint file says (Endpoint).
	Kind, Workbook string
	// Version is 012's, which the MCP server reports.
	Version string
	// Dir is the folder to listen in; "" is SocketDir.
	Dir string
}

// Listener is a session listening for agents.
type Listener struct {
	opts   Options
	ln     net.Listener
	socket string
	file   string

	mu     sync.Mutex
	conns  map[net.Conn]bool
	closed bool
	wg     sync.WaitGroup
}

// listeners numbers the sockets of one process.
var listeners atomic.Int64

// Listen starts listening for agents, on a socket only the user can
// reach.
func Listen(o Options) (*Listener, error) {
	dir := o.Dir
	if dir == "" {
		var err error
		if dir, err = SocketDir(); err != nil {
			return nil, err
		}
	}
	if err := ownDir(dir); err != nil {
		return nil, err
	}
	base := filepath.Join(dir, fmt.Sprintf("%d-%d", os.Getpid(), listeners.Add(1)))
	l := &Listener{opts: o, socket: base + ".sock", file: base + ".json", conns: map[net.Conn]bool{}}
	os.Remove(l.socket)
	ln, err := net.Listen("unix", l.socket)
	if err != nil {
		return nil, err
	}
	l.ln = ln
	if err := os.Chmod(l.socket, 0o600); err != nil {
		ln.Close()
		return nil, err
	}
	data, _ := json.Marshal(Endpoint{PID: os.Getpid(), Socket: l.socket, Kind: o.Kind, Workbook: o.Workbook, Started: time.Now()})
	if err := os.WriteFile(l.file, data, 0o600); err != nil {
		ln.Close()
		return nil, err
	}
	l.wg.Add(1)
	go l.accept()
	return l, nil
}

// Socket is the socket's path.
func (l *Listener) Socket() string { return l.socket }

// Close stops listening and ends every agent's connection; they leave
// their rooms once their servers stop, which Wait waits for. Close may
// be called in a turn: it takes no room's lock.
func (l *Listener) Close() error {
	l.mu.Lock()
	if l.closed {
		l.mu.Unlock()
		return nil
	}
	l.closed = true
	for c := range l.conns {
		c.Close()
	}
	l.mu.Unlock()
	err := l.ln.Close()
	os.Remove(l.file)
	os.Remove(l.socket)
	return err
}

// Wait waits for the connections to end after Close. It takes rooms'
// locks, so not in a turn.
func (l *Listener) Wait() { l.wg.Wait() }

func (l *Listener) accept() {
	defer l.wg.Done()
	for {
		c, err := l.ln.Accept()
		if err != nil {
			return
		}
		l.mu.Lock()
		if l.closed {
			l.mu.Unlock()
			c.Close()
			return
		}
		l.conns[c] = true
		l.wg.Add(1)
		l.mu.Unlock()
		go func() {
			defer l.wg.Done()
			l.serve(c)
			l.mu.Lock()
			delete(l.conns, c)
			l.mu.Unlock()
		}()
	}
}

// serve is one agent's connection: the hello, a seat in the room, and
// the MCP server on it until the agent goes.
func (l *Listener) serve(c net.Conn) {
	defer c.Close()
	br := bufio.NewReader(c)
	seat, err := l.greet(c, br)
	if err != nil {
		if !errors.Is(err, io.EOF) {
			json.NewEncoder(c).Encode(welcome{Error: err.Error()})
		}
		return
	}
	a := NewAgent(seat.seat)
	defer a.Leave()
	if err := json.NewEncoder(c).Encode(welcome{Workbook: a.Name(), Scope: a.Scope()}); err != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	srv := mcp.New(mcp.Options{Version: l.opts.Version, Live: a})
	ss, err := srv.Connect(ctx, &sdk.IOTransport{Reader: io.NopCloser(br), Writer: c}, nil)
	if err != nil {
		return
	}
	go a.watch(ctx, seat.kick, srv, ss)
	ss.Wait()
}

// greeted is the seat the agent was given, and what tells it the room
// changed.
type greeted struct {
	seat *room.Seat
	kick chan struct{}
}

// greet reads the agent's hello and seats it in its room.
func (l *Listener) greet(c net.Conn, br *bufio.Reader) (greeted, error) {
	if err := checkPeer(c); err != nil {
		return greeted{}, err
	}
	c.SetReadDeadline(time.Now().Add(10 * time.Second))
	line, err := br.ReadBytes('\n')
	if err != nil {
		return greeted{}, io.EOF // a probe (Endpoints) or a client gone
	}
	c.SetReadDeadline(time.Time{})
	var h hello
	if err := json.Unmarshal(line, &h); err != nil || h.Version != 1 {
		return greeted{}, errors.New("this isn't 012 mcp --attach of the same version: update the one the agent runs")
	}
	key, err := l.opts.Room(h.Room)
	if err != nil {
		return greeted{}, err
	}
	link := &agentLink{name: agentName(h.Client), kick: make(chan struct{}, 1)}
	seat, ok := l.opts.Registry.JoinOpen(key, link)
	if !ok {
		return greeted{}, errors.New("nobody has that workbook open any more")
	}
	return greeted{seat: seat, kick: link.kick}, nil
}

// agentName is how the others see the agent: its MCP client's name.
func agentName(client string) string {
	client = strings.TrimSpace(client)
	if client == "" {
		return "agent"
	}
	if r := []rune(client); len(r) > 20 {
		client = string(r[:20])
	}
	return client
}

// agentLink is the agent as a room's participant: telling it the room
// changed kicks its watch.
type agentLink struct {
	name string
	kick chan struct{}
}

func (p *agentLink) Name() string { return p.name }
func (p *agentLink) Agent()       {}

func (p *agentLink) Notify() {
	select {
	case p.kick <- struct{}{}:
	default:
	}
}

// watch is the agent's Notify: when the person settles one of its
// suggestions, the news waits for its next write and clients subscribed
// to the suggestions resource are told; when the room closes, the agent
// is let go. It runs until ctx ends.
func (a *Agent) watch(ctx context.Context, kick <-chan struct{}, srv *mcp.Server, ss *sdk.ServerSession) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-kick:
		}
		if len(a.report()) > 0 {
			srv.SuggestionsChanged(ctx)
		}
		closed := false
		a.seat.Do(func(*sheet.Workbook) { closed = a.seat.Closed() })
		if closed {
			ss.Close()
			return
		}
	}
}
