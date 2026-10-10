package retirement

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"

	fence "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirementfence"
)

// Private fake protocols prove ordering and fail-closed behavior only. They do
// not create native Window/Docker evidence, production approval or a full fence.
type aiStoppedUnitWindow struct {
	binding  fence.WindowBinding
	start    string
	recovery bool
	deadline time.Time
}

func (w *aiStoppedUnitWindow) Diagnostic(ctx context.Context) (fence.WindowBudgetReceipt, error) {
	if ctx.Err() != nil || time.Now().After(w.deadline) {
		return fence.WindowBudgetReceipt{}, ErrAIStoppedRuntime
	}
	d := fence.WindowBudgetReceipt{Binding: w.binding, StartSHA256: w.start, DirectoryLeaseHeld: true, RemainingMilliseconds: 1000}
	if w.recovery {
		d.RecoverySHA256 = strings.Repeat("e", 64)
	}
	return d, nil
}
func (w *aiStoppedUnitWindow) ForwardContext(ctx context.Context) (context.Context, context.CancelFunc, error) {
	if w.recovery {
		return nil, nil, ErrAIStoppedRuntime
	}
	q, c := context.WithDeadline(ctx, w.deadline)
	return q, c, nil
}
func (w *aiStoppedUnitWindow) RecoveryContext(ctx context.Context) (context.Context, context.CancelFunc, error) {
	w.recovery = true
	q, c := context.WithDeadline(ctx, w.deadline)
	return q, c, nil
}

type aiStoppedUnitProtocol struct {
	t                          *testing.T
	lease                      *AIStoppedRuntimeLease
	snap                       aiStoppedSnapshot
	stops, starts, removes     int
	stopErr, startErr, idleErr error
	stayRunning                bool
	nativeCID                  string
}

