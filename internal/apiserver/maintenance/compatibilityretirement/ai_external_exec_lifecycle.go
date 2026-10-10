package retirement

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// These are lifecycle observations, not business qualification, a writer fence,
// permission to retry an exec, or permission to release the operation lease.
const aiExecAPIVersion = "1.44"
const aiExecJournalLimit = 64 << 10
const aiExecReplyLimit = 64 << 10
const aiExecHeaderLimit = 32 << 10
const aiExecStderrLimit = 64 << 10
const aiExecFrameLimit = 65536

var ErrAIExternalExecUnknown SourceError = "ai_external_exec_lifecycle_unknown"
var ErrAIExternalExecJournal SourceError = "ai_external_exec_durable_journal_rejected"

type aiExecBinding struct {
	SourceSHA        string `json:"source_sha"`
	OperationID      string `json:"operation_id"`
	RunID            string `json:"run_id"`
	RuntimeSourceSHA string `json:"runtime_source_sha"`
	ImageID          string `json:"image_id"`
	ContainerID      string `json:"container_id"`
	// Empty on all original running-runtime journals. A stopped-final producer
	// retains the business CID and records its separate native execution owner.
	ExecutionContainerID string `json:"execution_container_id,omitempty"`
	StoppedLeaseSHA256   string `json:"stopped_lease_sha256,omitempty"`
	PythonSHA256         string `json:"python_sha256"`
	InputSHA256          string `json:"input_sha256"`
	// The producer inherits the original epoch deadline; neither a new attempt
	// nor reconciliation replaces it with now + another duration.
	DeadlineUnixNano int64 `json:"deadline_unix_nano"`
}

func (b aiExecBinding) valid() bool {
	parts := strings.Split(b.OperationID, "-")
	python := b.PythonSHA256 == aiExternalHostSHA && b.ExecutionContainerID == "" && b.StoppedLeaseSHA256 == ""
	if b.ExecutionContainerID != "" || b.StoppedLeaseSHA256 != "" {
		python = evidenceHash(b.ExecutionContainerID) && b.ExecutionContainerID != b.ContainerID && evidenceHash(b.StoppedLeaseSHA256) && b.PythonSHA256 == sourceSHA([]byte(aiStoppedCarrierHost))
	}
	return aiOriginalSourceSHA(b.SourceSHA) && len(parts) == 2 && aiExternalRunID(parts[0]) && aiExternalRunID(parts[1]) && aiExternalRunID(b.RunID) && aiOriginalSourceSHA(b.RuntimeSourceSHA) && aiExternalImageID(b.ImageID) && evidenceHash(b.ContainerID) && python && evidenceHash(b.InputSHA256) && b.DeadlineUnixNano > 0
}
func (b aiExecBinding) executionCID() string {
	if b.ExecutionContainerID != "" {
		return b.ExecutionContainerID
	}
	return b.ContainerID
}
func (b aiExecBinding) scope(ctx context.Context) (context.Context, context.CancelFunc, error) {
	if ctx == nil || !b.valid() {
		return nil, nil, ErrAIExternalExecUnknown
	}
	deadline, ok := ctx.Deadline()
	original := time.Unix(0, b.DeadlineUnixNano)
	if !ok || ctx.Err() != nil || !time.Now().Before(original) || deadline.After(original) {
		return nil, nil, ErrAIExternalExecUnknown
	}
	child, cancel := context.WithDeadline(ctx, original)
	return child, cancel, nil
}

type aiExecRecord struct {
	Protocol            string        `json:"protocol"`
	Sequence            uint64        `json:"sequence"`
	PreviousSHA256      string        `json:"previous_sha256"`
	Binding             aiExecBinding `json:"binding"`
	Stage               string        `json:"stage"`
	ExecID              string        `json:"exec_id"`
	EngineVersionSHA256 string        `json:"engine_version_sha256"`
	Running             *bool         `json:"running"`
	ExitCode            *int          `json:"exit_code"`
	AttachComplete      bool          `json:"attach_complete"`
	OutputBytes         uint64        `json:"output_bytes"`
	OutputSHA256        string        `json:"output_sha256"`
}

// Only the actual producer below can create a startable journal. A reopened
// journal is inspect-only, even if it says created/not_started. The host must
// use one fixed journal path for this operation across attempts and retain its
// original exclusion lease on UNKNOWN. This lock covers this file only.
type aiExecJournal struct {
	self       *aiExecJournal
	mu         sync.Mutex
	file       *os.File
	path       string
	initial    os.FileInfo
	lastStat   os.FileInfo
	parentStat os.FileInfo
	ownerUID   uint32
	binding    aiExecBinding
	last       aiExecRecord
	raw        []byte
	startable  bool
	closed     bool
}

func (*aiExecJournal) String() string {
	return "private Docker exec lifecycle journal; no execution or retirement authority"
}
func (j *aiExecJournal) GoString() string           { return j.String() }
func (*aiExecJournal) MarshalJSON() ([]byte, error) { return nil, ErrSourceSerialization }
func (*aiExecJournal) MarshalBSON() ([]byte, error) { return nil, ErrSourceSerialization }

