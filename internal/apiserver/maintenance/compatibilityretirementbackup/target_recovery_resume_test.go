package compatibilityretirementbackup

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
)

func targetResumeFixtureRequest() TargetRecoveryRequest {
	return TargetRecoveryRequest{SourceSHA: strings.Repeat("a", 40), OperationID: "123-1", OriginalRunID: "124-1", ActualRunID: "125-1", ManifestSHA256: strings.Repeat("b", 64), ArchiveSHA256: strings.Repeat("c", 64), SQLNonTargetSHA256: strings.Repeat("d", 64), MongoNonTargetSHA256: strings.Repeat("e", 64), SQLHead: 99, MongoHead: 38}
}

func targetResumeFixtureArchive() *Archive {
	a := &Archive{}
	a.data.Inventory.Targets = make([]SourceSnapshot, 4)
	a.data.SQL[0].DDL = "CREATE TABLE `domain_event_outbox` (`id` bigint NOT NULL)"
	a.data.SQL[1].DDL = "CREATE TABLE `ai_bridge_commands` (`command_id` varchar(36) NOT NULL)"
	a.data.SQL[2].DDL = "CREATE TABLE `ai_messaging_legacy_commands` (`command_id` varchar(36) NOT NULL)"
	a.data.Inventory.Targets[0].Records = 7
	return a
}

func targetResumeFixtureRecord(request TargetRecoveryRequest, a *Archive, i int, phase, result string, n uint64) targetStatementRecord {
	statement, e := targetExpectedStatement(a, i, phase)
	if e != nil {
		panic("fixture statement unsupported")
	}
	database := "mysql"
	if i == 3 {
		database = "mongodb"
	}
	return targetStatementRecord{RequestSHA256: jsonSHA(request), Index: i, Database: database, Name: targetNames[i], Phase: phase, StatementSHA256: sha([]byte(statement)), SQLConnectionID: "42", Result: result, Records: n}
}

func targetResumeFixtureJournal(t *testing.T) (string, TargetRecoveryRequest, *targetRecoveryJournal, *Archive) {
	t.Helper()
	dir := targetRecoveryTestDir(t)
	r := targetResumeFixtureRequest()
	a := targetResumeFixtureArchive()
	j, e := newTargetJournal(dir, r)
	if e != nil {
		t.Fatal(e)
	}
	return dir, r, j, a
}

func TestResumeJournalActualFilesTwoPassesAndExactBinding(t *testing.T) {
	dir, r, j, a := targetResumeFixtureJournal(t)
	if _, e := j.write("0-drop-intent", targetResumeFixtureRecord(r, a, 0, "drop", "intent", 0)); e != nil {
		t.Fatal(e)
	}
	if _, e := j.write("0-drop-result", targetResumeFixtureRecord(r, a, 0, "drop", "native_success_and_absent", 0)); e != nil {
		t.Fatal(e)
	}
	window := strings.Repeat("f", 64)
	first, e := inspectTargetJournal(context.Background(), dir, r, window)
	if e != nil {
		t.Fatal(e)
	}
	second, e := inspectTargetJournal(context.Background(), dir, r, window)
	if e != nil {
		t.Fatal(e)
	}
	if first.hash != second.hash || len(first.entries) != 3 {
		t.Fatal("physical two-pass journal mismatch")
	}
	other := r
	other.SourceSHA = strings.Repeat("0", 40)
	if _, e = inspectTargetJournal(context.Background(), dir, other, window); e != ErrRecoveryBinding {
		t.Fatal("wrong original source accepted")
	}
	other = r
	other.ActualRunID = "126-1"
	if _, e = inspectTargetJournal(context.Background(), dir, other, window); e != ErrRecoveryBinding {
		t.Fatal("new run substituted for original binding")
	}
	state, e := classifyTargetJournal(first, a, 0)
	if e != nil || state.state != "logged_drop_success" {
		t.Fatal("successful original journal not classified")
	}
	if len(first.entries) == 0 || !hashPattern.MatchString(first.hash) {
		t.Fatal("no actual physical binding")
	}
}

