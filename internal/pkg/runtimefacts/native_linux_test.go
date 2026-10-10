//go:build linux

package runtimefacts

import (
	"encoding/json"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestLinuxProcessStatUsesActualStartField(t *testing.T) {
	fields := []string{"S"}
	for n := 1; n < 19; n++ {
		fields = append(fields, "0")
	}
	fields = append(fields, "987654", "4096", "0")
	raw := []byte("123 (name with ) and spaces) " + strings.Join(fields, " ") + "\n")
	identity, err := parseProcessStat(raw, 123, 1000)
	if err != nil || identity.PID != 123 || identity.StartTimeTicks != 987654 || identity.UID != 1000 {
		t.Fatal("process stat identity parsed incorrectly")
	}
	for _, invalid := range [][]byte{[]byte("123 () S"), []byte(strings.Replace(string(raw), "987654", "0", 1)), raw} {
		expectedPID := 123
		if string(invalid) == string(raw) {
			expectedPID++
		}
		if _, err := parseProcessStat(invalid, expectedPID, 1000); err == nil {
			t.Fatal("invalid stat accepted")
		}
	}
}

// Native tests are explicit because they create a real UID-private Unix socket.
// A normal source test cannot stand in for this Linux process/peer proof.
func TestNativeLinuxPrivateQueryAndOwnedCleanup(t *testing.T) {
	if os.Getenv("QS_RUNTIME_FACTS_NATIVE_TEST") != "1" {
		t.Skip("requires explicit native Linux runtime-facts fixture")
	}
	seed, err := New("native-seed", strings.Repeat("a", 40))
	if err != nil {
		t.Fatal(err)
	}
	component := "native-test-" + seed.nonce[:12]
	_ = seed.Close()
	o, err := New(component, strings.Repeat("a", 40))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := o.Close(); err != nil {
			t.Error(err)
		}
	}()
	transport := testTransport(o, "native-publisher", "publisher")
	if err := o.Declare(transport); err != nil {
		t.Fatal(err)
	}
	if err := o.MarkStarted(transport.ID); err != nil {
		t.Fatal(err)
	}
	if err := o.Start(); err != nil {
		t.Fatal(err)
	}
	if got := o.Snapshot(); !got.ObservationComplete || got.BrokerConnectionsVerified || got.Process.PID != os.Getpid() || got.Process.UID != os.Getuid() || got.Process.StartTimeTicks == 0 || got.Process.BootID == "" {
		t.Fatal("native snapshot identity not qualified")
	}
	conn, err := net.DialUnix("unix", nil, &net.UnixAddr{Name: o.Path(), Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	if err := conn.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Write(queryLine(t)); err != nil {
		t.Fatal(err)
	}
	if err := conn.CloseWrite(); err != nil {
		t.Fatal(err)
	}
	raw, err := io.ReadAll(io.LimitReader(conn, maxResponseBytes+1))
	if err != nil {
		t.Fatal(err)
	}
	var response Response
	if len(raw) > maxResponseBytes || json.Unmarshal(raw, &response) != nil || response.Challenge != strings.Repeat("b", 64) || !response.Snapshot.ObservationComplete || response.Snapshot.ProcessNonce != o.nonce {
		t.Fatal("native response not bound")
	}
	if o.Start() == nil {
		t.Fatal("owner started twice")
	}
	if o.Declare(testTransport(o, "late-publisher", "publisher")) == nil {
		t.Fatal("declaration appended after snapshot opened")
	}
	path := o.Path()
	if err := o.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatal("owned socket remains")
	}
	_ = os.Remove("/tmp/qs-runtime-facts-" + strconv.Itoa(os.Getuid()) + "/" + component)
}

func TestNativeLinuxSocketCollisionIsPreserved(t *testing.T) {
	if os.Getenv("QS_RUNTIME_FACTS_NATIVE_TEST") != "1" {
		t.Skip("requires explicit native Linux runtime-facts fixture")
	}
	seed := testOwner(t)
	component := "native-collision-" + seed.nonce[:12]
	first, err := New(component, strings.Repeat("a", 40))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := first.Close(); err != nil {
			t.Error(err)
		}
	}()
	firstTransport := testTransport(first, "native-publisher", "publisher")
	if first.Declare(firstTransport) != nil || first.MarkStarted(firstTransport.ID) != nil || first.Start() != nil {
		t.Fatal("first native owner failed")
	}
	second, err := New(component, strings.Repeat("a", 40))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = second.Close() }()
	secondTransport := testTransport(second, "native-publisher", "publisher")
	if second.Declare(secondTransport) != nil || second.MarkStarted(secondTransport.ID) != nil || second.Start() == nil || second.Snapshot().ObservationComplete {
		t.Fatal("existing socket reused")
	}
	if !first.Snapshot().ObservationComplete {
		t.Fatal("colliding owner damaged original socket")
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	_ = os.Remove("/tmp/qs-runtime-facts-" + strconv.Itoa(os.Getuid()) + "/" + component)
}

func TestNativeLinuxReplacementCannotQualifyOrBeDeleted(t *testing.T) {
	if os.Getenv("QS_RUNTIME_FACTS_NATIVE_TEST") != "1" {
		t.Skip("requires explicit native Linux runtime-facts fixture")
	}
	seed := testOwner(t)
	component := "native-replace-" + seed.nonce[:12]
	o, err := New(component, strings.Repeat("a", 40))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = o.Close() }()
	transport := testTransport(o, "native-publisher", "publisher")
	if o.Declare(transport) != nil || o.MarkStarted(transport.ID) != nil || o.Start() != nil {
		t.Fatal("native owner failed")
	}
	path := o.Path()
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("test-owned replacement"), 0600); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.Remove(path)
		_ = os.Remove("/tmp/qs-runtime-facts-" + strconv.Itoa(os.Getuid()) + "/" + component)
	}()
	if got := o.Snapshot(); got.ObservationComplete || !contains(got.IncompleteReasons, "private_channel_identity_changed") {
		t.Fatal("replacement socket qualified")
	}
	if o.Close() == nil {
		t.Fatal("changed inode cleanup accepted")
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "test-owned replacement" {
		t.Fatal("foreign replacement was deleted or changed")
	}
}
