package compatibilityretirementstop

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

func ownedMaterialFixture(t *testing.T) *RootRemoteMaterials {
	t.Helper()
	root := t.TempDir()
	if e := os.Chmod(root, 0700); e != nil {
		t.Fatal(e)
	}
	a := &Approval{descriptor: descriptorFixture()}
	a.descriptor.HostRole = "server-d"
	m := newRemoteMaterials(root, a)
	t.Cleanup(func() { _ = m.Close() })
	for _, rel := range []string{"service-journal", "remote-budget", "service-invocations", "service-invocations/19-1"} {
		if e := os.Mkdir(filepath.Join(root, rel), 0700); e != nil {
			t.Fatal(e)
		}
	}
	for _, rel := range []string{".", "service-journal", "remote-budget", "service-invocations", "service-invocations/19-1"} {
		if e := m.openDir(rel, uint32(os.Geteuid())); e != nil {
			t.Fatal(e)
		}
	}
	for _, rel := range []string{"service-session-template.json", "approved-services.json", "service-invocations/19-1/service-session.json", "service-invocations/19-1/derived-service-session.intent.private.json", "service-journal/baseline.json", "remote-budget/grant-0001.json"} {
		raw := []byte("PRIVATE_TEMPORARY_BODY:" + rel)
		if e := os.WriteFile(filepath.Join(root, rel), raw, 0600); e != nil {
			t.Fatal(e)
		}
		if e := m.register(rel, digest(raw), uint32(os.Geteuid())); e != nil {
			t.Fatal(e)
		}
	}
	if e := m.checkLocked(); e != nil {
		t.Fatal(e)
	}
	return m
}

