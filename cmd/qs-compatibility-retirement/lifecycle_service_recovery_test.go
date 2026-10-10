package main

import (
	"context"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"

	backup "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirementbackup"
	fence "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirementfence"
	stop "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirementstop"
)

func recoveryServiceTestIdentity() (lifecycleRequest, lifecycleServiceControllerIdentity) {
	r := lifecycleRequest{ToolSourceSHA: sourceSHA, OriginalSourceSHA: strings.Repeat("a", 40),
		OperationID: "17-1", ActualRunID: "18-1", ManifestSHA256: strings.Repeat("b", 64),
		Recovery:       backup.TargetRecoveryRequest{OriginalRunID: "16-1"},
		ServiceControl: &lifecycleServiceControl{LocalDescriptorSHA256: strings.Repeat("c", 64), SSHChannelSHA256: strings.Repeat("d", 64)}}
	i := lifecycleServiceControllerIdentity{windowBinding: lifecycleWindowBinding(r), toolSourceSHA: r.ToolSourceSHA,
		actualRunID: r.ActualRunID, localDescriptorSHA256: r.ServiceControl.LocalDescriptorSHA256,
		sshChannelSHA256: r.ServiceControl.SSHChannelSHA256, remoteDescriptorSHA256: strings.Repeat("e", 64)}
	return r, i
}

func TestRecoveryContinuationExpectedBindingCannotChangeNativeOwner(t *testing.T) {
	r, i := recoveryServiceTestIdentity()
	if !i.matches(r) {
		t.Fatal("expected identity did not match itself")
	}
	for name, change := range map[string]func(*lifecycleRequest){
		"tool":             func(r *lifecycleRequest) { r.ToolSourceSHA = strings.Repeat("f", 40) },
		"original_source":  func(r *lifecycleRequest) { r.OriginalSourceSHA = strings.Repeat("f", 40) },
		"operation":        func(r *lifecycleRequest) { r.OperationID = "19-1" },
		"original_run":     func(r *lifecycleRequest) { r.Recovery.OriginalRunID = "19-1" },
		"actual_run":       func(r *lifecycleRequest) { r.ActualRunID = "19-1" },
		"manifest":         func(r *lifecycleRequest) { r.ManifestSHA256 = strings.Repeat("f", 64) },
		"local_descriptor": func(r *lifecycleRequest) { r.ServiceControl.LocalDescriptorSHA256 = strings.Repeat("f", 64) },
		"channel":          func(r *lifecycleRequest) { r.ServiceControl.SSHChannelSHA256 = strings.Repeat("f", 64) },
		"absent_control":   func(r *lifecycleRequest) { r.ServiceControl = nil },
	} {
		t.Run(name, func(t *testing.T) {
			q := r
			c := *r.ServiceControl
			q.ServiceControl = &c
			change(&q)
			if i.matches(q) {
				t.Fatal("changed original continuation binding accepted")
			}
		})
	}
}

// Zero opaque pointers below are sentinels for owner preservation only. They
// never satisfy a native constructor, and no test asserts service/DROP authority.
func TestRecoveryContinuationRefusesZeroWindowAndKeepsOriginalOwner(t *testing.T) {
	r, i := recoveryServiceTestIdentity()
	w := new(fence.MaintenanceWindow)
	local := new(stop.Lease)
	issuer := new(stop.BudgetIssuer)
	v := &lifecycleServiceController{window: w, approval: new(stop.Approval), local: local, issuer: issuer, identity: i, stopAttempted: true}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	for _, ctx := range []context.Context{nil, cancelled, context.Background()} {
		if err := v.UseOriginalRecoverySession(ctx, r, w); err == nil {
			t.Fatal("zero opaque authority produced a recovery continuation")
		}
		if v.local != local || v.issuer != issuer || v.window != w || v.recoveryAttempted {
			t.Fatal("rejected recovery discarded original ownership")
		}
	}
	if v.UseOriginalRecoverySession(context.Background(), r, new(fence.MaintenanceWindow)) == nil {
		t.Fatal("copied Window accepted")
	}
	h := &lifecycleFixedHost{services: v}
	if h.RestoreRollbackEntrypoints(context.Background(), r, w) == nil || h.services != v || v.local != local || v.issuer != issuer {
		t.Fatal("rejected host recovery recreated or discarded native owner")
	}
	if lifecycleEffectsPreflight(context.Background()) == nil {
		t.Fatal("service recovery activated missing production adapters")
	}
}

func TestForwardTransportReapPreservesOriginalLeaseAndIssuer(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=^TestServiceSSHOwnedChildHelper$")
	cmd.Env = append(os.Environ(), "QS_TEST_OWNED_SERVICE_CHILD=1")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	child, err := startLifecycleOwnedServiceChild(cmd)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = child.closeForRecovery() })
	local, issuer, w := new(stop.Lease), new(stop.BudgetIssuer), new(fence.MaintenanceWindow)
	v := &lifecycleServiceController{child: child, local: local, issuer: issuer, window: w, stopAttempted: true}
	if err = v.closeForwardTransport(); err != nil {
		t.Fatal(err)
	}
	if !child.reaped || syscall.Kill(-cmd.Process.Pid, 0) != syscall.ESRCH || v.child != nil || v.remote != nil {
		t.Fatal("old physical local transport not reaped/detached")
	}
	if v.local != local || v.issuer != issuer || v.window != w || !v.stopAttempted {
		t.Fatal("transport cleanup destroyed original same-process authority")
	}
	if err = v.closeForwardTransport(); err != nil {
		t.Fatal("second transport cleanup was not idempotent")
	}
}

// This is a local child/pipe regression only, not a remote action receipt.
func TestServiceSSHOwnedFinalReplyHelper(t *testing.T) {
	if os.Getenv("QS_TEST_OWNED_SERVICE_FINAL_REPLY") != "1" {
		return
	}
	if _, err := os.Stdout.WriteString("terminal-local-reply\n"); err != nil {
		os.Exit(9)
	}
	os.Exit(0)
}

func TestOwnedServiceSSHWaitDoesNotCloseUnreadTerminalReply(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=^TestServiceSSHOwnedFinalReplyHelper$")
	cmd.Env = append(os.Environ(), "QS_TEST_OWNED_SERVICE_FINAL_REPLY=1")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	child, err := startLifecycleOwnedServiceChild(cmd)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = child.closeForRecovery() })
	// Force the race's adverse ordering: Wait completes before any reply read.
	select {
	case <-child.done:
	case <-time.After(10 * time.Second):
		t.Fatal("terminal local child did not exit")
	}
	if child.waitErr != nil {
		t.Fatal("terminal local child failed")
	}
	raw, err := io.ReadAll(io.LimitReader(child.in, 128))
	if err != nil || string(raw) != "terminal-local-reply\n" {
		t.Fatal("Wait stole the caller-owned unread terminal reply")
	}
	if err = child.Close(); err != nil || !child.reaped || syscall.Kill(-cmd.Process.Pid, 0) != syscall.ESRCH {
		t.Fatal("terminal local child or owned pipes not closed/reaped")
	}
}
