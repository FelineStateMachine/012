package e2e

import (
	"bytes"
	"regexp"

	ghostty "go.mitchellh.com/libghostty"
)

// Keys as a terminal sends them. The program asks for the kitty keyboard
// protocol (Bubble Tea asks for its first level, 012 for key events too)
// and libghostty encodes keys for what it turned on, so sessions are
// Ghostty's. A session with legacyKeys never sees those requests, as a
// terminal without the protocol ignores them, and sends keys the legacy
// way.

// keyboardModes are the sequences that push, pop, set and query the kitty
// keyboard protocol's flags, and turn modifyOtherKeys on or off.
var keyboardModes = regexp.MustCompile(`\x1b\[(?:[>=<?][0-9;]*u|>4(?:;[0-9]*)?m)`)

// keyFilter takes keyboardModes out of the program's output, keeping a
// sequence split between reads until its end arrives.
type keyFilter struct {
	carry []byte
}

// filter returns data without keyboardModes. A nil filter changes
// nothing.
func (f *keyFilter) filter(data []byte) []byte {
	if f == nil {
		return data
	}
	data = append(f.carry, data...)
	f.carry = nil
	if i := bytes.LastIndexByte(data, 0x1b); i >= 0 && unfinished(data[i:]) {
		f.carry = append([]byte(nil), data[i:]...)
		data = data[:i]
	}
	return keyboardModes.ReplaceAll(data, nil)
}

// unfinished reports whether seq, which starts with Esc, may be the
// start of a CSI sequence yet to end.
func unfinished(seq []byte) bool {
	if len(seq) < 2 {
		return true
	}
	if seq[1] != '[' {
		return false
	}
	for _, c := range seq[2:] {
		if c >= 0x40 && c <= 0x7e {
			return false
		}
	}
	return true
}

// legacyBytes is what a terminal without the kitty keyboard protocol or
// modifyOtherKeys sends for the keys libghostty tells apart even then,
// and whether key is one: no release at all, Enter whatever the
// modifiers, Ctrl and a letter as a control character, and Alt and a
// letter as Esc before the letter.
func legacyBytes(action ghostty.KeyAction, key ghostty.Key, mods ghostty.Mods, unshifted rune) ([]byte, bool) {
	letter := unshifted >= 'a' && unshifted <= 'z'
	switch {
	case action == ghostty.KeyActionRelease:
		return nil, true
	case key == ghostty.KeyEnter && mods&ghostty.ModAlt != 0:
		return []byte("\x1b\r"), true
	case key == ghostty.KeyEnter:
		return []byte("\r"), true
	case letter && mods == ghostty.ModCtrl:
		return []byte{byte(unshifted) & 0x1f}, true
	case letter && mods == ghostty.ModAlt:
		return []byte{0x1b, byte(unshifted)}, true
	case letter && mods == ghostty.ModAlt|ghostty.ModShift:
		return []byte{0x1b, byte(unshifted) - 'a' + 'A'}, true
	}
	return nil, false
}

// hold presses a key written as for keys ("<space>", "<shift+enter>")
// and keeps it down: the terminal repeats it once, as it does for a key
// held.
func (s *session) hold(name string) {
	s.t.Helper()
	s.keys(name)
	s.namedKeyAction(ghostty.KeyActionRepeat, name)
}

// release lets go of a key held.
func (s *session) release(name string) {
	s.t.Helper()
	s.namedKeyAction(ghostty.KeyActionRelease, name)
}

// namedKeyAction sends a repeat or release of a key in angle brackets.
func (s *session) namedKeyAction(action ghostty.KeyAction, name string) {
	s.t.Helper()
	name = name[1 : len(name)-1]
	key, ok := namedKeys[name]
	if !ok {
		s.t.Fatalf("unknown key <%s>", name)
	}
	var cp rune
	if name == "space" {
		cp = ' '
	}
	s.keyEvent(action, key, 0, "", cp)
}

// kittyFlags are the keyboard protocol flags the program has on.
func (s *session) kittyFlags() ghostty.KittyKeyFlags {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, _ := s.vt.KittyKeyboardFlags()
	return f
}

var namedKeys = map[string]ghostty.Key{
	"enter": ghostty.KeyEnter, "esc": ghostty.KeyEscape, "backspace": ghostty.KeyBackspace,
	"tab": ghostty.KeyTab, "delete": ghostty.KeyDelete, "home": ghostty.KeyHome, "end": ghostty.KeyEnd,
	"up": ghostty.KeyArrowUp, "down": ghostty.KeyArrowDown,
	"left": ghostty.KeyArrowLeft, "right": ghostty.KeyArrowRight,
	"pgup": ghostty.KeyPageUp, "pgdown": ghostty.KeyPageDown,
	"f1": ghostty.KeyF1, "f2": ghostty.KeyF2, "f5": ghostty.KeyF5, "f9": ghostty.KeyF9, "f10": ghostty.KeyF10, "f11": ghostty.KeyF11,
	"space": ghostty.KeySpace,
}

type physKey struct {
	key  ghostty.Key
	mods ghostty.Mods
	base rune // unshifted character, for shifted symbols
}

// charKeys maps typed characters to physical keys on a US layout.
var charKeys = func() map[rune]physKey {
	m := map[rune]physKey{' ': {key: ghostty.KeySpace}}
	for i := range 26 {
		k := ghostty.KeyA + ghostty.Key(i)
		m[rune('a'+i)] = physKey{key: k}
		m[rune('A'+i)] = physKey{key: k, mods: ghostty.ModShift, base: rune('a' + i)}
	}
	digits := ")!@#$%^&*("
	for i := range 10 {
		k := ghostty.KeyDigit0 + ghostty.Key(i)
		m[rune('0'+i)] = physKey{key: k}
		m[rune(digits[i])] = physKey{key: k, mods: ghostty.ModShift, base: rune('0' + i)}
	}
	for _, p := range []struct {
		plain, shifted rune
		key            ghostty.Key
	}{
		{'-', '_', ghostty.KeyMinus}, {'=', '+', ghostty.KeyEqual},
		{',', '<', ghostty.KeyComma}, {'.', '>', ghostty.KeyPeriod},
		{'/', '?', ghostty.KeySlash}, {';', ':', ghostty.KeySemicolon},
		{'\'', '"', ghostty.KeyQuote}, {'`', '~', ghostty.KeyBackquote},
		{'\\', '|', ghostty.KeyBackslash}, {'[', '{', ghostty.KeyBracketLeft}, {']', '}', ghostty.KeyBracketRight},
	} {
		m[p.plain] = physKey{key: p.key}
		m[p.shifted] = physKey{key: p.key, mods: ghostty.ModShift, base: p.plain}
	}
	return m
}()
