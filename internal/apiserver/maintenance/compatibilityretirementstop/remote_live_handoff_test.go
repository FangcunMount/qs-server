package compatibilityretirementstop

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestRemoteTransportBoundCannotAuthorizeAServicePhase(t *testing.T) {
	s := remoteBudgetState{Counter: 1, TotalDeadline: 1800, ForwardDeadline: 1200}
	if limit, ok := remoteBudgetContextLimit(s, remoteSessionContext); !ok || limit != 1800 {
		t.Fatal("control transport lost original total bound")
	}
	if limit, ok := remoteBudgetContextLimit(s, remoteForwardContext); !ok || limit != 1200 {
		t.Fatal("forward action exceeded its bound")
	}
	if _, ok := remoteBudgetContextLimit(s, remoteRecoveryContext); ok {
		t.Fatal("unsigned recovery phase accepted")
	}
	s.RecoverySHA256, s.TotalDeadline, s.ForwardDeadline = strings.Repeat("a", 64), 900, 0
	if _, ok := remoteBudgetContextLimit(s, remoteForwardContext); ok {
		t.Fatal("forward survived recovery")
	}
	for _, kind := range []remoteBudgetContextKind{remoteRecoveryContext, remoteSessionContext} {
		if limit, ok := remoteBudgetContextLimit(s, kind); !ok || limit != 900 {
			t.Fatal("recovery transport/action did not shorten")
		}
	}
	if _, ok := remoteBudgetContextLimit(remoteBudgetState{}, remoteSessionContext); ok {
		t.Fatal("zero native grant acquired transport budget")
	}
	if _, _, e := (&RemoteBudget{}).sessionContext(context.Background()); e == nil {
		t.Fatal("projection manufactured a native budget")
	}
}

func TestRemoteControlStopsOnceAndRetainsOnlyRecoveryAfterRefusal(t *testing.T) {
	if !remoteSessionActionAllowed("bind", false, false, false, false) || !remoteSessionActionAllowed("stop", false, false, false, false) {
		t.Fatal("initial bound or legacy route rejected")
	}
	for _, action := range []string{"bind", "stop", "check", "resume_dependents", "restore", "restore_dependents"} {
		if remoteSessionActionAllowed(action, true, true, true, false) != recoveryAction(action) {
			t.Fatal("refused native action allowed a forward repeat: " + action)
		}
		if remoteSessionActionAllowed(action, true, true, true, true) {
			t.Fatal("recovery could be issued twice")
		}
	}
	if remoteSessionActionAllowed("bind", true, false, false, false) || remoteSessionActionAllowed("stop", true, true, false, false) || remoteSessionActionAllowed("restore", true, false, false, false) {
		t.Fatal("replay or invented stop baseline accepted")
	}
}

func TestManagementBindBelongsOnlyToTheRemoteProducer(t *testing.T) {
	for _, action := range []string{"bind", "stop", "check", "resume_dependents", "restore", "restore_dependents"} {
		for _, seq := range []uint64{1, 2} {
			raw, e := json.Marshal(SessionRequest{Protocol: sessionProtocol, Sequence: seq, Action: action})
			if e != nil {
				t.Fatal(e)
			}
			if _, e = parseRemoteSessionRequest(raw, seq); e != nil {
				t.Fatal("remote producer lost a fixed action")
			}
			_, e = parseSessionRequest(raw, seq)
			if (e != nil) != (action == "bind") {
				t.Fatal("local producer accepted an unimplemented bind or lost a fixed action")
			}
		}
	}
	for _, parser := range []func([]byte, uint64) (SessionRequest, error){parseSessionRequest, parseRemoteSessionRequest} {
		for _, raw := range []string{`{"protocol":"qs-fixed-host-service-session/v1","sequence":2,"action":"unknown"}`, `{"protocol":"qs-fixed-host-service-session/v1","sequence":2,"action":"bind","whole_writer_fence_proven":true}`, `{"protocol":"qs-fixed-host-service-session/v1","sequence":2,"action":"bind","action":"stop"}`} {
			if _, e := parser([]byte(raw), 2); e == nil {
				t.Fatal("unknown/ambiguous action or imported authority accepted")
			}
		}
	}
}

