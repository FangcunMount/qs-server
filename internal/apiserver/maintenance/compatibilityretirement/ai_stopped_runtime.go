package retirement

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"syscall"

	fence "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirementfence"
)

var ErrAIStoppedRuntime SourceError = "ai_original_stopped_runtime_or_carrier_unproven"

// Independently approved constraints on actual reads, never imported stop proof.
// NetworkID is the original Engine network ID; no caller chooses a network name,
// database URL, settings, executable, Docker arguments or completion assertion.
type AIStoppedRuntimeConstraints struct {
	SettingsSHA256 string `json:"settings_sha256"`
	NetworkID      string `json:"network_id"`
}

func (v AIStoppedRuntimeConstraints) Valid() bool {
	return evidenceHash(v.SettingsSHA256) && evidenceHash(v.NetworkID)
}

type aiStoppedSnapshot struct {
	Runtime                        aiExternalRuntime
	ConfigSHA256, HostConfigSHA256 string
	Settings                       map[string]string
	NetworkID                      string
	RestartPolicy                  string
	ExitCode                       int
	PID                            int
	OOM, Dead, Paused, Restarting  bool
	ExecIDs                        []string
}

func (v aiStoppedSnapshot) immutableSame(other aiStoppedSnapshot) bool {
	a, b := v.Runtime, other.Runtime
	a.Status, b.Status = "", ""
	a.StartedAt, b.StartedAt = "", ""
	a.Running, b.Running = false, false
	a.Restarts, b.Restarts = 0, 0
	return reflect.DeepEqual(a, b) && v.ConfigSHA256 == other.ConfigSHA256 && v.HostConfigSHA256 == other.HostConfigSHA256 && v.NetworkID == other.NetworkID && v.RestartPolicy == other.RestartPolicy && reflect.DeepEqual(v.Settings, other.Settings)
}
func (v aiStoppedSnapshot) stopped() bool {
	return !v.Runtime.Running && v.Runtime.Status == "exited" && v.PID == 0 && !v.OOM && !v.Dead && !v.Paused && !v.Restarting && v.ExitCode == 0
}

type aiStoppedProtocol interface {
	snapshot(context.Context, string) (aiStoppedSnapshot, error)
	network(context.Context, string) error
	stop(context.Context, string) error
	start(context.Context, string) error
	createCarrier(context.Context, aiStoppedSnapshot, string) (string, error)
	startCarrier(context.Context, string) error
	checkCarrier(context.Context, string, aiStoppedSnapshot, string) error
	inspectIdleCarrier(context.Context, string, aiStoppedSnapshot, string) (string, bool, error)
	removeCarrier(context.Context, string) error
	requireCarrierAbsent(context.Context, string) error
	requireOwnerCarriersAbsent(context.Context, string) error
}

// Private protocol seams support deterministic failure tests. The exported
// constructor always binds the actual original Window and fixed release reader.
type aiStoppedWindow interface {
	Diagnostic(context.Context) (fence.WindowBudgetReceipt, error)
	ForwardContext(context.Context) (context.Context, context.CancelFunc, error)
	RecoveryContext(context.Context) (context.Context, context.CancelFunc, error)
}

// The original same-process owner retains its Window and protected journal FD.
// Close releases only its descriptors; it never restores, adopts or retries.
// Any uncertain stop/create/start remains an owned recovery obligation.
type AIStoppedRuntimeLease struct {
	self                                                                *AIStoppedRuntimeLease
	mu                                                                  sync.Mutex
	window                                                              aiStoppedWindow
	readRelease                                                         func(string, string) (aiExternalRelease, error)
	binding                                                             fence.WindowBinding
	startSHA                                                            string
	input                                                               AIExternalExecutionInput
	baseline                                                            aiStoppedSnapshot
	constraints                                                         AIStoppedRuntimeConstraints
	protocol                                                            aiStoppedProtocol
	docker                                                              *aiExternalDockerExecutor
	journal                                                             *os.File
	carrierJournal                                                      *aiExecJournal
	journalPath                                                         string
	journalRaw                                                          []byte
	journalStat                                                         os.FileInfo
	settingsRaw                                                         []byte
	carrierID                                                           string
	carrierAttempted, carrierStarted, carrierUnknown, carrierZero       bool
	stopAttempted, stopped, restoreAttempted, restored, closed, unknown bool
	signalAttempted                                                     bool
}

