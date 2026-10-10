//go:build linux

package runtimefacts

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

const socketName = "snapshot.sock"

type inodeIdentity struct{ device, inode uint64 }

type linuxEndpoint struct {
	listener                                                                       *net.UnixListener
	tmpFD, baseFD, directoryFD                                                     int
	tmpIdentity, baseIdentity, directoryIdentity, socketIdentity, listenerIdentity inodeIdentity
	uid                                                                            uint32
	baseName, component, path                                                      string
}

func nativeIdentity() (ProcessIdentity, error) {
	stat, err := readSmallNativeFile("/proc/self/stat", 4096)
	if err != nil {
		return ProcessIdentity{}, errors.New("runtime facts process identity unavailable")
	}
	identity, err := parseProcessStat(stat, os.Getpid(), os.Getuid())
	if err != nil {
		return ProcessIdentity{}, err
	}
	boot, err := readSmallNativeFile("/proc/sys/kernel/random/boot_id", 128)
	if err != nil {
		return ProcessIdentity{}, errors.New("runtime facts boot identity unavailable")
	}
	bootID := strings.TrimSuffix(string(boot), "\n")
	if !validBootID(bootID) {
		return ProcessIdentity{}, errors.New("runtime facts boot identity invalid")
	}
	identity.BootID = bootID
	return identity, nil
}

func readSmallNativeFile(path string, maximum int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(io.LimitReader(file, maximum+1))
	if err != nil || int64(len(data)) > maximum {
		return nil, errors.New("runtime facts native identity bound exceeded")
	}
	return data, nil
}

func parseProcessStat(raw []byte, pid, uid int) (ProcessIdentity, error) {
	text := string(raw)
	start, end := strings.IndexByte(text, '('), strings.LastIndexByte(text, ')')
	if start <= 0 || end < start || end+1 >= len(text) {
		return ProcessIdentity{}, errors.New("runtime facts process stat invalid")
	}
	actualPID, err := strconv.Atoi(strings.TrimSpace(text[:start]))
	fields := strings.Fields(text[end+1:])
	if err != nil || actualPID != pid || pid <= 0 || uid < 0 || len(fields) < 20 {
		return ProcessIdentity{}, errors.New("runtime facts process stat identity mismatch")
	}
	ticks, err := strconv.ParseUint(fields[19], 10, 64)
	if err != nil || ticks == 0 {
		return ProcessIdentity{}, errors.New("runtime facts process start identity invalid")
	}
	return ProcessIdentity{PID: pid, UID: uid, StartTimeTicks: ticks}, nil
}

func validBootID(value string) bool {
	if len(value) != 36 {
		return false
	}
	for index, c := range value {
		if index == 8 || index == 13 || index == 18 || index == 23 {
			if c != '-' {
				return false
			}
			continue
		}
		if allowed := (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f'); !allowed {
			return false
		}
	}
	return true
}