func TestResumeJournalReplacementSameBytesChangesPhysicalBinding(t *testing.T) {
	dir, r, _, _ := targetResumeFixtureJournal(t)
	window := strings.Repeat("f", 64)
	old, e := inspectTargetJournal(context.Background(), dir, r, window)
	if e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(dir, "target-recovery-binding.json")
	held, e := os.Open(path)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if held.Close() != nil {
			t.Error("owned held file close failed")
		}
	})
	raw, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	if os.Remove(path) != nil || os.WriteFile(path, raw, 0600) != nil {
		t.Fatal("fixture replacement failed")
	}
	st, e := held.Stat()
	if e != nil {
		t.Fatal(e)
	}
	named, e := os.Stat(path)
	if e != nil || os.SameFile(st, named) {
		t.Fatal("fixture did not create a different inode")
	}
	fresh, e := inspectTargetJournal(context.Background(), dir, r, window)
	if e != nil {
		t.Fatal(e)
	}
	if old.hash == fresh.hash {
		t.Fatal("same bytes replaced file retained original physical authority")
	}
}

func TestResumeJournalRejectsUnknownSymlinkHardlinkFIFOAndNonPrivateFile(t *testing.T) {
	for _, kind := range []string{"unknown", "symlink", "hardlink", "fifo", "mode"} {
		t.Run(kind, func(t *testing.T) {
			dir, r, _, _ := targetResumeFixtureJournal(t)
			path := filepath.Join(dir, "target-recovery-0-drop-intent.json")
			switch kind {
			case "unknown":
				if os.WriteFile(filepath.Join(dir, "unknown.json"), []byte("{}"), 0600) != nil {
					t.Fatal("fixture")
				}
			case "symlink":
				if os.Symlink(filepath.Join(dir, "target-recovery-binding.json"), path) != nil {
					t.Fatal("fixture")
				}
			case "hardlink":
				if os.Link(filepath.Join(dir, "target-recovery-binding.json"), path) != nil {
					t.Fatal("fixture")
				}
			case "fifo":
				if syscall.Mkfifo(path, 0600) != nil {
					t.Fatal("fixture")
				}
			case "mode":
				if os.Chmod(filepath.Join(dir, "target-recovery-binding.json"), 0644) != nil {
					t.Fatal("fixture")
				}
			}
			if _, e := inspectTargetJournal(context.Background(), dir, r, strings.Repeat("f", 64)); e == nil {
				t.Fatal("unsafe private journal accepted")
			}
		})
	}
}

func TestResumeJournalRefusesDuplicateTruncatedAndNonOriginalJSON(t *testing.T) {
	for _, kind := range []string{"duplicate", "truncated", "whitespace"} {
		t.Run(kind, func(t *testing.T) {
			dir, r, _, _ := targetResumeFixtureJournal(t)
			path := filepath.Join(dir, "target-recovery-binding.json")
			raw, e := os.ReadFile(path)
			if e != nil {
				t.Fatal(e)
			}
			switch kind {
			case "duplicate":
				raw = append([]byte(`{"source_sha":"duplicate",`), raw[1:]...)
			case "truncated":
				raw = raw[:len(raw)-1]
			case "whitespace":
				raw = append(raw, '\n')
			}
			if os.WriteFile(path, raw, 0600) != nil {
				t.Fatal("fixture")
			}
			if _, e = inspectTargetJournal(context.Background(), dir, r, strings.Repeat("f", 64)); e == nil {
				t.Fatal("malformed/nonoriginal binding accepted")
			}
		})
	}
}

