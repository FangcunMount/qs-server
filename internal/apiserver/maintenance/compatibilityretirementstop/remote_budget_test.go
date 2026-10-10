package compatibilityretirementstop

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"

	fence "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirementfence"
)

func budgetCryptoFixture(t *testing.T) (remoteBudgetTrust, remoteBudgetPayload, ed25519.PrivateKey) {
	t.Helper()
	pub, key, e := ed25519.GenerateKey(rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	binding := fence.WindowBinding{TargetSHA256: fence.MaintenanceWindowTargetSHA256(), SourceSHA: strings.Repeat("a", 40), OperationID: "123-1", ManifestSHA256: strings.Repeat("b", 64), OriginalRunID: "122-1"}
	trust := remoteBudgetTrust{Protocol: budgetProtocol, Binding: binding, ToolSourceSHA: strings.Repeat("c", 40), MachineIDSHA256: strings.Repeat("d", 64), IssuerPublicKey: hex.EncodeToString(pub)}
	p := remoteBudgetPayload{ToolSourceSHA: trust.ToolSourceSHA, Protocol: budgetProtocol, Counter: 1, Nonce: strings.Repeat("e", 64), Action: "stop", RemoteDescriptorSHA256: strings.Repeat("f", 64), Binding: binding, StartSHA256: strings.Repeat("1", 64), RemainingMilliseconds: 1799000, ForwardMilliseconds: 1199000}
	return trust, p, key
}
func encodeBudgetFixture(t *testing.T, p remoteBudgetPayload, key ed25519.PrivateKey) []byte {
	t.Helper()
	raw, e := json.Marshal(p)
	if e != nil {
		t.Fatal(e)
	}
	signed, e := json.Marshal(signedRemoteBudget{p, hex.EncodeToString(ed25519.Sign(key, raw))})
	if e != nil {
		t.Fatal(e)
	}
	return signed
}
func TestBudgetSignatureRejectsChangedBudgetKeyHostBindingAndPhase(t *testing.T) {
	trust, p, key := budgetCryptoFixture(t)
	raw := encodeBudgetFixture(t, p, key)
	if _, e := decodeBudget(raw, trust, p.RemoteDescriptorSHA256); e != nil {
		t.Fatal("real signature refused")
	}
	var corrupted signedRemoteBudget
	if json.Unmarshal(raw, &corrupted) != nil {
		t.Fatal("test setup")
	}
	corrupted.Payload.RemainingMilliseconds++
	changed, _ := json.Marshal(corrupted)
	if _, e := decodeBudget(changed, trust, p.RemoteDescriptorSHA256); e == nil {
		t.Fatal("unsigned budget alteration admitted")
	}
	wrong := trust
	wrong.IssuerPublicKey = strings.Repeat("2", 64)
	if _, e := decodeBudget(raw, wrong, p.RemoteDescriptorSHA256); e == nil {
		t.Fatal("untrusted issuer admitted")
	}
	if _, e := decodeBudget(raw, trust, strings.Repeat("2", 64)); e == nil {
		t.Fatal("another D descriptor admitted")
	}
	wrong = trust
	wrong.Binding.OriginalRunID = "999-1"
	if _, e := decodeBudget(raw, wrong, p.RemoteDescriptorSHA256); e == nil {
		t.Fatal("another original run admitted")
	}
	p.ToolSourceSHA = strings.Repeat("7", 40)
	if _, e := decodeBudget(encodeBudgetFixture(t, p, key), trust, p.RemoteDescriptorSHA256); e == nil {
		t.Fatal("different actual tool source admitted")
	}
	p.ToolSourceSHA = trust.ToolSourceSHA
	p.Action = "restore_dependents"
	if _, e := decodeBudget(encodeBudgetFixture(t, p, key), trust, p.RemoteDescriptorSHA256); e == nil {
		t.Fatal("normal-window signature used for recovery")
	}
	p.RecoverySHA256 = strings.Repeat("2", 64)
	p.ForwardMilliseconds = 0
	p.RemainingMilliseconds = 600000
	if _, e := decodeBudget(encodeBudgetFixture(t, p, key), trust, p.RemoteDescriptorSHA256); e != nil {
		t.Fatal("valid signed recovery refused")
	}
	p.RemainingMilliseconds = 600001
	if _, e := decodeBudget(encodeBudgetFixture(t, p, key), trust, p.RemoteDescriptorSHA256); e == nil {
		t.Fatal("recovery reset beyond ten minutes")
	}
}
func TestRemoteBudgetAnchorsBeforeChallengeAndNeverRenewsOriginalBounds(t *testing.T) {
	_, p, _ := budgetCryptoFixture(t)
	before := int64(10 * time.Second)
	old, e := candidateBudgetState(remoteBudgetState{}, p, "boot-one", before)
	if e != nil {
		t.Fatal(e)
	}
	// Signature is received two seconds later: its deadline remains based on
	// the earlier native challenge sample, never the receipt time.
	received := before + int64(2*time.Second)
	if old.TotalDeadline != before+p.RemainingMilliseconds*int64(time.Millisecond) || old.TotalDeadline-received != (p.RemainingMilliseconds-2000)*int64(time.Millisecond) {
		t.Fatal("round trip was not deducted")
	}
	p.Counter = 2
	renewed, e := candidateBudgetState(old, p, "boot-one", received)
	if e != nil {
		t.Fatal(e)
	}
	if renewed.TotalDeadline != old.TotalDeadline || renewed.ForwardDeadline != old.ForwardDeadline {
		t.Fatal("fresh grant extended original deadline")
	}
	if _, e := candidateBudgetState(old, p, "another-boot", received); e == nil {
		t.Fatal("another boot admitted")
	}
	p.StartSHA256 = strings.Repeat("3", 64)
	if _, e := candidateBudgetState(old, p, "boot-one", received); e == nil {
		t.Fatal("reset A start admitted")
	}
	p.StartSHA256 = old.StartSHA256
	p.Counter = 1
	if _, e := candidateBudgetState(old, p, "boot-one", received); e == nil {
		t.Fatal("replayed counter admitted")
	}
	p.Counter = 2
	p.Action = "restore_dependents"
	p.RecoverySHA256 = strings.Repeat("2", 64)
	p.RemainingMilliseconds = 300000
	p.ForwardMilliseconds = 0
	recovered, e := candidateBudgetState(old, p, "boot-one", received)
	if e != nil || recovered.ForwardDeadline != 0 || recovered.TotalDeadline > old.TotalDeadline {
		t.Fatal("recovery grew the original budget")
	}
	p.Counter = 3
	p.RecoverySHA256 = ""
	p.Action = "check"
	if _, e := candidateBudgetState(recovered, p, "boot-one", received); e == nil {
		t.Fatal("recovery reverted to forward")
	}
	p.Counter = 1
	p.RemainingMilliseconds = 1800000
	if _, e := candidateBudgetState(remoteBudgetState{}, p, "boot-one", math.MaxInt64); e == nil {
		t.Fatal("clock overflow admitted")
	}
}
func TestRemoteBudgetCannotImportOpaqueAuthorityOrKey(t *testing.T) {
	for _, value := range []any{&RootBudgetKey{}, &BudgetIssuer{}, &RemoteBudget{}, &BudgetChallenge{}, &RemoteController{}} {
		if _, e := json.Marshal(value); e == nil {
			t.Fatal("opaque capability serialized")
		}
	}
	if _, e := (&BudgetIssuer{}).IssueFreshBudget(context.Background(), []byte(`{"remaining_milliseconds":1800000}`)); e == nil {
		t.Fatal("zero issuer grants imported budget")
	}
	if _, e := (&RemoteBudget{}).BeginChallenge(context.Background(), "stop"); e == nil {
		t.Fatal("zero budget creates native challenge")
	}
	if e := (&RemoteBudget{}).AcceptGrant(context.Background(), &BudgetChallenge{}, []byte(`{"ready":true}`)); e == nil {
		t.Fatal("imported success admitted")
	}
	if _, _, e := (&RemoteBudget{}).ForwardContext(context.Background()); e == nil {
		t.Fatal("zero budget opens forward")
	}
	if _, _, e := (&RemoteBudget{}).RecoveryContext(context.Background()); e == nil {
		t.Fatal("zero budget opens recovery")
	}
	d := descriptorFixture()
	d.BudgetTrustSHA256 = strings.Repeat("1", 64)
	if validDescriptor(d) {
		t.Fatal("D trust admitted in A descriptor")
	}
}

func TestDisconnectedStopCannotReportStillRunningDependentRestored(t *testing.T) {
	id := strings.Repeat("1", 64)
	intent := []byte(`{"id":"` + id + `","timeout_seconds":510}`)
	v := actualContainer{Container: Container{ID: id, Component: "qs-worker", Running: true}, PID: 25}
	if e := validateStopCompletion(intent, nil, true, v); e == nil {
		t.Fatal("pending daemon stop was declared restored")
	}
	v.Running = false
	if e := validateStopCompletion(intent, nil, true, v); e == nil {
		t.Fatal("nonzero live PID was declared stopped")
	}
	v.PID = 0
	if e := validateStopCompletion(intent, nil, true, v); e != nil {
		t.Fatal("actual stopped PID-zero dependent blocked recovery")
	}
	badResult := []byte(`{"id":"` + id + `","exit_code":137}`)
	if e := validateStopCompletion(intent, badResult, false, v); e == nil {
		t.Fatal("forced exit was relabelled completed graceful stop")
	}
}

func TestSignedRecoveryReadKeepsOriginalBudgetAndRestoreEligibility(t *testing.T) {
	trust, p, key := budgetCryptoFixture(t)
	for _, action := range []string{"check_recovery", "check_running_recovery"} {
		p.Action = action
		if _, e := decodeBudget(encodeBudgetFixture(t, p, key), trust, p.RemoteDescriptorSHA256); e == nil {
			t.Fatal("forward grant admitted recovery read", action)
		}
	}
	p.RecoverySHA256 = strings.Repeat("2", 64)
	p.ForwardMilliseconds = 0
	p.RemainingMilliseconds = 600000
	var state remoteBudgetState
	for i, action := range []string{"check_recovery", "check_running_recovery", "restore_dependents"} {
		p.Action, p.Counter = action, uint64(i+1)
		decoded, e := decodeBudget(encodeBudgetFixture(t, p, key), trust, p.RemoteDescriptorSHA256)
		if e != nil {
			t.Fatal("original signed recovery denied", action, e)
		}
		next, e := candidateBudgetState(state, decoded.Payload, "same-boot", int64(time.Second)+int64(i)*int64(time.Millisecond))
		if e != nil || next.ForwardDeadline != 0 || i > 0 && next.TotalDeadline != state.TotalDeadline {
			t.Fatal("recovery read renewed original deadline or consumed restore", action, e)
		}
		state = next
	}
	p.Counter++
	p.RecoverySHA256, p.Action = "", "check"
	if _, e := candidateBudgetState(state, p, "same-boot", int64(time.Second)); e == nil {
		t.Fatal("recovery read returned to forward epoch")
	}
}
