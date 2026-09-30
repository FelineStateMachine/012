package cowork

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"path/filepath"
)

// Attach is 012 mcp --attach: it finds the session target names (see
// Pick), refusing a socket that isn't the user's alone, and carries the
// MCP client's messages from in to it and its answers to out until
// either side ends. The client's first message, its initialize request,
// names it to the others. When the session can't be reached, the
// initialize request is answered with the reason, which hosts show, and
// Attach returns it.
func Attach(target string, in io.Reader, out io.Writer) error {
	br := bufio.NewReader(in)
	first, err := br.ReadBytes('\n')
	if err != nil && len(first) == 0 {
		return err
	}
	c, err := dial(target, clientName(first))
	if err != nil {
		refuse(out, first, err)
		return err
	}
	defer c.conn.Close()
	if _, err := c.conn.Write(first); err != nil {
		return err
	}
	go func() {
		io.Copy(c.conn, br)
		if uc, ok := c.conn.(*net.UnixConn); ok {
			uc.CloseWrite()
		}
	}()
	_, err = io.Copy(out, c.r)
	return err
}

// attached is a connection past the welcome.
type attached struct {
	conn net.Conn
	r    *bufio.Reader
	w    welcome
}

// dial connects to the session target names, as client.
func dial(target, client string) (attached, error) {
	dir, err := SocketDir()
	if err != nil {
		return attached{}, err
	}
	eps, err := Endpoints(dir)
	if err != nil {
		return attached{}, err
	}
	ep, roomName, err := Pick(eps, target)
	if err != nil {
		return attached{}, err
	}
	if err := checkOwner(filepath.Dir(ep.Socket)); err != nil {
		return attached{}, err
	}
	if err := checkOwner(ep.Socket); err != nil {
		return attached{}, err
	}
	conn, err := net.Dial("unix", ep.Socket)
	if err != nil {
		return attached{}, fmt.Errorf("the session doesn't answer: %w", err)
	}
	if err := json.NewEncoder(conn).Encode(hello{Version: 1, Room: roomName, Client: client}); err != nil {
		conn.Close()
		return attached{}, err
	}
	a := attached{conn: conn, r: bufio.NewReader(conn)}
	line, err := a.r.ReadBytes('\n')
	if err == nil {
		err = json.Unmarshal(line, &a.w)
	}
	switch {
	case err != nil:
		conn.Close()
		return attached{}, fmt.Errorf("the session refused: %w", err)
	case a.w.Error != "":
		conn.Close()
		return attached{}, errors.New(a.w.Error)
	}
	return a, nil
}

// clientName is the MCP client's name from its initialize request: its
// title, else its name.
func clientName(initialize []byte) string {
	var req struct {
		Params struct {
			ClientInfo struct {
				Name  string `json:"name"`
				Title string `json:"title"`
			} `json:"clientInfo"`
		} `json:"params"`
	}
	json.Unmarshal(initialize, &req)
	if t := req.Params.ClientInfo.Title; t != "" {
		return t
	}
	return req.Params.ClientInfo.Name
}

// refuse answers the client's first request with err, as a JSON-RPC
// error its host shows.
func refuse(out io.Writer, first []byte, err error) {
	var req struct {
		ID json.RawMessage `json:"id"`
	}
	if json.Unmarshal(first, &req) != nil || len(req.ID) == 0 {
		return
	}
	resp, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": req.ID, "error": map[string]any{"code": -32000, "message": "012: " + err.Error()}})
	out.Write(append(resp, '\n'))
}
