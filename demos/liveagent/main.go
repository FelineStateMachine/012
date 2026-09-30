// Command liveagent is the agent of the live-mode tape (live.tape): it
// writes a household budget, then attaches to the 012 listening on it
// with 012 mcp --attach, as a host would, and works through a script:
// it points at the bills, suggests March's figures, waits for the
// person to settle them, asks whether to add a line for the gym and,
// told yes, suggests it.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"time"
)

const budget = `{"version": 2, "cells": {"A1": "Household budget", "A3": "Rent", "B3": "1200", "A4": "Food", "B4": "450",
	"A5": "Power", "B5": "90", "A6": "Total", "B6": "=SUM(B3:B5)"}}`

func main() {
	write := flag.String("write", "", "write the budget to this file and exit")
	bin := flag.String("012", "../bin/012", "the 012 to attach with")
	flag.Parse()
	if *write != "" {
		if err := os.WriteFile(*write, []byte(budget), 0o644); err != nil {
			fail(err)
		}
		return
	}
	a, err := attach(*bin)
	if err != nil {
		fail(err)
	}
	a.script()
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "liveagent:", err)
	os.Exit(1)
}

// agent speaks MCP's JSON-RPC to 012 mcp --attach.
type agent struct {
	in   *json.Encoder
	out  *bufio.Reader
	next int
}

// attach starts 012 mcp --attach once a session listens, and
// initializes the connection as the host called claude.
func attach(bin string) (*agent, error) {
	time.Sleep(3 * time.Second) // the session starts listening
	cmd := exec.Command(bin, "mcp", "--attach")
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	a := &agent{in: json.NewEncoder(in), out: bufio.NewReader(out)}
	a.call("initialize", map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{},
		"clientInfo": map[string]any{"name": "claude", "version": "1"}})
	a.in.Encode(map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"})
	return a, nil
}

// call sends a request and returns its result.
func (a *agent) call(method string, params any) map[string]any {
	a.next++
	a.in.Encode(map[string]any{"jsonrpc": "2.0", "id": a.next, "method": method, "params": params})
	for {
		line, err := a.out.ReadBytes('\n')
		if err != nil {
			fail(err)
		}
		var msg struct {
			ID     int             `json:"id"`
			Result map[string]any  `json:"result"`
			Error  json.RawMessage `json:"error"`
		}
		if json.Unmarshal(line, &msg) != nil || msg.ID != a.next {
			continue
		}
		if msg.Error != nil {
			fail(fmt.Errorf("%s: %s", method, msg.Error))
		}
		return msg.Result
	}
}

// tool calls a tool and returns its structured result.
func (a *agent) tool(name string, args map[string]any) map[string]any {
	res := a.call("tools/call", map[string]any{"name": name, "arguments": args})
	out, _ := res["structuredContent"].(map[string]any)
	return out
}

// settled waits until none of the agent's suggestions is pending.
func (a *agent) settled() {
	for {
		list, _ := a.tool("suggestions", nil)["suggestions"].([]any)
		pending := false
		for _, s := range list {
			if s.(map[string]any)["state"] == "pending" {
				pending = true
			}
		}
		if !pending {
			return
		}
		time.Sleep(300 * time.Millisecond)
	}
}

func (a *agent) script() {
	a.tool("read_range", map[string]any{"ref": "A1:B6"})
	a.tool("focus", map[string]any{"ref": "B3:B5"})
	time.Sleep(1500 * time.Millisecond)
	a.tool("write_cells", map[string]any{"message": "March's bills came in higher",
		"entries": []map[string]any{{"ref": "B4", "input": "480"}, {"ref": "B5", "input": "95"}}})
	a.settled()
	time.Sleep(1200 * time.Millisecond)
	answer := a.tool("ask", map[string]any{"message": "Add a line for the gym?"})
	if answer["action"] != "accept" {
		return
	}
	a.tool("apply_operations", map[string]any{"message": "the gym, as you said", "operations": []map[string]any{
		{"op": "insert_rows", "ref": "A6"},
		{"op": "set", "ref": "A6", "input": "Gym"},
		{"op": "set", "ref": "B6", "input": "40"}}})
	a.settled()
	time.Sleep(3 * time.Second)
}