func createNativeEndpoint(component string) (nativeEndpoint, error) {
	e := &linuxEndpoint{tmpFD: -1, baseFD: -1, directoryFD: -1, uid: uint32(os.Getuid()), baseName: "qs-runtime-facts-" + strconv.Itoa(os.Getuid()), component: component}
	success := false
	defer func() {
		if !success {
			e.closeFDs()
		}
	}()
	var err error
	e.tmpFD, err = unix.Open("/tmp", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, errors.New("runtime facts private root unavailable")
	}
	var tmp unix.Stat_t
	if err = unix.Fstat(e.tmpFD, &tmp); err != nil || tmp.Mode&unix.S_IFMT != unix.S_IFDIR || tmp.Uid != 0 || (tmp.Mode&0022 != 0 && tmp.Mode&unix.S_ISVTX == 0) {
		return nil, errors.New("runtime facts private root unsafe")
	}
	e.tmpIdentity = statIdentity(tmp)
	e.baseFD, e.baseIdentity, err = openPrivateDirectory(e.tmpFD, e.baseName, e.uid)
	if err != nil {
		return nil, err
	}
	e.directoryFD, e.directoryIdentity, err = openPrivateDirectory(e.baseFD, component, e.uid)
	if err != nil {
		return nil, err
	}
	e.path = filepath.Join("/tmp", e.baseName, component, socketName)
	if e.verifyDirectories() != nil {
		return nil, errors.New("runtime facts private directory identity changed")
	}
	var existing unix.Stat_t
	if err = unix.Fstatat(e.directoryFD, socketName, &existing, unix.AT_SYMLINK_NOFOLLOW); err == nil || !errors.Is(err, unix.ENOENT) {
		return nil, errors.New("runtime facts socket collision")
	}
	// Binding through the original directory FD prevents a pathname replacement
	// from redirecting creation into another directory.
	address := &net.UnixAddr{Name: fmt.Sprintf("/proc/self/fd/%d/%s", e.directoryFD, socketName), Net: "unix"}
	e.listener, err = net.ListenUnix("unix", address)
	if err != nil {
		return nil, errors.New("runtime facts private socket unavailable")
	}
	e.listener.SetUnlinkOnClose(false)
	created := false
	defer func() {
		if !success {
			_ = e.listener.Close()
			if created {
				_ = e.unlinkOwnedSocket()
			}
		}
	}()
	var socket unix.Stat_t
	if err = unix.Fstatat(e.directoryFD, socketName, &socket, unix.AT_SYMLINK_NOFOLLOW); err != nil || socket.Mode&unix.S_IFMT != unix.S_IFSOCK || socket.Uid != e.uid {
		return nil, errors.New("runtime facts private socket identity unavailable")
	}
	e.socketIdentity, created = statIdentity(socket), true
	if err = unix.Fchmodat(e.directoryFD, socketName, 0600, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return nil, errors.New("runtime facts private socket permissions unavailable")
	}
	if err = e.listenerStat(func(stat unix.Stat_t) error { e.listenerIdentity = statIdentity(stat); return nil }); err != nil {
		return nil, err
	}
	if err = e.Verify(); err != nil {
		return nil, err
	}
	success = true
	return e, nil
}

func openPrivateDirectory(parent int, name string, uid uint32) (int, inodeIdentity, error) {
	if err := unix.Mkdirat(parent, name, 0700); err != nil && !errors.Is(err, unix.EEXIST) {
		return -1, inodeIdentity{}, errors.New("runtime facts private directory unavailable")
	}
	fd, err := unix.Openat(parent, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return -1, inodeIdentity{}, errors.New("runtime facts private directory unsafe")
	}
	var stat unix.Stat_t
	if err = unix.Fstat(fd, &stat); err != nil || stat.Mode&unix.S_IFMT != unix.S_IFDIR || stat.Mode&07777 != 0700 || stat.Uid != uid {
		_ = unix.Close(fd)
		return -1, inodeIdentity{}, errors.New("runtime facts private directory ownership or mode invalid")
	}
	return fd, statIdentity(stat), nil
}

func (e *linuxEndpoint) Listener() *net.UnixListener { return e.listener }
func (e *linuxEndpoint) Path() string                { return e.path }

func (e *linuxEndpoint) verifyDirectories() error {
	var actual unix.Stat_t
	if unix.Lstat("/tmp", &actual) != nil || !e.validRoot(actual) || unix.Fstat(e.tmpFD, &actual) != nil || !e.validRoot(actual) {
		return errors.New("runtime facts original root changed")
	}
	for _, entry := range []struct {
		fd, parent int
		name       string
		identity   inodeIdentity
	}{{e.baseFD, e.tmpFD, e.baseName, e.baseIdentity}, {e.directoryFD, e.baseFD, e.component, e.directoryIdentity}} {
		if unix.Fstat(entry.fd, &actual) != nil || statIdentity(actual) != entry.identity || actual.Mode&unix.S_IFMT != unix.S_IFDIR || actual.Mode&07777 != 0700 || actual.Uid != e.uid {
			return errors.New("runtime facts original private directory changed")
		}
		if unix.Fstatat(entry.parent, entry.name, &actual, unix.AT_SYMLINK_NOFOLLOW) != nil || statIdentity(actual) != entry.identity || actual.Mode&unix.S_IFMT != unix.S_IFDIR || actual.Mode&07777 != 0700 || actual.Uid != e.uid {
			return errors.New("runtime facts private directory path changed")
		}
	}
	return nil
}

