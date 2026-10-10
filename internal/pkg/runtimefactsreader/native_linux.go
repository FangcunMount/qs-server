//go:build linux

package runtimefactsreader

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"

	"github.com/FangcunMount/qs-server/internal/pkg/runtimefacts"
	"golang.org/x/sys/unix"
)

type fileIdentity struct {
	device, inode uint64
	uid, mode     uint32
	links         uint64
}

func fileID(s unix.Stat_t) fileIdentity {
	return fileIdentity{uint64(s.Dev), s.Ino, s.Uid, s.Mode, uint64(s.Nlink)}
}

// An ancestor's link count legitimately changes when an unrelated sibling
// directory is added. Device, inode, owner and mode remain pinned. The private
// component leaf and socket continue to use the complete identity.
func ancestorID(s unix.Stat_t) fileIdentity {
	identity := fileID(s)
	identity.links = 0
	return identity
}

func endpointDirectoryID(index int, s unix.Stat_t) fileIdentity {
	if index < 2 { // tmp and qs-runtime-facts-UID are ancestors of component.
		return ancestorID(s)
	}
	return fileID(s)
}

type queryEndpoint struct {
	process    BorrowedProcess
	root       fileIdentity
	fds        []int
	parents    []int
	names      []string
	identities []fileIdentity
	socketFD   int
}

// QuerySnapshot is Linux root-only and never manufactures a root grant. The
// caller supplies the original owner's proc/root FDs and pinned identity. The
// reader owns only its newly opened child directory/socket FDs and query conn.
func QuerySnapshot(ctx context.Context, p BorrowedProcess) (runtimefacts.Snapshot, NativeIdentity, error) {
	var snapshot runtimefacts.Snapshot
	var identity NativeIdentity
	if ctx == nil || ctx.Err() != nil || os.Getuid() != 0 || os.Geteuid() != 0 || p.Proc == nil || p.Root == nil || p.HostPID <= 0 || p.UID < 0 || p.StartTimeTicks == 0 || !validComponent(p.Component) || !validHex(p.SourceSHA, 40) {
		return snapshot, identity, errors.New("runtime facts original root process binding unavailable")
	}
	q, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	endpoint, e := openEndpoint(p)
	if e != nil {
		return snapshot, identity, e
	}
	defer endpoint.close()
	if e = endpoint.verify(); e != nil {
		return snapshot, identity, e
	}
	challenge, e := newChallenge()
	if e != nil {
		return snapshot, identity, e
	}
	directory := endpoint.fds[len(endpoint.fds)-1]
	address := fmt.Sprintf("/proc/self/fd/%d/snapshot.sock", directory)
	conn, e := (&net.Dialer{}).DialContext(q, "unix", address)
	if e != nil {
		return snapshot, identity, errors.New("runtime facts original socket query unavailable")
	}
	defer func() { _ = conn.Close() }()
	unixConn, ok := conn.(*net.UnixConn)
	if !ok {
		return snapshot, identity, errors.New("runtime facts native socket unavailable")
	}
	deadline, ok := q.Deadline()
	if !ok || unixConn.SetDeadline(deadline) != nil {
		return snapshot, identity, errors.New("runtime facts query deadline unavailable")
	}
	stop := context.AfterFunc(q, func() { _ = unixConn.Close() })
	defer stop()
	if e = verifyServerPeer(unixConn, p.HostPID, p.UID); e != nil {
		return snapshot, identity, e
	}
	if e = endpoint.verify(); e != nil {
		return snapshot, identity, e
	}
	request, e := json.Marshal(runtimefacts.Query{FormatVersion: runtimefacts.QueryVersion, Action: "GET", Challenge: challenge})
	if e != nil {
		return snapshot, identity, errors.New("runtime facts query encoding unavailable")
	}
	request = append(request, '\n')
	for len(request) > 0 {
		n, e := unixConn.Write(request)
		if e != nil || n <= 0 {
			return snapshot, identity, errors.New("runtime facts query write incomplete")
		}
		request = request[n:]
	}
	if unixConn.CloseWrite() != nil {
		return snapshot, identity, errors.New("runtime facts query half-close failed")
	}
	raw, e := io.ReadAll(io.LimitReader(unixConn, queryLimit+1))
	if e != nil || len(raw) > queryLimit {
		return snapshot, identity, errors.New("runtime facts query response incomplete")
	}
	snapshot, e = decodeResponse(raw, challenge)
	if e != nil {
		return snapshot, identity, e
	}
	if snapshot.Component != p.Component || snapshot.SourceSHA != p.SourceSHA || snapshot.Process.UID != p.UID || snapshot.Process.PID != 1 || snapshot.Process.StartTimeTicks != p.StartTimeTicks || snapshot.Process.BootID != p.BootID {
		return runtimefacts.Snapshot{}, identity, errors.New("runtime facts response process binding changed")
	}
	if e = verifyServerPeer(unixConn, p.HostPID, p.UID); e != nil {
		return runtimefacts.Snapshot{}, identity, e
	}
	if q.Err() != nil || endpoint.verify() != nil {
		return runtimefacts.Snapshot{}, identity, errors.New("runtime facts native identity changed after query")
	}
	socket := endpoint.identities[len(endpoint.identities)-1]
	identity = NativeIdentity{HostPID: p.HostPID, UID: p.UID, StartTimeTicks: p.StartTimeTicks, BootID: p.BootID, RootDevice: endpoint.root.device, RootInode: endpoint.root.inode, SocketDevice: socket.device, SocketInode: socket.inode}
	return snapshot, identity, nil
}