func (p *aiStoppedUnitProtocol) snapshot(context.Context, string) (aiStoppedSnapshot, error) {
	return p.snap, nil
}
func (*aiStoppedUnitProtocol) network(context.Context, string) error { return nil }
func (p *aiStoppedUnitProtocol) stop(_ context.Context, cid string) error {
	p.stops++
	if cid != p.lease.input.ContainerID || !p.lease.stopAttempted || !p.lease.signalAttempted || !bytes.Contains(p.lease.journalRaw, []byte(`"Stage":"stop_intent"`)) {
		p.t.Fatal("signal preceded retained owner/durable intent")
	}
	if !p.stayRunning {
		p.snap.Runtime.Running = false
		p.snap.Runtime.Status = "exited"
		p.snap.PID = 0
	}
	return p.stopErr
}
func (p *aiStoppedUnitProtocol) start(_ context.Context, cid string) error {
	p.starts++
	if cid != p.lease.input.ContainerID || !p.lease.restoreAttempted || !bytes.Contains(p.lease.journalRaw, []byte(`"Stage":"restore_intent"`)) {
		p.t.Fatal("restoration changed CID or preceded durable intent")
	}
	p.snap.Runtime.Running = true
	p.snap.Runtime.Status = "running"
	p.snap.Runtime.StartedAt = time.Now().Add(-time.Second).UTC().Format(time.RFC3339Nano)
	p.snap.HealthStart = time.Now().Add(-time.Millisecond)
	p.snap.HealthEnd = time.Now()
	p.snap.PID = 122
	return p.startErr
}
func (*aiStoppedUnitProtocol) createCarrier(context.Context, aiStoppedSnapshot, string) (string, error) {
	return "", ErrAIStoppedRuntime
}
func (*aiStoppedUnitProtocol) startCarrier(context.Context, string) error { return ErrAIStoppedRuntime }
func (*aiStoppedUnitProtocol) checkCarrier(context.Context, string, aiStoppedSnapshot, string) error {
	return ErrAIStoppedRuntime
}
func (p *aiStoppedUnitProtocol) inspectIdleCarrier(_ context.Context, id string, _ aiStoppedSnapshot, op string) (string, bool, error) {
	if id != "qs-retirement-ai-final-"+op && id != p.nativeCID {
		return "", false, ErrAIStoppedRuntime
	}
	return p.nativeCID, true, p.idleErr
}
func (p *aiStoppedUnitProtocol) removeCarrier(_ context.Context, id string) error {
	if id != p.nativeCID || !p.lease.carrierAttempted || !bytes.Contains(p.lease.journalRaw, []byte(`"Stage":"carrier_remove_intent"`)) {
		p.t.Fatal("remove preceded native ownership/durable intent")
	}
	p.removes++
	return nil
}
func (p *aiStoppedUnitProtocol) requireCarrierAbsent(context.Context, string) error {
	if p.removes != 1 {
		return ErrAIStoppedRuntime
	}
	return nil
}
func (p *aiStoppedUnitProtocol) requireOwnerCarriersAbsent(context.Context, string) error {
	return p.requireCarrierAbsent(nil, "")
}
func aiStoppedUnitLease(t *testing.T) (*AIStoppedRuntimeLease, *aiStoppedUnitProtocol) {
	t.Helper()
	dir, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	if os.Chmod(dir, 0700) != nil {
		t.Fatal("private directory")
	}
	path := filepath.Join(dir, "original-stop.jsonl")
	f, e := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if e != nil {
		t.Fatal(e)
	}
	if syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) != nil {
		t.Fatal("journal lock")
	}
	st, e := aiExecFileCheck(f, path, nil)
	if e != nil {
		t.Fatal(e)
	}
	w := &aiStoppedUnitWindow{binding: fence.WindowBinding{SourceSHA: strings.Repeat("a", 40), OperationID: "900-1"}, start: strings.Repeat("b", 64), deadline: time.Now().Add(time.Minute)}
	s := aiStoppedSnapshot{Runtime: aiExternalRuntime{ContainerID: strings.Repeat("c", 64), ImageID: "sha256:" + strings.Repeat("d", 64), Status: "running", Running: true, StartedAt: time.Now().Add(-time.Hour).UTC().Format(time.RFC3339Nano)}, ConfigSHA256: strings.Repeat("1", 64), HostConfigSHA256: strings.Repeat("2", 64), NetworkID: strings.Repeat("3", 64), RestartPolicy: "unless-stopped", Settings: map[string]string{"QS_AI_DATABASE_URL": "synthetic-secret"}, PID: 121, HealthcheckTest: []string{"CMD", "/app/.venv/bin/python", "-c", "import urllib.request; urllib.request.urlopen('http://127.0.0.1:8000/readyz', timeout=3)"}, HealthStatus: "healthy", HealthExitCode: aiExecQuiescenceCode(0), HealthStart: time.Now().Add(-time.Second), HealthEnd: time.Now()}
	l := &AIStoppedRuntimeLease{window: w, binding: w.binding, startSHA: w.start, input: AIExternalExecutionInput{ContainerID: s.Runtime.ContainerID, ApprovedAIRuntimeBindingSHA256: strings.Repeat("4", 64)}, baseline: s, journal: f, journalPath: path, journalStat: st, readRelease: func(string, string) (aiExternalRelease, error) {
		return aiExternalRelease{seal: strings.Repeat("4", 64)}, nil
	}}
	l.self = l
	p := &aiStoppedUnitProtocol{t: t, lease: l, snap: s, nativeCID: strings.Repeat("5", 64)}
	l.protocol = p
	if l.append("prepared", "") != nil {
		t.Fatal("journal preparation")
	}
	t.Cleanup(func() { _ = l.Close() })
	return l, p
}
func TestAIStoppedRuntimeSignalAndExactRecoveryAreSingleAttempt(t *testing.T) {
	l, p := aiStoppedUnitLease(t)
	if e := l.Stop(t.Context()); e != nil {
		t.Fatal(e)
	}
	if !l.stopped || p.stops != 1 || bytes.Contains(l.journalRaw, []byte("synthetic-secret")) {
		t.Fatal("stop proof or privacy")
	}
	if l.Stop(t.Context()) == nil || p.stops != 1 {
		t.Fatal("stop was retried")
	}
	if e := l.Restore(t.Context()); e != nil {
		t.Fatal(e)
	}
	if p.starts != 1 || !l.restored || l.Restore(t.Context()) != nil || p.starts != 1 {
		t.Fatal("restoration was repeated")
	}
}
func TestAIStoppedRuntimeUnknownStopNeverBecomesRunningSuccess(t *testing.T) {
	l, p := aiStoppedUnitLease(t)
	p.stopErr = io.ErrUnexpectedEOF
	p.stayRunning = true
	if l.Stop(t.Context()) == nil || !l.unknown || !l.stopAttempted || !l.signalAttempted {
		t.Fatal("unknown signal responsibility lost")
	}
	if l.Stop(t.Context()) == nil || l.Restore(t.Context()) == nil || p.stops != 1 || p.starts != 0 || l.restored {
		t.Fatal("unknown stop was retried or running sample erased it")
	}
}
func TestAIStoppedRuntimeUnknownResponseObservedStoppedCanRecoverExactCID(t *testing.T) {
	l, p := aiStoppedUnitLease(t)
	p.stopErr = io.ErrUnexpectedEOF
	if l.Stop(t.Context()) == nil || l.CheckStopped(t.Context()) == nil {
		t.Fatal("unknown result acquired forward proof")
	}
	if e := l.Restore(t.Context()); e != nil || p.starts != 1 || p.stops != 1 {
		t.Fatal("known stopped original could not recover without repeating signal")
	}
}
func TestAIStoppedRuntimeUnknownStartNeverRetriesAndNonzeroStopIsRecoveryOnly(t *testing.T) {
	l, p := aiStoppedUnitLease(t)
	p.snap.ExitCode = 137
	if l.Stop(t.Context()) == nil || l.CheckStopped(t.Context()) == nil {
		t.Fatal("nonzero stop became graceful forward evidence")
	}
	p.startErr = io.ErrUnexpectedEOF
	if l.Restore(t.Context()) == nil || !l.restoreAttempted || l.restored || p.starts != 1 {
		t.Fatal("unknown restoration result lost")
	}
	if l.Restore(t.Context()) == nil || p.starts != 1 {
		t.Fatal("unknown original start was repeated")
	}
}
func TestAIStoppedRuntimeChangedConfigAndTornIntentPreventSignal(t *testing.T) {
	for _, change := range []string{"config", "journal"} {
		t.Run(change, func(t *testing.T) {
			l, p := aiStoppedUnitLease(t)
			if change == "config" {
				p.snap.ConfigSHA256 = strings.Repeat("6", 64)
			} else {
				if _, e := l.journal.WriteAt([]byte("x"), 0); e != nil {
					t.Fatal(e)
				}
			}
			if l.Stop(t.Context()) == nil || p.stops != 0 {
				t.Fatal("changed native baseline or journal signaled")
			}
		})
	}
}
func TestAIStoppedRuntimeIdleCarrierUnknownCreateCleanupDoesNotCreateOrStart(t *testing.T) {
	l, p := aiStoppedUnitLease(t)
	if e := l.Stop(t.Context()); e != nil {
		t.Fatal(e)
	}
	l.carrierAttempted = true
	l.carrierUnknown = true
	if l.append("carrier_create_unknown", "") != nil {
		t.Fatal("intent")
	}
	if e := l.CleanupCarrierForRecovery(t.Context()); e != nil || !l.carrierZero || p.removes != 1 || l.carrierID != p.nativeCID {
		t.Fatal("exact owner was not registered/removed")
	}
	if e := l.Restore(t.Context()); e != nil || p.starts != 1 {
		t.Fatal("original restore remained blocked after actual zero")
	}
}
func TestAIStoppedRuntimeUnprovenCarrierBlocksCleanupAndOriginalRestart(t *testing.T) {
	l, p := aiStoppedUnitLease(t)
	if e := l.Stop(t.Context()); e != nil {
		t.Fatal(e)
	}
	l.carrierAttempted = true
	l.carrierUnknown = true
	p.idleErr = errors.New("permission_or_owner_or_unknown")
	if l.CleanupCarrierForRecovery(t.Context()) == nil || l.Restore(t.Context()) == nil || p.removes != 0 || p.starts != 0 || l.carrierZero {
		t.Fatal("unknown native owner became cleanup/restart authority")
	}
}
func TestAIStoppedSettingsActualNestedContractAndExcludedProviderKeys(t *testing.T) {
	in := AIExternalExecutionInput{RuntimeSourceSHA: strings.Repeat("a", 40)}
	env := []string{"PATH=/usr/bin", "QS_AI_ENVIRONMENT=production", "QS_AI_RELEASE_SHA=" + in.RuntimeSourceSHA, "QS_AI_DATABASE_URL=mysql+asyncmy://synthetic:private@mysql/ai", "QS_AI_GENERATION__ENABLED=true", "QS_AI_EVALUATION__PARALLEL_CALLS=2", "QS_AI_MESSAGING={}", "QS_AI_DEEPSEEK_API_KEY=excluded-secret"}
	s, e := aiStoppedSettings(env, in)
	if e != nil || len(s) != 6 || strings.Contains(string(aiJSONBytes(s)), "excluded-secret") {
		t.Fatal("actual nested settings unsupported or unnecessary credentials copied")
	}
	for _, bad := range []string{"QS_AI_UNKNOWN=x", "QS_AI_DATABASE_URL=duplicate", "QS_AI_MESSAGING={}\n"} {
		if _, e := aiStoppedSettings(append(append([]string(nil), env...), bad), in); e == nil {
			t.Fatal("unknown/duplicate/multiline configuration accepted")
		}
	}
}
func TestAIStoppedFinalBindingPreservesOldCanonicalJSONAndExecutionIdentity(t *testing.T) {
	b := aiExecUnitBinding([]byte(`{"protocol":"synthetic"}`))
	raw, _ := json.Marshal(b)
	if bytes.Contains(raw, []byte("execution_container_id")) || bytes.Contains(raw, []byte("stopped_lease_sha256")) || !b.valid() {
		t.Fatal("old running journal changed")
	}
	b.ExecutionContainerID = strings.Repeat("6", 64)
	b.StoppedLeaseSHA256 = strings.Repeat("7", 64)
	b.PythonSHA256 = sourceSHA([]byte(aiStoppedCarrierHost))
	if !b.valid() || b.executionCID() == b.ContainerID {
		t.Fatal("carrier business/execution identity lost")
	}
	for _, mutate := range []func(*aiExecBinding){func(b *aiExecBinding) { b.ExecutionContainerID = b.ContainerID }, func(b *aiExecBinding) { b.PythonSHA256 = aiExternalHostSHA }, func(b *aiExecBinding) { b.StoppedLeaseSHA256 = "" }} {
		v := b
		mutate(&v)
		if v.valid() {
			t.Fatal("mixed old/new producer binding accepted")
		}
	}
}