func TestBoundNativeRefusalNeverMeansObservedSuccess(t *testing.T) {
	if !remoteDiagnosticOutcomeValid(SessionDiagnostic{Outcome: "refused", ErrorCategory: "journal_unknown"}) {
		t.Fatal("fixed refusal category lost")
	}
	for _, v := range []SessionDiagnostic{{Outcome: "refused", ErrorCategory: "none"}, {Outcome: "observed", ErrorCategory: "journal_unknown"}, {Outcome: "refused", ErrorCategory: "arbitrary raw error"}, {Outcome: "complete", ErrorCategory: "none"}} {
		if remoteDiagnosticOutcomeValid(v) {
			t.Fatal("ambiguous completion/refusal category accepted")
		}
	}
	if (&RemoteController{}).ValidateRecoveryContinuation(context.Background()) == nil {
		t.Fatal("zero controller became live recovery")
	}
}

func TestDependentAPIFixedMigrationFlagBoundary(t *testing.T) {
	original := descriptorFixture().Containers[0]
	for _, value := range []string{"true", "false"} {
		v := original
		v.Command = append(append([]string(nil), original.Command...), "--migration.enabled="+value)
		if !validDependentScopeAPI(v) {
			t.Fatal("approved explicit migration shape refused")
		}
	}
	for _, extra := range [][]string{{"--migration.enabled=true", "--migration.enabled=false"}, {"--migration.enabled=true", "--migration.enabled=true"}, {"--migration.enabled=unknown"}, {"--migration.enabled", "true"}, {"--unknown"}} {
		v := original
		v.Command = append(append([]string(nil), original.Command...), extra...)
		if validDependentScopeAPI(v) {
			t.Fatal("duplicate/conflicting/unknown API command adopted")
		}
	}
	wrong := original
	wrong.Component = "qs-worker"
	if validDependentScopeAPI(wrong) {
		t.Fatal("different component adopted")
	}
}

func TestManagementCatalogParserCannotAdoptForeignOrChangedRuntime(t *testing.T) {
	expected := descriptorFixture().Containers
	actual := make([]actualContainer, len(expected))
	for i, v := range expected {
		actual[i].Container, actual[i].PID = v, 100+i
	}
	if !remoteManagementCatalogMatches(actual, expected) {
		t.Fatal("expected catalog projection refused")
	}
	for _, change := range []func(*actualContainer){
		func(v *actualContainer) { v.ID = strings.Repeat("9", 64) },
		func(v *actualContainer) { v.Image = "sha256:" + strings.Repeat("9", 64) },
		func(v *actualContainer) { v.Running = false },
		func(v *actualContainer) { v.PID = 0 },
		func(v *actualContainer) { v.StartedAt = "changed" },
		func(v *actualContainer) { v.Dead = true },
	} {
		altered := append([]actualContainer(nil), actual...)
		change(&altered[0])
		if remoteManagementCatalogMatches(altered, expected) {
			t.Fatal("foreign or changed runtime adopted")
		}
	}
	if validateRemoteManagementCatalog(context.Background(), &Approval{}, &RemoteBudget{}) == nil {
		t.Fatal("parser projection became native catalog proof")
	}
}

// This runs a real unprivileged child and anonymous Unix pipes. It exchanges
// control requests only, never a grant, Approval, Window, Lease or service proof.
func TestLiveControlPipeChild(t *testing.T) {
	if os.Getenv("QS_TEST_LIVE_CONTROL_PIPE") != "1" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	for seq, action := range []string{"bind", "restore_dependents"} {
		raw, e := sessionRead(ctx, os.Stdin)
		if e != nil {
			os.Exit(21)
		}
		r, e := parseRemoteSessionRequest(raw, uint64(seq+1))
		if e != nil || r.Action != action {
			os.Exit(22)
		}
		if sessionWriteRaw(ctx, os.Stdout, []byte("transport-control-only")) != nil {
			os.Exit(23)
		}
	}
	os.Exit(0)
}