func TestResumeJournalUnknownIntentIsNotSuccessfulDrop(t *testing.T) {
	for _, kind := range []string{"no_result", "unknown_result"} {
		t.Run(kind, func(t *testing.T) {
			dir, r, j, a := targetResumeFixtureJournal(t)
			if _, e := j.write("0-drop-intent", targetResumeFixtureRecord(r, a, 0, "drop", "intent", 0)); e != nil {
				t.Fatal(e)
			}
			if kind == "unknown_result" {
				if _, e := j.write("0-drop-result", targetResumeFixtureRecord(r, a, 0, "drop", "unknown", 0)); e != nil {
					t.Fatal(e)
				}
			}
			s, e := inspectTargetJournal(context.Background(), dir, r, strings.Repeat("f", 64))
			if e != nil {
				t.Fatal(e)
			}
			state, e := classifyTargetJournal(s, a, 0)
			if e != nil || !strings.HasPrefix(state.state, "unresolved_") || state.reason == "" {
				t.Fatal("unknown original execution became native success")
			}
		})
	}
}

func TestResumeJournalRestorationRequiresEveryOriginalResultAndRecordCount(t *testing.T) {
	for _, kind := range []string{"full", "no_load_result", "no_verify", "wrong_load_count", "changed_target"} {
		t.Run(kind, func(t *testing.T) {
			dir, r, j, a := targetResumeFixtureJournal(t)
			steps := []struct {
				name, phase, result string
				records             uint64
			}{
				{"drop-intent", "drop", "intent", 0}, {"drop-result", "drop", "native_success_and_absent", 0},
				{"create-intent", "create", "intent", 0}, {"create-result", "create", "native_success", 0},
				{"load-intent", "load", "intent", 0}, {"load-result", "load", "native_success", 7},
				{"verify-result", "verify", "equal", 7},
			}
			for _, step := range steps {
				if kind == "no_load_result" && (step.name == "load-result" || step.name == "verify-result") {
					continue
				}
				if kind == "no_verify" && step.name == "verify-result" {
					continue
				}
				record := targetResumeFixtureRecord(r, a, 0, step.phase, step.result, step.records)
				if kind == "wrong_load_count" && step.name == "load-result" {
					record.Records = 6
				}
				if kind == "changed_target" && step.name == "drop-result" {
					record.Name = "rm_outbox"
				}
				if _, e := j.write("0-"+step.name, record); e != nil {
					t.Fatal(e)
				}
			}
			s, e := inspectTargetJournal(context.Background(), dir, r, strings.Repeat("f", 64))
			if e != nil {
				t.Fatal(e)
			}
			state, e := classifyTargetJournal(s, a, 0)
			switch kind {
			case "full":
				if e != nil || state.state != "logged_restore_success" {
					t.Fatal("complete original restore rejected")
				}
			case "no_load_result", "no_verify":
				if e != nil || state.reason == "" {
					t.Fatal("unfinished restore accepted")
				}
			default:
				if e != ErrRecoveryJournal {
					t.Fatal("conflicting original responsibility accepted")
				}
			}
		})
	}
}

func TestResumeJournalStatementHashAndConnectionAreOriginal(t *testing.T) {
	for _, kind := range []string{"statement", "connection"} {
		t.Run(kind, func(t *testing.T) {
			dir, r, j, a := targetResumeFixtureJournal(t)
			intent := targetResumeFixtureRecord(r, a, 0, "drop", "intent", 0)
			result := targetResumeFixtureRecord(r, a, 0, "drop", "native_success_and_absent", 0)
			if kind == "statement" {
				result.StatementSHA256 = sha([]byte("DROP TABLE `rm_outbox`"))
			} else {
				result.SQLConnectionID = strconv.Itoa(43)
			}
			if _, e := j.write("0-drop-intent", intent); e != nil {
				t.Fatal(e)
			}
			if _, e := j.write("0-drop-result", result); e != nil {
				t.Fatal(e)
			}
			s, e := inspectTargetJournal(context.Background(), dir, r, strings.Repeat("f", 64))
			if e != nil {
				t.Fatal(e)
			}
			if _, e = classifyTargetJournal(s, a, 0); e != ErrRecoveryJournal {
				t.Fatal("changed exact target statement/connection accepted")
			}
		})
	}
}