type aiStoppedUnitExecProtocol struct {
	*aiExecUnitProtocol
	execution string
}

func (p *aiStoppedUnitExecProtocol) create(ctx context.Context, cid string, host []byte) (string, error) {
	if cid != p.execution || sourceSHA(host) != sourceSHA([]byte(aiStoppedCarrierHost)) {
		p.t.Fatal("native create used original business CID or arbitrary wrapper")
	}
	return p.aiExecUnitProtocol.create(ctx, cid, host)
}
func (p *aiStoppedUnitExecProtocol) inspect(ctx context.Context, id, cid, hostSHA string) (aiExecInspect, error) {
	if cid != p.execution || hostSHA != sourceSHA([]byte(aiStoppedCarrierHost)) {
		p.t.Fatal("native inspect lost carrier binding")
	}
	return p.aiExecUnitProtocol.inspect(ctx, id, cid, hostSHA)
}
func TestAIStoppedFinalExecUsesOnlyExecutionCIDAndRetainsBusinessBinding(t *testing.T) {
	input := []byte(`{"protocol":"synthetic-test-only"}`)
	b := aiExecUnitBinding(input)
	b.ExecutionContainerID = strings.Repeat("6", 64)
	b.StoppedLeaseSHA256 = strings.Repeat("7", 64)
	b.PythonSHA256 = sourceSHA([]byte(aiStoppedCarrierHost))
	j := aiExecUnitJournal(t, b)
	p := &aiStoppedUnitExecProtocol{aiExecUnitProtocol: aiExecUnitProtocolFor(t, j), execution: b.ExecutionContainerID}
	ctx, cancel := context.WithDeadline(t.Context(), time.Unix(0, b.DeadlineUnixNano))
	defer cancel()
	v, e := aiExecProduce(ctx, p, j, []byte(aiStoppedCarrierHost), input)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = v.actualOutput(ctx); e != nil {
		t.Fatal(e)
	}
	if j.binding.ContainerID != b.ContainerID || !bytes.Contains(j.raw, []byte(b.ExecutionContainerID)) || bytes.Contains(j.raw, input) {
		t.Fatal("original identity or anonymous input privacy lost")
	}
}
func TestAIStoppedRuntimeLeaseFormattingAndImportCannotExposeOrAdoptSecrets(t *testing.T) {
	l, _ := aiStoppedUnitLease(t)
	for _, format := range []string{"%v", "%+v", "%#v"} {
		if strings.Contains(fmt.Sprintf(format, l), "synthetic-secret") {
			t.Fatal("private settings exposed")
		}
	}
	if _, e := json.Marshal(l); e == nil || l.UnmarshalJSON([]byte(`{}`)) == nil {
		t.Fatal("lease serialized/adopted")
	}
	copy := reflect.New(reflect.TypeOf(l).Elem()).Elem()
	copy.Set(reflect.ValueOf(l).Elem())
	cloned := copy.Addr().Interface().(*AIStoppedRuntimeLease)
	if cloned.Stop(t.Context()) == nil || cloned.Close() == nil || l.closed {
		t.Fatal("value copy acquired or closed original owner")
	}
}
func TestAIStoppedFinalPythonWrapperRejectsUntrustedPacketWithoutOutput(t *testing.T) {
	python, e := exec.LookPath("python3")
	if e != nil {
		t.Skip("local Python unavailable; no native producer proof")
	}
	for _, raw := range []string{`{}`, `{"protocol":"qs-ai-stopped-final-pipe/v1","settings":"e30=","settings_sha256":"invalid","host":"","packet":""}`} {
		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		cmd := exec.CommandContext(ctx, python, "-I", "-B", "-c", aiStoppedCarrierHost)
		cmd.Stdin = strings.NewReader(raw)
		cmd.Env = []string{"PATH=/usr/bin:/bin"}
		output, e := cmd.CombinedOutput()
		cancel()
		var exit *exec.ExitError
		if !errors.As(e, &exit) || exit.ExitCode() != 71 || len(output) != 0 {
			t.Fatal("fixed anonymous wrapper accepted untrusted input or emitted private contents")
		}
	}
}

