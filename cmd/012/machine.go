package main

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"

	"github.com/FelineStateMachine/012/internal/config"
)

// machineID identifies this computer to macros: files record the id of
// the computer their macros were made or trusted on, and 012 asks before
// running macros from anywhere else. It is a random id kept in the user's
// config directory (config.Dir),
// made on first use; "" when that isn't possible, in which case every
// file's macros ask once per session.
func machineID() string {
	dir, err := config.Dir()
	if err != nil {
		return ""
	}
	path := filepath.Join(dir, "machine-id")
	if data, err := os.ReadFile(path); err == nil {
		if id := strings.TrimSpace(string(data)); len(id) >= 16 {
			return id
		}
	}
	b := make([]byte, 16)
	rand.Read(b)
	id := hex.EncodeToString(b)
	if os.MkdirAll(dir, 0o700) != nil || os.WriteFile(path, []byte(id+"\n"), 0o600) != nil {
		return ""
	}
	return id
}
