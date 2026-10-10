package retirement

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// This checks original exec liveness only. It cannot authorize retry, create,
// recover lost stdout, release a writer fence, import qualifications or run DDL.
// The real host must already hold this operation's original operation.lock.
type AIExternalExecQuiescenceInput struct {
	OperationDirectory                     string
	SourceSHA, OperationID                 string
	RuntimeSourceSHA, ImageID, ContainerID string
	SudoDocker                             bool
	// Set only by the root caller from its authenticated original invocation.
	SourceUID *uint32
}

func (AIExternalExecQuiescenceInput) String() string {
	return "private original external exec quiescence inputs; no authority"
}
func (v AIExternalExecQuiescenceInput) GoString() string { return v.String() }
func (AIExternalExecQuiescenceInput) MarshalJSON() ([]byte, error) {
	return nil, ErrSourceSerialization
}

// Original binding comes from the protected immutable journal, not a caller's
// terminal/complete claim. Keep the actual original read-only FD and lock;
// never copy, change ownership, or reopen that source journal for writing.
func aiExecQuiescenceJournal(path string, in AIExternalExecQuiescenceInput) (*aiExecJournal, error) {
	uid := uint32(os.Geteuid())
	if in.SourceUID != nil {
		uid = *in.SourceUID
	}
	if aiExecParentAs(path, uid) != nil {
		return nil, ErrAIExternalExecJournal
	}
	fd, e := syscall.Open(path, syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if e != nil {
		return nil, ErrAIExternalExecJournal
	}
	f := os.NewFile(uintptr(fd), path)
	reject := func() (*aiExecJournal, error) { _ = f.Close(); return nil, ErrAIExternalExecJournal }
	if syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB) != nil {
		return reject()
	}
	parent, e := os.Lstat(filepath.Dir(path))
	if e != nil {
		return reject()
	}
	before, e := aiExecFileCheckAs(f, path, nil, uid)
	if e != nil {
		_ = f.Close()
		return nil, e
	}
	raw, re := io.ReadAll(io.LimitReader(f, aiExecJournalLimit+1))
	after, ae := aiExecFileCheckAs(f, path, before, uid)
	if re != nil || ae != nil || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) || int64(len(raw)) != after.Size() || len(raw) == 0 || len(raw) > aiExecJournalLimit {
		return reject()
	}
	end := bytes.IndexByte(raw, '\n')
	if end <= 0 {
		return reject()
	}
	var first aiExecRecord
	d := json.NewDecoder(bytes.NewReader(raw[:end]))
	d.DisallowUnknownFields()
	if strictJSON(raw[:end]) != nil || d.Decode(&first) != nil || d.Decode(&struct{}{}) != io.EOF {
		return reject()
	}
	b := first.Binding
	if !b.valid() || b.SourceSHA != in.SourceSHA || b.OperationID != in.OperationID || b.RuntimeSourceSHA != in.RuntimeSourceSHA || b.ImageID != in.ImageID || b.ContainerID != in.ContainerID {
		return reject()
	}
	records, e := aiExecDecodeJournal(raw, b)
	if e != nil {
		_ = f.Close()
		return nil, e
	}
	j := &aiExecJournal{file: f, path: path, initial: before, lastStat: after, parentStat: parent, ownerUID: uid, binding: b, raw: raw, last: records[len(records)-1]}
	j.self = j
	if j.checkLocked() != nil {
		return reject()
	}
	return j, nil
}

// A private seam tests GET-only protocol handling. Public production constructs
// aiExecDockerProtocol itself; callers cannot supply a protocol or terminal DTO.
func aiExecRequireQuiescent(ctx context.Context, p aiExecProtocol, j *aiExecJournal) error {
	if ctx == nil || ctx.Err() != nil || p == nil || j == nil {
		return ErrAIExternalExecUnknown
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.checkLocked() != nil || !evidenceHash(j.last.ExecID) {
		return ErrAIExternalExecUnknown
	}
	version, e := p.version(ctx)
	if e != nil || version != j.last.EngineVersionSHA256 {
		return ErrAIExternalExecUnknown
	}
	actual, e := p.inspect(ctx, j.last.ExecID, j.binding.ContainerID, j.binding.PythonSHA256)
	if e != nil || actual.Running || actual.ExitCode == nil || *actual.ExitCode < 0 || *actual.ExitCode > 255 || j.checkLocked() != nil {
		return ErrAIExternalExecUnknown
	}
	// Even nonzero exit can prove this original process ended. It says nothing
	// about output, bounds approval, command acceptance or execution completion.
	return nil
}

func RequireAIExternalExecQuiescence(ctx context.Context, in AIExternalExecQuiescenceInput) error {
	if ctx == nil || ctx.Err() != nil || !aiOriginalSourceSHA(in.SourceSHA) || !aiOriginalSourceSHA(in.RuntimeSourceSHA) || !aiExternalImageID(in.ImageID) || !evidenceHash(in.ContainerID) {
		return ErrAIExternalExecJournal
	}
	var docker *aiExternalDockerExecutor
	for _, mode := range []aiExternalExecMode{aiExternalBoundsMode, aiExternalVerifyMode, aiExternalFinalVerifyMode} {
		path, e := aiExternalExecModePathAs(in.OperationDirectory, in.OperationID, mode, in.SourceUID)
		if e != nil {
			return e
		}
		if _, e = os.Lstat(path); os.IsNotExist(e) {
			continue
		} else if e != nil {
			return ErrAIExternalExecJournal
		}
		j, e := aiExecQuiescenceJournal(path, in)
		if e != nil {
			return e
		}
		if docker == nil {
			docker, e = aiExternalDocker(in.SudoDocker)
			if e != nil {
				_ = j.Close()
				return e
			}
		}
		// A new bounded GET context only observes the SAME exec ID. Its original
		// journal execution deadline remains unchanged and cannot be reused to start.
		scope, cancel := context.WithTimeout(ctx, 15*time.Second)
		e = aiExecRequireQuiescent(scope, &aiExecDockerProtocol{docker: docker}, j)
		cancel()
		closeErr := j.Close()
		if e != nil {
			return e
		}
		if closeErr != nil {
			return ErrAIExternalExecJournal
		}
	}
	return nil
}
