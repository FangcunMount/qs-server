package main

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"

	backup "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirementbackup"
	fence "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirementfence"
)

func TestServicePortsRefuseMissingNativeAuthority(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, q := range []context.Context{nil, ctx} {
		if _, err := loadLifecyclePreparationBudgetRole(q, "/missing", "", "1-1", "1-1"); err == nil {
			t.Fatal("preparation budget role accepted absent native authority")
		}
		if err := runLifecycleServiceSession(q, "host-services-d", "/missing", "", "1-1", "1-1", nil, nil); err == nil {
			t.Fatal("service session accepted absent authority")
		}
		for _, mode := range []string{"host-budget-key-create", "host-budget-key-open"} {
			r, err := runLifecycleServiceKey(q, mode, "/missing", "", "1-1", "1-1")
			if err == nil || r.KeyAvailable || r.WholeWriterFenceProven || r.DropReady || r.PublicKey != "" {
				t.Fatal("budget key accepted absent native authority")
			}
		}
	}
	r := lifecycleRequest{ServiceControl: &lifecycleServiceControl{LocalDescriptorSHA256: strings.Repeat("a", 64), SSHChannelSHA256: strings.Repeat("b", 64)}}
	if v, err := openLifecycleServiceController(context.Background(), r, new(fence.MaintenanceWindow), false); err == nil || v != nil {
		t.Fatal("imported/zero Window accepted")
	}
	v := new(lifecycleServiceController)
	for _, call := range []func(context.Context) error{v.StopAndDrain, v.Check, v.RestoreDependents} {
		if call(context.Background()) == nil {
			t.Fatal("zero service controller produced success")
		}
	}
}

func TestServiceSessionInputsRejectImportedCapabilitiesEvenNull(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "service-session.json")
	for _, name := range []string{"drop_ready", "whole_writer_fence_proven", "window", "lease", "permit", "public_key", "private_seed", "Tool_Source_SHA"} {
		for _, value := range []string{"null", "true", "{}"} {
			raw := []byte(`{"` + name + `":` + value + `}`)
			if err := os.WriteFile(path, raw, 0600); err != nil {
				t.Fatal(err)
			}
			var r lifecycleServiceSessionRequest
			if readLifecyclePrivate(path, digestRaw(raw), &r) == nil {
				t.Fatal("imported capability or alias accepted")
			}
		}
	}
}

func TestServiceLeaseCannotActivateRemainingProductionAdapters(t *testing.T) {
	h := &lifecycleFixedHost{services: new(lifecycleServiceController)}
	ctx := context.Background()
	r := lifecycleRequest{}
	a := new(backup.Archive)
	for _, call := range []func() error{
		func() error { return lifecycleEffectsPreflight(ctx) },
		func() error { return h.CheckWholeWriterFence(ctx, r) },
		func() error { return h.FinalDifferenceAndEOF(ctx, r, a) },
		func() error { return h.DeployBInline(ctx, r, nil, nil) },
		func() error { return h.VerifyAcceptance(ctx, r, a) },
		func() error { return h.CheckActualDDLStopped(ctx, r) },
		func() error { return h.DeployRollbackInline(ctx, r, nil, nil) },
		func() error { return h.PurgeTemporaryCopies(ctx, r) },
		func() error { return h.VerifyTemporaryMaterialsZero(ctx, r) },
		func() error { return h.ResumeAcceptedEntrypoints(ctx, r) },
	} {
		if call() == nil {
			t.Fatal("service-only capability activated missing production adapter")
		}
	}
}