func (*AIStoppedRuntimeLease) MarshalJSON() ([]byte, error) { return nil, ErrSourceSerialization }
func (*AIStoppedRuntimeLease) String() string {
	return "opaque original AI runtime stop/carrier lease; no whole-writer fence"
}
func (l *AIStoppedRuntimeLease) GoString() string           { return l.String() }
func (*AIStoppedRuntimeLease) MarshalBSON() ([]byte, error) { return nil, ErrSourceSerialization }
func (*AIStoppedRuntimeLease) UnmarshalJSON([]byte) error   { return ErrSourceSerialization }
func (*AIStoppedRuntimeLease) UnmarshalBSON([]byte) error   { return ErrSourceSerialization }
func (l *AIStoppedRuntimeLease) checkWindow(ctx context.Context, recovery bool) error {
	if l == nil || l.self != l || l.closed || ctx == nil || ctx.Err() != nil || l.window == nil {
		return ErrAIStoppedRuntime
	}
	d, e := l.window.Diagnostic(ctx)
	if e != nil || d.Binding != l.binding || d.StartSHA256 != l.startSHA || !d.DirectoryLeaseHeld || d.RemainingMilliseconds <= 0 || recovery && d.RecoverySHA256 == "" || !recovery && d.RecoverySHA256 != "" {
		return ErrAIStoppedRuntime
	}
	return nil
}