func aiExecParent(path string) error {
	return aiExecParentAs(path, uint32(os.Geteuid()))
}
func aiExecParentAs(path string, uid uint32) error {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return ErrAIExternalExecJournal
	}
	// The caller supplies its already-controlled operation directory. Check its
	// immediate directory, not a new root authority or a guessed host path.
	actual, e := filepath.EvalSymlinks(filepath.Dir(path))
	if e != nil || actual != filepath.Dir(path) {
		return ErrAIExternalExecJournal
	}
	dir, e := os.Lstat(filepath.Dir(path))
	if e != nil || !dir.IsDir() || dir.Mode()&os.ModeSymlink != 0 || dir.Mode().Perm() != 0700 {
		return ErrAIExternalExecJournal
	}
	u, ok := dir.Sys().(*syscall.Stat_t)
	if !ok || u.Uid != uid {
		return ErrAIExternalExecJournal
	}
	return nil
}
func aiExecFileCheck(file *os.File, path string, initial os.FileInfo) (os.FileInfo, error) {
	return aiExecFileCheckAs(file, path, initial, uint32(os.Geteuid()))
}
func aiExecFileCheckAs(file *os.File, path string, initial os.FileInfo, uid uint32) (os.FileInfo, error) {
	if file == nil {
		return nil, ErrAIExternalExecJournal
	}
	s, e := file.Stat()
	if e != nil || !s.Mode().IsRegular() || s.Mode().Perm() != 0600 || s.Size() < 0 || s.Size() > aiExecJournalLimit {
		return nil, ErrAIExternalExecJournal
	}
	u, ok := s.Sys().(*syscall.Stat_t)
	if !ok || u.Uid != uid || u.Nlink != 1 {
		return nil, ErrAIExternalExecJournal
	}
	named, e := os.Lstat(path)
	if e != nil || !os.SameFile(s, named) || named.Mode() != s.Mode() {
		return nil, ErrAIExternalExecJournal
	}
	nu, ok := named.Sys().(*syscall.Stat_t)
	if !ok || nu.Uid != u.Uid || nu.Gid != u.Gid || nu.Nlink != 1 {
		return nil, ErrAIExternalExecJournal
	}
	if initial != nil {
		previous, ok := initial.Sys().(*syscall.Stat_t)
		if !ok || !os.SameFile(initial, s) || initial.Mode() != s.Mode() || previous.Uid != u.Uid || previous.Gid != u.Gid || previous.Nlink != 1 {
			return nil, ErrAIExternalExecJournal
		}
	}
	return s, nil
}
func aiExecOpenJournal(path string, b aiExecBinding, create bool) (*aiExecJournal, error) {
	if !b.valid() || aiExecParent(path) != nil {
		return nil, ErrAIExternalExecJournal
	}
	flags := syscall.O_RDWR | syscall.O_NOFOLLOW | syscall.O_NONBLOCK
	if create {
		flags |= syscall.O_CREAT | syscall.O_EXCL
	}
	fd, e := syscall.Open(path, flags, 0600)
	if e != nil {
		return nil, ErrAIExternalExecJournal
	}
	file := os.NewFile(uintptr(fd), path)
	reject := func() (*aiExecJournal, error) { _ = file.Close(); return nil, ErrAIExternalExecJournal } // Rejection cannot grant any capability.
	if syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB) != nil {
		return reject()
	}
	s, e := aiExecFileCheck(file, path, nil)
	if e != nil || create && s.Size() != 0 {
		return reject()
	}
	raw := make([]byte, s.Size())
	if len(raw) > 0 {
		if _, e = file.ReadAt(raw, 0); e != nil {
			return reject()
		}
	}
	parentStat, e := os.Lstat(filepath.Dir(path))
	if e != nil {
		return reject()
	}
	j := &aiExecJournal{file: file, path: path, initial: s, lastStat: s, parentStat: parentStat, ownerUID: uint32(os.Geteuid()), binding: b, raw: raw, startable: create}
	j.self = j
	if !create {
		records, e := aiExecDecodeJournal(raw, b)
		if e != nil {
			return reject()
		}
		j.last = records[len(records)-1]
	} else {
		if file.Sync() != nil {
			return reject()
		}
		parentFD, e := syscall.Open(filepath.Dir(path), syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
		if e != nil {
			return reject()
		}
		parent := os.NewFile(uintptr(parentFD), "private_operation_directory")
		syncErr := parent.Sync()
		closeErr := parent.Close()
		if syncErr != nil || closeErr != nil {
			return reject()
		}
	}
	if j.checkLocked() != nil {
		return reject()
	}
	return j, nil
}
func (j *aiExecJournal) Close() error {
	if j == nil || j.self == nil {
		return nil
	}
	if j.self != j {
		return ErrAIExternalExecJournal
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.closed {
		return nil
	}
	j.closed = true
	// Only this constructor's owned journal fd; never a borrowed connection or stdin.
	return j.file.Close()
}
func (j *aiExecJournal) checkLocked() error {
	if j == nil || j.self != j || j.closed || aiExecParentAs(j.path, j.ownerUID) != nil {
		return ErrAIExternalExecJournal
	}
	parent, e := os.Lstat(filepath.Dir(j.path))
	if e != nil || !os.SameFile(parent, j.parentStat) {
		return ErrAIExternalExecJournal
	}
	s, e := aiExecFileCheckAs(j.file, j.path, j.initial, j.ownerUID)
	if e != nil || s.Size() != int64(len(j.raw)) || j.lastStat == nil || !s.ModTime().Equal(j.lastStat.ModTime()) {
		return ErrAIExternalExecJournal
	}
	raw := make([]byte, len(j.raw))
	if len(raw) > 0 {
		if _, e = j.file.ReadAt(raw, 0); e != nil {
			return ErrAIExternalExecJournal
		}
	}
	if !bytes.Equal(raw, j.raw) {
		return ErrAIExternalExecJournal
	}
	repeated, e := aiExecFileCheckAs(j.file, j.path, j.initial, j.ownerUID)
	if e != nil || !s.ModTime().Equal(repeated.ModTime()) || s.Size() != repeated.Size() {
		return ErrAIExternalExecJournal
	}
	return nil
}
func (j *aiExecJournal) appendLocked(r aiExecRecord) error {
	if j.checkLocked() != nil {
		return ErrAIExternalExecJournal
	}
	r.Protocol = "qs-ai-exec-lifecycle/v1"
	r.Binding = j.binding
	r.Sequence = j.last.Sequence + 1
	if len(j.raw) != 0 {
		r.PreviousSHA256 = sourceSHA(j.raw)
	}
	raw, e := json.Marshal(r)
	if e != nil || len(raw)+1 > aiExecJournalLimit-len(j.raw) {
		return ErrAIExternalExecJournal
	}
	next := append(append([]byte(nil), j.raw...), append(raw, '\n')...)
	if _, e = aiExecDecodeJournal(next, j.binding); e != nil {
		return e
	}
	n, e := j.file.WriteAt(append(raw, '\n'), int64(len(j.raw)))
	if e != nil || n != len(raw)+1 || j.file.Sync() != nil {
		return ErrAIExternalExecJournal
	}
	j.raw = next
	j.last = r
	updated, e := aiExecFileCheckAs(j.file, j.path, j.initial, j.ownerUID)
	if e != nil || updated.Size() != int64(len(next)) {
		return ErrAIExternalExecJournal
	}
	j.lastStat = updated
	return j.checkLocked()
}
func aiExecDecodeJournal(raw []byte, b aiExecBinding) ([]aiExecRecord, error) {
	if len(raw) == 0 || len(raw) > aiExecJournalLimit || raw[len(raw)-1] != '\n' {
		return nil, ErrAIExternalExecJournal
	}
	lines := bytes.Split(raw[:len(raw)-1], []byte{'\n'})
	if len(lines) > 64 {
		return nil, ErrAIExternalExecJournal
	}
	records := make([]aiExecRecord, 0, len(lines))
	offset := 0
	var id, version string
	started, attached := false, false
	var outputBytes uint64
	outputSHA := ""
	for i, line := range lines {
		var r aiExecRecord
		var fields map[string]json.RawMessage
		required := []string{"protocol", "sequence", "previous_sha256", "binding", "stage", "exec_id", "engine_version_sha256", "running", "exit_code", "attach_complete", "output_bytes", "output_sha256"}
		if json.Unmarshal(line, &fields) != nil || len(fields) != len(required) {
			return nil, ErrAIExternalExecJournal
		}
		for _, name := range required {
			value, ok := fields[name]
			if !ok || name != "running" && name != "exit_code" && bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
				return nil, ErrAIExternalExecJournal
			}
		}
		d := json.NewDecoder(bytes.NewReader(line))
		d.DisallowUnknownFields()
		if strictJSON(line) != nil || d.Decode(&r) != nil || d.Decode(&struct{}{}) != io.EOF || r.Protocol != "qs-ai-exec-lifecycle/v1" || r.Sequence != uint64(i+1) || r.Binding != b || i == 0 && r.PreviousSHA256 != "" || i > 0 && r.PreviousSHA256 != sourceSHA(raw[:offset]) {
			return nil, ErrAIExternalExecJournal
		}
		if i == 0 {
			if r.Stage != "create_intent" || r.ExecID != "" || !evidenceHash(r.EngineVersionSHA256) {
				return nil, ErrAIExternalExecJournal
			}
			version = r.EngineVersionSHA256
		}
		if r.EngineVersionSHA256 != version {
			return nil, ErrAIExternalExecJournal
		}
		switch r.Stage {
		case "create_intent":
			if i != 0 {
				return nil, ErrAIExternalExecJournal
			}
		case "created":
			if i != 1 || id != "" || !evidenceHash(r.ExecID) {
				return nil, ErrAIExternalExecJournal
			}
			id = r.ExecID
		case "start_intent":
			if i != 2 || id == "" || started {
				return nil, ErrAIExternalExecJournal
			}
			started = true
		case "attached":
			if i != 3 || !started || attached || !r.AttachComplete || r.OutputBytes == 0 || r.OutputBytes > aiExternalResultLimit || !evidenceHash(r.OutputSHA256) {
				return nil, ErrAIExternalExecJournal
			}
			attached = true
			outputBytes = r.OutputBytes
			outputSHA = r.OutputSHA256
		case "unknown":
		case "observed":
			if id == "" || r.Running == nil || !*r.Running && r.ExitCode != nil && (*r.ExitCode < 0 || *r.ExitCode > 255) || *r.Running && r.ExitCode != nil {
				return nil, ErrAIExternalExecJournal
			}
		default:
			return nil, ErrAIExternalExecJournal
		}
		if r.ExecID != id || r.AttachComplete != attached || r.OutputBytes != outputBytes || r.OutputSHA256 != outputSHA {
			return nil, ErrAIExternalExecJournal
		}
		if r.Stage != "observed" && (r.Running != nil || r.ExitCode != nil) {
			return nil, ErrAIExternalExecJournal
		}
		records = append(records, r)
		offset += len(line) + 1
	}
	return records, nil
}