func TestPinnedServiceSSHRejectsUnboundServerKey(t *testing.T) {
	blob := []byte("only offline parser input, never authentication proof")
	h := sha256.Sum256(blob)
	channel := lifecycleServiceSSHChannel{Host: "server-d.example", Port: 22, HostKeyFingerprint: "SHA256:" + base64.RawStdEncoding.EncodeToString(h[:])}
	line := []byte(channel.Host + " ssh-ed25519 " + base64.StdEncoding.EncodeToString(blob) + "\n")
	if !lifecycleSSHKnownHostMatches(line, channel) {
		t.Fatal("expected exact pin parser")
	}
	for _, raw := range [][]byte{append(append([]byte(nil), line...), line...), []byte("*.example ssh-ed25519 " + base64.StdEncoding.EncodeToString(blob)), []byte("@cert-authority " + string(line)), []byte("server-a.example ssh-ed25519 " + base64.StdEncoding.EncodeToString(blob)), []byte(channel.Host + " ssh-ed25519 invalid")} {
		if lifecycleSSHKnownHostMatches(raw, channel) {
			t.Fatal("unbound or ambiguous host pin accepted")
		}
	}
	channel.Port = 2200
	if lifecycleSSHKnownHostMatches(line, channel) {
		t.Fatal("other port reused host pin")
	}
}

func TestServiceSSHCommandIsFixedAndDoesNotDispatchProductionWorkflow(t *testing.T) {
	r := lifecycleRequest{OperationID: "17-1", ActualRunID: "19-2"}
	c := lifecycleServiceSSHChannel{Host: "server-d.example", Port: 22, User: "qs", RemoteRequestSHA256: strings.Repeat("a", 64)}
	args := lifecycleServiceSSHArgs(r, c, false)
	want := []string{"-F", "/dev/null", "-T"}
	if !reflect.DeepEqual(args[:3], want) {
		t.Fatal("ambient SSH config/PTY enabled")
	}
	joined := strings.Join(args, " ")
	for _, required := range []string{"StrictHostKeyChecking=yes", "ControlMaster=no", "ClearAllForwardings=yes", "PermitLocalCommand=no", "/usr/bin/sudo -n -- /opt/qs-server/qs-worker/compatibility-retirement/17-1/qs-compatibility-retirement", "--mode host-services-d", "--run-id 19-2"} {
		if !strings.Contains(joined, required) {
			t.Fatal("required fixed channel setting missing")
		}
	}
	for _, denied := range []string{"accept-new", "workflow", "dispatch", "SUDO_PASSWORD", "SSH_AUTH_SOCK", "migration.enabled=true", "sh -c"} {
		if strings.Contains(joined, denied) {
			t.Fatal("unexpected channel authority")
		}
	}
	if !strings.Contains(strings.Join(lifecycleServiceSSHArgs(r, c, true), " "), "--mode host-services-d-recovery") {
		t.Fatal("recovery mode not separate")
	}
}

// This helper verifies only the local owned-child lifecycle. It never opens
// SSH, a root management route, Docker, databases or an opaque retirement proof.
func TestServiceSSHOwnedChildHelper(t *testing.T) {
	if os.Getenv("QS_TEST_OWNED_SERVICE_CHILD") != "1" {
		return
	}
	time.Sleep(time.Minute)
}
func TestOwnedServiceSSHCloseReapsGroupAndIsIdempotent(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=^TestServiceSSHOwnedChildHelper$")
	cmd.Env = append(os.Environ(), "QS_TEST_OWNED_SERVICE_CHILD=1")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	in, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	out, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	v := &lifecycleOwnedServiceSSH{cmd: cmd, in: in.(*os.File), out: out.(*os.File), done: make(chan struct{})}
	go func() { v.waitErr = cmd.Wait(); close(v.done) }()
	first := v.Close()
	if first == nil {
		t.Fatal("terminated child was incorrectly called normal exit")
	}
	if second := v.Close(); second != first {
		t.Fatal("second close re-signalled a reaped group")
	}
	select {
	case <-v.done:
	default:
		t.Fatal("owned child not reaped")
	}
	if err = syscall.Kill(-cmd.Process.Pid, 0); err != syscall.ESRCH {
		t.Fatal("owned process group remains")
	}
	if err = v.closeForRecovery(); err != nil {
		t.Fatal("actual reaped local child could not be separately reconciled")
	}
}
