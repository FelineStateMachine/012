package e2e

import (
	"sync"

	ghostty "go.mitchellh.com/libghostty"
)

func init() {
	namedKeys["f4"] = ghostty.KeyF4
}

// clipboard records what the program writes to the system clipboard
// through the terminal (OSC 52).
type clipboard struct {
	mu   sync.Mutex
	text string
}

func (c *clipboard) get() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.text
}

// watchClipboard makes the terminal accept clipboard writes and records
// them.
func (s *session) watchClipboard() *clipboard {
	c := &clipboard{}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.vt.SetEffectClipboardWrite(func(_ *ghostty.Terminal, w ghostty.ClipboardWrite) ghostty.ClipboardWriteReply {
		c.mu.Lock()
		defer c.mu.Unlock()
		for _, content := range w.Contents {
			if content.MIME == "text/plain" || c.text == "" {
				c.text = string(content.Data)
			}
		}
		return ghostty.ClipboardWriteReply{Result: ghostty.ClipboardWriteSuccess}
	})
	return c
}

// paste pastes text as the terminal would, with bracketed paste when the
// program asked for it.
func (s *session) paste(text string) {
	s.t.Helper()
	s.mu.Lock()
	bracketed, _ := s.vt.Mode(ghostty.ModeBracketedPaste)
	s.mu.Unlock()
	data, err := ghostty.PasteEncode([]byte(text), bracketed)
	if err != nil {
		s.t.Fatal(err)
	}
	if _, err := s.pty.Write(data); err != nil {
		s.t.Fatal(err)
	}
}
