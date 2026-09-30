//go:build linux || darwin || freebsd

package cowork

import (
	"errors"
	"fmt"
	"net"
	"os"
	"syscall"
)

// checkPeer refuses a connection from another user's process: the
// socket's permissions already keep them out, and this holds even if
// they were loosened.
func checkPeer(c net.Conn) error {
	uc, ok := c.(*net.UnixConn)
	if !ok {
		return errors.New("not a Unix socket")
	}
	raw, err := uc.SyscallConn()
	if err != nil {
		return err
	}
	uid, perr := 0, error(nil)
	if err := raw.Control(func(fd uintptr) { uid, perr = peerUID(fd) }); err != nil {
		return err
	}
	if perr != nil {
		return perr
	}
	if uid != os.Getuid() {
		return fmt.Errorf("refused a connection from user %d: only %d's own processes may attach", uid, os.Getuid())
	}
	return nil
}

// checkOwner refuses a file or folder another user owns, or that
// others can reach.
func checkOwner(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return nil
	}
	if int(st.Uid) != os.Getuid() {
		return fmt.Errorf("%s belongs to user %d, not you: 012 attaches only to your own sessions", path, st.Uid)
	}
	if info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("%s can be reached by other users (%v): 012 attaches only to sockets that are yours alone", path, info.Mode().Perm())
	}
	return nil
}
