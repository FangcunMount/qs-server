//go:build linux

package compatibilityretirementstop

import (
	"golang.org/x/sys/unix"
	"unsafe"
)

func unnamedSessionStream(fd int) bool {
	// x/sys renders both unnamed and empty abstract Linux addresses as "@".
	// Only the raw AF_UNIX family itself may be present at both stream ends.
	unnamed := func(trap uintptr) bool {
		var address unix.RawSockaddrAny
		length := uint32(unix.SizeofSockaddrAny)
		_, _, errno := unix.Syscall(trap, uintptr(fd), uintptr(unsafe.Pointer(&address)), uintptr(unsafe.Pointer(&length)))
		return errno == 0 && length == 2 && address.Addr.Family == unix.AF_UNIX
	}
	return unnamed(unix.SYS_GETSOCKNAME) && unnamed(unix.SYS_GETPEERNAME)
}
