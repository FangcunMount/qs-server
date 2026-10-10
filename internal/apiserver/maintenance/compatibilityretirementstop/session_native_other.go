//go:build !linux

package compatibilityretirementstop

import "golang.org/x/sys/unix"

func unnamedSessionStream(fd int) bool {
	local, err := unix.Getsockname(fd)
	if err != nil {
		return false
	}
	remote, err := unix.Getpeername(fd)
	if err != nil {
		return false
	}
	l, ok := local.(*unix.SockaddrUnix)
	r, peerOK := remote.(*unix.SockaddrUnix)
	return ok && peerOK && l.Name == "" && r.Name == ""
}
