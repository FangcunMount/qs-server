//go:build linux

package runtimefacts

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

type pidNamespacePacket struct {
	Phase     string `json:"phase"`
	Path      string `json:"path"`
	Namespace string `json:"namespace"`
	PeerPID   int32  `json:"peer_pid"`
	PeerUID   uint32 `json:"peer_uid"`
}

// This opt-in fixture uses an actual parent root client and a CLONE_NEWPID
// server. It neither supplies credentials nor changes a production peer gate.
func TestNativeLinuxRootPeerAcrossPIDNamespace(t *testing.T) {
	if os.Getenv("QS_RUNTIME_FACTS_PIDNS_TEST") != "1" {
		t.Skip("requires the explicit Linux root CAP_SYS_ADMIN PID namespace fixture")
	}
	if os.Getuid() != 0 || os.Geteuid() != 0 {
		t.Fatal("PID namespace native fixture requires actual root")
	}
	if os.Getenv("QS_RUNTIME_FACTS_PIDNS_CHILD") == "1" {
		servePIDNamespaceFixture(t)
		return
	}
	source := os.Getenv("QS_RUNTIME_FACTS_PIDNS_SOURCE_SHA")
	if len(source) != 40 || strings.Trim(source, "0123456789abcdef") != "" {
		t.Fatal("PID namespace fixture source binding missing")
	}
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		t.Fatal(err)
	}
	component := "native-pidns-" + hex.EncodeToString(nonce[:8])
	directory, err := os.MkdirTemp("/tmp", "qs-runtime-pidns-")
	if err != nil {
		t.Fatal(err)
	}
	var directoryIdentity unix.Stat_t
	if unix.Lstat(directory, &directoryIdentity) != nil || directoryIdentity.Uid != 0 || directoryIdentity.Mode&07777 != 0700 {
		t.Fatal("native fixture directory identity invalid")
	}
	parentNamespace, err := os.Readlink("/proc/self/ns/pid")
	if err != nil {
		t.Fatal(err)
	}
	program, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, program, "-test.run=^TestNativeLinuxRootPeerAcrossPIDNamespace$", "-test.timeout=20s")
	cmd.SysProcAttr = &syscall.SysProcAttr{Cloneflags: unix.CLONE_NEWPID | unix.CLONE_NEWNS, Pdeathsig: syscall.SIGKILL}
	cmd.Env = []string{"PATH=/usr/bin:/bin", "QS_RUNTIME_FACTS_PIDNS_TEST=1", "QS_RUNTIME_FACTS_PIDNS_CHILD=1", "QS_RUNTIME_FACTS_PIDNS_DIRECTORY=" + directory, "QS_RUNTIME_FACTS_PIDNS_COMPONENT=" + component, "QS_RUNTIME_FACTS_PIDNS_SOURCE_SHA=" + source}
	// Caller-owned pipes preserve the final cleanup reply even when the child
	// exits immediately; Cmd.Wait cannot close these read/write descriptors.
	outRead, outWrite, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = outRead.Close(); _ = outWrite.Close() }()
	inRead, inWrite, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = inRead.Close(); _ = inWrite.Close() }()
	var stderr bytes.Buffer
	cmd.Stdin, cmd.Stdout, cmd.Stderr = inRead, outWrite, &stderr
	if err = cmd.Start(); err != nil {
		t.Fatalf("native CLONE_NEWPID/CLONE_NEWNS start failed: %v", err)
	}
	_ = inRead.Close()
	_ = outWrite.Close()
	_ = outRead.SetReadDeadline(time.Now().Add(24 * time.Second))
	wait := make(chan error, 1)
	go func() { wait <- cmd.Wait() }()
	decoder := json.NewDecoder(io.LimitReader(outRead, 8192))
	decoder.DisallowUnknownFields()
	ownerStarted := false
	defer func() {
		_, _ = io.WriteString(inWrite, "finish\n")
		_ = inWrite.Close()
		var closed pidNamespacePacket
		closedOK := ownerStarted && decoder.Decode(&closed) == nil && closed.Phase == "owner_closed"
		waitError := <-wait
		if waitError != nil || !closedOK {
			t.Errorf("native child terminal/cleanup unproven; fixture retained: exit=%v stderr=%q", waitError, stderr.String())
			return
		}
		var current unix.Stat_t
		if unix.Lstat(directory, &current) != nil || statIdentity(current) != statIdentity(directoryIdentity) || current.Uid != 0 || current.Mode&07777 != 0700 || os.Remove(directory) != nil {
			t.Error("owned fixture directory cleanup failed")
		}
	}()
	var probe pidNamespacePacket
	if decoder.Decode(&probe) != nil || probe.Phase != "probe_ready" || probe.Path != filepath.Join(directory, "probe.sock") || probe.Namespace == parentNamespace {
		t.Fatal("real child PID namespace/probe binding missing")
	}
	conn := dialPIDNamespaceFixture(t, probe.Path, cmd.Process.Pid)
	_ = conn.Close()
	var ready pidNamespacePacket
	if decoder.Decode(&ready) != nil || ready.Phase != "owner_ready" || ready.PeerPID != 0 || ready.PeerUID != 0 || ready.Namespace != probe.Namespace || ready.Path != filepath.Join("/tmp", "qs-runtime-facts-0", component, socketName) {
		t.Fatal("actual kernel outer root credentials were not PID 0 / UID 0")
	}
	ownerStarted = true
	conn = dialPIDNamespaceFixture(t, ready.Path, cmd.Process.Pid)
	defer func() { _ = conn.Close() }()
	var challenge [32]byte
	if _, err = rand.Read(challenge[:]); err != nil {
		t.Fatal(err)
	}
	query := Query{FormatVersion: QueryVersion, Action: "snapshot", Challenge: hex.EncodeToString(challenge[:])}
	raw, err := json.Marshal(query)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = conn.Write(append(raw, '\n')); err != nil {
		t.Fatal(err)
	}
	if err = conn.CloseWrite(); err != nil {
		t.Fatal(err)
	}
	raw, err = io.ReadAll(io.LimitReader(conn, maxResponseBytes+1))
	if err != nil {
		t.Fatalf("root PID namespace query failed: %v", err)
	}
	var response Response
	if len(raw) == 0 || len(raw) > maxResponseBytes || json.Unmarshal(raw, &response) != nil || response.FormatVersion != QueryVersion || response.Challenge != query.Challenge || response.Snapshot.SourceSHA != source || response.Snapshot.Component != component || response.Snapshot.Process.PID != 1 || response.Snapshot.Process.UID != 0 || response.Snapshot.Process.StartTimeTicks == 0 || !response.Snapshot.ObservationComplete || response.Snapshot.BrokerConnectionsVerified {
		t.Fatal("public snapshot did not accept the actual outer root peer with PID 0")
	}
	t.Logf("native_pidns_root_query_complete=true kernel_client_pid=0 kernel_client_uid=0 server_namespace_pid=1 server_host_pid=%d broker_connections_verified=false", cmd.Process.Pid)
}

