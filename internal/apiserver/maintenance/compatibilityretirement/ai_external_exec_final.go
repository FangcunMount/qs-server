package retirement

import (
	"context"
	"time"
)

// This is an execution-order check, not adoption of prior output or a Q.
// Missing/conflicting/unknown original chains fail before the new final create.
func aiExecOpenFinalPredecessor(ctx context.Context, docker *aiExternalDockerExecutor, directory string, owner HistoricalCoordinatorBinding, runtime, image, cid string, uid *uint32) (*aiExecJournal, error) {
	path, e := aiExternalExecModePathAs(directory, owner.OperationID, aiExternalVerifyMode, uid)
	if e != nil {
		return nil, e
	}
	j, e := aiExecQuiescenceJournal(path, AIExternalExecQuiescenceInput{SourceUID: uid, OperationDirectory: directory, SourceSHA: owner.SourceSHA, OperationID: owner.OperationID, RuntimeSourceSHA: runtime, ImageID: image, ContainerID: cid})
	if e != nil {
		return nil, e
	}
	q, c := context.WithTimeout(ctx, 15*time.Second)
	defer c()
	if e = aiExecRequireFinalPredecessor(q, &aiExecDockerProtocol{docker: docker}, j); e != nil {
		_ = j.Close()
		return nil, e
	}
	return j, nil // Keep original FD+flock until final native create/attach/inspect.
}

// The private protocol seam tests ordering only. Public production supplies
// aiExecDockerProtocol and GETs the exact original native Engine exec ID.
func aiExecRequireFinalPredecessor(ctx context.Context, p aiExecProtocol, j *aiExecJournal) error {
	if ctx == nil || ctx.Err() != nil || p == nil || j == nil {
		return ErrAIExternalExecUnknown
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.checkLocked() != nil {
		return ErrAIExternalExecJournal
	}
	records, e := aiExecDecodeJournal(j.raw, j.binding)
	if e != nil {
		return e
	}
	for _, r := range records {
		if r.Stage == "unknown" {
			return ErrAIExternalExecUnknown
		}
	}
	r := j.last
	if r.Stage != "observed" || !r.AttachComplete || r.OutputBytes == 0 || !evidenceHash(r.OutputSHA256) || !evidenceHash(r.ExecID) || r.Running == nil || *r.Running || r.ExitCode == nil || *r.ExitCode != 0 {
		return ErrAIExternalExecUnknown
	}
	version, e := p.version(ctx)
	if e != nil || version != r.EngineVersionSHA256 {
		return ErrAIExternalExecUnknown
	}
	actual, e := p.inspect(ctx, r.ExecID, j.binding.ContainerID, j.binding.PythonSHA256)
	if e != nil || actual.Running || actual.ExitCode == nil || *actual.ExitCode != 0 || j.checkLocked() != nil {
		return ErrAIExternalExecUnknown
	}
	return nil
}
