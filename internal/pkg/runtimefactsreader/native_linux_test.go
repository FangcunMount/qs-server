//go:build linux

package runtimefactsreader

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/FangcunMount/qs-server/internal/pkg/runtimefacts"
	"golang.org/x/sys/unix"
)

func TestNativeParserKeepsPID1AndUIDBinding(t *testing.T) {
	fields := []string{"S"}
	for n := 1; n < 19; n++ {
		fields = append(fields, "0")
	}
	fields = append(fields, "9876", "4096")
	raw := []byte("123 (tricky ) name) " + strings.Join(fields, " ") + "\n")
	if ticks, e := parseStat(raw, 123); e != nil || ticks != 9876 {
		t.Fatal("actual stat field invalid", e)
	}
	if _, e := parseStat(raw, 124); e == nil {
		t.Fatal("different PID accepted")
	}
	good := []byte("Name:\tworker\nUid:\t501 501 501 501\nNSpid:\t123 1\n")
	if uid, pid, e := parseStatus(good); e != nil || uid != 501 || pid != 1 {
		t.Fatal("UID/PID namespace parse invalid", e)
	}
	for _, raw := range [][]byte{[]byte("Uid: 501 0 501 501\nNSpid: 123 1\n"), []byte("Uid: 501 501 501 501\n"), append(append([]byte{}, good...), []byte("NSpid: 123 1\n")...)} {
		if _, _, e := parseStatus(raw); e == nil {
			t.Fatal("ambiguous UID/namespace accepted")
		}
	}
}

