package retirement

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"path/filepath"
	"reflect"
	"time"
)

// A separate fixed final-only host. The original host and its three frozen
// modules remain byte-identical. Secrets are received only on attached stdin.
// No bootstrap, migration, broker consumer or provider process is imported.
const aiStoppedCarrierHost = `import base64, hashlib, io, json, os, sys
try:
    raw = sys.stdin.buffer.read(25165825)
    if len(raw) > 25165824:
        raise ValueError()
    envelope = json.loads(raw)
    if set(envelope) != {"protocol", "settings", "settings_sha256", "host", "packet"} or envelope["protocol"] != "qs-ai-stopped-final-pipe/v1":
        raise ValueError()
    settings_raw = base64.b64decode(envelope["settings"], validate=True)
    if len(settings_raw) > 32768 or hashlib.sha256(settings_raw).hexdigest() != envelope["settings_sha256"]:
        raise ValueError()
    settings = json.loads(settings_raw)
    allowed = {"QS_AI_ENVIRONMENT", "QS_AI_RELEASE_SHA", "QS_AI_DATABASE_URL", "QS_AI_MESSAGING", "QS_AI_MODELS", "QS_AI_GOVERNANCE_MODELS", "QS_AI_PARTICIPANT_CAPACITY", "QS_AI_QUOTA_CEILINGS", "QS_AI_GENERATION", "QS_AI_LOGGING", "QS_AI_HTTP", "QS_AI_DATABASE", "QS_AI_DIAGNOSTICS", "QS_AI_WORKER", "QS_AI_EVALUATION", "QS_AI_MODEL_CAPACITY", "QS_AI_GRPC", "QS_AI_DELIVERY"}
    allowed |= {"QS_AI_GENERATION__ENABLED", "QS_AI_GENERATION__ENDPOINT", "QS_AI_GRPC__GOVERNANCE_ENABLED", "QS_AI_GRPC__ACCESS_ADDRESS", "QS_AI_GRPC__RESULT_ADDRESS", "QS_AI_EVALUATION__ENABLED", "QS_AI_EVALUATION__CANDIDATE_MODE_ENABLED", "QS_AI_EVALUATION__PARALLEL_CALLS", "QS_AI_EVALUATION__PER_RUN_PARALLEL_CALLS", "QS_AI_EVALUATION__CONCURRENCY", "QS_AI_MODELS__V2_WRITES_ENABLED", "QS_AI_MODEL_CAPACITY__DEEPSEEK__TOTAL", "QS_AI_MODEL_CAPACITY__ZHIPU__TOTAL"}
    if not isinstance(settings, dict) or not set(settings) <= allowed or not all(isinstance(k, str) and isinstance(v, str) and not any(c in v for c in "\x00\r\n") for k, v in settings.items()):
        raise ValueError()
    host = base64.b64decode(envelope["host"], validate=True)
    if hashlib.sha256(host).hexdigest() != "c77f47ed775d38b19f880b03c67c1d2f054660d82ccd4546f6c6ecc8b16f1940":
        raise ValueError()
    packet = base64.b64decode(envelope["packet"], validate=True)
    identity = json.loads(packet)
    if identity["protocol"] != "qs-ai-readonly-host-input/v3" or settings["QS_AI_ENVIRONMENT"] != "production" or settings["QS_AI_RELEASE_SHA"] != identity["runtime_source_sha"] or not settings["QS_AI_DATABASE_URL"].startswith("mysql+asyncmy://"):
        raise ValueError()
    for key in list(os.environ):
        if key.startswith("QS_AI_"):
            del os.environ[key]
    os.environ.update(settings)
    sys.stdin = io.TextIOWrapper(io.BytesIO(packet), encoding="utf-8")
except Exception:
    raise SystemExit(71)
exec(compile(host, "qs-ai-retirement-readonly-host.py", "exec"), {"__name__": "__main__", "__file__": "qs-ai-retirement-readonly-host.py"})
`