func verifyProcess(p BorrowedProcess) (fileIdentity, error) {
	var original, current unix.Stat_t
	var filesystem unix.Statfs_t
	procFD := int(p.Proc.Fd())
	if unix.Fstatfs(procFD, &filesystem) != nil || uint64(filesystem.Type) != uint64(unix.PROC_SUPER_MAGIC) || unix.Fstat(procFD, &original) != nil || original.Mode&unix.S_IFMT != unix.S_IFDIR {
		return fileIdentity{}, errors.New("runtime facts original proc descriptor invalid")
	}
	// Re-open only the fixed native /proc PID directory to compare against the
	// borrowed FD. No arbitrary symlink supplied by a request is followed.
	fresh, e := unix.Open("/proc/"+strconv.Itoa(p.HostPID), unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if e != nil {
		return fileIdentity{}, errors.New("runtime facts pinned process missing")
	}
	defer func() { _ = unix.Close(fresh) }()
	if unix.Fstat(fresh, &current) != nil || ancestorID(original) != ancestorID(current) {
		return fileIdentity{}, errors.New("runtime facts pinned proc descriptor changed")
	}
	stat, e := readAt(procFD, "stat", 4096)
	if e != nil {
		return fileIdentity{}, e
	}
	ticks, e := parseStat(stat, p.HostPID)
	if e != nil || ticks != p.StartTimeTicks {
		return fileIdentity{}, errors.New("runtime facts pinned process start changed")
	}
	status, e := readAt(procFD, "status", 16384)
	if e != nil {
		return fileIdentity{}, e
	}
	uid, nativePID, e := parseStatus(status)
	if e != nil || uid != p.UID || nativePID != 1 {
		return fileIdentity{}, errors.New("runtime facts pinned process UID or PID1 changed")
	}
	boot, e := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if e != nil || len(boot) > 128 || strings.TrimSuffix(string(boot), "\n") != p.BootID || !validBoot(p.BootID) {
		return fileIdentity{}, errors.New("runtime facts pinned boot identity changed")
	}
	// /proc/PID/root is a kernel magic link. Its only permitted use is this exact
	// original process comparison. Arbitrary directory traversal uses nofollow.
	root, e := unix.Openat(procFD, "root", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if e != nil {
		return fileIdentity{}, errors.New("runtime facts pinned namespace root unavailable")
	}
	defer func() { _ = unix.Close(root) }()
	if unix.Fstat(int(p.Root.Fd()), &original) != nil || unix.Fstat(root, &current) != nil || original.Mode&unix.S_IFMT != unix.S_IFDIR || ancestorID(original) != ancestorID(current) {
		return fileIdentity{}, errors.New("runtime facts original namespace root changed")
	}
	return ancestorID(original), nil
}

func openEndpoint(p BorrowedProcess) (*queryEndpoint, error) {
	root, e := verifyProcess(p)
	if e != nil {
		return nil, e
	}
	endpoint := &queryEndpoint{process: p, root: root, socketFD: -1}
	success := false
	defer func() {
		if !success {
			endpoint.close()
		}
	}()
	parent := int(p.Root.Fd())
	for i, name := range []string{"tmp", "qs-runtime-facts-" + strconv.Itoa(p.UID), p.Component} {
		fd, e := unix.Openat(parent, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if e != nil {
			return nil, errors.New("runtime facts original private directory unavailable")
		}
		endpoint.fds = append(endpoint.fds, fd)
		endpoint.parents = append(endpoint.parents, parent)
		endpoint.names = append(endpoint.names, name)
		var st unix.Stat_t
		if unix.Fstat(fd, &st) != nil || st.Mode&unix.S_IFMT != unix.S_IFDIR || (i == 0 && (st.Uid != 0 || st.Mode&0022 != 0 && st.Mode&unix.S_ISVTX == 0)) || (i > 0 && (st.Uid != uint32(p.UID) || st.Mode&07777 != 0700)) {
			return nil, errors.New("runtime facts private directory ownership invalid")
		}
		endpoint.identities = append(endpoint.identities, endpointDirectoryID(i, st))
		parent = fd
	}
	fd, e := unix.Openat(parent, "snapshot.sock", unix.O_PATH|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if e != nil {
		return nil, errors.New("runtime facts original socket descriptor unavailable")
	}
	endpoint.socketFD = fd
	var socket unix.Stat_t
	if unix.Fstat(fd, &socket) != nil || socket.Mode&unix.S_IFMT != unix.S_IFSOCK || socket.Uid != uint32(p.UID) || socket.Mode&07777 != 0600 || socket.Nlink != 1 {
		return nil, errors.New("runtime facts original socket identity invalid")
	}
	endpoint.identities = append(endpoint.identities, fileID(socket))
	success = true
	return endpoint, nil
}

func (e *queryEndpoint) verify() error {
	root, err := verifyProcess(e.process)
	if err != nil || root != e.root {
		return errors.New("runtime facts original process root changed")
	}
	var actual unix.Stat_t
	for i, fd := range e.fds {
		if unix.Fstat(fd, &actual) != nil || endpointDirectoryID(i, actual) != e.identities[i] || unix.Fstatat(e.parents[i], e.names[i], &actual, unix.AT_SYMLINK_NOFOLLOW) != nil || endpointDirectoryID(i, actual) != e.identities[i] {
			return errors.New("runtime facts original directory path changed")
		}
	}
	original := e.identities[len(e.identities)-1]
	if unix.Fstat(e.socketFD, &actual) != nil || fileID(actual) != original || unix.Fstatat(e.fds[len(e.fds)-1], "snapshot.sock", &actual, unix.AT_SYMLINK_NOFOLLOW) != nil || fileID(actual) != original {
		return errors.New("runtime facts original socket path changed")
	}
	return nil
}
func (e *queryEndpoint) close() {
	if e.socketFD >= 0 {
		_ = unix.Close(e.socketFD)
	}
	for _, fd := range e.fds {
		_ = unix.Close(fd)
	}
}

func verifyServerPeer(c *net.UnixConn, pid, uid int) error {
	raw, e := c.SyscallConn()
	if e != nil {
		return errors.New("runtime facts server credential unavailable")
	}
	var credential *unix.Ucred
	var inner error
	e = raw.Control(func(fd uintptr) { credential, inner = unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED) })
	if e != nil || inner != nil || credential == nil || int(credential.Pid) != pid || int(credential.Uid) != uid {
		return errors.New("runtime facts original server credential mismatch")
	}
	return nil
}
func readAt(fd int, name string, max int64) ([]byte, error) {
	opened, e := unix.Openat(fd, name, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if e != nil {
		return nil, errors.New("runtime facts original proc field unavailable")
	}
	f := os.NewFile(uintptr(opened), name)
	defer func() { _ = f.Close() }()
	b, e := io.ReadAll(io.LimitReader(f, max+1))
	if e != nil || int64(len(b)) > max {
		return nil, errors.New("runtime facts original proc field bound exceeded")
	}
	return b, nil
}
func parseStat(b []byte, pid int) (uint64, error) {
	s := string(b)
	start, end := strings.IndexByte(s, '('), strings.LastIndexByte(s, ')')
	if start <= 0 || end < start || end+1 >= len(s) {
		return 0, errors.New("runtime facts proc stat malformed")
	}
	actual, e := strconv.Atoi(strings.TrimSpace(s[:start]))
	fields := strings.Fields(s[end+1:])
	if e != nil || actual != pid || len(fields) < 20 {
		return 0, errors.New("runtime facts proc stat mismatch")
	}
	ticks, e := strconv.ParseUint(fields[19], 10, 64)
	if e != nil || ticks == 0 {
		return 0, errors.New("runtime facts proc start invalid")
	}
	return ticks, nil
}
func parseStatus(b []byte) (int, int, error) {
	uid, pid := -1, -1
	seenUID, seenPID := false, false
	for _, line := range strings.Split(string(b), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if fields[0] == "Uid:" {
			if seenUID || len(fields) != 5 {
				return 0, 0, errors.New("runtime facts proc UID malformed")
			}
			seenUID = true
			for _, v := range fields[1:] {
				n, e := strconv.Atoi(v)
				if e != nil || n < 0 || uid >= 0 && n != uid {
					return 0, 0, errors.New("runtime facts proc UID inconsistent")
				}
				uid = n
			}
		}
		if fields[0] == "NSpid:" {
			if seenPID || len(fields) < 2 {
				return 0, 0, errors.New("runtime facts proc PID namespace malformed")
			}
			seenPID = true
			for _, v := range fields[1:] {
				n, e := strconv.Atoi(v)
				if e != nil || n <= 0 {
					return 0, 0, errors.New("runtime facts proc namespace PID invalid")
				}
				pid = n
			}
		}
	}
	if !seenUID || !seenPID {
		return 0, 0, errors.New("runtime facts proc UID or namespace missing")
	}
	return uid, pid, nil
}
func validBoot(s string) bool {
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
	return true
}
