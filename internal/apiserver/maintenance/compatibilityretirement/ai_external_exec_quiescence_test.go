package retirement

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAIExecQuiescenceActualInspectorOverridesSavedCompletion(t *testing.T) {
	for _, tc := range []struct {
		name       string
		running    bool
		exit       *int
		inspectErr error
		accept     bool
	}{
		{name: "terminal_zero", exit: aiExecQuiescenceCode(0), accept: true},
		{name: "terminal_failure_only_drained", exit: aiExecQuiescenceCode(127), accept: true},
		{name: "saved_terminal_actual_running", running: true},
		{name: "missing_original_exit"},
		{name: "missing_original_exec", inspectErr: errors.New("fixed missing original")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := []byte("quiescence-unit")
			b := aiExecUnitBinding(input)
			j := aiExecUnitJournal(t, b)
			p := &aiExecUnitProtocol{t: t, journal: j, id: strings.Repeat("f", 64), output: []byte("test-only"), after: aiExecInspect{ExitCode: aiExecQuiescenceCode(0)}}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			if _, e := aiExecProduce(ctx, p, j, aiExecUnitHost(t), input); e != nil {
				t.Fatal(e)
			}
			before, e := os.ReadFile(j.path)
			if e != nil {
				t.Fatal(e)
			}
			originalDeadline := j.binding.DeadlineUnixNano
			p.calls = nil
			p.after = aiExecInspect{Running: tc.running, ExitCode: tc.exit}
			p.inspectErr = tc.inspectErr
			e = aiExecRequireQuiescent(ctx, p, j)
			if (e == nil) != tc.accept {
				t.Fatalf("actual GET terminal mismatch: %v", e)
			}
			if len(p.calls) != 2 || p.calls[0] != "version" || p.calls[1] != "inspect" {
				t.Fatal("reconcile created/started another exec")
			}
			after, re := os.ReadFile(j.path)
			if re != nil || string(before) != string(after) || j.binding.DeadlineUnixNano != originalDeadline {
				t.Fatal("GET diagnostic reset original journal/deadline")
			}
			// None of these read-only results can recover attach output or q.
		})
	}
}
func aiExecQuiescenceCode(code int) *int { return &code }

func TestAIExecQuiescenceUnknownIntentAndProtectedJournal(t *testing.T) {
	b := aiExecUnitBinding([]byte("intent"))
	j := aiExecUnitJournal(t, b)
	j.mu.Lock()
	e := j.appendLocked(aiExecRecord{Protocol: "qs-ai-exec-lifecycle/v1", Binding: b, Stage: "create_intent", EngineVersionSHA256: strings.Repeat("e", 64)})
	j.mu.Unlock()
	if e != nil {
		t.Fatal(e)
	}
	p := &aiExecUnitProtocol{t: t, journal: j, id: strings.Repeat("f", 64)}
	if e = aiExecRequireQuiescent(context.Background(), p, j); e == nil || len(p.calls) != 0 {
		t.Fatal("adopted unknown create intent")
	}
	in := AIExternalExecQuiescenceInput{SourceSHA: b.SourceSHA, OperationID: b.OperationID, RuntimeSourceSHA: b.RuntimeSourceSHA, ImageID: b.ImageID, ContainerID: b.ContainerID}
	if e = j.Close(); e != nil {
		t.Fatal(e)
	}
	reopened, e := aiExecQuiescenceJournal(j.path, in)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = reopened.file.WriteAt([]byte("x"), 0); e == nil {
		t.Fatal("original journal reopened for writing")
	}
	if e = reopened.Close(); e != nil {
		t.Fatal(e)
	}
	wrongUID := uint32(os.Geteuid()) + 1
	if bad, e := aiExecQuiescenceJournal(j.path, AIExternalExecQuiescenceInput{SourceUID: &wrongUID}); e == nil {
		_ = bad.Close()
		t.Fatal("original UID ignored")
	}
	changed := in
	changed.ContainerID = strings.Repeat("1", 64)
	if bad, e := aiExecQuiescenceJournal(j.path, changed); e == nil {
		_ = bad.Close()
		t.Fatal("accepted different original container")
	}
	path := filepath.Join(filepath.Dir(j.path), "completion.jsonl")
	if e = os.WriteFile(path, []byte("{\"terminal\":true}\n"), 0600); e != nil {
		t.Fatal(e)
	}
	if bad, e := aiExecQuiescenceJournal(path, in); e == nil {
		_ = bad.Close()
		t.Fatal("imported JSON completion as native journal")
	}
}

func TestAIExecQuiescenceNoPriorJournalIsNotExecutionAuthority(t *testing.T) {
	directory, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	op := filepath.Join(directory, "backups", "qs-server", "compatibility-retirement", "900-1")
	if e = os.MkdirAll(op, 0700); e != nil {
		t.Fatal(e)
	}
	b := aiExecUnitBinding([]byte("none"))
	in := AIExternalExecQuiescenceInput{OperationDirectory: op, SourceSHA: b.SourceSHA, OperationID: b.OperationID, RuntimeSourceSHA: b.RuntimeSourceSHA, ImageID: b.ImageID, ContainerID: b.ContainerID}
	if e = RequireAIExternalExecQuiescence(context.Background(), in); e != nil {
		t.Fatal(e)
	}
	// No Docker call is needed when both fixed journal names are actually absent.
	path, e := aiExternalExecModePath(op, b.OperationID, aiExternalBoundsMode)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(path, []byte("{\"complete\":true}\n"), 0600); e != nil {
		t.Fatal(e)
	}
	if e = RequireAIExternalExecQuiescence(context.Background(), in); !errors.Is(e, ErrAIExternalExecJournal) {
		t.Fatal("torn/untrusted journal was ignored")
	}
}
