//go:build !linux && !darwin && !freebsd

package cowork

import "net"

// checkPeer has no kernel to ask here (Windows, the other BSDs): the
// socket's folder, the user's own, keeps other users out.
func checkPeer(net.Conn) error { return nil }

// checkOwner leaves ownership to the folder's permissions here.
func checkOwner(string) error { return nil }