func TestResumeNoAuthorityOrOriginalProofMinted(t *testing.T) {
	if _, e := ReconcileTargetRecovery(context.Background(), nil, TargetRecoveryBorrowed{}, TargetRecoveryResumeRequest{}, "", nil); e != ErrRecoveryBinding {
		t.Fatal("nil handles accepted")
	}
	var original TargetRecoveryPlan
	original.self = &original
	if original.validDrop(0) {
		t.Fatal("journal summary recreated original native drop proof")
	}
	var zero TargetRecoveryReconciliation
	if e := zero.Recheck(context.Background(), TargetRecoveryBorrowed{}, nil); e != ErrRecoveryBinding {
		t.Fatal("zero reconciliation accepted")
	}
	if _, e := json.Marshal(&zero); !errors.Is(e, ErrSerialization) {
		t.Fatal("private reconciliation serializable")
	}
	sealed := TargetRecoveryJournalSummary{}
	if sealed.MutationAllowed || sealed.ProductionFenceVerified {
		t.Fatal("journal seal minted authority")
	}
}

func TestResumeJournalCurrentRunIsDistinctFromOriginalDropResponsibility(t *testing.T) {
	for _, kind := range []string{"complete", "binding_only", "wrong_source", "wrong_window", "wrong_current_run", "old_connection", "missing_binding"} {
		t.Run(kind, func(t *testing.T) {
			dir, original, j, a := targetResumeFixtureJournal(t)
			window := strings.Repeat("f", 64)
			for _, result := range []string{"intent", "native_success_and_absent"} {
				suffix := "intent"
				if result != "intent" {
					suffix = "result"
				}
				if _, e := j.write("0-drop-"+suffix, targetResumeFixtureRecord(original, a, 0, "drop", result, 0)); e != nil {
					t.Fatal(e)
				}
			}
			current := original
			current.ActualRunID = "127-2"
			binding := targetResumeBindingRecord{OriginalRequestSHA256: jsonSHA(original), WindowStartSHA256: window, Request: current, SQLConnectionID: "99"}
			switch kind {
			case "wrong_source":
				binding.Request.SourceSHA = strings.Repeat("0", 40)
			case "wrong_window":
				binding.WindowStartSHA256 = strings.Repeat("0", 64)
			case "wrong_current_run":
				binding.Request.ActualRunID = "invalid"
			}
			if kind != "missing_binding" {
				if _, e := j.write("0-resume-binding", binding); e != nil {
					t.Fatal(e)
				}
			}
			if kind != "binding_only" {
				for _, step := range []struct {
					name, phase, result string
					n                   uint64
				}{
					{"create-intent", "create", "intent", 0}, {"create-result", "create", "native_success", 0},
					{"load-intent", "load", "intent", 0}, {"load-result", "load", "native_success", 7}, {"verify-result", "verify", "equal", 7},
				} {
					record := targetResumeFixtureRecord(current, a, 0, step.phase, step.result, step.n)
					record.SQLConnectionID = "99"
					if kind == "old_connection" {
						record.SQLConnectionID = "42"
					}
					if _, e := j.write("0-"+step.name, record); e != nil {
						t.Fatal(e)
					}
				}
			}
			s, e := inspectTargetJournal(context.Background(), dir, original, window)
			if e != nil {
				t.Fatal(e)
			}
			state, e := classifyTargetJournal(s, a, 0)
			switch kind {
			case "complete":
				if e != nil || state.state != "logged_restore_success" {
					t.Fatal("real new-run binding rejected")
				}
			case "binding_only":
				if e != nil || state.state != "unresolved_resume_intent" || state.reason == "" {
					t.Fatal("interrupted resume adopted as success")
				}
			default:
				if e != ErrRecoveryJournal {
					t.Fatal("changed source/window/run/connection accepted")
				}
			}
		})
	}
}