func dialPIDNamespaceFixture(t *testing.T, path string, serverPID int) *net.UnixConn {
	t.Helper()
	conn, err := net.DialUnix("unix", nil, &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	if err = conn.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		_ = conn.Close()
		t.Fatal(err)
	}
	credential, err := actualPIDNamespacePeer(conn)
	if err != nil || credential.Pid != int32(serverPID) || credential.Uid != 0 {
		_ = conn.Close()
		t.Fatal("client did not observe the exact real server host PID/root UID")
	}
	return conn
}

func actualPIDNamespacePeer(conn *net.UnixConn) (*unix.Ucred, error) {
	raw, err := conn.SyscallConn()
	if err != nil {
		return nil, err
	}
	var credential *unix.Ucred
	var inner error
	if err = raw.Control(func(fd uintptr) { credential, inner = unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED) }); err != nil {
		return nil, err
	}
	if inner != nil || credential == nil {
		return nil, fmt.Errorf("actual kernel peer credentials unavailable: %w", inner)
	}
	return credential, nil
}

func servePIDNamespaceFixture(t *testing.T) {
	t.Helper()
	if os.Getpid() != 1 {
		t.Fatal("child is not the actual new PID namespace init")
	}
	// /proc must describe this actual child namespace. The separate mount
	// namespace and private propagation prevent changing the parent mount.
	if unix.Mount("", "/", "", unix.MS_REC|unix.MS_PRIVATE, "") != nil || unix.Mount("proc", "/proc", "proc", unix.MS_NOSUID|unix.MS_NODEV|unix.MS_NOEXEC, "") != nil {
		t.Fatal("native private proc mount failed")
	}
	namespace, err := os.Readlink("/proc/self/ns/pid")
	if err != nil {
		t.Fatal(err)
	}
	probePath := filepath.Join(os.Getenv("QS_RUNTIME_FACTS_PIDNS_DIRECTORY"), "probe.sock")
	probe, err := net.ListenUnix("unix", &net.UnixAddr{Name: probePath, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	probe.SetUnlinkOnClose(false)
	if os.Chmod(probePath, 0600) != nil {
		t.Fatal("native probe mode failed")
	}
	var stamp unix.Stat_t
	if unix.Lstat(probePath, &stamp) != nil || stamp.Uid != 0 || stamp.Mode&unix.S_IFMT != unix.S_IFSOCK || stamp.Mode&07777 != 0600 {
		t.Fatal("native probe identity failed")
	}
	encoder := json.NewEncoder(os.Stdout)
	if encoder.Encode(pidNamespacePacket{Phase: "probe_ready", Path: probePath, Namespace: namespace}) != nil {
		t.Fatal("probe binding output failed")
	}
	conn, err := probe.AcceptUnix()
	if err != nil {
		t.Fatal(err)
	}
	credential, err := actualPIDNamespacePeer(conn)
	_ = conn.Close()
	_ = probe.Close()
	var current unix.Stat_t
	if err != nil || unix.Lstat(probePath, &current) != nil || statIdentity(current) != statIdentity(stamp) || current.Uid != 0 || current.Mode&07777 != 0600 || os.Remove(probePath) != nil {
		t.Fatal("probe actual credentials/owned cleanup failed")
	}
	component := os.Getenv("QS_RUNTIME_FACTS_PIDNS_COMPONENT")
	owner, err := New(component, os.Getenv("QS_RUNTIME_FACTS_PIDNS_SOURCE_SHA"))
	if err != nil {
		t.Fatal(err)
	}
	transport := testTransport(owner, "native-pidns-publisher", "publisher")
	if owner.Declare(transport) != nil || owner.MarkStarted(transport.ID) != nil || owner.Start() != nil {
		t.Fatal("public native owner startup failed")
	}
	if encoder.Encode(pidNamespacePacket{Phase: "owner_ready", Path: owner.Path(), Namespace: namespace, PeerPID: credential.Pid, PeerUID: credential.Uid}) != nil {
		t.Fatal("owner binding output failed")
	}
	finish, err := io.ReadAll(io.LimitReader(os.Stdin, 32))
	if err != nil || string(finish) != "finish\n" || owner.Close() != nil {
		t.Fatal("original native owner terminal/cleanup failed")
	}
	if _, err = os.Lstat(owner.Path()); !os.IsNotExist(err) || os.Remove(filepath.Dir(owner.Path())) != nil {
		t.Fatal("original native owner socket/leaf remains")
	}
	if encoder.Encode(pidNamespacePacket{Phase: "owner_closed"}) != nil {
		t.Fatal("owner cleanup output failed")
	}
}
