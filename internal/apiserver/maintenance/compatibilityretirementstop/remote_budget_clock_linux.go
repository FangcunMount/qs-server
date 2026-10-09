//go:build linux

package compatibilityretirementstop

import (
	"bytes"
	"golang.org/x/sys/unix"
	"io"
	"os"
	"regexp"
)

var remoteBootIDPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func readRemoteBootID() (boot string, err error) {
	fd, e := unix.Open("/proc/sys/kernel/random/boot_id", unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if e != nil {
		return "", ErrRemoteBudget
	}
	f := os.NewFile(uintptr(fd), "kernel-remote-budget-boot-id")
	defer func() {
		if f.Close() != nil {
			err = ErrRemoteBudget
		}
	}()
	var st unix.Stat_t
	var fs unix.Statfs_t
	if unix.Fstat(fd, &st) != nil || st.Mode&unix.S_IFMT != unix.S_IFREG || unix.Fstatfs(fd, &fs) != nil || fs.Type != unix.PROC_SUPER_MAGIC {
		return "", ErrRemoteBudget
	}
	raw, e := io.ReadAll(io.LimitReader(f, 38))
	if e != nil || len(raw) != 37 || raw[36] != '\n' || bytes.IndexByte(raw[:36], 0) >= 0 || !remoteBootIDPattern.MatchString(string(raw[:36])) {
		return "", ErrRemoteBudget
	}
	return string(raw[:36]), nil
}
func remoteBootClock() (string, int64, error) {
	before, e := readRemoteBootID()
	if e != nil {
		return "", 0, e
	}
	var t unix.Timespec
	if unix.ClockGettime(unix.CLOCK_BOOTTIME, &t) != nil || t.Sec < 0 || t.Nsec < 0 || t.Nsec >= 1e9 || t.Sec > (int64(^uint64(0)>>1)-t.Nsec)/1e9 {
		return "", 0, ErrRemoteBudget
	}
	after, e := readRemoteBootID()
	if e != nil || before != after {
		return "", 0, ErrRemoteBudget
	}
	return before, t.Nano(), nil
}