// This private interface is for protocol tests only. Production constructs it
// solely from the existing fixed, ancestor-checked Docker executor below.
type aiExecProtocol interface {
	version(context.Context) (string, error)
	create(context.Context, string, []byte) (string, error)
	attach(context.Context, string, []byte) ([]byte, error)
	inspect(context.Context, string, string, string) (aiExecInspect, error)
}
type aiExecInspect struct {
	Running  bool
	ExitCode *int
}
type aiExternalExecObservation struct {
	self     *aiExternalExecObservation
	journal  *aiExecJournal
	binding  aiExecBinding
	execID   string
	output   []byte
	running  bool
	exitCode *int
	complete bool
}

func (*aiExternalExecObservation) MarshalJSON() ([]byte, error) { return nil, ErrSourceSerialization }
func (v *aiExternalExecObservation) actualOutput(ctx context.Context) ([]byte, error) {
	if v == nil || v.self != v || v.journal == nil || v.journal.self != v.journal || !v.complete || v.running || v.exitCode == nil || *v.exitCode != 0 || len(v.output) == 0 {
		return nil, ErrAIExternalExecUnknown
	}
	j := v.journal
	j.mu.Lock()
	defer j.mu.Unlock()
	scope, cancel, e := v.binding.scope(ctx)
	if e != nil {
		return nil, e
	}
	defer cancel()
	if scope.Err() != nil || j.checkLocked() != nil || j.binding != v.binding || j.last.Stage != "observed" || j.last.ExecID != v.execID || !j.last.AttachComplete || j.last.Running == nil || *j.last.Running || j.last.ExitCode == nil || *j.last.ExitCode != 0 || j.last.OutputSHA256 != sourceSHA(v.output) || j.last.OutputBytes != uint64(len(v.output)) {
		return nil, ErrAIExternalExecUnknown
	}
	return append([]byte(nil), v.output...), nil
}
func (*aiExternalExecObservation) String() string {
	return "actual Docker exec lifecycle observation; no business, exclusion-release, CAS or DROP qualification"
}
func (v *aiExternalExecObservation) GoString() string           { return v.String() }
func (*aiExternalExecObservation) MarshalBSON() ([]byte, error) { return nil, ErrSourceSerialization }
func (*aiExternalExecObservation) UnmarshalJSON([]byte) error   { return ErrSourceSerialization }
func (*aiExternalExecObservation) UnmarshalBSON([]byte) error   { return ErrSourceSerialization }

