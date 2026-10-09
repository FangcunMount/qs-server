package retirement

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
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
}

func (AIExternalExecQuiescenceInput) String() string {
	return "private original external exec quiescence inputs; no authority"
}
func (v AIExternalExecQuiescenceInput) GoString() string { return v.String() }
func (AIExternalExecQuiescenceInput) MarshalJSON() ([]byte, error) {
	return nil, ErrSourceSerialization
}

// Original binding comes from the protected immutable journal, not a caller's
// terminal/complete claim. Its exact chain is revalidated by aiExecOpenJournal.
func aiExecQuiescenceJournal(path string, in AIExternalExecQuiescenceInput) (*aiExecJournal, error) {
	if aiExecParent(path) != nil {
		return nil, ErrAIExternalExecJournal
	}
	fd, e := syscall.Open(path, syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if e != nil {
		return nil, ErrAIExternalExecJournal
	}
	f := os.NewFile(uintptr(fd), path)
	before, e := aiExecFileCheck(f, path, nil)
	if e != nil {
		_ = f.Close()
		return nil, e
	}
	raw, re := io.ReadAll(io.LimitReader(f, aiExecJournalLimit+1))
	after, ae := aiExecFileCheck(f, path, before)
	ce := f.Close()
	if re != nil || ae != nil || ce != nil || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) || int64(len(raw)) != after.Size() || len(raw) == 0 || len(raw) > aiExecJournalLimit {
		return nil, ErrAIExternalExecJournal
	}
	end := bytes.IndexByte(raw, '\n')
	if end <= 0 {
		return nil, ErrAIExternalExecJournal
	}
	var first aiExecRecord
	d := json.NewDecoder(bytes.NewReader(raw[:end]))
	d.DisallowUnknownFields()
	if strictJSON(raw[:end]) != nil || d.Decode(&first) != nil || d.Decode(&struct{}{}) != io.EOF {
		return nil, ErrAIExternalExecJournal
	}
	b := first.Binding
	if !b.valid() || b.SourceSHA != in.SourceSHA || b.OperationID != in.OperationID || b.RuntimeSourceSHA != in.RuntimeSourceSHA || b.ImageID != in.ImageID || b.ContainerID != in.ContainerID {
		return nil, ErrAIExternalExecJournal
	}
	if _, e = aiExecDecodeJournal(raw, b); e != nil {
		return nil, e
	}
	j, e := aiExecOpenJournal(path, b, false)
	if e != nil {
		return nil, e
	}
	if !bytes.Equal(j.raw, raw) {
		_ = j.Close()
		return nil, ErrAIExternalExecJournal
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
		path, e := aiExternalExecModePath(in.OperationDirectory, in.OperationID, mode)
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