func TestLiveOriginalControlPipeSurvivesEarlierDeadlineAndFinalChildExit(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestLiveControlPipeChild$")
	cmd.Env = append(os.Environ(), "QS_TEST_LIVE_CONTROL_PIPE=1")
	in, writer, e := os.Pipe()
	if e != nil {
		t.Fatal(e)
	}
	reader, out, e := os.Pipe()
	if e != nil {
		_ = in.Close()
		_ = writer.Close()
		t.Fatal(e)
	}
	defer func() {
		if reader.Close() != nil {
			t.Error("reader cleanup failed")
		}
	}()
	defer func() {
		if writer.Close() != nil {
			t.Error("writer cleanup failed")
		}
	}()
	cmd.Stdin, cmd.Stdout = in, out
	if e = cmd.Start(); e != nil {
		_ = in.Close()
		_ = out.Close()
		t.Fatal(e)
	}
	_ = in.Close()
	_ = out.Close()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	t.Cleanup(func() {
		cancel()
		<-done
	})
	for seq, action := range []string{"bind", "restore_dependents"} {
		if seq != 0 {
			earlier, stop := context.WithTimeout(ctx, time.Millisecond)
			<-earlier.Done()
			stop() // Expiring another phase never closes this original control pipe.
		}
		raw, _ := json.Marshal(SessionRequest{Protocol: sessionProtocol, Sequence: uint64(seq + 1), Action: action})
		if e = sessionWriteRaw(ctx, writer, raw); e != nil {
			t.Fatal(e)
		}
		if seq == 1 {
			// Wait owns no parentIOPipe: consume the final native write after exit.
			err := <-done
			done <- err
			if err != nil {
				t.Fatal(err)
			}
		}
		response, e := sessionRead(ctx, reader)
		if e != nil || string(response) != "transport-control-only" {
			t.Fatal("same original transport lost its control packet")
		}
	}
}

func TestRemoteTerminalZeroCannotAdoptAnotherOrOldRuntime(t *testing.T) {
	for _, z := range []*RemoteMaterialZero{nil, {}} {
		for _, o := range []*RemoteRuntimeObservation{nil, {}} {
			if z.ValidateOriginalRuntime(t.Context(), o) == nil {
				t.Fatal("unissued terminal/runtime pair became original handoff")
			}
		}
	}
}

func TestRecoveryOwnerCannotAcquireForwardOrImportedManagement(t *testing.T) {
	for _, l := range []*Lease{nil, {}, {failed: true, recoveryDependentsOnly: true}} {
		if l != nil {
			l.self = l
		}
		if l.CheckRecoveryStopped(t.Context()) == nil || l.Check(t.Context()) == nil || l.CheckRecoveryStoppedWithInlineAPI(t.Context(), strings.Repeat("1", 64)) == nil {
			t.Fatal("missing original native owner became a service fence")
		}
	}
	for _, c := range []*RemoteController{{recoveryOnly: true}, {recoveryOnly: true, recoveryChecked: true}, {recoveryOnly: true, stopIssued: true}} {
		c.self = c
		for _, action := range []string{"bind", "stop", "controlled_resume", "purge_materials", "restore_dependents", "check_recovery"} {
			if _, e := c.Do(t.Context(), action); e == nil {
				t.Fatal("imported management/native signature accepted", action)
			}
		}
	}
}
func TestRecoveryDependentCatalogPreservesEveryOriginalDependent(t *testing.T) {
	d := descriptorFixture()
	l := &Lease{baseline: d.Containers, recoveryDependentsOnly: true, failed: true}
	l.self = l
	actual := []actualContainer{}
	for _, c := range d.Containers {
		actual = append(actual, actualContainer{Container: c})
	}
	got, want, e := l.recoveryDependentCatalog(actual, nil)
	if e != nil || len(got) != len(d.Containers)-1 || len(want) != len(got) || !l.failed {
		t.Fatal("recovery lost original dependencies", e)
	}
	duplicate := append(append([]actualContainer(nil), actual...), actual[0])
	if _, _, e = l.recoveryDependentCatalog(duplicate, nil); e == nil {
		t.Fatal("duplicate API was hidden")
	}
	if _, _, e = l.recoveryDependentCatalog(actual, ErrState); e == nil {
		t.Fatal("failed catalog became an empty successful scope")
	}
}
