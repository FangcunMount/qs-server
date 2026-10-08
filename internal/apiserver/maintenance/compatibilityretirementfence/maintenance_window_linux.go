//go:build linux

package compatibilityretirementfence

import (
	"bytes"
	"golang.org/x/sys/unix"
	"io"
	"os"
	"regexp"
	"time"
)

var windowBootIDPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func windowBootIDValid(id string) bool              { return windowBootIDPattern.MatchString(id) }
func windowStatTimes(st unix.Stat_t) (int64, int64) { return st.Mtim.Nano(), st.Ctim.Nano() }
func productionWindowOptions() (windowOptions, error) {
	if os.Geteuid() != 0 {
		return windowOptions{}, ErrWindowStore
	}
	return windowOptions{owner: 0, anchor: "/", clockSource: "linux_clock_boottime_v1", clock: systemWindowClock}, nil
}
func systemWindowBootID() (id string, err error) {
	fd, e := unix.Open("/proc/sys/kernel/random/boot_id", unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if e != nil {
		return "", ErrWindowClock
	}
	f := os.NewFile(uintptr(fd), "kernel-boot-identity")
	defer func() {
		if f.Close() != nil {
			err = ErrWindowClock
		}
	}()
	var st unix.Stat_t
	var fs unix.Statfs_t
	if unix.Fstat(fd, &st) != nil || st.Mode&unix.S_IFMT != unix.S_IFREG || unix.Fstatfs(fd, &fs) != nil || fs.Type != unix.PROC_SUPER_MAGIC {
		return "", ErrWindowClock
	}
	raw, e := io.ReadAll(io.LimitReader(f, 38))
	if e != nil || len(raw) != 37 || raw[36] != '\n' || !windowBootIDValid(string(raw[:36])) || bytes.IndexByte(raw[:36], 0) >= 0 {
		return "", ErrWindowClock
	}
	return string(raw[:36]), nil
}
func systemWindowClock() (windowClockSample, error) {
	before, e := systemWindowBootID()
	if e != nil {
		return windowClockSample{}, e
	}
	var t unix.Timespec
	if unix.ClockGettime(unix.CLOCK_BOOTTIME, &t) != nil || t.Sec < 0 || t.Nsec < 0 || t.Nsec >= int64(time.Second) || t.Sec > (int64(^uint64(0)>>1)-t.Nsec)/int64(time.Second) {
		return windowClockSample{}, ErrWindowClock
	}
	after, e := systemWindowBootID()
	if e != nil || before != after {
		return windowClockSample{}, ErrWindowClock
	}
	return windowClockSample{before, t.Nano(), time.Now().UTC()}, nil
}
