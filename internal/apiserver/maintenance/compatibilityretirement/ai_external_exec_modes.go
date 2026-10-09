package retirement

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"time"
)

type aiExternalExecMode string

const aiExternalVerifyMode aiExternalExecMode = "verify"
const aiExternalBoundsMode aiExternalExecMode = "bounds"

// This is the existing host operation_directory(root, operation_id), not a new
// root authorization mechanism. Fixed names remain identical across attempts.
func aiExternalExecModePath(directory, operation string, mode aiExternalExecMode) (string, error) {
	if !aiLocalOperationID(operation) || !filepath.IsAbs(directory) || filepath.Clean(directory) != directory || filepath.Base(directory) != operation {
		return "", ErrAIExternalExecJournal
	}
	root := filepath.Dir(directory)
	if filepath.Base(root) != "compatibility-retirement" || filepath.Base(filepath.Dir(root)) != "qs-server" || filepath.Base(filepath.Dir(filepath.Dir(root))) != "backups" {
		return "", ErrAIExternalExecJournal
	}
	name := ""
	switch mode {
	case aiExternalVerifyMode:
		name = "qs-ai-external-verify.exec.jsonl"
	case aiExternalBoundsMode:
		name = "qs-ai-external-bounds.exec.jsonl"
	default:
		return "", ErrAIExternalExecJournal
	}
	path := filepath.Join(directory, name)
	if aiExecParent(path) != nil {
		return "", ErrAIExternalExecJournal
	}
	return path, nil
}

func aiExternalExecModeBinding(ctx context.Context, mode aiExternalExecMode, owner HistoricalCoordinatorBinding, run, runtime, image, cid string, host, input []byte) (aiExecBinding, error) {
	var empty aiExecBinding
	if ctx == nil || ctx.Err() != nil || len(input) == 0 || len(input) > aiExternalInputLimit || sourceSHA(host) != aiExternalHostSHA || strictJSON(input) != nil {
		return empty, ErrAIExternalExecJournal
	}
	deadline, ok := ctx.Deadline()
	if !ok || !time.Now().Before(deadline) {
		return empty, ErrAIExternalExecUnknown
	}
	// These actual control fields must match the private packet just built by
	// its fixed factory; a bounds protocol cannot be executed under verify.
	var packet struct {
		Protocol    string `json:"protocol"`
		SourceSHA   string `json:"source_sha"`
		OperationID string `json:"operation_id"`
		RunID       string `json:"run_id"`
		RuntimeSHA  string `json:"runtime_source_sha"`
		ImageID     string `json:"image_id"`
		ContainerID string `json:"container_id"`
	}
	if json.Unmarshal(input, &packet) != nil || packet.SourceSHA != owner.SourceSHA || packet.OperationID != owner.OperationID || packet.RunID != run || packet.RuntimeSHA != runtime || packet.ImageID != image || packet.ContainerID != cid {
		return empty, ErrAIExternalExecJournal
	}
	protocol := ""
	switch mode {
	case aiExternalVerifyMode:
		protocol = "qs-ai-readonly-host-input/v2"
	case aiExternalBoundsMode:
		protocol = "qs-ai-readonly-bounds-discovery-input/v1"
	default:
		return empty, ErrAIExternalExecJournal
	}
	if packet.Protocol != protocol {
		return empty, ErrAIExternalExecJournal
	}
	binding := aiExecBinding{SourceSHA: owner.SourceSHA, OperationID: owner.OperationID, RunID: run, RuntimeSourceSHA: runtime, ImageID: image, ContainerID: cid, PythonSHA256: aiExternalHostSHA, InputSHA256: sourceSHA(input), DeadlineUnixNano: deadline.UnixNano()}
	if !binding.valid() {
		return empty, ErrAIExternalExecJournal
	}
	return binding, nil
}

// Called only after the actual factory's full private source/local/runtime
// checks. No caller-provided command, complete flag, new budget or endpoint.
func aiExternalExecuteMode(ctx context.Context, docker *aiExternalDockerExecutor, directory string, mode aiExternalExecMode, owner HistoricalCoordinatorBinding, run, runtime, image, cid string, host, input []byte) ([]byte, error) {
	binding, e := aiExternalExecModeBinding(ctx, mode, owner, run, runtime, image, cid, host, input)
	if e != nil {
		return nil, e
	}
	path, e := aiExternalExecModePath(directory, owner.OperationID, mode)
	if e != nil {
		return nil, e
	}
	journal, e := aiExecOpenJournal(path, binding, true)
	if e != nil {
		return nil, e
	}
	// Keep every journal, including a torn/unknown one. This owned file close
	// is not permission to release the host's operation exclusion lease.
	defer func() { _ = journal.Close() }()
	observed, e := aiExternalExecTracked(ctx, docker, journal, host, input)
	if e != nil {
		return nil, e
	}
	raw, e := observed.actualOutput(ctx)
	if e != nil {
		return nil, e
	}
	if e = journal.Close(); e != nil {
		return nil, ErrAIExternalExecJournal
	}
	return raw, nil
}