func (l *AIStoppedRuntimeLease) executeFinal(ctx context.Context, owner HistoricalCoordinatorBinding, in AIExternalExecutionInput, host, packet []byte) ([]byte, error) {
	if l == nil || l.self != l {
		return nil, ErrAIStoppedRuntime
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	q, cancel, e := l.forwardScope(ctx)
	if e != nil {
		return nil, e
	}
	defer cancel()
	ctx = q
	if l.checkStopped(ctx) != nil || l.carrierAttempted || l.binding.SourceSHA != owner.SourceSHA || l.binding.OperationID != owner.OperationID || !reflect.DeepEqual(l.input, in) || sourceSHA(host) != aiExternalHostSHA || sourceSHA(l.settingsRaw) != l.constraints.SettingsSHA256 {
		return nil, ErrAIStoppedRuntime
	}
	// Validate the original packet and deadline before any new side effect.
	b, e := aiExternalExecModeBinding(ctx, aiExternalFinalVerifyMode, owner, in.RunID, in.RuntimeSourceSHA, in.ImageID, in.ContainerID, host, packet)
	if e != nil {
		return nil, e
	}
	prior, e := aiExecOpenFinalPredecessor(ctx, l.docker, in.OperationDirectory, owner, in.RuntimeSourceSHA, in.ImageID, in.ContainerID, in.SourceUID)
	if e != nil {
		return nil, e
	}
	defer func() { _ = prior.Close() }()
	envelope, e := json.Marshal(struct {
		Protocol       string `json:"protocol"`
		Settings       string `json:"settings"`
		SettingsSHA256 string `json:"settings_sha256"`
		Host           string `json:"host"`
		Packet         string `json:"packet"`
	}{"qs-ai-stopped-final-pipe/v1", base64.StdEncoding.EncodeToString(l.settingsRaw), l.constraints.SettingsSHA256, base64.StdEncoding.EncodeToString(host), base64.StdEncoding.EncodeToString(packet)})
	if e != nil || len(envelope) > aiExternalInputLimit {
		return nil, ErrAIStoppedRuntime
	}
	defer clear(envelope)
	if l.protocol.requireCarrierAbsent(ctx, "qs-retirement-ai-final-"+l.binding.OperationID) != nil || l.protocol.requireOwnerCarriersAbsent(ctx, l.binding.OperationID) != nil {
		return nil, ErrAIStoppedRuntime
	}
	l.carrierAttempted = true // Responsibility precedes durable intent and create.
	if l.append("carrier_create_intent", "") != nil {
		l.carrierUnknown = true
		return nil, ErrAIStoppedRuntime
	}
	id, createErr := l.protocol.createCarrier(ctx, l.baseline, l.binding.OperationID)
	if evidenceHash(id) {
		l.carrierID = id // Keep known ownership even if local Wait is unknown.
		if l.append("carrier_created", id) != nil {
			l.carrierUnknown = true
			return nil, ErrAIStoppedRuntime
		}
	}
	if createErr != nil || !evidenceHash(id) {
		l.carrierUnknown = true
		_ = l.append("carrier_create_unknown", id)
		return nil, ErrAIStoppedRuntime
	}
	if l.append("carrier_start_intent", id) != nil {
		l.carrierUnknown = true
		return nil, ErrAIStoppedRuntime
	}
	if l.protocol.startCarrier(ctx, id) != nil || l.protocol.checkCarrier(ctx, id, l.baseline, l.binding.OperationID) != nil || l.checkStopped(ctx) != nil {
		l.carrierUnknown = true
		_ = l.append("carrier_start_unknown", id)
		return nil, ErrAIStoppedRuntime
	}
	l.carrierStarted = true
	if l.append("carrier_started", id) != nil {
		l.carrierUnknown = true
		return nil, ErrAIStoppedRuntime
	}
	b.ExecutionContainerID, b.StoppedLeaseSHA256 = id, sourceSHA(l.journalRaw)
	b.PythonSHA256, b.InputSHA256 = sourceSHA([]byte(aiStoppedCarrierHost)), sourceSHA(envelope)
	if !b.valid() {
		return nil, ErrAIStoppedRuntime
	}
	// Fixed independent path: never replace or append to old raw final journals.
	path := filepath.Join(in.StoppedJournalDirectory, "qs-ai-external-stopped-final-verify.exec.jsonl")
	j, e := aiExecOpenJournal(path, b, true)
	if e != nil {
		return nil, e
	}
	l.carrierJournal = j // Retain exact FD/ID for partial and UNKNOWN recovery.
	observed, e := aiExternalExecTracked(ctx, l.docker, j, []byte(aiStoppedCarrierHost), envelope)
	if e != nil {
		l.carrierUnknown = true
		_ = l.append("carrier_exec_unknown", id)
		return nil, e
	}
	output, e := observed.actualOutput(ctx)
	if e != nil || l.protocol.checkCarrier(ctx, id, l.baseline, l.binding.OperationID) != nil || l.checkStopped(ctx) != nil {
		l.carrierUnknown = true
		return nil, ErrAIStoppedRuntime
	}
	prior.mu.Lock()
	e = prior.checkLocked()
	prior.mu.Unlock()
	if e != nil {
		return nil, e
	}
	if e = l.cleanupCarrier(ctx, false); e != nil {
		return nil, e
	}
	if l.checkStopped(ctx) != nil {
		return nil, ErrAIStoppedRuntime
	}
	return output, nil
}

// Cleanup performs no new execution and cannot recover a lost result. It only
// reconciles the exact retained exec, then removes this exact carrier. Unknown
// creation with no native ID remains owned and blocks restoration and DROP.
func (l *AIStoppedRuntimeLease) CleanupCarrierForRecovery(ctx context.Context) error {
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
	return l.cleanupCarrier(q, true)
}
func (l *AIStoppedRuntimeLease) cleanupCarrier(ctx context.Context, recovery bool) error {
	if l.checkWindow(ctx, recovery) != nil {
		return ErrAIStoppedRuntime
	}
	if !l.carrierAttempted || l.carrierZero {
		return nil
	}
	if l.carrierJournal == nil {
		// A failed create/start before any exec may be reconciled by this exact
		// durable name and native owner labels. Never repeat create or start.
		name := l.carrierID
		if name == "" {
			name = "qs-retirement-ai-final-" + l.binding.OperationID
		}
		id, _, e := l.protocol.inspectIdleCarrier(ctx, name, l.baseline, l.binding.OperationID)
		if e != nil {
			return e
		}
		l.carrierID = id
		if l.append("carrier_recovery_observed_idle", id) != nil {
			return ErrAIStoppedRuntime
		}
	} else if !evidenceHash(l.carrierID) || l.protocol.checkCarrier(ctx, l.carrierID, l.baseline, l.binding.OperationID) != nil {
		return ErrAIStoppedRuntime
	}
	if l.carrierJournal != nil {
		q, c := context.WithTimeout(ctx, 14*time.Second)
		defer c()
		observed, e := aiExternalExecReconcile(q, l.docker, l.carrierJournal)
		if e != nil || observed == nil || observed.running || observed.exitCode == nil {
			return ErrAIStoppedRuntime
		}
		// Nonzero known terminal is cleanup only; it never yields Q.
		if !recovery && *observed.exitCode != 0 {
			return ErrAIStoppedRuntime
		}
	}
	if l.append("carrier_remove_intent", l.carrierID) != nil {
		return ErrAIStoppedRuntime
	}
	if l.protocol.removeCarrier(ctx, l.carrierID) != nil || l.protocol.requireCarrierAbsent(ctx, l.carrierID) != nil || l.protocol.requireOwnerCarriersAbsent(ctx, l.binding.OperationID) != nil {
		l.carrierUnknown = true
		_ = l.append("carrier_remove_unknown", l.carrierID)
		return ErrAIStoppedRuntime
	}
	if l.append("carrier_removed_and_zero", l.carrierID) != nil {
		return ErrAIStoppedRuntime
	}
	l.carrierZero = true
	return nil
}

// VerifyCarrierZero rechecks actual Engine absence on both exact CID and owner
// labels. It is separate from the two database restore-engine material scopes.
func (l *AIStoppedRuntimeLease) VerifyCarrierZero(ctx context.Context) error {
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
	if l.checkWindow(ctx, false) != nil || !l.carrierZero || l.protocol.requireCarrierAbsent(ctx, l.carrierID) != nil || l.protocol.requireOwnerCarriersAbsent(ctx, l.binding.OperationID) != nil {
		return ErrAIStoppedRuntime
	}
	return nil
}
