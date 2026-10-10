package compatibilityretirementstop

import (
	"context"
	"encoding/json"
	"go.mongodb.org/mongo-driver/bson"
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func descriptorFixture() Descriptor {
	return Descriptor{Version: 1, SourceSHA: strings.Repeat("a", 40), RuntimeSourceSHA: strings.Repeat("e", 40), ToolSourceSHA: strings.Repeat("b", 40), OriginalRunID: "124-1", OperationID: "123-1", ManifestSHA256: strings.Repeat("b", 64), HostRole: "server-a", MachineIDSHA256: strings.Repeat("c", 64), DockerPath: "/usr/bin/docker", DockerSHA256: strings.Repeat("d", 64), Containers: []Container{
		{ID: strings.Repeat("1", 64), Name: "/qs-apiserver", Image: "sha256:" + strings.Repeat("e", 64), Component: "qs-apiserver", Service: "qs-apiserver", Entrypoint: []string{"/app/qs-apiserver"}, Command: []string{"--config=/app/configs/apiserver.prod.yaml"}, Running: true, StartedAt: "2026-10-09T01:02:03Z", RestartPolicy: "unless-stopped"},
		{ID: strings.Repeat("2", 64), Name: "/qs-collection-server-1", Image: "sha256:" + strings.Repeat("f", 64), Component: "qs-collection-server", Project: "qs-collection", Service: "server", Entrypoint: []string{"/app/collection-server"}, Command: []string{"--config=/app/configs/collection-server.prod.yaml"}, Running: true, StartedAt: "2026-10-09T01:02:03Z", RestartPolicy: "unless-stopped"},
	}}
}
func TestDescriptorRequiresIndependentRuntimeSource(t *testing.T) {
	d := descriptorFixture()
	if d.SourceSHA == d.RuntimeSourceSHA || !validDescriptor(d) {
		t.Fatal("independent batch and runtime sources rejected")
	}
	for _, invalid := range []string{"", strings.Repeat("a", 39), strings.Repeat("A", 40), strings.Repeat("a", 41)} {
		v := d
		v.RuntimeSourceSHA = invalid
		if validDescriptor(v) {
			t.Fatal("missing or malformed runtime source accepted")
		}
	}
}

func TestDescriptorScopesActualSourceTopology(t *testing.T) {
	d := descriptorFixture()
	if !validDescriptor(d) {
		t.Fatal("source topology rejected")
	}
	d.HostRole = "server-b"
	if validDescriptor(d) {
		t.Fatal("IAM host adopted")
	}
	d = descriptorFixture()
	d.Containers[1].Service = "qs-collection-server"
	if validDescriptor(d) {
		t.Fatal("unconfirmed legacy service adopted")
	}
	d = descriptorFixture()
	d.Containers[1].ID = d.Containers[0].ID
	if validDescriptor(d) {
		t.Fatal("duplicate ID adopted")
	}
	d = descriptorFixture()
	d.Containers[0].Command = []string{"--config=/app/configs/dev.yaml"}
	if validDescriptor(d) {
		t.Fatal("wrong config adopted")
	}
}
func TestApprovedDescriptorCannotImportEvidence(t *testing.T) {
	for _, v := range []any{&Approval{}, &Lease{}, &DrainObservation{}} {
		if _, e := json.Marshal(v); e == nil {
			t.Fatal("opaque authority serialized")
		}
	}
	if _, err := (&Approval{}).WindowBinding(context.Background()); err == nil {
		t.Fatal("unapproved binding exported")
	}
	if (&Lease{}).Close() == nil {
		t.Fatal("zero lease closes FD")
	}
	if (&DrainObservation{}).Summary().Samples != 0 {
		t.Fatal("zero observation claims drain")
	}
}
func TestDuplicateOrUnknownApprovalKeysRejected(t *testing.T) {
	var d Descriptor
	for _, b := range []string{`{"version":1,"version":2}`, `{"containers":[{"id":"x","id":"y"}]}`, `{"ready":true}`, `{} {}`} {
		if exactJSON([]byte(b), &d) == nil {
			t.Fatal("ambiguous input accepted")
		}
	}
}
func TestMongoReadOnlyObservationRefusesMutationAndAmbiguity(t *testing.T) {
	samples := []struct {
		cmd  bson.M
		want bool
	}{
		{bson.M{"find": "a", "filter": bson.M{}}, true},
		{bson.M{"insert": "a", "documents": bson.A{}}, false},
		{bson.M{"find": "a", "insert": "a"}, false},
		{bson.M{"find": "a", "getMore": int64(2)}, false},
		{bson.M{"aggregate": 1, "pipeline": bson.A{bson.M{"$currentOp": bson.M{"allUsers": true}}, bson.M{"$match": bson.M{}}}}, true},
		{bson.M{"aggregate": "a", "pipeline": bson.A{bson.M{"$merge": "b"}}}, false},
		{bson.M{"aggregate": "a", "pipeline": bson.A{bson.M{"$lookup": bson.M{"pipeline": bson.A{bson.M{"$out": "b"}}}}}}, false},
		{bson.M{"aggregate": "a", "pipeline": bson.A{bson.M{"$unknownStage": 1}}}, false},
		{bson.M{"customCommand": 1}, false},
	}
	for i, s := range samples {
		if got := readOnlyCommand(s.cmd); got != s.want {
			t.Fatalf("sample %d: got %v", i, got)
		}
	}
}

func TestSessionControlRefusesImportedSuccessAndReplayedSequence(t *testing.T) {
	good := []byte(`{"protocol":"qs-fixed-host-service-session/v1","sequence":1,"action":"stop"}`)
	if _, e := parseSessionRequest(good, 1); e != nil {
		t.Fatal("fixed control rejected")
	}
	if _, e := parseSessionRequest(good, 2); e == nil {
		t.Fatal("replayed control accepted")
	}
	for _, v := range []string{`{"protocol":"qs-fixed-host-service-session/v1","sequence":1,"action":"drop"}`, `{"protocol":"qs-fixed-host-service-session/v1","sequence":1,"action":"stop","ready":true}`} {
		if _, e := parseSessionRequest([]byte(v), 1); e == nil {
			t.Fatal("unbound authority accepted")
		}
	}
}

func TestLiveSessionTransportsRequirePipeOrAnonymousUnixStream(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	defer func() { _ = w.Close() }()
	if !validSessionFD(r) || !validSessionFD(w) {
		t.Fatal("live pipe rejected")
	}
	fds, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM, 0)
	if err != nil {
		t.Fatal(err)
	}
	a, b := os.NewFile(uintptr(fds[0]), "session-in"), os.NewFile(uintptr(fds[1]), "session-out")
	defer func() { _ = a.Close() }()
	defer func() { _ = b.Close() }()
	if !validSessionFD(a) || !validSessionFD(b) {
		t.Fatal("connected anonymous Unix stream rejected")
	}
	if err := sessionWrite(context.Background(), a, SessionDiagnostic{Protocol: sessionProtocol, Sequence: 1, Outcome: "observed"}); err != nil {
		t.Fatal(err)
	}
	raw, err := sessionRead(context.Background(), b)
	if err != nil {
		t.Fatal(err)
	}
	var d SessionDiagnostic
	if err := exactJSON(raw, &d); err != nil || d.Sequence != 1 {
		t.Fatal("live stream reply did not round trip")
	}
	file, err := os.CreateTemp(t.TempDir(), "receipt")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	if validSessionFD(file) || validSessionFD(nil) {
		t.Fatal("imported file or missing transport accepted")
	}
	dgrams, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_DGRAM, 0)
	if err != nil {
		t.Fatal(err)
	}
	dgramA, dgramB := os.NewFile(uintptr(dgrams[0]), "datagram-a"), os.NewFile(uintptr(dgrams[1]), "datagram-b")
	defer func() { _ = dgramA.Close() }()
	defer func() { _ = dgramB.Close() }()
	if validSessionFD(dgramA) || validSessionFD(dgramB) {
		t.Fatal("datagram control transport accepted")
	}
}

func TestLiveSessionRejectsLinuxBoundLocalAndPeerUnixAddresses(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("actual Linux raw Unix address lengths required")
	}
	for _, name := range []string{"filesystem", "abstract", "empty_abstract"} {
		t.Run(name, func(t *testing.T) {
			fds, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM, 0)
			if err != nil {
				t.Fatal(err)
			}
			a, b := os.NewFile(uintptr(fds[0]), "bound-local"), os.NewFile(uintptr(fds[1]), "bound-peer")
			defer func() { _ = a.Close() }()
			defer func() { _ = b.Close() }()
			address := filepath.Join(t.TempDir(), "session.sock")
			if name == "abstract" {
				address = "@qs-retirement-" + filepath.Base(t.TempDir())
			} else if name == "empty_abstract" {
				address = "@"
			}
			if err = unix.Bind(fds[0], &unix.SockaddrUnix{Name: address}); err != nil {
				t.Fatal("actual bound fixture failed", err)
			}
			if validSessionFD(a) || validSessionFD(b) {
				t.Fatal("bound local or peer address accepted as anonymous")
			}
		})
	}
}