// Read-only diagnostic input must reference the ORIGINAL durable packet/run/
// deadline. A new attempt must not rehash a new packet or extend this value.
// None of these fields can start an exec or construct execution qualification.
type AIExternalExecReconcileInput struct {
	OperationDirectory                     string
	Mode                                   string
	SourceSHA, OperationID, OriginalRunID  string
	RuntimeSourceSHA, ImageID, ContainerID string
	OriginalInputSHA256                    string
	OriginalDeadlineUnixNano               int64
	SudoDocker                             bool
}

func (AIExternalExecReconcileInput) String() string {
	return "private original external exec reconciliation inputs; read-only"
}
func (v AIExternalExecReconcileInput) GoString() string           { return v.String() }
func (AIExternalExecReconcileInput) MarshalJSON() ([]byte, error) { return nil, ErrSourceSerialization }

type AIExternalExecLifecycleSummary struct {
	Scope                        string `json:"scope"`
	Mode                         string `json:"mode"`
	ExecIDSHA256                 string `json:"exec_id_sha256"`
	OriginalDeadlineUnixNano     int64  `json:"original_deadline_unix_nano"`
	Running                      bool   `json:"running"`
	TerminalObserved             bool   `json:"terminal_observed"`
	ExitCode                     *int   `json:"exit_code"`
	PermissionToRetry            bool   `json:"permission_to_retry"`
	PermissionToReleaseExclusion bool   `json:"permission_to_release_exclusion"`
	CASAuthority                 bool   `json:"cas_authority"`
	DropReady                    bool   `json:"drop_ready"`
}

// ObserveAIExternalExecLifecycle only GETs version and the ORIGINAL exec ID.
// Even a terminal zero exit is diagnostic only. It does not recover lost output,
// start/attach, approve source/business closure, settle UNKNOWN commits, release
// the host lease, or allow DDL. The host supplies an explicit <=15s read context.
func ObserveAIExternalExecLifecycle(ctx context.Context, in AIExternalExecReconcileInput) (AIExternalExecLifecycleSummary, error) {
	var empty AIExternalExecLifecycleSummary
	mode := aiExternalExecMode(in.Mode)
	path, e := aiExternalExecModePath(in.OperationDirectory, in.OperationID, mode)
	if e != nil {
		return empty, e
	}
	binding := aiExecBinding{SourceSHA: in.SourceSHA, OperationID: in.OperationID, RunID: in.OriginalRunID, RuntimeSourceSHA: in.RuntimeSourceSHA, ImageID: in.ImageID, ContainerID: in.ContainerID, PythonSHA256: aiExternalHostSHA, InputSHA256: in.OriginalInputSHA256, DeadlineUnixNano: in.OriginalDeadlineUnixNano}
	if !binding.valid() {
		return empty, ErrAIExternalExecJournal
	}
	journal, e := aiExecOpenJournal(path, binding, false)
	if e != nil {
		return empty, e
	}
	defer func() { _ = journal.Close() }()
	docker, e := aiExternalDocker(in.SudoDocker)
	if e != nil {
		return empty, e
	}
	observed, e := aiExternalExecReconcile(ctx, docker, journal)
	if e != nil {
		return empty, e
	}
	if observed == nil || observed.self != observed || observed.binding != binding || observed.complete || len(observed.output) != 0 || !bytes.Equal([]byte(observed.execID), []byte(journal.last.ExecID)) {
		return empty, ErrAIExternalExecUnknown
	}
	result := AIExternalExecLifecycleSummary{Scope: "actual-original-exec-lifecycle-only-no-release-or-retirement-authority", Mode: in.Mode, ExecIDSHA256: sourceSHA([]byte(observed.execID)), OriginalDeadlineUnixNano: binding.DeadlineUnixNano, Running: observed.running, TerminalObserved: !observed.running && observed.exitCode != nil}
	if observed.exitCode != nil {
		code := *observed.exitCode
		result.ExitCode = &code
	}
	if journal.Close() != nil {
		return empty, ErrAIExternalExecJournal
	}
	return result, nil
}