// Known native start is not readiness, and observing it again never sends a
// second start. These are private protocol tests, not real Engine acceptance.
func TestAIStoppedReadinessRejectsStaleHealthAndChangedOriginal(t *testing.T) {
	l, p := aiStoppedUnitLease(t)
	if e := l.Stop(t.Context()); e != nil {
		t.Fatal(e)
	}
	if e := l.Resume(t.Context()); e != nil {
		t.Fatal(e)
	}
	p.snap.HealthEnd, p.snap.HealthStart = time.Now().Add(-time.Hour), time.Now().Add(-time.Hour)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
	defer cancel()
	if l.Resume(ctx) == nil || p.starts != 1 {
		t.Fatal("stale healthy sample accepted or start repeated")
	}
	p.snap.HealthStart, p.snap.HealthEnd = time.Now().Add(-time.Millisecond), time.Now()
	p.snap.PID++
	if l.Resume(t.Context()) == nil || p.starts != 1 {
		t.Fatal("different original process adopted")
	}
}

func TestAIStoppedJournalHandoffClosesWritersBeforeUnlinkAndFinalResume(t *testing.T) {
	l, p := aiStoppedUnitLease(t)
	if e := l.Stop(t.Context()); e != nil {
		t.Fatal(e)
	}
	l.carrierAttempted, l.carrierZero, l.carrierID, p.removes = true, true, p.nativeCID, 1
	j := aiExecUnitJournal(t, aiExecUnitBinding([]byte("test-only")))
	j.mu.Lock()
	e := j.appendLocked(aiExecRecord{Stage: "create_intent", EngineVersionSHA256: strings.Repeat("e", 64)})
	j.mu.Unlock()
	if e != nil {
		t.Fatal(e)
	}
	l.carrierJournal = j
	if e = l.Resume(t.Context()); e != nil {
		t.Fatal(e)
	}
	if l.VerifyResumed(t.Context()) == nil {
		t.Fatal("open writers accepted as final resume")
	}
	stopWriter, execWriter := l.journal, j.file
	var readers []*os.File
	err := l.SealTemporaryJournals(t.Context(), func(path string, writer *os.File, expected string) error {
		f, e := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
		if e != nil {
			return e
		}
		readers = append(readers, f)
		w, we := writer.Stat()
		actual, ae := f.Stat()
		raw, re := io.ReadAll(f)
		if we != nil || ae != nil || re != nil || !os.SameFile(w, actual) || sourceSHA(raw) != expected {
			return ErrAIStoppedRuntime
		}
		return nil
	})
	defer func() {
		for _, f := range readers {
			_ = f.Close()
		}
	}()
	if err != nil || len(readers) != 2 {
		t.Fatalf("actual double registration: %v", err)
	}
	if _, e = stopWriter.WriteAt([]byte("x"), 0); e == nil {
		t.Fatal("stop writer remained open")
	}
	if _, e = execWriter.WriteAt([]byte("x"), 0); e == nil {
		t.Fatal("exec writer remained open")
	}
	if e = os.Remove(l.journalPath); e != nil {
		t.Fatal(e)
	}
	if e = os.Remove(j.path); e != nil {
		t.Fatal(e)
	}
	before := append([]byte(nil), l.journalRaw...)
	if e = l.VerifyResumed(t.Context()); e != nil || p.starts != 1 || !bytes.Equal(before, l.journalRaw) {
		t.Fatal("final resume started/appended after purge")
	}
}
