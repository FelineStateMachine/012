package cowork

import "golang.org/x/sys/unix"

// peerUID is the user of the process at the other end of a Unix
// socket, as the kernel says (SO_PEERCRED).
func peerUID(fd uintptr) (int, error) {
	c, err := unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
	if err != nil {
		return 0, err
	}
	return int(c.Uid), nil
}