// Only the existing private filesystem kernel is exercised. These actual test
// FDs confer neither Linux-root approval nor an original Window/D terminal.
func TestLocalMaterialSealClosesWritersBeforeHeldFDHandoff(t *testing.T) {
	for _, scenario := range []string{"exact", "foreign", "handoff-failed"} {
		t.Run(scenario, func(t *testing.T) {
			m := ownedMaterialFixture(t)
			m.local = true
			m.approval.materials = m
			issuerFD, e := unix.Open(filepath.Join(m.root, "remote-budget"), unix.O_RDONLY|unix.O_DIRECTORY, 0)
			if e != nil {
				t.Fatal(e)
			}
			leaseFD, e := unix.Open(filepath.Join(m.root, "service-journal"), unix.O_RDONLY|unix.O_DIRECTORY, 0)
			if e != nil {
				t.Fatal(e)
			}
			i := &BudgetIssuer{approval: m.approval, dirFD: issuerFD}
			i.self = i
			l := &Lease{approval: m.approval, dirFD: leaseFD}
			l.self = l
			t.Cleanup(func() { _ = i.Close(); _ = l.Close() })
			if scenario == "foreign" {
				if e = os.WriteFile(filepath.Join(m.root, "foreign"), []byte("unregistered"), 0600); e != nil {
					t.Fatal(e)
				}
			}
			files, dirs := 0, 0
			checkClosed := func() {
				t.Helper()
				var st unix.Stat_t
				if !i.closed || !l.closed || i.dirFD != -1 || l.dirFD != -1 || !errors.Is(unix.Fstat(issuerFD, &st), unix.EBADF) || !errors.Is(unix.Fstat(leaseFD, &st), unix.EBADF) {
					t.Fatal("actual writers remained open at material handoff")
				}
			}
			e = m.sealLocalRegistered(t.Context(), i, l, func(path string, held *os.File) error {
				checkClosed()
				if _, e := held.Stat(); e != nil || !strings.HasPrefix(path, m.root) {
					t.Fatal("handoff lost its original held directory")
				}
				dirs++
				if scenario == "handoff-failed" {
					return errors.New("consumer refused original FD")
				}
				return nil
			}, func(path string, held *os.File, hash string) error {
				checkClosed()
				got, e := materialFileHash(held)
				if e != nil || got != hash || !strings.HasPrefix(path, m.root) {
					t.Fatal("handoff lost its original bytes")
				}
				files++
				return nil
			})
			if scenario == "exact" {
				if e != nil || files != 6 || dirs != 5 || m.LocalMaterialsSealed(i, l) != nil {
					t.Fatal("actual material handoff failed", files, dirs, e)
				}
				for _, v := range m.files {
					if _, e := v.file.Stat(); !errors.Is(e, os.ErrClosed) {
						t.Fatal("original raw material FD remained open")
					}
				}
				copy := &RootRemoteMaterials{self: m, local: true, approval: m.approval, sealed: true, closed: true}
				if copy.LocalMaterialsSealed(i, l) == nil {
					t.Fatal("copied owner became original sealed proof")
				}
			} else if e == nil || !m.failed.Load() || m.LocalMaterialsSealed(i, l) == nil || scenario == "foreign" && (files != 0 || dirs != 0) {
				t.Fatal("partial/foreign handoff became completion", files, dirs, e)
			}
			if _, e := os.Lstat(filepath.Join(m.root, "approved-services.json")); e != nil {
				t.Fatal("sealing deleted original material")
			}
		})
	}
	if _, e := OpenRootLocalMaterials(t.Context(), nil, nil, "", "", "", ""); e == nil {
		t.Fatal("missing native A approval issued material owner")
	}
	if new(RootRemoteMaterials).SealLocalMaterials(t.Context(), new(BudgetIssuer), new(Lease), new(RemoteController), new(RemoteMaterialZero), func(string, *os.File) error { return nil }, func(string, *os.File, string) error { return nil }) == nil {
		t.Fatal("saved/zero Window/D result sealed A materials")
	}
}
func TestRemoteMaterialsActualFileRegistrationRejectsDrift(t *testing.T) {
	for _, kind := range []string{"same-bytes-new-inode", "changed-body", "changed-mode", "hardlink", "symlink", "foreign", "foreign-directory"} {
		t.Run(kind, func(t *testing.T) {
			m := ownedMaterialFixture(t)
			p := filepath.Join(m.root, "approved-services.json")
			raw, e := os.ReadFile(p)
			if e != nil {
				t.Fatal(e)
			}
			switch kind {
			case "same-bytes-new-inode":
				if e = os.Remove(p); e == nil {
					e = os.WriteFile(p, raw, 0600)
				}
			case "changed-body":
				e = os.WriteFile(p, []byte("changed"), 0600)
			case "changed-mode":
				e = os.Chmod(p, 0640)
			case "hardlink":
				e = os.Link(p, filepath.Join(m.root, "foreign"))
			case "symlink":
				if e = os.Remove(p); e == nil {
					e = os.Symlink(filepath.Join(m.root, "service-session-template.json"), p)
				}
			case "foreign":
				e = os.WriteFile(filepath.Join(m.root, "foreign"), []byte("FOREIGN"), 0600)
			case "foreign-directory":
				e = os.Mkdir(filepath.Join(m.root, "foreign"), 0700)
			}
			if e != nil {
				t.Fatal(e)
			}
			if m.checkLocked() == nil {
				t.Fatal("unregistered/drifted body accepted")
			}
			if _, e = os.Lstat(filepath.Join(m.root, "service-session-template.json")); e != nil {
				t.Fatal("read-only preflight removed original assets")
			}
		})
	}
}
func TestRemoteMaterialsFilesystemKernelExactPurgeAndZero(t *testing.T) {
	m := ownedMaterialFixture(t)
	scope := m.scopeHash()
	// This is only the private POSIX kernel under current fixture UID. The public
	// Linux-root factory, signed native Window and Docker readback are not invoked.
	z, e := m.purgeRegistered(context.Background(), uint32(os.Geteuid()), strings.Repeat("a", 64), 7)
	if e != nil || z.ScopeSHA256 != scope || z.FilesRemoved != 6 || z.DirectoriesRemoved != 4 || z.RemainingTemporaryFiles != 0 || !m.zero {
		t.Fatalf("actual fixture cleanup failed: %+v %v", z, e)
	}
	names, e := os.ReadDir(m.root)
	if e != nil || len(names) != 2 {
		t.Fatal("extra/remaining temporary body")
	}
	for _, name := range names {
		raw, e := os.ReadFile(filepath.Join(m.root, name.Name()))
		if e != nil || strings.Contains(string(raw), "PRIVATE_TEMPORARY_BODY") {
			t.Fatal("raw material leaked to receipts")
		}
	}
	for _, f := range m.files {
		if !f.removed {
			t.Fatal("unremoved body")
		}
		if _, e = f.file.Stat(); e == nil {
			t.Fatal("unlinked body FD remained open")
		}
	}
}
func TestRemoteMaterialsUnknownAndUnissuedCannotPurge(t *testing.T) {
	m := ownedMaterialFixture(t)
	m.markUnknown()
	if _, e := m.purge(context.Background(), nil, nil); e == nil {
		t.Fatal("unknown/unissued native owner purged")
	}
	for _, rel := range []string{"approved-services.json", "service-journal/baseline.json", "remote-budget/grant-0001.json"} {
		if _, e := os.Stat(filepath.Join(m.root, rel)); e != nil {
			t.Fatal("unknown assets were removed")
		}
	}
	if _, e := OpenRootRemoteMaterials(context.Background(), nil, "", "", "19-1", ""); e == nil {
		t.Fatal("unissued root factory accepted")
	}
	if _, e := json.Marshal(m); e == nil {
		t.Fatal("root owner serialized")
	}
	if _, e := (&RemoteMaterialZero{}).Snapshot(); e == nil {
		t.Fatal("saved zero assertion became original-channel proof")
	}
	if _, e := (&RemoteRuntimeObservation{}).Snapshot(); e == nil {
		t.Fatal("saved runtime assertion became original-channel proof")
	}
	if _, e := (&RemoteController{}).PurgeOwnedMaterials(context.Background(), new(RemoteRuntimeObservation)); e == nil {
		t.Fatal("unissued controller purged")
	}
}