func (e *linuxEndpoint) validRoot(actual unix.Stat_t) bool {
	return statIdentity(actual) == e.tmpIdentity && actual.Mode&unix.S_IFMT == unix.S_IFDIR && actual.Uid == 0 && (actual.Mode&0022 == 0 || actual.Mode&unix.S_ISVTX != 0)
}

func (e *linuxEndpoint) Verify() error {
	if err := e.verifyDirectories(); err != nil {
		return err
	}
	var socket unix.Stat_t
	if unix.Fstatat(e.directoryFD, socketName, &socket, unix.AT_SYMLINK_NOFOLLOW) != nil || statIdentity(socket) != e.socketIdentity || socket.Mode&unix.S_IFMT != unix.S_IFSOCK || socket.Mode&07777 != 0600 || socket.Uid != e.uid {
		return errors.New("runtime facts original private socket changed")
	}
	return e.listenerStat(func(actual unix.Stat_t) error {
		if statIdentity(actual) != e.listenerIdentity || actual.Mode&unix.S_IFMT != unix.S_IFSOCK {
			return errors.New("runtime facts original listener changed")
		}
		return nil
	})
}

func (e *linuxEndpoint) listenerStat(check func(unix.Stat_t) error) error {
	connection, err := e.listener.SyscallConn()
	if err != nil {
		return errors.New("runtime facts listener descriptor unavailable")
	}
	var inner error
	if err = connection.Control(func(fd uintptr) {
		var stat unix.Stat_t
		inner = unix.Fstat(int(fd), &stat)
		if inner == nil {
			inner = check(stat)
		}
	}); err != nil || inner != nil {
		return errors.New("runtime facts listener descriptor identity unavailable")
	}
	return nil
}

func (e *linuxEndpoint) Close() error {
	_ = e.listener.Close()
	defer e.closeFDs()
	return e.unlinkOwnedSocket()
}

func (e *linuxEndpoint) unlinkOwnedSocket() error {
	if err := e.verifyDirectories(); err != nil {
		return err
	}
	var socket unix.Stat_t
	if unix.Fstatat(e.directoryFD, socketName, &socket, unix.AT_SYMLINK_NOFOLLOW) != nil || statIdentity(socket) != e.socketIdentity || socket.Mode&unix.S_IFMT != unix.S_IFSOCK || socket.Uid != e.uid {
		return errors.New("runtime facts socket cleanup identity changed")
	}
	if err := unix.Unlinkat(e.directoryFD, socketName, 0); err != nil {
		return errors.New("runtime facts owned socket cleanup failed")
	}
	return nil
}

func (e *linuxEndpoint) closeFDs() {
	for _, fd := range []int{e.directoryFD, e.baseFD, e.tmpFD} {
		if fd >= 0 {
			_ = unix.Close(fd)
		}
	}
	e.directoryFD, e.baseFD, e.tmpFD = -1, -1, -1
}

func statIdentity(stat unix.Stat_t) inodeIdentity {
	return inodeIdentity{device: uint64(stat.Dev), inode: stat.Ino}
}

func verifyNativePeer(conn *net.UnixConn, expectedUID int) error {
	raw, err := conn.SyscallConn()
	if err != nil {
		return errors.New("runtime facts peer credentials unavailable")
	}
	var credential *unix.Ucred
	var inner error
	// An ancestor PID namespace's peer is not visible in this namespace, so
	// Linux can report PID 0. Only the kernel-authenticated root UID may use
	// that case; PID 0 supplies no process identity authority.
	if err = raw.Control(func(fd uintptr) { credential, inner = unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED) }); err != nil || inner != nil || credential == nil || credential.Pid < 0 || (credential.Pid == 0 && credential.Uid != 0) || expectedUID < 0 || (credential.Uid != 0 && credential.Uid != uint32(expectedUID)) {
		return errors.New("runtime facts peer credentials rejected")
	}
	return nil
}
