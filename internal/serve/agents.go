package serve

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/FelineStateMachine/012/internal/cowork"
)

// ListenForAgents lets agents on the server's machine, run by the
// server's user, join the files people have open (live mode,
// docs/agents/live.md): 012 mcp --attach names a file in the served
// folder, or a named room (@name), and the agent sits in its room with
// the people there. It needs sharing on.
func (s *Server) ListenForAgents(version string) (*cowork.Listener, error) {
	if s.rooms == nil {
		return nil, errors.New("sharing is off (--share off): agents join shared workbooks")
	}
	return cowork.Listen(cowork.Options{Registry: s.rooms, Room: s.agentRoom, Kind: "serve", Workbook: s.Dir(), Version: version})
}

// agentRoom is the room of the file the agent names, which someone must
// have open.
func (s *Server) agentRoom(name string) (string, error) {
	if name == "" {
		return "", errors.New("name the file to join: 012 mcp --attach budget.012")
	}
	key := name
	if !strings.HasPrefix(name, "@") {
		p, err := s.root.Resolve(name)
		if err != nil {
			return "", fmt.Errorf("%s: %w", name, err)
		}
		if key, err = filepath.Abs(p); err != nil {
			return "", err
		}
	}
	if !s.rooms.Has(key) {
		return "", fmt.Errorf("nobody has %s open in 012 serve: an agent joins a file someone has open", name)
	}
	return key, nil
}
