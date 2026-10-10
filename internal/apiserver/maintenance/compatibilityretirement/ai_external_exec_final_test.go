package retirement

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// Real private files and durable chain handling; the protocol seam below is
// only GET/create ordering, never Engine execution or a usable Q capability.
func TestAIFinalVerifyRequiresKnownHistoricalSuccessAndActualOriginalTerminal(t *testing.T) {
	for _, tc := range []struct {
		name       string
		unknown    bool
		oldExit    int
		actual     aiExecInspect
		inspectErr error
		accept     bool
	}{
		{name: "known_actual_terminal", actual: aiExecInspect{ExitCode: aiExecQuiescenceCode(0)}, accept: true},
		{name: "historical_unknown_terminal_now", unknown: true, actual: aiExecInspect{ExitCode: aiExecQuiescenceCode(0)}},
		{name: "historical_nonzero", oldExit: 1, actual: aiExecInspect{ExitCode: aiExecQuiescenceCode(0)}},
		{name: "actual_running", actual: aiExecInspect{Running: true}},
		{name: "actual_missing_exit", actual: aiExecInspect{}},
		{name: "actual_nonzero", actual: aiExecInspect{ExitCode: aiExecQuiescenceCode(2)}},
		{name: "actual_inspect_failed", inspectErr: errors.New("fixed inspect failure")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := []byte("test-only-prior-native-packet")
			b := aiExecUnitBinding(input)
			j := aiExecUnitJournal(t, b)
			p := aiExecUnitProtocolFor(t, j)
			p.after.ExitCode = aiExecQuiescenceCode(tc.oldExit)
			if tc.unknown {
				p.attachErr = errors.New("fixed attach unknown")
			}
			ctx, c := context.WithTimeout(t.Context(), time.Second)
			defer c()
			_, e := aiExecProduce(ctx, p, j, aiExecUnitHost(t), input)
			if (tc.unknown || tc.oldExit != 0) && e == nil {
				t.Fatal("fixture failed to produce original unknown")
			}
			before, e := os.ReadFile(j.path)
			if e != nil {
				t.Fatal(e)
			}
			p.calls = nil
			p.after = tc.actual
			p.inspectErr = tc.inspectErr
			e = aiExecRequireFinalPredecessor(ctx, p, j)
			if (e == nil) != tc.accept {
				t.Fatal("original unknown/current terminal mismatch", e)
			}
			if tc.unknown || tc.oldExit != 0 {
				if len(p.calls) != 0 {
					t.Fatal("known-blocked original reached any new action")
				}
			} else if !reflect.DeepEqual(p.calls, []string{"version", "inspect"}) {
				t.Fatal("predecessor started/attached/retried an exec")
			}
			after, re := os.ReadFile(j.path)
			if re != nil || string(before) != string(after) || j.binding != b {
				t.Fatal("original journal/deadline was rewritten")
			}
		})
	}
}

func TestAIFinalVerifyFixedPhaseIsSeparateExclusiveAndRejectsMissingOrImportedPredecessor(t *testing.T) {
	directory, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	op := filepath.Join(directory, "backups", "qs-server", "compatibility-retirement", "900-1")
	if os.MkdirAll(op, 0700) != nil {
		t.Fatal("private fixture")
	}
	old, e := aiExternalExecModePath(op, "900-1", aiExternalVerifyMode)
	if e != nil {
		t.Fatal(e)
	}
	final, e := aiExternalExecModePath(op, "900-1", aiExternalFinalVerifyMode)
	if e != nil || final == old || filepath.Base(final) != "qs-ai-external-final-verify.exec.jsonl" {
		t.Fatal("final mode changed original journal identity")
	}
	owner := HistoricalCoordinatorBinding{SourceSHA: strings.Repeat("a", 40), OperationID: "900-1"}
	if j, e := aiExecOpenFinalPredecessor(t.Context(), nil, op, owner, strings.Repeat("b", 40), "sha256:"+strings.Repeat("c", 64), strings.Repeat("d", 64), nil); e == nil || j != nil {
		t.Fatal("missing prior chain admitted final")
	}
	if os.WriteFile(old, []byte("{\"complete\":true,\"exit_code\":0}\n"), 0600) != nil {
		t.Fatal("fixture JSON")
	}
	if j, e := aiExecOpenFinalPredecessor(t.Context(), nil, op, owner, strings.Repeat("b", 40), "sha256:"+strings.Repeat("c", 64), strings.Repeat("d", 64), nil); e == nil || j != nil {
		t.Fatal("JSON success imported")
	}
	b := aiExecUnitBinding([]byte("fresh-final-packet"))
	j, e := aiExecOpenJournal(final, b, true)
	if e != nil {
		t.Fatal(e)
	}
	if e = j.Close(); e != nil {
		t.Fatal(e)
	}
	if again, e := aiExecOpenJournal(final, b, true); e == nil {
		_ = again.Close()
		t.Fatal("fixed final phase replayed")
	}
	if raw, e := os.ReadFile(old); e != nil || string(raw) != "{\"complete\":true,\"exit_code\":0}\n" {
		t.Fatal("old materials changed")
	}
	for _, mode := range []aiExternalExecMode{"retry", "final", "verify-123", ""} {
		if _, e := aiExternalExecModePath(op, "900-1", mode); e == nil {
			t.Fatal("arbitrary input selected new phase")
		}
	}
}