// OpenAIStoppedRuntimeLease performs actual preflight only. The caller must
// retain the returned owner BEFORE Stop; this constructor never signals a process.
func OpenAIStoppedRuntimeLease(ctx context.Context, in AIExternalExecutionInput, expected AIStoppedRuntimeConstraints, w *fence.MaintenanceWindow) (*AIStoppedRuntimeLease, error) {
	if runtime.GOOS != "linux" || os.Getuid() != 0 || os.Geteuid() != 0 || ctx == nil || ctx.Err() != nil || w == nil || !expected.Valid() {
		return nil, ErrAIStoppedRuntime
	}
	d, e := w.Diagnostic(ctx)
	if e != nil || !d.DirectoryLeaseHeld || d.RecoverySHA256 != "" || d.Binding.OperationID != filepath.Base(in.OperationDirectory) || !aiOriginalSourceSHA(in.RuntimeSourceSHA) || !aiExternalImageID(in.ImageID) || !evidenceHash(in.ContainerID) || !evidenceHash(in.ApprovedAIRuntimeBindingSHA256) {
		return nil, ErrAIStoppedRuntime
	}
	if e = RequireAIExternalExecQuiescence(ctx, AIExternalExecQuiescenceInput{OperationDirectory: in.OperationDirectory, SourceSHA: d.Binding.SourceSHA, OperationID: d.Binding.OperationID, RuntimeSourceSHA: in.RuntimeSourceSHA, ImageID: in.ImageID, ContainerID: in.ContainerID, SudoDocker: in.SudoDocker}); e != nil {
		return nil, e
	}
	release, e := aiExternalReadRelease(in.RuntimeSourceSHA, in.ImageID)
	if e != nil || release.seal != in.ApprovedAIRuntimeBindingSHA256 {
		return nil, ErrAIStoppedRuntime
	}
	docker, e := aiExternalDocker(in.SudoDocker)
	if e != nil {
		return nil, e
	}
	p := &aiStoppedDocker{docker: docker, wire: &aiExecDockerProtocol{docker: docker}, input: in}
	snap, e := p.snapshot(ctx, in.ContainerID)
	if e != nil || !snap.Runtime.matches(in) || !release.matchesMounts(snap.Runtime.Mounts) || snap.RestartPolicy != "unless-stopped" || snap.NetworkID != expected.NetworkID || sourceSHA(aiJSONBytes(snap.Settings)) != expected.SettingsSHA256 || len(snap.ExecIDs) != 0 || p.network(ctx, snap.NetworkID) != nil {
		return nil, ErrAIStoppedRuntime
	}
	if e = aiStoppedReleaseSettings(in, snap.Settings); e != nil {
		return nil, e
	}
	path := filepath.Join(in.OperationDirectory, "qs-ai-original-stop-carrier.jsonl")
	if aiExecParent(path) != nil {
		return nil, ErrAIStoppedRuntime
	}
	fd, e := syscall.Open(path, syscall.O_RDWR|syscall.O_CREAT|syscall.O_EXCL|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0600)
	if e != nil {
		return nil, ErrAIStoppedRuntime
	}
	file := os.NewFile(uintptr(fd), "original-ai-stop-journal")
	st, e := aiExecFileCheck(file, path, nil)
	if e != nil || syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB) != nil {
		_ = file.Close()
		return nil, ErrAIStoppedRuntime
	}
	l := &AIStoppedRuntimeLease{window: w, readRelease: aiExternalReadRelease, binding: d.Binding, startSHA: d.StartSHA256, input: in, baseline: snap, constraints: expected, protocol: p, docker: docker, journal: file, journalPath: path, journalStat: st, settingsRaw: aiJSONBytes(snap.Settings)}
	l.self = l
	if l.append("prepared", "") != nil || l.checkWindow(ctx, false) != nil {
		_ = l.Close()
		return nil, ErrAIStoppedRuntime
	}
	return l, nil
}
func aiJSONBytes(v any) []byte { b, _ := json.Marshal(v); return b }
func (l *AIStoppedRuntimeLease) append(stage, id string) error {
	if l == nil || l.self != l || l.closed || l.journal == nil {
		return ErrAIStoppedRuntime
	}
	st, e := aiExecFileCheck(l.journal, l.journalPath, l.journalStat)
	old := make([]byte, len(l.journalRaw))
	_, re := l.journal.ReadAt(old, 0)
	if e != nil || st.Size() != int64(len(old)) || len(old) > 0 && re != nil || !bytes.Equal(old, l.journalRaw) {
		return ErrAIStoppedRuntime
	}
	record := struct{ Protocol, Stage, SourceSHA, OperationID, OriginalRunID, ManifestSHA256, WindowStartSHA256, OriginalContainerID, ImageID, RuntimeBindingSHA256, SettingsSHA256, NetworkID, CarrierID, PreviousSHA256 string }{"qs-ai-original-stop-carrier/v1", stage, l.binding.SourceSHA, l.binding.OperationID, l.binding.OriginalRunID, l.binding.ManifestSHA256, l.startSHA, l.input.ContainerID, l.input.ImageID, l.input.ApprovedAIRuntimeBindingSHA256, l.constraints.SettingsSHA256, l.constraints.NetworkID, id, sourceSHA(l.journalRaw)}
	raw := append(aiJSONBytes(record), '\n')
	if len(l.journalRaw)+len(raw) > aiExecJournalLimit {
		return ErrAIStoppedRuntime
	}
	n, e := l.journal.WriteAt(raw, int64(len(l.journalRaw)))
	if e != nil || n != len(raw) || l.journal.Sync() != nil {
		return ErrAIStoppedRuntime
	}
	l.journalRaw = append(l.journalRaw, raw...)
	dir, e := os.Open(filepath.Dir(l.journalPath))
	if e != nil {
		return ErrAIStoppedRuntime
	}
	se, ce := dir.Sync(), dir.Close()
	if se != nil || ce != nil {
		return ErrAIStoppedRuntime
	}
	return nil
}
func (l *AIStoppedRuntimeLease) Stop(ctx context.Context) error {
	if l == nil || l.self != l {
		return ErrAIStoppedRuntime
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	q, cancel, e := l.forwardScope(ctx)
	if e != nil {
		return e
	}
	defer cancel()
	ctx = q
	if l.checkWindow(ctx, false) != nil || l.stopAttempted || l.restored {
		return ErrAIStoppedRuntime
	}
	before, e := l.protocol.snapshot(ctx, l.input.ContainerID)
	if e != nil || !l.baseline.immutableSame(before) || !reflect.DeepEqual(l.baseline.Runtime, before.Runtime) || len(before.ExecIDs) != 0 {
		return ErrAIStoppedRuntime
	}
	l.stopAttempted = true // Arm responsibility BEFORE durable intent and signal.
	if l.append("stop_intent", "") != nil {
		l.unknown = true
		return ErrAIStoppedRuntime
	}
	l.signalAttempted = true
	e = l.protocol.stop(ctx, l.input.ContainerID)
	after, ae := l.protocol.snapshot(ctx, l.input.ContainerID)
	if e != nil || ae != nil || !l.baseline.immutableSame(after) || !after.stopped() || len(after.ExecIDs) != 0 || l.checkWindow(ctx, false) != nil {
		l.unknown = true
		_ = l.append("stop_unknown", "")
		return ErrAIStoppedRuntime
	}
	if l.append("stopped", "") != nil {
		l.unknown = true
		return ErrAIStoppedRuntime
	}
	l.stopped = true
	return nil
}
func (l *AIStoppedRuntimeLease) CheckStopped(ctx context.Context) error {
	if l == nil || l.self != l {
		return ErrAIStoppedRuntime
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	q, cancel, e := l.forwardScope(ctx)
	if e != nil {
		return e
	}
	defer cancel()
	return l.checkStopped(q)
}
func (l *AIStoppedRuntimeLease) forwardScope(ctx context.Context) (context.Context, context.CancelFunc, error) {
	if l == nil || l.self != l || l.closed || l.window == nil {
		return nil, nil, ErrAIStoppedRuntime
	}
	return l.window.ForwardContext(ctx)
}
func (l *AIStoppedRuntimeLease) checkStopped(ctx context.Context) error {
	if l.checkWindow(ctx, false) != nil || !l.stopped || l.unknown || l.restored {
		return ErrAIStoppedRuntime
	}
	current, e := l.protocol.snapshot(ctx, l.input.ContainerID)
	if l.readRelease == nil {
		return ErrAIStoppedRuntime
	}
	release, re := l.readRelease(l.input.RuntimeSourceSHA, l.input.ImageID)
	if e != nil || re != nil || release.seal != l.input.ApprovedAIRuntimeBindingSHA256 || !l.baseline.immutableSame(current) || !current.stopped() || len(current.ExecIDs) != 0 {
		return ErrAIStoppedRuntime
	}
	return nil
}

// Restore only starts this exact original CID under the original recovery budget.
// It never uses Compose, recreates an image/container, changes config or migrates.
func (l *AIStoppedRuntimeLease) Restore(ctx context.Context) error {
	if l == nil || l.self != l {
		return ErrAIStoppedRuntime
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.window == nil {
		return ErrAIStoppedRuntime
	}
	q, cancel, e := l.window.RecoveryContext(ctx)
	if e != nil {
		return e
	}
	defer cancel()
	ctx = q
	return l.restoreOriginal(ctx, true)
}

// Resume is the successful forward path. It starts the same original stopped
// CID after its carrier is proved absent; it has no migration/recreate path.
func (l *AIStoppedRuntimeLease) Resume(ctx context.Context) error {
	if l == nil || l.self != l {
		return ErrAIStoppedRuntime
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	q, cancel, e := l.forwardScope(ctx)
	if e != nil {
		return e
	}
	defer cancel()
	return l.restoreOriginal(q, false)
}
func (l *AIStoppedRuntimeLease) restoreOriginal(ctx context.Context, recovery bool) error {
	if l.checkWindow(ctx, recovery) != nil {
		return ErrAIStoppedRuntime
	}
	if !l.stopAttempted {
		return nil
	} // Preflight owner was armed; no signal sent.
	if l.restoreAttempted || l.carrierAttempted && !l.carrierZero || !recovery && l.unknown {
		return ErrAIStoppedRuntime
	}
	snap, e := l.protocol.snapshot(ctx, l.input.ContainerID)
	if l.readRelease == nil {
		return ErrAIStoppedRuntime
	}
	release, re := l.readRelease(l.input.RuntimeSourceSHA, l.input.ImageID)
	if e != nil || re != nil || release.seal != l.input.ApprovedAIRuntimeBindingSHA256 || !l.baseline.immutableSame(snap) || len(snap.ExecIDs) != 0 || snap.OOM || snap.Dead || snap.Restarting || snap.Paused {
		return ErrAIStoppedRuntime
	}
	if snap.Runtime.Running { // Failed stop may have left original process unchanged.
		// A sent stop with unknown completion may still finish later. A running
		// sample must never erase that outstanding effect or permit a retry.
		if l.signalAttempted || !reflect.DeepEqual(snap.Runtime, l.baseline.Runtime) {
			return ErrAIStoppedRuntime
		}
		l.restoreAttempted = true
		l.restored = true
		return l.append("original_unchanged", "")
	}
	if snap.Runtime.Running || snap.Runtime.Status != "exited" || snap.PID != 0 || snap.ExitCode < 0 || snap.ExitCode > 255 {
		return ErrAIStoppedRuntime
	}
	l.restoreAttempted = true
	if l.append("restore_intent", "") != nil {
		return ErrAIStoppedRuntime
	}
	e = l.protocol.start(ctx, l.input.ContainerID)
	after, ae := l.protocol.snapshot(ctx, l.input.ContainerID)
	if e != nil || ae != nil || !l.baseline.immutableSame(after) || !after.Runtime.Running || after.Runtime.Status != "running" || after.PID <= 0 || after.OOM || after.Dead || after.Restarting || l.checkWindow(ctx, recovery) != nil {
		l.unknown = true
		_ = l.append("restore_unknown", "")
		return ErrAIStoppedRuntime
	}
	if l.append("restored", "") != nil {
		return ErrAIStoppedRuntime
	}
	l.restored = true
	return nil
}
func (l *AIStoppedRuntimeLease) Close() error {
	if l == nil || l.self != l {
		return ErrAIStoppedRuntime
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return nil
	}
	l.closed = true
	clear(l.settingsRaw)
	l.settingsRaw = nil
	clear(l.baseline.Settings)
	if l.carrierJournal != nil {
		if e := l.carrierJournal.Close(); e != nil {
			return e
		}
	}
	if l.journal != nil {
		return l.journal.Close()
	}
	return nil
}

var aiStoppedSettingsKeys = map[string]bool{"QS_AI_ENVIRONMENT": true, "QS_AI_RELEASE_SHA": true, "QS_AI_DATABASE_URL": true, "QS_AI_MESSAGING": true, "QS_AI_MODELS": true, "QS_AI_GOVERNANCE_MODELS": true, "QS_AI_PARTICIPANT_CAPACITY": true, "QS_AI_QUOTA_CEILINGS": true, "QS_AI_GENERATION": true, "QS_AI_LOGGING": true, "QS_AI_HTTP": true, "QS_AI_DATABASE": true, "QS_AI_DIAGNOSTICS": true, "QS_AI_WORKER": true, "QS_AI_EVALUATION": true, "QS_AI_MODEL_CAPACITY": true, "QS_AI_GRPC": true, "QS_AI_DELIVERY": true}

func init() {
	// The concrete nested fields emitted by qs-ai scripts/cd/deploy.py. Unknown
	// settings fail rather than silently falling back to image defaults.
	for _, key := range []string{"QS_AI_GENERATION__ENABLED", "QS_AI_GENERATION__ENDPOINT", "QS_AI_GRPC__GOVERNANCE_ENABLED", "QS_AI_GRPC__ACCESS_ADDRESS", "QS_AI_GRPC__RESULT_ADDRESS", "QS_AI_EVALUATION__ENABLED", "QS_AI_EVALUATION__CANDIDATE_MODE_ENABLED", "QS_AI_EVALUATION__PARALLEL_CALLS", "QS_AI_EVALUATION__PER_RUN_PARALLEL_CALLS", "QS_AI_EVALUATION__CONCURRENCY", "QS_AI_MODELS__V2_WRITES_ENABLED", "QS_AI_MODEL_CAPACITY__DEEPSEEK__TOTAL", "QS_AI_MODEL_CAPACITY__ZHIPU__TOTAL"} {
		aiStoppedSettingsKeys[key] = true
	}
}

func aiStoppedSettings(env []string, in AIExternalExecutionInput) (map[string]string, error) {
	out := map[string]string{}
	seen := map[string]bool{}
	for _, raw := range env {
		key, value, ok := strings.Cut(raw, "=")
		if !ok || seen[key] || strings.ContainsAny(raw, "\x00\r\n") {
			return nil, ErrAIStoppedRuntime
		}
		seen[key] = true
		if aiStoppedSettingsKeys[key] {
			out[key] = value
		} else if strings.HasPrefix(key, "QS_AI_") && key != "QS_AI_MODEL_API_KEY" && key != "QS_AI_DEEPSEEK_API_KEY" && key != "QS_AI_ZHIPU_API_KEY" {
			return nil, ErrAIStoppedRuntime
		}
	}
	if out["QS_AI_ENVIRONMENT"] != "production" || out["QS_AI_RELEASE_SHA"] != in.RuntimeSourceSHA || !strings.HasPrefix(out["QS_AI_DATABASE_URL"], "mysql+asyncmy://") || len(aiJSONBytes(out)) > 32768 {
		return nil, ErrAIStoppedRuntime
	}
	return out, nil
}
func aiStoppedReleaseSettings(in AIExternalExecutionInput, actual map[string]string) error {
	state, e := aiExternalReleaseFile("/opt/qs-ai/state.json", false)
	if e != nil {
		return e
	}
	var s struct {
		Current string `json:"current"`
	}
	if json.Unmarshal(state, &s) != nil || !strings.HasPrefix(s.Current, in.RuntimeSourceSHA+"-") {
		return ErrAIStoppedRuntime
	}
	raw, e := aiExternalReleaseFile("/opt/qs-ai/releases/"+s.Current+"/runtime.json", false)
	if e != nil {
		return e
	}
	var v struct {
		Services map[string]struct {
			Environment map[string]json.RawMessage `json:"environment"`
		} `json:"services"`
	}
	if json.Unmarshal(raw, &v) != nil {
		return ErrAIStoppedRuntime
	}
	for key, value := range v.Services["qs-ai"].Environment {
		if !aiStoppedSettingsKeys[key] {
			continue
		}
		var expected string
		if json.Unmarshal(value, &expected) != nil || actual[key] != strings.ReplaceAll(expected, "$$", "$") {
			return ErrAIStoppedRuntime
		}
	}
	return nil
}

type aiStoppedDocker struct {
	docker *aiExternalDockerExecutor
	wire   *aiExecDockerProtocol
	input  AIExternalExecutionInput
}

func (p *aiStoppedDocker) snapshot(ctx context.Context, cid string) (aiStoppedSnapshot, error) {
	var out aiStoppedSnapshot
	if !evidenceHash(cid) {
		return out, ErrAIStoppedRuntime
	}
	raw, e := p.wire.ordinary(ctx, http.MethodGet, "/v"+aiExecAPIVersion+"/containers/"+cid+"/json", nil, http.StatusOK)
	if e != nil || strictJSON(raw) != nil {
		return out, ErrAIStoppedRuntime
	}
	defer clear(raw)
	var v struct {
		ID         string `json:"Id"`
		Config     json.RawMessage
		HostConfig json.RawMessage
		State      struct {
			ExitCode, Pid                       int
			OOMKilled, Dead, Paused, Restarting bool
		}
		ExecIDs         []string
		NetworkSettings struct {
			Networks map[string]struct{ NetworkID string }
		}
	}
	if json.Unmarshal(raw, &v) != nil || v.ID != cid || len(v.Config) == 0 || len(v.HostConfig) == 0 || len(v.NetworkSettings.Networks) != 1 {
		return out, ErrAIStoppedRuntime
	}
	var config struct{ Env []string }
	var host struct {
		RestartPolicy struct {
			Name              string
			MaximumRetryCount int
		}
	}
	if json.Unmarshal(v.Config, &config) != nil || json.Unmarshal(v.HostConfig, &host) != nil || host.RestartPolicy.MaximumRetryCount != 0 {
		return out, ErrAIStoppedRuntime
	}
	out.Settings, e = aiStoppedSettings(config.Env, p.input)
	if e != nil {
		return out, e
	}
	out.Runtime, e = p.docker.inspect(ctx, cid)
	if e != nil {
		return out, e
	}
	out.ConfigSHA256 = sourceSHA(v.Config)
	out.HostConfigSHA256 = sourceSHA(v.HostConfig)
	out.RestartPolicy = host.RestartPolicy.Name
	for _, n := range v.NetworkSettings.Networks {
		out.NetworkID = n.NetworkID
	}
	if !evidenceHash(out.NetworkID) {
		return out, ErrAIStoppedRuntime
	}
	out.ExitCode, out.PID, out.OOM, out.Dead, out.Paused, out.Restarting, out.ExecIDs = v.State.ExitCode, v.State.Pid, v.State.OOMKilled, v.State.Dead, v.State.Paused, v.State.Restarting, v.ExecIDs
	return out, nil
}
func (p *aiStoppedDocker) network(ctx context.Context, id string) error {
	raw, e := p.wire.ordinary(ctx, http.MethodGet, "/v"+aiExecAPIVersion+"/networks/"+id, nil, http.StatusOK)
	var v struct {
		ID            string `json:"Id"`
		Driver, Scope string
	}
	if e != nil || strictJSON(raw) != nil || json.Unmarshal(raw, &v) != nil || v.ID != id || v.Driver != "bridge" || v.Scope != "local" {
		return ErrAIStoppedRuntime
	}
	return nil
}
func (p *aiStoppedDocker) stop(ctx context.Context, id string) error {
	_, e := p.wire.ordinary(ctx, http.MethodPost, "/v"+aiExecAPIVersion+"/containers/"+id+"/stop?t=210", nil, http.StatusNoContent)
	return e
}
func (p *aiStoppedDocker) start(ctx context.Context, id string) error {
	_, e := p.wire.ordinary(ctx, http.MethodPost, "/v"+aiExecAPIVersion+"/containers/"+id+"/start", nil, http.StatusNoContent)
	return e
}

// The carrier has no credential Env, ports, restart, service bootstrap or engine
// socket. The only credentials arrive later in the anonymous attached exec pipe.
const aiStoppedCarrierIdle = "import signal; signal.pause()"

func (p *aiStoppedDocker) createCarrier(ctx context.Context, s aiStoppedSnapshot, op string) (string, error) {
	if !aiLocalOperationID(op) {
		return "", ErrAIStoppedRuntime
	}
	mounts := []map[string]any{}
	for _, m := range s.Runtime.Mounts {
		if m.Type != "bind" || m.RW {
			return "", ErrAIStoppedRuntime
		}
		mounts = append(mounts, map[string]any{"Type": "bind", "Source": m.Source, "Target": m.Destination, "ReadOnly": true})
	}
	payload := map[string]any{"Image": s.Runtime.ImageID, "Entrypoint": []string{"/app/.venv/bin/python"}, "Cmd": []string{"-I", "-B", "-c", aiStoppedCarrierIdle}, "Env": []string{}, "Labels": map[string]string{"qs.retirement.kind": "ai-final-readonly-carrier", "qs.retirement.operation": op, "qs.retirement.original": s.Runtime.ContainerID}, "HostConfig": map[string]any{"ReadonlyRootfs": true, "NetworkMode": s.NetworkID, "Mounts": mounts, "Tmpfs": map[string]string{"/tmp": "rw,noexec,nosuid,size=64m"}, "CapDrop": []string{"ALL"}, "SecurityOpt": []string{"no-new-privileges"}, "RestartPolicy": map[string]any{"Name": "no"}, "LogConfig": map[string]any{"Type": "none"}, "Memory": int64(768 << 20), "NanoCpus": int64(750000000)}}
	raw, e := p.wire.ordinary(ctx, http.MethodPost, "/v"+aiExecAPIVersion+"/containers/create?name=qs-retirement-ai-final-"+op, aiJSONBytes(payload), http.StatusCreated)
	var result struct {
		ID       string `json:"Id"`
		Warnings []string
	}
	if strictJSON(raw) != nil || json.Unmarshal(raw, &result) != nil || !evidenceHash(result.ID) {
		return "", ErrAIStoppedRuntime
	}
	if len(result.Warnings) != 0 {
		return result.ID, ErrAIStoppedRuntime
	}
	return result.ID, e
}
func (p *aiStoppedDocker) startCarrier(ctx context.Context, id string) error { return p.start(ctx, id) }
func (p *aiStoppedDocker) checkCarrier(ctx context.Context, id string, s aiStoppedSnapshot, op string) error {
	_, running, e := p.inspectCarrier(ctx, id, s, op, true)
	if e != nil || !running {
		return ErrAIStoppedRuntime
	}
	return nil
}
func (p *aiStoppedDocker) inspectIdleCarrier(ctx context.Context, id string, s aiStoppedSnapshot, op string) (string, bool, error) {
	return p.inspectCarrier(ctx, id, s, op, true)
}
func (p *aiStoppedDocker) inspectCarrier(ctx context.Context, id string, s aiStoppedSnapshot, op string, requireIdle bool) (string, bool, error) {
	if !aiLocalOperationID(op) || (!evidenceHash(id) && id != "qs-retirement-ai-final-"+op) {
		return "", false, ErrAIStoppedRuntime
	}
	raw, e := p.wire.ordinary(ctx, http.MethodGet, "/v"+aiExecAPIVersion+"/containers/"+id+"/json", nil, http.StatusOK)
	if e != nil || strictJSON(raw) != nil {
		return "", false, ErrAIStoppedRuntime
	}
	var v struct {
		ID     string `json:"Id"`
		Name   string
		Image  string
		Config struct {
			Entrypoint, Cmd, Env []string
			Labels               map[string]string
		}
		HostConfig struct {
			ReadonlyRootfs, Privileged   bool
			NetworkMode                  string
			CapDrop, CapAdd, SecurityOpt []string
			PortBindings                 map[string]any
			RestartPolicy                struct{ Name string }
			LogConfig                    struct{ Type string }
			Tmpfs                        map[string]string
			Memory, NanoCpus             int64
			Binds, VolumesFrom           []string
		}
		Mounts []aiExternalMount
		State  struct {
			Running, Paused, Restarting, Dead bool
			Pid                               int
			Status                            string
		}
		ExecIDs []string
	}
	if json.Unmarshal(raw, &v) != nil || !evidenceHash(v.ID) || evidenceHash(id) && v.ID != id || v.Name != "/qs-retirement-ai-final-"+op || v.Image != s.Runtime.ImageID || !reflect.DeepEqual(v.Config.Entrypoint, []string{"/app/.venv/bin/python"}) || !reflect.DeepEqual(v.Config.Cmd, []string{"-I", "-B", "-c", aiStoppedCarrierIdle}) || v.Config.Labels["qs.retirement.operation"] != op || v.Config.Labels["qs.retirement.original"] != s.Runtime.ContainerID || v.Config.Labels["qs.retirement.kind"] != "ai-final-readonly-carrier" || !v.HostConfig.ReadonlyRootfs || v.HostConfig.Privileged || v.HostConfig.NetworkMode != s.NetworkID || len(v.HostConfig.PortBindings) != 0 || len(v.HostConfig.CapAdd) != 0 || !reflect.DeepEqual(v.HostConfig.CapDrop, []string{"ALL"}) || !reflect.DeepEqual(v.HostConfig.SecurityOpt, []string{"no-new-privileges"}) && !reflect.DeepEqual(v.HostConfig.SecurityOpt, []string{"no-new-privileges=true"}) || v.HostConfig.RestartPolicy.Name != "no" || v.HostConfig.LogConfig.Type != "none" || !reflect.DeepEqual(v.HostConfig.Tmpfs, map[string]string{"/tmp": "rw,noexec,nosuid,size=64m"}) || v.HostConfig.Memory != 768<<20 || v.HostConfig.NanoCpus != 750000000 || len(v.HostConfig.Binds) != 0 || len(v.HostConfig.VolumesFrom) != 0 || requireIdle && len(v.ExecIDs) != 0 || !v.State.Running && (v.State.Pid != 0 || v.State.Status != "created" && v.State.Status != "exited") || v.State.Paused || v.State.Restarting || v.State.Dead {
		return "", false, ErrAIStoppedRuntime
	}
	for _, env := range v.Config.Env {
		if strings.HasPrefix(env, "QS_AI_") {
			// This fixed public image build argument is not a credential. The
			// anonymous wrapper clears it before applying the sealed Settings.
			if env != "QS_AI_RELEASE_SHA="+s.Runtime.ImageRevision {
				return "", false, ErrAIStoppedRuntime
			}
		}
	}
	seen := map[string]bool{}
	for _, m := range v.Mounts {
		if m.Destination == "/tmp" && m.Type == "tmpfs" {
			continue
		}
		found := false
		for _, want := range s.Runtime.Mounts {
			if reflect.DeepEqual(m, want) {
				found = true
				seen[m.Destination] = true
			}
		}
		if !found {
			return "", false, ErrAIStoppedRuntime
		}
	}
	if len(seen) != len(s.Runtime.Mounts) {
		return "", false, ErrAIStoppedRuntime
	}
	return v.ID, v.State.Running, nil
}
func (p *aiStoppedDocker) removeCarrier(ctx context.Context, id string) error {
	// Only called after the exact owned exec's actual terminal output/inspection.
	state, e := p.carrierState(ctx, id)
	if e != nil {
		return e
	}
	if state.Running {
		if _, e := p.wire.ordinary(ctx, http.MethodPost, "/v"+aiExecAPIVersion+"/containers/"+id+"/stop?t=5", nil, http.StatusNoContent); e != nil {
			return e
		}
	}
	state, e = p.carrierState(ctx, id)
	if e != nil || state.Running || state.Pid != 0 || state.Dead || state.Restarting || state.Paused || state.Status != "created" && state.Status != "exited" {
		return ErrAIStoppedRuntime
	}
	_, e = p.wire.ordinary(ctx, http.MethodDelete, "/v"+aiExecAPIVersion+"/containers/"+id+"?v=false&force=false", nil, http.StatusNoContent)
	return e
}

type aiStoppedCarrierState struct {
	Running, Dead, Restarting, Paused bool
	Pid                               int
	Status                            string
}

func (p *aiStoppedDocker) carrierState(ctx context.Context, id string) (aiStoppedCarrierState, error) {
	var v struct {
		ID    string `json:"Id"`
		State aiStoppedCarrierState
	}
	raw, e := p.wire.ordinary(ctx, http.MethodGet, "/v"+aiExecAPIVersion+"/containers/"+id+"/json", nil, http.StatusOK)
	if e != nil || !evidenceHash(id) || strictJSON(raw) != nil || json.Unmarshal(raw, &v) != nil || v.ID != id {
		return aiStoppedCarrierState{}, ErrAIStoppedRuntime
	}
	return v.State, nil
}
func (p *aiStoppedDocker) requireCarrierAbsent(ctx context.Context, id string) error {
	raw, e := p.wire.ordinary(ctx, http.MethodGet, "/v"+aiExecAPIVersion+"/containers/"+id+"/json", nil, http.StatusNotFound)
	if e != nil || strictJSON(raw) != nil {
		return ErrAIStoppedRuntime
	}
	var v struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(raw, &v) != nil || v.Message != "No such container: "+id {
		return ErrAIStoppedRuntime
	}
	return nil
}
func (p *aiStoppedDocker) requireOwnerCarriersAbsent(ctx context.Context, op string) error {
	if !aiLocalOperationID(op) {
		return ErrAIStoppedRuntime
	}
	filters := aiJSONBytes(map[string][]string{"label": {"qs.retirement.kind=ai-final-readonly-carrier", "qs.retirement.operation=" + op}})
	raw, e := p.wire.ordinary(ctx, http.MethodGet, "/v"+aiExecAPIVersion+"/containers/json?all=1&filters="+url.QueryEscape(string(filters)), nil, http.StatusOK)
	var rows []json.RawMessage
	if e != nil || strictJSON(raw) != nil || json.Unmarshal(raw, &rows) != nil || rows == nil || len(rows) != 0 {
		return ErrAIStoppedRuntime
	}
	return nil
}
