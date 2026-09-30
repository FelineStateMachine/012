package cowork

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/FelineStateMachine/012/internal/config"
)

// Where sessions listen: a folder of the user's own (0700), the runtime
// folder's 012 when the system has one ($XDG_RUNTIME_DIR), else agents
// in 012's config folder. Each listening session has a Unix socket
// there (0600), <pid>-<n>.sock, and beside it an endpoint file,
// <pid>-<n>.json, saying what it is, which 012 mcp --attach reads to
// find it. Windows has Unix sockets too, and its folder is the user's.

// Endpoint is what a listening session says of itself.
type Endpoint struct {
	PID    int    `json:"pid"`
	Socket string `json:"socket"`
	// Kind is "session" for a 012 on a terminal, "serve" for 012 serve,
	// whose agents name the file they join.
	Kind string `json:"kind"`
	// Workbook is the session's workbook, as its title names it, or the
	// folder 012 serve serves.
	Workbook string    `json:"workbook"`
	Started  time.Time `json:"started"`
}

// SocketDir is the folder sessions listen in.
func SocketDir() (string, error) {
	if d := os.Getenv("XDG_RUNTIME_DIR"); d != "" && filepath.IsAbs(d) {
		return filepath.Join(d, "012"), nil
	}
	d, err := config.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "agents"), nil
}

// ownDir makes dir the user's own, refusing one others can reach.
func ownDir(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return err
	}
	return checkOwner(dir)
}

// Endpoints are the sessions listening in dir, newest first. Files
// left by sessions that are gone are removed.
func Endpoints(dir string) ([]Endpoint, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Endpoint
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var ep Endpoint
		if json.Unmarshal(data, &ep) != nil || !alive(ep.Socket) {
			os.Remove(path)
			os.Remove(strings.TrimSuffix(path, ".json") + ".sock")
			continue
		}
		out = append(out, ep)
	}
	slices.SortFunc(out, func(a, b Endpoint) int { return b.Started.Compare(a.Started) })
	return out, nil
}

// alive reports whether a session answers on socket.
func alive(socket string) bool {
	c, err := net.DialTimeout("unix", socket, time.Second)
	if err != nil {
		return false
	}
	c.Close()
	return true
}

// Pick is the endpoint target names among eps, and the file an agent
// of 012 serve joins: "" when only one session listens, a workbook's
// name, a pid, a socket's path, or with one 012 serve listening, the
// file to join.
func Pick(eps []Endpoint, target string) (Endpoint, string, error) {
	if strings.HasSuffix(target, ".sock") {
		return Endpoint{Socket: target, Kind: "session"}, "", nil
	}
	var match []Endpoint
	for _, ep := range eps {
		if target == "" || ep.Kind == "session" && (strings.EqualFold(ep.Workbook, target) || fmt.Sprint(ep.PID) == target ||
			strings.EqualFold(ep.Workbook, strings.TrimSuffix(filepath.Base(target), filepath.Ext(target)))) {
			match = append(match, ep)
		}
	}
	if len(match) == 0 && target != "" {
		for _, ep := range eps {
			if ep.Kind == "serve" {
				match = append(match, ep)
			}
		}
		if len(match) == 1 {
			return match[0], target, nil
		}
	}
	switch len(match) {
	case 1:
		if match[0].Kind == "serve" && target == "" {
			return Endpoint{}, "", fmt.Errorf("012 serve listens for agents on %s: name the file to join, 012 mcp --attach budget.012", match[0].Workbook)
		}
		return match[0], "", nil
	case 0:
		if target == "" {
			return Endpoint{}, "", errors.New("no 012 session is listening for agents: start one with 012 --listen, or File > Invite an agent")
		}
		return Endpoint{}, "", fmt.Errorf("no 012 session listening for agents has %s open%s", target, listing(eps))
	}
	return Endpoint{}, "", fmt.Errorf("several 012 sessions are listening: name one, 012 mcp --attach <name>%s", listing(match))
}

// listing lists endpoints for an error.
func listing(eps []Endpoint) string {
	if len(eps) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(":")
	for _, ep := range eps {
		what := ep.Workbook
		if ep.Kind == "serve" {
			what = "012 serve of " + ep.Workbook
		}
		fmt.Fprintf(&b, "\n  %s (pid %d)", what, ep.PID)
	}
	return b.String()
}
