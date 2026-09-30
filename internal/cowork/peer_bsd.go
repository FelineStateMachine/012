//go:build darwin || freebsd

package cowork

import "golang.org/x/sys/unix"

// peerUID is the user of the process at the other end of a Unix
// socket, as the kernel says (LOCAL_PEERCRED).
func peerUID(fd uintptr) (int, error) {
	c, err := unix.GetsockoptXucred(int(fd), unix.SOL_LOCAL, unix.LOCAL_PEERCRED)
	if err != nil {
		return 0, err
	}
	return int(c.Uid), nil
}