// This opt-in fixture must actually run as Linux root PID1 in an isolated
// namespace. It is not a Docker owner/source-image/SSH/full Broker proof.
func TestNativeRootPID1QueryOriginalFDs(t *testing.T) {
	if os.Getenv("QS_RUNTIME_FACTS_READER_NATIVE_TEST") != "1" {
		t.Skip("requires explicit isolated Linux root PID1 fixture")
	}
	if os.Getuid() != 0 || os.Geteuid() != 0 || os.Getpid() != 1 {
		t.Fatal("fixture needs actual root PID1")
	}
	owner, e := runtimefacts.New("worker", strings.Repeat("a", 40))
	if e != nil {
		t.Fatal(e)
	}
	declared := runtimefacts.Transport{ID: "fixture-reader", Provider: "nsq", Direction: "consumer", LookupdAddresses: []string{"127.0.0.1:4161"}, ClientID: owner.ClientID("fixture-reader"), Hostname: owner.Hostname()}
	if e = owner.Declare(declared); e != nil {
		t.Fatal(e)
	}
	if e = owner.MarkSubscribed(declared.ID, runtimefacts.Subscription{Topic: "fixture-topic", Channel: "fixture-channel"}); e != nil {
		t.Fatal(e)
	}
	if e = owner.MarkStarted(declared.ID); e != nil {
		t.Fatal(e)
	}
	if e = owner.Start(); e != nil {
		t.Fatal(e)
	}
	defer func() {
		if e := owner.Close(); e != nil {
			t.Error(e)
		}
	}()
	actual := owner.Snapshot()
	proc, e := os.Open("/proc/" + strconv.Itoa(os.Getpid()))
	if e != nil {
		t.Fatal(e)
	}
	defer func() { _ = proc.Close() }()
	root, e := os.Open("/proc/self/root")
	if e != nil {
		t.Fatal(e)
	}
	defer func() { _ = root.Close() }()
	bound := BorrowedProcess{Proc: proc, Root: root, HostPID: os.Getpid(), UID: os.Getuid(), StartTimeTicks: actual.Process.StartTimeTicks, BootID: actual.Process.BootID, Component: "worker", SourceSHA: actual.SourceSHA}
	s, native, e := QuerySnapshot(context.Background(), bound)
	if e != nil || s.ProcessNonce != actual.ProcessNonce || native.HostPID != 1 || native.SocketInode == 0 {
		t.Fatal("actual original FD query failed", e)
	}
	if _, e = proc.Stat(); e != nil {
		t.Fatal("borrowed proc closed")
	}
	if _, e = root.Stat(); e != nil {
		t.Fatal("borrowed root closed")
	}
	t.Run("ancestor-sibling-keeps-original-leaf-and-socket-strict", func(t *testing.T) {
		endpoint, e := openEndpoint(bound)
		if e != nil {
			t.Fatal(e)
		}
		defer endpoint.close()
		for _, ancestor := range []string{"/tmp", "/tmp/qs-runtime-facts-" + strconv.Itoa(bound.UID)} {
			sibling, e := os.MkdirTemp(ancestor, "reader-owned-sibling-")
			if e != nil {
				t.Fatal(e)
			}
			defer func() {
				if e := os.Remove(sibling); e != nil {
					t.Error("exact owned sibling cleanup failed", e)
				}
			}()
			if e = endpoint.verify(); e != nil {
				t.Fatal("unrelated ancestor sibling invalidated original endpoint", e)
			}
		}
		if _, _, e = QuerySnapshot(context.Background(), bound); e != nil {
			t.Fatal("actual query after ancestor sibling churn failed", e)
		}
		leaf := "/tmp/qs-runtime-facts-" + strconv.Itoa(bound.UID) + "/worker"
		unexpected, e := os.MkdirTemp(leaf, "reader-owned-unexpected-")
		if e != nil {
			t.Fatal(e)
		}
		defer func() { _ = os.Remove(unexpected) }()
		if e = endpoint.verify(); e == nil {
			t.Fatal("private component leaf link mutation accepted")
		}
		if e = os.Remove(unexpected); e != nil {
			t.Fatal(e)
		}
		if e = endpoint.verify(); e != nil {
			t.Fatal("original leaf identity did not return after exact fixture cleanup", e)
		}
		socket := filepath.Join(leaf, "snapshot.sock")
		if e = os.Chmod(socket, 0640); e != nil {
			t.Fatal(e)
		}
		defer func() { _ = os.Chmod(socket, 0600) }()
		if e = endpoint.verify(); e == nil {
			t.Fatal("private socket mode mutation accepted")
		}
		if e = os.Chmod(socket, 0600); e != nil {
			t.Fatal(e)
		}
	})
	bad := bound
	bad.StartTimeTicks++
	if _, _, e = QuerySnapshot(context.Background(), bad); e == nil {
		t.Fatal("changed process accepted")
	}
	bad = bound
	bad.SourceSHA = strings.Repeat("b", 40)
	if _, _, e = QuerySnapshot(context.Background(), bad); e == nil {
		t.Fatal("changed source accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, e = QuerySnapshot(ctx, bound); e == nil {
		t.Fatal("cancelled native query accepted")
	}
}

func TestNativeAncestorIdentityUsesOriginalFDWithoutSiblingLinkCount(t *testing.T) {
	parent := t.TempDir()
	fd, e := unix.Open(parent, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if e != nil {
		t.Fatal(e)
	}
	defer func() { _ = unix.Close(fd) }()
	var before, after unix.Stat_t
	if e = unix.Fstat(fd, &before); e != nil {
		t.Fatal(e)
	}
	if e = os.Mkdir(filepath.Join(parent, "owned-sibling"), 0700); e != nil {
		t.Fatal(e)
	}
	if e = unix.Fstat(fd, &after); e != nil {
		t.Fatal(e)
	}
	if before.Nlink == after.Nlink {
		t.Fatal("fixture filesystem did not expose actual directory link churn")
	}
	if ancestorID(before) != ancestorID(after) || endpointDirectoryID(0, before) != endpointDirectoryID(0, after) || endpointDirectoryID(1, before) != endpointDirectoryID(1, after) {
		t.Fatal("same original ancestor FD invalidated by sibling")
	}
	if endpointDirectoryID(2, before) == endpointDirectoryID(2, after) || fileID(before) == fileID(after) {
		t.Fatal("leaf identity lost strict links")
	}
	changed := after
	changed.Mode ^= 0020
	if ancestorID(after) == ancestorID(changed) {
		t.Fatal("ancestor mode mutation accepted")
	}
	changed = after
	changed.Ino++
	if ancestorID(after) == ancestorID(changed) {
		t.Fatal("ancestor inode mutation accepted")
	}
}