func TestRemoteMaterialZeroCannotValidateUnissuedOrForeignController(t *testing.T) {
	for _, zero := range []*RemoteMaterialZero{nil, {}, {sequence: 1}} {
		for _, c := range []*RemoteController{nil, {}, {closed: true, seq: 1}} {
			if zero.ValidateOriginalController(context.Background(), c) == nil {
				t.Fatal("unissued fields/saved zero acquired original terminal-controller identity")
			}
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if new(RemoteMaterialZero).ValidateOriginalController(ctx, new(RemoteController)) == nil {
		t.Fatal("cancelled observation accepted")
	}
}
func TestRemoteMaterialsWireHasClosedBodyFreeScope(t *testing.T) {
	for _, action := range []string{"controlled_resume", "check_running", "purge_materials"} {
		raw, _ := json.Marshal(SessionRequest{sessionProtocol, 3, action})
		if _, e := parseRemoteSessionRequest(raw, 3); e != nil {
			t.Fatal(e)
		}
		if _, e := parseSessionRequest(raw, 3); e == nil {
			t.Fatal("controlled action escaped remote producer")
		}
	}
	v := SessionDiagnostic{Action: "purge_materials", Outcome: "observed", Materials: &RemoteMaterialSnapshot{ScopeSHA256: strings.Repeat("a", 64), FilesRemoved: 9, DirectoriesRemoved: 4}}
	if !remoteMaterialsDiagnosticValid(v) {
		t.Fatal("native bounded receipt rejected")
	}
	for _, change := range []func(*SessionDiagnostic){func(v *SessionDiagnostic) { v.Materials.RemainingTemporaryFiles = 1 }, func(v *SessionDiagnostic) { v.Materials.ScopeSHA256 = "" }, func(v *SessionDiagnostic) { v.Action = "check" }, func(v *SessionDiagnostic) { v.Outcome = "refused" }, func(v *SessionDiagnostic) { v.Materials.FilesRemoved = 9000 }} {
		q := v
		copy := *v.Materials
		q.Materials = &copy
		change(&q)
		if remoteMaterialsDiagnosticValid(q) {
			t.Fatal("unknown/foreign/nonzero receipt accepted")
		}
	}
}

func TestRemoteMaterialsCancelledBeforeMutationKeepsOriginalBatch(t *testing.T) {
	m := ownedMaterialFixture(t)
	q, c := context.WithCancel(context.Background())
	c()
	if _, e := m.purgeRegistered(q, uint32(os.Geteuid()), strings.Repeat("a", 64), 7); e == nil {
		t.Fatal("cancelled owner deleted materials")
	}
	if m.terminal || m.zero {
		t.Fatal("cancelled preflight began cleanup")
	}
	if e := m.checkLocked(); e != nil {
		t.Fatal("cancelled preflight changed original scope")
	}
}
func TestRemoteMaterialsUnknownDeletePreservesIntentAndNoReplay(t *testing.T) {
	m := ownedMaterialFixture(t)
	path := filepath.Join(m.root, "approved-services.json")
	if e := os.WriteFile(path, []byte("PRIVATE_CONCURRENT_BODY"), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e := m.purgeRegistered(context.Background(), uint32(os.Geteuid()), strings.Repeat("a", 64), 7); e == nil {
		t.Fatal("concurrent field drift overwritten")
	}
	if !m.failed.Load() || m.zero {
		t.Fatal("unknown delete became success")
	}
	original, e := os.ReadFile(filepath.Join(m.root, "remote-owned-purge.intent.json"))
	if e != nil {
		t.Fatal("actual attempted cleanup lost exclusive intent")
	}
	if _, e = m.purgeRegistered(context.Background(), uint32(os.Geteuid()), strings.Repeat("b", 64), 8); e == nil {
		t.Fatal("unknown mutation was replayed")
	}
	after, e := os.ReadFile(filepath.Join(m.root, "remote-owned-purge.intent.json"))
	if e != nil || string(after) != string(original) {
		t.Fatal("old intent changed")
	}
	raw, e := os.ReadFile(path)
	if e != nil || string(raw) != "PRIVATE_CONCURRENT_BODY" {
		t.Fatal("concurrent root body deleted or restored")
	}
}
