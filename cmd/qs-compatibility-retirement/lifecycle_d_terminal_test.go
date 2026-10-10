package main

import (
	"context"
	"encoding/json"
	"os/exec"
	"syscall"
	"testing"
	"time"

	stop "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirementstop"
)

// These actual local children prove only original parent pipe/Wait/group
// observations. They are not SSH, native D zero, a Window or writer isolation.
func terminalLocalChild(t *testing.T, command string) *lifecycleOwnedServiceSSH {
	t.Helper()
	cmd := exec.Command("/bin/sh", "-c", command)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	child, e := startLifecycleOwnedServiceChild(cmd)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = child.closeForRecovery() })
	return child
}

func terminalLocalChildWait(t *testing.T, child *lifecycleOwnedServiceSSH) {
	t.Helper()
	select {
	case <-child.done:
	case <-time.After(3 * time.Second):
		t.Fatal("owned local child did not terminate")
	}
}

func TestDTerminalRequiresActualOriginalSuccessfulChildCompletion(t *testing.T) {
	t.Run("live-cannot-be-terminal", func(t *testing.T) {
		child := terminalLocalChild(t, "read line")
		if child.requireLive() != nil || lifecycleOriginalDChildTerminal(child, child.cmd.Process.Pid) == nil {
			t.Fatal("live native-management eligibility became terminal completion")
		}
	})
	t.Run("actual-exit-wait-close-and-group", func(t *testing.T) {
		child := terminalLocalChild(t, "exit 0")
		terminalLocalChildWait(t, child)
		pid := child.cmd.Process.Pid
		if lifecycleOriginalDChildTerminal(child, pid) == nil {
			t.Fatal("Wait without original Close/pipe completion became terminal")
		}
		if e := child.Close(); e != nil {
			t.Fatal(e)
		}
		if child.requireLive() == nil || lifecycleOriginalDChildTerminal(child, pid) != nil {
			t.Fatal("successful owned termination requires a live child or loses actual completion")
		}
		if lifecycleOriginalDChildTerminal(child, pid+1) == nil {
			t.Fatal("another PID replaced the original child")
		}
	})
	t.Run("actual-nonzero-wait-is-unknown", func(t *testing.T) {
		child := terminalLocalChild(t, "exit 7")
		terminalLocalChildWait(t, child)
		if child.Close() == nil || lifecycleOriginalDChildTerminal(child, child.cmd.Process.Pid) == nil {
			t.Fatal("failed actual child became a successful terminal phase")
		}
	})
}

func TestDTerminalCannotImportOrUseUnissuedNativeZero(t *testing.T) {
	h := new(lifecycleFixedHost)
	r := lifecycleRequest{}
	for _, v := range []*lifecycleDTerminal{nil, {}, {host: h}} {
		if v.validate(t.Context(), h, r) == nil || h.observeWholeWriterScopesAfterDTerminal(t.Context(), r, v) == nil {
			t.Fatal("unissued original terminal proof relaxed writer isolation")
		}
	}
	var v lifecycleDTerminal
	v.self = &v
	copy := v
	if copy.validate(t.Context(), h, r) == nil {
		t.Fatal("copied terminal fact adopted original process identity")
	}
	if _, e := json.Marshal(&v); e == nil || json.Unmarshal([]byte(`{"terminal":true,"exit_code":0}`), &v) == nil {
		t.Fatal("JSON became a terminal phase")
	}
	for _, native := range []*stop.RemoteMaterialZero{nil, {}} {
		if _, e := h.closePurgedOriginalD(t.Context(), r, native); e == nil {
			t.Fatal("unissued D zero permitted original child closure")
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if v.validate(ctx, h, r) == nil || lifecycleEffectsPreflight(context.Background()) == nil {
		t.Fatal("terminal phase expanded the window or activated incomplete effects")
	}
}

func TestDTerminalActualCallerPreservesPlatformAndOrdinaryLiveChecks(t *testing.T) {
	calls := preBComparisonProductionCalls(t, "lifecycle_controlled_runtime.go", "purgeAcceptedRemoteMaterials")
	purge, closeOriginal, postTerminal := -1, -1, -1
	for i, c := range calls {
		switch c {
		case "PurgeOwnedMaterials":
			purge = i
		case "closePurgedOriginalD":
			closeOriginal = i
		case "observeWholeWriterScopesAfterDTerminal":
			postTerminal = i
		case "CheckWholeWriterFence":
			if purge >= 0 {
				t.Fatal("post-terminal whole fence still requires D to be live")
			}
		}
	}
	if purge < 0 || closeOriginal <= purge || postTerminal <= closeOriginal {
		t.Fatal("terminal phase bypassed original native zero or same child completion")
	}
	calls = preBComparisonProductionCalls(t, "lifecycle_writer_scope.go", "observePlatformQuarantineForOriginalD")
	before, platform, after := -1, -1, -1
	for i, c := range calls {
		if c == "checkOriginalDManagementPhase" {
			if before < 0 {
				before = i
			} else {
				after = i
			}
		}
		if c == "ObservePlatformQuarantine" {
			platform = i
		}
	}
	if before < 0 || platform <= before || after <= platform {
		t.Fatal("original live/terminal phase or actual platform GET recheck was removed")
	}
	h := &lifecycleFixedHost{services: &lifecycleServiceController{child: terminalLocalChild(t, "read line")}}
	if h.checkOriginalDManagementPhase(t.Context(), lifecycleRequest{}, nil) != nil || h.checkOriginalDManagementPhase(t.Context(), lifecycleRequest{}, new(lifecycleDTerminal)) == nil {
		t.Fatal("ordinary intermediate phase stopped requiring the original live child")
	}
}

// Registration follows the actual D purge/terminal phase. The existing real
// child tests above cover live/Wait/closed pipes/group observations; this caller
// check cannot mint native zero, a catalog or a complete external writer fence.
func TestDTerminalJournalRegistrationConsumesOriginalTerminalScope(t *testing.T) {
	terminal, seal := -1, -1
	for i, name := range preBComparisonProductionCalls(t, "lifecycle_material_purge.go", "registerAIStoppedMaterials") {
		switch name {
		case "CheckWholeWriterFence":
			t.Fatal("accepted D-terminal catalog still requires the closed child to be live")
		case "observeWholeWriterScopesAfterDTerminal":
			terminal = i
		case "SealTemporaryJournals":
			seal = i
		}
	}
	if terminal < 0 || seal <= terminal {
		t.Fatal("journal sealing bypassed the original validated D terminal and external scopes")
	}
}
