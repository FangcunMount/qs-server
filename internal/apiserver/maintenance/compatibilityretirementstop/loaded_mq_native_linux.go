//go:build linux

package compatibilityretirementstop

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"strconv"
	"strings"

	reader "github.com/FangcunMount/qs-server/internal/pkg/runtimefactsreader"
	"golang.org/x/sys/unix"
)

func openLoadedMQProcess(ctx context.Context, v actualContainer, source, programHash string) (_ *loadedMQProcess, result error) {
	if ctx == nil || ctx.Err() != nil || os.Getuid() != 0 || os.Geteuid() != 0 || !v.Running || v.PID <= 0 || v.Component != "qs-worker" || !sha40.MatchString(source) || !hash64.MatchString(programHash) {
		return nil, ErrLoadedMQ
	}
	fd, e := unix.Open("/proc/"+strconv.Itoa(v.PID), unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if e != nil {
		return nil, ErrLoadedMQ
	}
	owner := &loadedMQProcess{process: reader.BorrowedProcess{Proc: os.NewFile(uintptr(fd), "loaded-worker-proc"), HostPID: v.PID, Component: "worker", SourceSHA: source}}
	defer func() {
		if result != nil {
			_ = owner.close()
		}
	}()
	var st unix.Stat_t
	var fs unix.Statfs_t
	if unix.Fstat(fd, &st) != nil || st.Mode&unix.S_IFMT != unix.S_IFDIR || unix.Fstatfs(fd, &fs) != nil || uint64(fs.Type) != uint64(unix.PROC_SUPER_MAGIC) {
		return nil, ErrLoadedMQ
	}
	status, e := readLoadedMQAt(ctx, fd, "status", 16384)
	if e != nil {
		return nil, e
	}
	uid, e := loadedMQUID(status)
	if e != nil {
		return nil, e
	}
	stat, e := readLoadedMQAt(ctx, fd, "stat", 4096)
	if e != nil {
		return nil, e
	}
	ticks, e := loadedMQStart(stat, v.PID)
	if e != nil {
		return nil, e
	}
	boot, e := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if e != nil || len(boot) > 128 || !validLoadedMQBoot(strings.TrimSuffix(string(boot), "\n")) {
		return nil, ErrLoadedMQ
	}
	root, e := unix.Openat(fd, "root", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0) // fixed kernel magic link only
	if e != nil {
		return nil, ErrLoadedMQ
	}
	owner.process.Root = os.NewFile(uintptr(root), "loaded-worker-root")
	owner.process.UID, owner.process.StartTimeTicks, owner.process.BootID = uid, ticks, strings.TrimSuffix(string(boot), "\n")
	if e = verifyLoadedMQExecutable(ctx, owner, programHash); e != nil {
		return nil, e
	}
	return owner, nil
}
func readLoadedMQAt(ctx context.Context, parent int, name string, limit int64) ([]byte, error) {
	fd, e := unix.Openat(parent, name, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if e != nil {
		return nil, ErrLoadedMQ
	}
	f := os.NewFile(uintptr(fd), "loaded-worker-metadata")
	b, e := io.ReadAll(io.LimitReader(f, limit+1))
	ce := f.Close()
	if e != nil || ce != nil || int64(len(b)) > limit || ctx.Err() != nil {
		return nil, ErrLoadedMQ
	}
	return b, nil
}
func verifyLoadedMQExecutable(ctx context.Context, p *loadedMQProcess, expected string) error {
	if ctx == nil || ctx.Err() != nil || p == nil || p.process.Proc == nil || !hash64.MatchString(expected) {
		return ErrLoadedMQ
	}
	fd, e := unix.Openat(int(p.process.Proc.Fd()), "exe", unix.O_RDONLY|unix.O_CLOEXEC, 0) // only original PID's kernel magic link
	if e != nil {
		return ErrLoadedMQ
	}
	f := os.NewFile(uintptr(fd), "loaded-worker-executable")
	h := sha256.New()
	b := make([]byte, 64<<10)
	total := int64(0)
	for {
		if ctx.Err() != nil {
			e = ErrLoadedMQ
			break
		}
		n, re := f.Read(b)
		total += int64(n)
		if total > 256<<20 {
			e = ErrLoadedMQ
			break
		}
		if n > 0 {
			_, _ = h.Write(b[:n])
		}
		if re == io.EOF {
			break
		}
		if re != nil {
			e = ErrLoadedMQ
			break
		}
	}
	ce := f.Close()
	if e != nil || ce != nil || total == 0 || ctx.Err() != nil || hex.EncodeToString(h.Sum(nil)) != expected {
		return ErrLoadedMQ
	}
	return nil
}
func loadedMQStart(raw []byte, pid int) (uint64, error) {
	s := string(raw)
	close := strings.LastIndex(s, ")")
	if !strings.HasPrefix(s, strconv.Itoa(pid)+" (") || close < 0 {
		return 0, ErrLoadedMQ
	}
	fields := strings.Fields(s[close+1:])
	if len(fields) < 20 {
		return 0, ErrLoadedMQ
	}
	value, e := strconv.ParseUint(fields[19], 10, 64)
	if e != nil || value == 0 {
		return 0, ErrLoadedMQ
	}
	return value, nil
}
func loadedMQUID(raw []byte) (int, error) {
	seenUID, seenPID := false, false
	uid := -1
	for _, line := range strings.Split(string(raw), "\n") {
		f := strings.Fields(line)
		if len(f) == 0 {
			continue
		}
		switch f[0] {
		case "Uid:":
			if seenUID || len(f) != 5 {
				return 0, ErrLoadedMQ
			}
			seenUID = true
			for _, s := range f[1:] {
				n, e := strconv.ParseUint(s, 10, 31)
				if e != nil || uid >= 0 && uid != int(n) {
					return 0, ErrLoadedMQ
				}
				uid = int(n)
			}
		case "NSpid:":
			if seenPID || len(f) < 2 || len(f) > 64 {
				return 0, ErrLoadedMQ
			}
			seenPID = true
			for i, s := range f[1:] {
				n, e := strconv.ParseUint(s, 10, 31)
				if e != nil || n == 0 || i == len(f)-2 && n != 1 {
					return 0, ErrLoadedMQ
				}
			}
		}
	}
	if !seenUID || !seenPID || uid < 0 {
		return 0, ErrLoadedMQ
	}
	return uid, nil
}
func validLoadedMQBoot(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				return false
			}
		} else if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return s != "00000000-0000-0000-0000-000000000000"
}