func TestResumeBindingCannotInventOriginalDrop(t *testing.T) {
	dir, original, j, a := targetResumeFixtureJournal(t)
	window := strings.Repeat("f", 64)
	current := original
	current.ActualRunID = "127-2"
	if _, e := j.write("0-resume-binding", targetResumeBindingRecord{jsonSHA(original), window, current, "99"}); e != nil {
		t.Fatal(e)
	}
	s, e := inspectTargetJournal(context.Background(), dir, original, window)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = classifyTargetJournal(s, a, 0); e != ErrRecoveryJournal {
		t.Fatal("resume binding replaced original DROP proof")
	}
}

func TestReopenedRecoveryJournalUsesActualUnchangedPhysicalFiles(t *testing.T) {
	dir, r, _, _ := targetResumeFixtureJournal(t)
	window := strings.Repeat("f", 64)
	s, e := inspectTargetJournal(context.Background(), dir, r, window)
	if e != nil {
		t.Fatal(e)
	}
	j, e := reopenTargetRecoveryJournal(context.Background(), dir, s)
	if e != nil || j.validate() != nil {
		t.Fatal("actual journal reopen failed")
	}
	path := filepath.Join(dir, "target-recovery-binding.json")
	held, e := os.Open(path)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if held.Close() != nil {
			t.Error("held file close failed")
		}
	})
	raw, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	if os.Remove(path) != nil || os.WriteFile(path, raw, 0600) != nil {
		t.Fatal("replacement fixture failed")
	}
	if _, e = reopenTargetRecoveryJournal(context.Background(), dir, s); e != ErrRecoveryJournal {
		t.Fatal("old physical seal accepted replaced file")
	}
}

func TestLifecycleKernelRejectsZeroAndCopiedOpaqueObjects(t *testing.T) {
	ctx := context.Background()
	if _, e := ApplyTargets(ctx, nil); e != ErrRecoveryBinding {
		t.Fatal("nil apply accepted")
	}
	if _, e := VerifyDroppedTargets(ctx, nil); e != ErrRecoveryBinding {
		t.Fatal("nil verify accepted")
	}
	original := &TargetRecoveryPlan{}
	original.self = original
	copyPlan := &TargetRecoveryPlan{self: original}
	if _, e := ApplyTargets(ctx, copyPlan); e != ErrRecoveryBinding {
		t.Fatal("copied plan accepted")
	}
	if _, e := VerifyDroppedTargets(ctx, copyPlan); e != ErrRecoveryBinding {
		t.Fatal("copied plan verified")
	}
	zero := &TargetRecoveryReconciliation{}
	if _, e := ResumeTargetRecovery(ctx, zero, TargetRecoveryBorrowed{}, nil); e != ErrRecoveryBinding {
		t.Fatal("zero reconciliation resumed")
	}
	if _, e := InspectTargetRecoveryJournal(ctx, nil, TargetRecoveryRequest{}, "", nil); e != ErrRecoveryBinding {
		t.Fatal("zero journal binding accepted")
	}
	result := &TargetLifecycleResult{summary: TargetLifecycleSummary{Targets: TargetRecoverySummary{Targets: []TargetRecoveryObservation{{State: "restored_exact"}}}, WriterFenceRequired: true}}
	result.self = result
	view := result.Summary()
	view.Targets.Targets[0].State = "dropped_native"
	if result.Summary().Targets.Targets[0].State != "restored_exact" || view.Targets.ProductionAuthorityIntegrated || view.Targets.WholeWriterFenceProven || view.Targets.DropReady || view.Journal.ProductionFenceVerified || view.Journal.MutationAllowed {
		t.Fatal("readback alias or production authority")
	}
	if _, e := json.Marshal(result); !errors.Is(e, ErrSerialization) {
		t.Fatal("lifecycle readback became serializable authority")
	}
}