func aiExternalExecTracked(ctx context.Context, d *aiExternalDockerExecutor, j *aiExecJournal, host, input []byte) (*aiExternalExecObservation, error) {
	if d == nil {
		return nil, ErrAIExternalExecUnknown
	}
	return aiExecProduce(ctx, &aiExecDockerProtocol{docker: d}, j, host, input)
}
func aiExecProduce(ctx context.Context, p aiExecProtocol, j *aiExecJournal, host, input []byte) (*aiExternalExecObservation, error) {
	if j == nil || j.self != j || p == nil {
		aiExternalExecutionFailure("exec_owner", ErrAIExternalExecUnknown)
		return nil, ErrAIExternalExecUnknown
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.checkLocked() != nil || !j.startable || len(j.raw) != 0 || sourceSHA(host) != j.binding.PythonSHA256 || sourceSHA(input) != j.binding.InputSHA256 || len(input) == 0 || len(input) > aiExternalInputLimit {
		aiExternalExecutionFailure("exec_journal", ErrAIExternalExecJournal)
		return nil, ErrAIExternalExecJournal
	}
	work, cancel, e := j.binding.scope(ctx)
	if e != nil {
		aiExternalExecutionFailure("exec_scope", e)
		return nil, e
	}
	defer cancel()
	version, e := p.version(work)
	if e != nil || !evidenceHash(version) {
		aiExternalExecutionFailure("exec_version", e)
		return nil, ErrAIExternalExecUnknown
	}
	// The on-disk create intent is durable BEFORE the only create attempt.
	r := aiExecRecord{Stage: "create_intent", EngineVersionSHA256: version}
	if e = j.appendLocked(r); e != nil {
		aiExternalExecutionFailure("exec_create_intent", e)
		return nil, e
	}
	j.startable = false
	markUnknown := func() (*aiExternalExecObservation, error) {
		r.Stage = "unknown"
		r.Running = nil
		r.ExitCode = nil
		if j.appendLocked(r) != nil {
			aiExternalExecutionFailure("exec_unknown_journal", ErrAIExternalExecJournal)
			return nil, ErrAIExternalExecJournal
		}
		return nil, ErrAIExternalExecUnknown
	}
	id, createErr := p.create(work, j.binding.executionCID(), host)
	if !evidenceHash(id) {
		aiExternalExecutionFailure("exec_create", createErr)
		return markUnknown()
	}
	r.Stage = "created"
	r.ExecID = id
	if e = j.appendLocked(r); e != nil {
		aiExternalExecutionFailure("exec_created_journal", e)
		return nil, e
	}
	if createErr != nil {
		aiExternalExecutionFailure("exec_create_transport", createErr)
		return markUnknown()
	}
	original, e := p.inspect(work, id, j.binding.executionCID(), j.binding.PythonSHA256)
	if e != nil || original.Running || original.ExitCode != nil {
		aiExternalExecutionFailure("exec_inspect_before", e)
		return markUnknown()
	}
	r.Stage = "start_intent"
	if e = j.appendLocked(r); e != nil {
		aiExternalExecutionFailure("exec_start_intent", e)
		return nil, e
	}
	output, attachErr := p.attach(work, id, input)
	if attachErr == nil && len(output) > 0 && len(output) <= aiExternalResultLimit && work.Err() == nil {
		r.Stage = "attached"
		r.AttachComplete = true
		r.OutputBytes = uint64(len(output))
		r.OutputSHA256 = sourceSHA(output)
		if e = j.appendLocked(r); e != nil {
			aiExternalExecutionFailure("exec_attached_journal", e)
			return nil, e
		}
	} else {
		aiExternalExecutionFailure("exec_attach", attachErr)
		r.Stage = "unknown"
		if e = j.appendLocked(r); e != nil {
			aiExternalExecutionFailure("exec_unknown_journal", e)
			return nil, e
		}
		output = nil
	}
	// Never infer process exit from local Wait/EOF/cancellation. Only inspect
	// the original Engine exec ID, with the original absolute budget still set.
	observed, e := p.inspect(work, id, j.binding.executionCID(), j.binding.PythonSHA256)
	if e != nil {
		aiExternalExecutionFailure("exec_inspect_after", e)
		return markUnknown()
	}
	r.Stage = "observed"
	r.Running = &observed.Running
	r.ExitCode = observed.ExitCode
	if e = j.appendLocked(r); e != nil {
		aiExternalExecutionFailure("exec_observed_journal", e)
		return nil, e
	}
	v := &aiExternalExecObservation{journal: j, binding: j.binding, execID: id, running: observed.Running, exitCode: observed.ExitCode, output: output, complete: r.AttachComplete}
	v.self = v
	if attachErr != nil || work.Err() != nil || !v.complete || v.running || v.exitCode == nil || *v.exitCode != 0 {
		aiExternalExecutionFailure("exec_terminal", ErrAIExternalExecUnknown)
		return v, ErrAIExternalExecUnknown
	}
	return v, nil
}

// Reconcile performs only version/inspect reads for the durable original ID.
// It cannot start, attach, recreate, recover lost output or mint qualification.
// A missing exec (including Engine garbage collection) stays UNKNOWN. The
// original execution deadline remains recorded even after budget expiration.
func aiExternalExecReconcile(ctx context.Context, d *aiExternalDockerExecutor, j *aiExecJournal) (*aiExternalExecObservation, error) {
	if d == nil || j == nil || j.self != j {
		return nil, ErrAIExternalExecUnknown
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.checkLocked() != nil || j.last.ExecID == "" || !evidenceHash(j.last.ExecID) {
		return nil, ErrAIExternalExecUnknown
	}
	// A short read-only diagnostic context may observe the original ID after
	// its execution budget expires. It does not alter Binding.DeadlineUnixNano
	// or allow start/create, qualification, lease release, CAS or DDL.
	if ctx == nil || ctx.Err() != nil {
		return nil, ErrAIExternalExecUnknown
	}
	diagnosticDeadline, ok := ctx.Deadline()
	if !ok || !diagnosticDeadline.After(time.Now()) || diagnosticDeadline.After(time.Now().Add(15*time.Second)) {
		return nil, ErrAIExternalExecUnknown
	}
	p := &aiExecDockerProtocol{docker: d}
	return aiExecReconcileProtocolLocked(ctx, p, j)
}
func aiExecReconcileProtocolLocked(work context.Context, p aiExecProtocol, j *aiExecJournal) (*aiExternalExecObservation, error) {
	version, e := p.version(work)
	if e != nil || version != j.last.EngineVersionSHA256 {
		return nil, ErrAIExternalExecUnknown
	}
	observed, e := p.inspect(work, j.last.ExecID, j.binding.executionCID(), j.binding.PythonSHA256)
	if e != nil {
		return nil, ErrAIExternalExecUnknown
	}
	r := j.last
	r.Stage = "observed"
	r.Running = &observed.Running
	r.ExitCode = observed.ExitCode
	if e = j.appendLocked(r); e != nil {
		return nil, e
	}
	v := &aiExternalExecObservation{journal: j, binding: j.binding, execID: r.ExecID, running: observed.Running, exitCode: observed.ExitCode}
	v.self = v
	// Even a zero exit is only a lifecycle observation: output is not restored.
	return v, nil
}

type aiExecDockerProtocol struct{ docker *aiExternalDockerExecutor }
type aiExecDial struct {
	cmd    *exec.Cmd
	in     io.WriteCloser
	out    io.ReadCloser
	waited bool
}

func (d *aiExternalDockerExecutor) execDial(ctx context.Context) (*aiExecDial, error) {
	if ctx == nil || ctx.Err() != nil || d == nil {
		return nil, ErrAIExternalExecUnknown
	}
	// Exactly the executor/context used by the runtime preflight. No -H,
	// DOCKER_HOST, context import, endpoint guess or automatic sudo fallback.
	cmd := exec.CommandContext(ctx, d.executable, append(append([]string(nil), d.prefix...), "system", "dial-stdio")...)
	cmd.Env = []string{"PATH=/usr/bin:/bin:/usr/local/bin", "HOME=/", "LC_ALL=C"}
	cmd.Stderr = io.Discard
	cmd.WaitDelay = 2 * time.Second
	in, e := cmd.StdinPipe()
	if e != nil {
		return nil, ErrAIExternalExecUnknown
	}
	out, e := cmd.StdoutPipe()
	if e != nil {
		_ = in.Close()
		return nil, ErrAIExternalExecUnknown
	}
	if cmd.Start() != nil {
		_ = in.Close()
		_ = out.Close()
		return nil, ErrAIExternalExecUnknown
	}
	return &aiExecDial{cmd: cmd, in: in, out: out}, nil
}
func (s *aiExecDial) finish(abort bool) error {
	if s == nil || s.waited {
		return ErrAIExternalExecUnknown
	}
	s.waited = true
	_ = s.in.Close()
	if abort {
		_ = s.out.Close()
		if s.cmd.Process != nil {
			_ = s.cmd.Process.Kill()
		}
	}
	e := s.cmd.Wait()
	closeErr := s.out.Close()
	if e != nil || closeErr != nil && !errors.Is(closeErr, os.ErrClosed) {
		return ErrAIExternalExecUnknown
	}
	return nil
}
func aiExecRequest(method, path string, raw []byte, upgrade bool) (*http.Request, error) {
	req, e := http.NewRequest(method, "http://docker"+path, bytes.NewReader(raw))
	if e != nil {
		return nil, ErrAIExternalExecUnknown
	}
	req.Header.Set("Content-Type", "application/json")
	req.Close = !upgrade
	if upgrade {
		req.Header.Set("Connection", "Upgrade")
		req.Header.Set("Upgrade", "tcp")
	}
	return req, nil
}
func (p *aiExecDockerProtocol) ordinary(ctx context.Context, method, path string, raw []byte, status int) ([]byte, error) {
	req, e := aiExecRequest(method, path, raw, false)
	if e != nil {
		aiExternalExecutionFailure("http_request", e)
		return nil, e
	}
	s, e := p.docker.execDial(ctx)
	if e != nil {
		aiExternalExecutionFailure("http_dial", e)
		return nil, e
	}
	done := false
	defer func() {
		if !done {
			_ = s.finish(true)
		}
	}()
	// Premature dial-stdio stdin EOF can cancel the Docker HTTP handler; close after the reply.
	if req.Write(s.in) != nil {
		aiExternalExecutionFailure("http_write", ErrAIExternalExecUnknown)
		return nil, ErrAIExternalExecUnknown
	}
	resp, e := http.ReadResponse(bufio.NewReader(&aiExecHeaderReader{reader: io.LimitReader(s.out, aiExecReplyLimit+aiExecHeaderLimit+1)}), req)
	if e != nil {
		aiExternalExecutionFailure("http_header", e)
		return nil, ErrAIExternalExecUnknown
	}
	if resp.StatusCode != status {
		actual, expected := "unknown", "unknown"
		if resp.StatusCode >= 100 && resp.StatusCode <= 599 {
			actual = strconv.Itoa(resp.StatusCode)
		}
		if status >= 100 && status <= 599 {
			expected = strconv.Itoa(status)
		}
		_, _ = fmt.Fprintln(os.Stderr, "QS_AI_HTTP_STATUS_DIAGNOSTIC actual="+actual+" expected="+expected)
		aiExternalExecutionFailure("http_status", ErrAIExternalExecUnknown)
		_ = resp.Body.Close()
		return nil, ErrAIExternalExecUnknown
	}
	body, e := io.ReadAll(io.LimitReader(resp.Body, aiExecReplyLimit+1))
	closed := resp.Body.Close()
	if e != nil || closed != nil || len(body) > aiExecReplyLimit || ctx.Err() != nil {
		aiExternalExecutionFailure("http_body", e)
		return nil, ErrAIExternalExecUnknown
	}
	if s.finish(false) != nil {
		aiExternalExecutionFailure("http_finish", ErrAIExternalExecUnknown)
		done = true
		return body, ErrAIExternalExecUnknown
	}
	done = true
	return body, nil
}
func (p *aiExecDockerProtocol) version(ctx context.Context) (string, error) {
	raw, e := p.ordinary(ctx, http.MethodGet, "/version", nil, http.StatusOK)
	if e != nil || strictJSON(raw) != nil {
		aiExternalExecutionFailure("version_response", e)
		return "", ErrAIExternalExecUnknown
	}
	var v struct {
		APIVersion string `json:"ApiVersion"`
		Minimum    string `json:"MinAPIVersion"`
		OS         string `json:"Os"`
	}
	if json.Unmarshal(raw, &v) != nil || v.OS != "linux" || !aiExecVersionSupports(v.Minimum, v.APIVersion) {
		aiExternalExecutionFailure("version_contract", ErrAIExternalExecUnknown)
		return "", ErrAIExternalExecUnknown
	}
	return sourceSHA(raw), nil
}
func aiExecVersionSupports(minimum, maximum string) bool {
	parse := func(s string) (int, bool) {
		pieces := strings.Split(s, ".")
		if len(pieces) != 2 || pieces[0] != "1" {
			return 0, false
		}
		n, e := strconv.Atoi(pieces[1])
		return n, e == nil && n >= 0 && strconv.Itoa(n) == pieces[1]
	}
	low, a := parse(minimum)
	high, b := parse(maximum)
	return a && b && low <= 44 && high >= 44 && low <= high
}
func (p *aiExecDockerProtocol) create(ctx context.Context, cid string, host []byte) (string, error) {
	if !evidenceHash(cid) || (sourceSHA(host) != aiExternalHostSHA && sourceSHA(host) != sourceSHA([]byte(aiStoppedCarrierHost))) {
		aiExternalExecutionFailure("create_request", ErrAIExternalExecUnknown)
		return "", ErrAIExternalExecUnknown
	}
	request := struct {
		AttachStdin, AttachStdout, AttachStderr, Tty, Privileged bool
		Cmd                                                      []string
	}{true, true, true, false, false, []string{"/app/.venv/bin/python", "-I", "-B", "-c", string(host)}}
	raw, e := json.Marshal(request)
	if e != nil {
		aiExternalExecutionFailure("create_request", e)
		return "", ErrAIExternalExecUnknown
	}
	result, e := p.ordinary(ctx, http.MethodPost, "/v"+aiExecAPIVersion+"/containers/"+cid+"/exec", raw, http.StatusCreated)
	var v struct {
		ID string `json:"Id"`
	}
	decoder := json.NewDecoder(bytes.NewReader(result))
	decoder.DisallowUnknownFields()
	if strictJSON(result) != nil || decoder.Decode(&v) != nil || decoder.Decode(&struct{}{}) != io.EOF || !evidenceHash(v.ID) {
		aiExternalExecutionFailure("create_response", ErrAIExternalExecUnknown)
		return "", ErrAIExternalExecUnknown
	}
	// A validated Engine response still yields its ID if local transport Wait
	// is unknown. Persist that ID, but never start after this error.
	return v.ID, e
}
func aiExecDecodeInspect(raw []byte, id, cid, pythonSHA string) (aiExecInspect, error) {
	var v struct {
		ID                                string
		ContainerID                       string
		Running                           *bool
		ExitCode                          *int
		OpenStdin, OpenStdout, OpenStderr *bool
		ProcessConfig                     *struct {
			Tty        *bool    `json:"tty"`
			Entrypoint string   `json:"entrypoint"`
			Arguments  []string `json:"arguments"`
			Privileged *bool    `json:"privileged"`
		}
	}
	if strictJSON(raw) != nil || json.Unmarshal(raw, &v) != nil || v.ID != id || v.ContainerID != cid || v.Running == nil || v.OpenStdin == nil || !*v.OpenStdin || v.OpenStdout == nil || !*v.OpenStdout || v.OpenStderr == nil || !*v.OpenStderr || v.ProcessConfig == nil || v.ProcessConfig.Tty == nil || *v.ProcessConfig.Tty || v.ProcessConfig.Privileged == nil || *v.ProcessConfig.Privileged || v.ProcessConfig.Entrypoint != "/app/.venv/bin/python" || len(v.ProcessConfig.Arguments) != 4 || !reflect.DeepEqual(v.ProcessConfig.Arguments[:3], []string{"-I", "-B", "-c"}) || sourceSHA([]byte(v.ProcessConfig.Arguments[3])) != pythonSHA {
		aiExternalExecutionFailure("inspect_decode", ErrAIExternalExecUnknown)
		return aiExecInspect{}, ErrAIExternalExecUnknown
	}
	if *v.Running && v.ExitCode != nil || v.ExitCode != nil && (*v.ExitCode < 0 || *v.ExitCode > 255) {
		aiExternalExecutionFailure("inspect_decode", ErrAIExternalExecUnknown)
		return aiExecInspect{}, ErrAIExternalExecUnknown
	}
	return aiExecInspect{Running: *v.Running, ExitCode: v.ExitCode}, nil
}
func (p *aiExecDockerProtocol) inspect(ctx context.Context, id, cid, pythonSHA string) (aiExecInspect, error) {
	if !evidenceHash(id) || !evidenceHash(cid) || (pythonSHA != aiExternalHostSHA && pythonSHA != sourceSHA([]byte(aiStoppedCarrierHost))) {
		aiExternalExecutionFailure("inspect_input", ErrAIExternalExecUnknown)
		return aiExecInspect{}, ErrAIExternalExecUnknown
	}
	raw, e := p.ordinary(ctx, http.MethodGet, "/v"+aiExecAPIVersion+"/exec/"+id+"/json", nil, http.StatusOK)
	if e != nil {
		aiExternalExecutionFailure("inspect_read", e)
		return aiExecInspect{}, e
	}
	return aiExecDecodeInspect(raw, id, cid, pythonSHA)
}

// Header parsing is bounded separately from the multiplex payload. No raw
// stdout, stderr, HTTP error body, source code or stdin is written to a journal.
type aiExecHeaderReader struct {
	reader   io.Reader
	count    int
	matched  int
	complete bool
}

func (r *aiExecHeaderReader) Read(b []byte) (int, error) {
	if r.complete {
		return r.reader.Read(b)
	}
	if r.count >= aiExecHeaderLimit {
		return 0, ErrAIExternalExecUnknown
	}
	if len(b) == 0 {
		return 0, nil
	}
	n, e := r.reader.Read(b[:1])
	if n == 1 {
		r.count++
		end := []byte("\r\n\r\n")
		switch b[0] {
		case end[r.matched]:
			r.matched++
		case '\r':
			r.matched = 1
		default:
			r.matched = 0
		}
		if r.matched == 4 {
			r.complete = true
		}
	}
	return n, e
}
func aiExecMultiplex(reader io.Reader) ([]byte, error) {
	var output bytes.Buffer
	stderr, totalFrames := 0, 0
	for {
		var header [8]byte
		n, e := io.ReadFull(reader, header[:])
		if e == io.EOF && n == 0 {
			if output.Len() == 0 {
				return nil, ErrAIExternalExecUnknown
			}
			return output.Bytes(), nil
		}
		if e != nil || header[1] != 0 || header[2] != 0 || header[3] != 0 || header[0] != 1 && header[0] != 2 {
			return nil, ErrAIExternalExecUnknown
		}
		totalFrames++
		size := uint64(binary.BigEndian.Uint32(header[4:]))
		if totalFrames > aiExecFrameLimit {
			return nil, ErrAIExternalExecUnknown
		}
		if header[0] == 1 {
			if size > uint64(aiExternalResultLimit-output.Len()) {
				return nil, ErrAIExternalExecUnknown
			}
			if _, e = io.CopyN(&output, reader, int64(size)); e != nil {
				return nil, ErrAIExternalExecUnknown
			}
		} else {
			if size > uint64(aiExecStderrLimit-stderr) {
				return nil, ErrAIExternalExecUnknown
			}
			if _, e = io.CopyN(io.Discard, reader, int64(size)); e != nil {
				return nil, ErrAIExternalExecUnknown
			}
			stderr += int(size)
		}
	}
}
func (p *aiExecDockerProtocol) attach(ctx context.Context, id string, input []byte) ([]byte, error) {
	if !evidenceHash(id) || len(input) == 0 || len(input) > aiExternalInputLimit {
		aiExternalExecutionFailure("attach_input", ErrAIExternalExecUnknown)
		return nil, ErrAIExternalExecUnknown
	}
	req, e := aiExecRequest(http.MethodPost, "/v"+aiExecAPIVersion+"/exec/"+id+"/start", []byte(`{"Detach":false,"Tty":false}`), true)
	if e != nil {
		aiExternalExecutionFailure("attach_request", e)
		return nil, e
	}
	s, e := p.docker.execDial(ctx)
	if e != nil {
		aiExternalExecutionFailure("attach_dial", e)
		return nil, e
	}
	done := false
	defer func() {
		if !done {
			_ = s.finish(true)
		}
	}()
	if req.Write(s.in) != nil {
		aiExternalExecutionFailure("attach_start", ErrAIExternalExecUnknown)
		return nil, ErrAIExternalExecUnknown
	}
	reader := bufio.NewReader(&aiExecHeaderReader{reader: s.out})
	resp, e := http.ReadResponse(reader, req)
	if e != nil || resp.StatusCode != http.StatusSwitchingProtocols || !strings.EqualFold(resp.Header.Get("Upgrade"), "tcp") || !strings.EqualFold(resp.Header.Get("Connection"), "Upgrade") || resp.Header.Get("Content-Type") != "application/vnd.docker.multiplexed-stream" {
		aiExternalExecutionFailure("attach_upgrade", e)
		return nil, ErrAIExternalExecUnknown
	}
	// Write stdin after the real Engine upgrade; EOF half-closes daemon input
	// through dial-stdio's official copier, while output remains attached.
	written := make(chan error, 1)
	go func() {
		writtenBytes, writeErr := s.in.Write(input)
		closeErr := s.in.Close()
		if writtenBytes != len(input) || writeErr != nil || closeErr != nil {
			written <- ErrAIExternalExecUnknown
		} else {
			written <- nil
		}
	}()
	output, readErr := aiExecMultiplex(reader)
	if readErr != nil {
		aiExternalExecutionFailure("attach_read", readErr)
		_ = s.finish(true)
		done = true
		<-written
		return nil, ErrAIExternalExecUnknown
	}
	writeErr := <-written
	finishErr := s.finish(false)
	done = true
	if writeErr != nil || finishErr != nil || ctx.Err() != nil {
		aiExternalExecutionFailure("attach_finish", ErrAIExternalExecUnknown)
		return nil, ErrAIExternalExecUnknown
	}
	return output, nil
}
