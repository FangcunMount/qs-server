package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
)

type graphMemory struct {
	*memoryStore
	preflightLimit int
	preflights     int
}

func (s *graphMemory) readExactBatch(ctx context.Context, objects []object) ([]graphReadResult, error) {
	out := make([]graphReadResult, len(objects))
	for i, o := range objects {
		r, exists, e := s.read(ctx, o)
		if e != nil {
			return nil, e
		}
		out[i] = graphReadResult{r, exists}
	}
	return out, nil
}
func (s *graphMemory) preflightExactBatch(_ context.Context, records []record, _ bool) error {
	s.preflights++
	if s.preflightLimit > 0 && len(records) > s.preflightLimit {
		return errGraphBatchBudget
	}
	return nil
}
func (s *graphMemory) mutateExactBatch(ctx context.Context, records []record, restore bool) error {
	return s.mutate(ctx, records, restore)
}
func graphEngineFixture(t *testing.T) (*graphMemory, group, protected, graphIndex, graphArchive, string, string) {
	t.Helper()
	s, g, p, _, _ := fixture(t)
	dir := privateTestDir(t)
	scope := privateTestDir(t)
	root := graphSQLFixture("testee", "1", map[string]string{"source": "daily_simulation", "org_id": "1"})
	root.Definition = append([]byte{}, s.definitions["mysql/testee"]...)
	s.rows[objectID(root.Object)] = root
	s.definitions["mysql/testee"] = root.Definition
	// Keep complete original SQL defaults and BSON bytes in separate namespaces.
	score := s.rows[objectID(g.QS.Objects[0])]
	score.Object.Fingerprint = fingerprint(score)
	sheet := graphMongoFixture(t, "answersheets", bson.D{{Key: "domain_id", Value: int64(20)}, {Key: "org_id", Value: int64(1)}, {Key: "testee_id", Value: int64(1)}, {Key: "admission", Value: bson.D{{Key: "purpose", Value: "independent_questionnaire"}}}})
	s.rows[objectID(sheet.Object)] = sheet
	s.definitions["mongo/answersheets"] = sheet.Definition
	index := graphIndex{Shards: []graphShard{}, Counts: map[string]int64{}}
	w := graphWriter{dir: scope, index: &index, protected: map[string]bool{}}
	for _, v := range []struct {
		r     record
		rank  int
		links map[string]string
	}{{root, 10, map[string]string{"id": "1"}}, {score, 70, map[string]string{"id": "42", "testee_id": "1"}}, {sheet, 50, map[string]string{"domain_id": "20", "testee_id": "1"}}} {
		o := v.r.Object
		o.Fingerprint = fingerprint(v.r)
		if e := w.add(graphObject{Object: o, Links: v.links, RootIDs: []string{"1"}}, v.rank); e != nil {
			t.Fatal(e)
		}
	}
	if e := w.flush(); e != nil {
		t.Fatal(e)
	}
	g.QS.Objects = []object{}
	g.QS.Exclusions = []string{}
	g.QS.Graph = &graphBinding{ArchiveDirectory: dir, RootIDs: []string{"1"}, WindowStarted: time.Now().Add(-time.Minute), WindowExpires: time.Now().Add(29 * time.Minute)}
	store := &graphMemory{memoryStore: s}
	m, e := captureGraph(t.Context(), store, g, p, index, scope, dir, strings.Repeat("c", 64), strings.Repeat("d", 40))
	if e != nil {
		t.Fatal(e)
	}
	if e = validateGraphCoverage(m, g, p, index, scope, dir); e != nil {
		t.Fatal(e)
	}
	return store, g, p, index, m, scope, dir
}
func TestGraphCompleteArchiveCoverageAndRawRestore(t *testing.T) {
	s, g, p, index, m, scope, dir := graphEngineFixture(t)
	if len(m.Targets) != 3 || m.ProtectedCount != 2 || s.writes != 0 {
		t.Fatal("capture must preserve the exact graph and guards without database writes")
	}
	bad := m
	bad.Targets = append([]graphShard{}, m.Targets[:2]...)
	if e := validateGraphCoverage(bad, g, p, index, scope, dir); e == nil {
		t.Fatal("truncated complete material admitted")
	}
	for _, ref := range m.Targets {
		records, e := readGraphArchivePage(dir, m, ref)
		if e != nil {
			t.Fatal(e)
		}
		for _, r := range records {
			if !sameRecord(r, s.rows[objectID(r.Object)]) {
				t.Fatal("original raw values changed")
			}
		}
	}
	original := m
	original.Graph.RootIDs = []string{"999"}
	if e := validateGraphArchive(original, g, m.GroupSHA); e == nil {
		t.Fatal("scope expansion admitted")
	}
}
func TestGraphUnknownCommitReconcilesAllChunksWithoutAutomaticReplay(t *testing.T) {
	s, _, _, _, m, _, dir := graphEngineFixture(t)
	j, e := openGraphJournal(dir, true)
	if e != nil {
		t.Fatal(e)
	}
	defer func() { _ = j.close() }()
	a := graphApproval{approval: approval{ArchiveSHA: strings.Repeat("1", 64), Expires: time.Now().Add(time.Minute)}}
	authority := func() error { return nil }
	s.ackUnknown = true
	if e = mutateGraph(t.Context(), s, m, dir, j, a, strings.Repeat("2", 64), false, authority, authority); e == nil || s.writes != 1 {
		t.Fatal("unknown ACK must fail after one actual batch")
	}
	if e = mutateGraph(t.Context(), s, m, dir, j, a, strings.Repeat("3", 64), false, authority, authority); e == nil || s.writes != 1 {
		t.Fatal("unknown was automatically retried")
	}
	if _, e = os.Lstat(filepath.Join(dir, "approval-"+strings.Repeat("3", 64)+".consumed")); !errors.Is(e, os.ErrNotExist) {
		t.Fatal("unknown state consumed a new approval")
	}
	if e = graphVerify(t.Context(), s, m, dir, j, false); e == nil {
		t.Fatal("partial graph must not receive whole clearance")
	}
	for _, entry := range j.latest {
		if entry.State == "unknown" || entry.State == "intent" {
			t.Fatal("readonly verification did not reconcile the completed unknown prefix")
		}
	}
	s.ackUnknown = false
	if e = mutateGraph(t.Context(), s, m, dir, j, a, strings.Repeat("3", 64), false, authority, authority); e != nil || s.writes != 3 {
		t.Fatalf("remaining acknowledged batches: %v/%d", e, s.writes)
	}
	a.RecoveryStarted = time.Now()
	a.RecoveryExpires = a.RecoveryStarted.Add(time.Hour)
	if e = mutateGraph(t.Context(), s, m, dir, j, a, strings.Repeat("4", 64), true, authority, authority); e != nil {
		t.Fatal(e)
	}
	if e = graphVerify(t.Context(), s, m, dir, j, true); e != nil {
		t.Fatal(e)
	}
}
func TestGraphProtectionAndNewFactConflictStopBeforeClaim(t *testing.T) {
	s, _, p, _, m, _, dir := graphEngineFixture(t)
	j, e := openGraphJournal(dir, true)
	if e != nil {
		t.Fatal(e)
	}
	defer func() { _ = j.close() }()
	r := s.rows[objectID(p.QS.Clack[0])]
	r.Columns[1].Raw = []byte(`{"real":"changed"}`)
	s.rows[objectID(r.Object)] = r
	if e = mutateGraph(t.Context(), s, m, dir, j, graphApproval{}, strings.Repeat("a", 64), false, func() error { return nil }, func() error { return nil }); e == nil || s.writes != 0 {
		t.Fatal("protected change did not stop before claim")
	}
	if _, e = os.Lstat(filepath.Join(dir, "approval-"+strings.Repeat("a", 64)+".consumed")); !errors.Is(e, os.ErrNotExist) {
		t.Fatal("protection conflict consumed approval")
	}
}
func TestGraphNewTargetFactAndCanceledBudgetDoNotConsumeApproval(t *testing.T) {
	s, _, _, _, m, _, dir := graphEngineFixture(t)
	j, e := openGraphJournal(dir, true)
	if e != nil {
		t.Fatal(e)
	}
	defer func() { _ = j.close() }()
	target := m.Targets[1]
	old, e := readGraphArchivePage(dir, m, target)
	if e != nil {
		t.Fatal(e)
	}
	changed := s.rows[objectID(old[0].Object)]
	changed.Columns[1].Raw = []byte(`{"new":"retained fact"}`)
	s.rows[objectID(changed.Object)] = changed
	sha := strings.Repeat("9", 64)
	if e = mutateGraph(t.Context(), s, m, dir, j, graphApproval{}, sha, true, func() error { return nil }, func() error { return nil }); e == nil || s.writes != 0 {
		t.Fatal("new fact restore conflict must stop before claim")
	}
	if _, e = os.Lstat(filepath.Join(dir, "approval-"+sha+".consumed")); !errors.Is(e, os.ErrNotExist) {
		t.Fatal("new fact consumed restore approval")
	}
	s.rows[objectID(changed.Object)] = old[0]
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if e = mutateGraph(ctx, s, m, dir, j, graphApproval{}, sha, false, func() error { return nil }, func() error { return nil }); !errors.Is(e, context.Canceled) {
		t.Fatal("expired caller budget was not retained", e)
	}
	if _, e = os.Lstat(filepath.Join(dir, "approval-"+sha+".consumed")); !errors.Is(e, os.ErrNotExist) {
		t.Fatal("expired budget consumed approval")
	}
	if second, e := openGraphJournal(dir, true); e == nil {
		_ = second.close()
		t.Fatal("parallel executor acquired the same active journal")
	}
}

func TestGraphFixedWindowAndRecoveryDecisionCannotReset(t *testing.T) {
	_, g, _, _, m, _, dir := graphEngineFixture(t)
	now := time.Now()
	a := graphApproval{approval: approval{Format: 1, Kind: "mock_cohort_action_approval", Action: "apply", Operation: m.Operation, GroupSHA: m.GroupSHA, ProtectedSHA: m.ProtectionSHA, ArchiveSHA: strings.Repeat("1", 64), ToolSource: m.Source, FreezeSHA: strings.Repeat("2", 64), SettlementSHA: strings.Repeat("2", 64), Issued: now.Add(-time.Second), Expires: now.Add(time.Minute)}}
	if e := graphCheckApproval(a, m, g, a.ArchiveSHA, "apply", m.Source, now); e != nil {
		t.Fatal(e)
	}
	a.Expires = m.Graph.WindowExpires.Add(time.Second)
	if e := graphCheckApproval(a, m, g, a.ArchiveSHA, "apply", m.Source, now); e == nil {
		t.Fatal("apply deadline reset admitted")
	}
	a.Action = "restore"
	a.BeforeReopen = true
	a.RecoveryStarted = now.Add(-time.Second)
	a.RecoveryExpires = now.Add(59 * time.Minute)
	a.Expires = now.Add(time.Minute)
	if e := graphCheckApproval(a, m, g, a.ArchiveSHA, "restore", m.Source, now); e != nil {
		t.Fatal(e)
	}
	if e := graphRecoveryClaim(dir, m, a.ArchiveSHA, a); e != nil {
		t.Fatal(e)
	}
	changed := a
	changed.RecoveryStarted = changed.RecoveryStarted.Add(time.Second)
	if e := graphRecoveryClaim(dir, m, a.ArchiveSHA, changed); e == nil {
		t.Fatal("retry reset fixed recovery decision")
	}
	if e := writeNew(filepath.Join(dir, "forward-only.json"), []byte(`{"first_business_start_dispatched":true}`)); e != nil {
		t.Fatal(e)
	}
	if e := graphCheckApproval(a, m, g, a.ArchiveSHA, "restore", m.Source, now); e == nil {
		t.Fatal("old archive authorized restore after first-business boundary")
	}
}
func TestGraphCaptureProofRejectsFailedProcessAndResidualMaterial(t *testing.T) {
	_, _, _, _, m, _, dir := graphEngineFixture(t)
	sha := strings.Repeat("a", 64)
	r := actionReceipt{Format: 1, Kind: "mock_cohort_qs_action_result", Action: "capture", Operation: m.Operation, GroupSHA: m.GroupSHA, Source: m.Source, MaterialSHA: sha, Complete: true, Released: true}
	raw := append(encode(r), '\n')
	receipt := filepath.Join(dir, "original-capture.stdout")
	if e := writeNew(receipt, raw); e != nil {
		t.Fatal(e)
	}
	code := 1
	evidence := graphCaptureExecution{1, "mock_cohort_capture_execution", receipt, digest(raw), "exited", &code}
	file := filepath.Join(dir, "failed-process.json")
	if e := writeNew(file, encode(evidence)); e != nil {
		t.Fatal(e)
	}
	if e := readGraphCaptureProof(file, digest(encode(evidence)), m, sha); e == nil {
		t.Fatal("failed process with complete material/stdout admitted")
	}
	code = 0
	success := filepath.Join(dir, "successful-process.json")
	if e := writeNew(success, encode(evidence)); e != nil {
		t.Fatal(e)
	}
	if e := readGraphCaptureProof(success, digest(encode(evidence)), m, sha); e != nil {
		t.Fatal(e)
	}
	if e := readGraphCaptureProof(success, digest(encode(evidence)), m, strings.Repeat("b", 64)); e == nil {
		t.Fatal("proof substituted another material")
	}
	for _, action := range []string{"plan", "capture", "plan-cohort"} {
		if _, e := flags([]string{"--action", action, "--capture-evidence-file="}); e == nil {
			t.Fatal("explicit empty capture proof flag accepted by producer")
		}
	}
}
func TestGraphCaptureSplitsOnlyReadOnlyBudgetBeforeArchive(t *testing.T) {
	s, _, _, _, _, _, _ := graphEngineFixture(t)
	objects := []object{}
	for _, id := range []string{"100", "101", "102"} {
		r := graphSQLFixture("assessment_score", id, map[string]string{})
		s.rows[objectID(r.Object)] = r
		s.definitions["mysql/assessment_score"] = r.Definition
		objects = append(objects, r.Object)
	}
	s.preflightLimit = 1
	chunks := 0
	if e := graphReadBounded(t.Context(), s, objects, true, func(records []record) error {
		chunks++
		if len(records) != 1 {
			t.Fatal("oversized archive admission")
		}
		return nil
	}); e != nil || chunks != 3 || s.writes != 0 {
		t.Fatalf("bounded read-only split: %v", e)
	}
}

func TestGraphClearanceFindsNewChildrenAndSharedFillerUser(t *testing.T) {
	_, g, _, index, _, scope, _ := graphEngineFixture(t)
	reader := &graphFake{sqlRows: map[string][]record{}, mongoRows: map[string][]record{}, mongoRootScans: map[string]int{}}
	if e := graphNoRemainingReferences(t.Context(), reader, g, index, scope); e != nil {
		t.Fatal(e)
	}
	// Equal numeric values in independent domains do not make an unrelated sheet a target.
	real := graphMongoFixture(t, "answersheets", bson.D{{Key: "domain_id", Value: int64(1)}, {Key: "testee_id", Value: int64(2)}, {Key: "filler_id", Value: int64(900)}})
	reader.mongoRows["answersheets"] = []record{real}
	if e := graphNoRemainingReferences(t.Context(), reader, g, index, scope); e != nil {
		t.Fatal("same numeric domain value widened the cohort", e)
	}
	current := graphMongoFixture(t, "answersheets", bson.D{{Key: "domain_id", Value: int64(999)}, {Key: "testee_id", Value: int64(1)}})
	reader.mongoRows["answersheets"] = []record{current}
	if e := graphNoRemainingReferences(t.Context(), reader, g, index, scope); e == nil {
		t.Fatal("new child with a different physical PK escaped final clearance")
	}
	g.IAM = []byte(`{"user_ids":["900"],"profile_ids":[],"login_identity_ids":[]}`)
	reader.mongoRows["answersheets"] = []record{real}
	if e := graphNoRemainingReferences(t.Context(), reader, g, index, scope); e == nil {
		t.Fatal("shared real sheet's filler user was falsely cleared for IAM deletion")
	}
}

func TestGraphForeignParentLinksBlockCaptureAndClearance(t *testing.T) {
	source, selector, p, scope := graphFixture(t)
	index, e := planCohort(t.Context(), source, selector, p, scope, digest(encode(selector)), strings.Repeat("a", 40))
	if e != nil {
		t.Fatal(e)
	}
	g := group{IAM: []byte(`{"user_ids":[],"profile_ids":[],"login_identity_ids":[]}`), QS: qsScope{Graph: &graphBinding{RootIDs: []string{"1"}}, Selector: selector.Selection}}
	reader := &graphFake{sqlRows: map[string][]record{}, mongoRows: map[string][]record{}, mongoRootScans: map[string]int{}}
	row := graphMongoFixture(t, "interpret_report_artifacts", bson.D{{Key: "domain_id", Value: int64(999)}, {Key: "org_id", Value: int64(1)}, {Key: "testee_id", Value: int64(2)}, {Key: "assessment_id", Value: int64(100)}, {Key: "outcome_id", Value: int64(200)}})
	reader.mongoRows["interpret_report_artifacts"] = []record{row}
	for _, allow := range []bool{true, false} {
		if e = graphRelationshipCheck(t.Context(), reader, g, index, scope, allow); e == nil {
			t.Fatal("retained Testee hid a new artifact reference to a frozen target parent")
		}
	}
	// Discovery must report the component before capture, without claiming the
	// retained document as a writable mock object or blocking unrelated roots.
	source.mongoRows["interpret_report_artifacts"] = append(source.mongoRows["interpret_report_artifacts"], row)
	next := privateTestDir(t)
	discovered, e := planCohort(t.Context(), source, selector, p, next, digest(encode(selector)), strings.Repeat("a", 40))
	if e != nil {
		t.Fatal(e)
	}
	if discovered.Holds["interpret_report_artifacts:retained_object_links_target_parent"] != 1 {
		t.Fatal("foreign crosslink was not assigned to the exact target component")
	}
	if e = validateGraphHolds(next, discovered, map[string]bool{"1": true}); e == nil {
		t.Fatal("held target component admitted to a write subset")
	}
	if e = validateGraphHolds(next, discovered, map[string]bool{"9999": true}); e != nil {
		t.Fatal("unrelated root was globally blocked", e)
	}
}
func TestGraphStatisticSourceDomainMustOwnTheTargetTestee(t *testing.T) {
	reader, selector, _, dir := graphFixture(t)
	index := graphIndex{Counts: map[string]int64{}, Holds: map[string]int64{}, HoldShards: []graphHoldShard{}, RequiredActions: map[string]int64{}}
	planner := cohortPlanner{reader: reader, selector: selector, dir: dir, writer: &graphWriter{dir: dir, index: &index, protected: map[string]bool{}}, referenceOwners: map[string]graphObject{}}
	row := graphSQLFixture("statistics_assessment_fact", "999", map[string]string{"testee_id": "1", "org_id": "1"})
	row.Columns = append(row.Columns, cell{Name: "source_type", Type: "VARCHAR", Raw: []byte("assessment")}, cell{Name: "source_ref", Type: "VARCHAR", Raw: []byte("101")})
	if e := planner.statisticBatch(t.Context(), []record{row}); e != nil {
		t.Fatal(e)
	}
	if index.Holds["statistics_assessment_fact:typed_statistic_source_ownership_conflict"] != 1 {
		t.Fatal("a denormalized Testee column authorized deletion of a retained Assessment fact")
	}
	if e := planner.writer.flush(); e != nil {
		t.Fatal(e)
	}
	if e := planner.flushHolds(); e != nil {
		t.Fatal(e)
	}
	if e := validateGraphHolds(dir, index, map[string]bool{"1": true}); e == nil {
		t.Fatal("crosslinked statistics escaped per-component preclaim hold")
	}
}

func TestGraphImmutableInputRevisionRejectsInPlaceEditAndReplacement(t *testing.T) {
	dir := privateTestDir(t)
	path := filepath.Join(dir, "original-group.json")
	raw := []byte(`{"fixed":"old"}`)
	if e := writeNew(path, raw); e != nil {
		t.Fatal(e)
	}
	info, change, e := graphPrivateInfo(path)
	if e != nil {
		t.Fatal(e)
	}
	guard := graphInputGuard{files: []graphOriginalInput{{path: path, sha: digest(raw), max: 1024, info: info, changeTime: change}}}
	if e = guard.full(); e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(path, []byte(`{"fixed":"new"}`), 0600); e != nil {
		t.Fatal(e)
	}
	// Restoring mtime does not restore ctime. A repeated small guard must stop,
	// even though no per-chunk reread of the large immutable group is required.
	if e = os.Chtimes(path, info.ModTime(), info.ModTime()); e != nil {
		t.Fatal(e)
	}
	if e = guard.check(); e == nil {
		t.Fatal("same inode/size/mtime edit escaped the running action guard")
	}
	replacement := filepath.Join(dir, "replacement.json")
	if e = writeNew(replacement, raw); e != nil {
		t.Fatal(e)
	}
	if e = os.Chtimes(replacement, info.ModTime(), info.ModTime()); e != nil {
		t.Fatal(e)
	}
	if e = os.Rename(replacement, path); e != nil {
		t.Fatal(e)
	}
	if e = guard.check(); e == nil {
		t.Fatal("replacement with original bytes escaped the bound inode")
	}
}

func TestGraphOpaqueEvaluationRunAndNumericaliasRemainNamedRelations(t *testing.T) {
	_, g, _, index, _, scope, _ := graphEngineFixture(t)
	run := graphSQLFixture("runtime_checkpoint", "88", map[string]string{"assessment_id": "100"})
	run.Columns = append(run.Columns, cell{Name: "scope", Type: "VARCHAR", Raw: []byte("evaluation_run")}, cell{Name: "resource_id", Type: "VARCHAR", Raw: []byte("100:1")})
	run.Object.Fingerprint = fingerprint(run)
	writer := graphWriter{dir: scope, index: &index, protected: map[string]bool{}}
	if e := writer.add(graphObject{Object: run.Object, RootIDs: []string{"1"}, Links: map[string]string{"id": "88", "scope": "evaluation_run", "resource_id": "100:1", "assessment_id": "100"}}, 60); e != nil {
		t.Fatal(e)
	}
	if e := writer.flush(); e != nil {
		t.Fatal(e)
	}
	frozen, e := frozenGraphReferences(scope, index, []string{"1"}, false)
	if e != nil {
		t.Fatal(e)
	}
	if !frozen.ids["runtime/evaluation_run"]["100:1"] {
		t.Fatal("opaque evaluation RunID was discarded as non-decimal")
	}
	for _, alias := range []any{float64(1), "01"} {
		reader := &graphFake{sqlRows: map[string][]record{}, mongoRows: map[string][]record{}, mongoRootScans: map[string]int{}}
		reader.mongoRows["answersheets"] = []record{graphMongoFixture(t, "answersheets", bson.D{{Key: "domain_id", Value: int64(999)}, {Key: "testee_id", Value: alias}})}
		if e = graphNoRemainingReferences(t.Context(), reader, g, index, scope); e == nil {
			t.Fatal("same-domain numeric alias escaped current-reference clearance")
		}
	}
}
func TestGraphReadonlyQualificationCannotResetWindowOrReopenBoundary(t *testing.T) {
	_, _, _, _, m, _, dir := graphEngineFixture(t)
	now := time.Now()
	m.Graph.WindowStarted = now.Add(-40 * time.Minute)
	m.Graph.WindowExpires = now.Add(-10 * time.Minute)
	sha := strings.Repeat("8", 64)
	if _, cancel, e := graphReadContext(t.Context(), m, sha, false); e == nil {
		cancel()
		t.Fatal("new verify CLI reset the expired maintenance window")
	}
	boundary := graphRecoveryBoundary{1, "mock_cohort_recovery_boundary", m.Operation, m.GroupSHA, sha, now.Add(-15 * time.Minute), now.Add(45 * time.Minute)}
	if e := writeNew(filepath.Join(dir, "recovery-boundary.json"), encode(boundary)); e != nil {
		t.Fatal(e)
	}
	for range 2 {
		ctx, cancel, e := graphReadContext(context.Background(), m, sha, true)
		if e != nil {
			t.Fatal(e)
		}
		deadline, ok := ctx.Deadline()
		cancel()
		if !ok || !deadline.Equal(boundary.Expires.Add(-5*time.Minute)) {
			t.Fatal("original verification reset the fixed recovery deadline")
		}
	}
	if e := writeNew(filepath.Join(dir, "forward-only.json"), []byte("first start dispatched")); e != nil {
		t.Fatal(e)
	}
	if _, cancel, e := graphReadContext(t.Context(), m, sha, true); e == nil {
		cancel()
		t.Fatal("post-reopen verify issued a qualified recovery receipt")
	}
}

func TestGraphRetainedReplayGuardIsArchivedUnchangedAndAllowedOnlyByExactPreimage(t *testing.T) {
	s, g, p, index, _, scope, _ := graphEngineFixture(t)
	guard := graphSQLFixture("interpretation_attention_projection", "91", map[string]string{"testee_id": "1", "assessment_id": "100", "status": "succeeded"})
	guard.Columns = append(guard.Columns, cell{Name: "event_id", Type: "VARCHAR", Raw: []byte("original-report-event")}, cell{Name: "report_id", Type: "VARCHAR", Raw: []byte("700")})
	s.rows[objectID(guard.Object)] = guard
	s.definitions["mysql/interpretation_attention_projection"] = guard.Definition
	guard.Object.Fingerprint = fingerprint(guard)
	writer := graphWriter{dir: scope, index: &index, protected: map[string]bool{}}
	if e := writer.add(graphObject{Object: guard.Object, RootIDs: []string{"1"}, Links: map[string]string{"testee_id": "1", "assessment_id": "100", "report_id": "700"}}, 0); e != nil {
		t.Fatal(e)
	}
	if e := writer.flush(); e != nil {
		t.Fatal(e)
	}
	dir := privateTestDir(t)
	g.QS.Graph.ArchiveDirectory = dir
	m, e := captureGraph(t.Context(), s, g, p, index, scope, dir, strings.Repeat("c", 64), strings.Repeat("d", 40))
	if e != nil {
		t.Fatal(e)
	}
	if len(m.Retained) != 1 || len(m.Targets) != 3 {
		t.Fatal("online replay evidence was omitted or admitted as a delete target")
	}
	if e = validateGraphCoverage(m, g, p, index, scope, dir); e != nil {
		t.Fatal(e)
	}
	bad := m
	bad.Retained = nil
	if e = validateGraphCoverage(bad, g, p, index, scope, dir); e == nil {
		t.Fatal("omitted retained raw evidence was accepted")
	}
	journal, e := openGraphJournal(dir, true)
	if e != nil {
		t.Fatal(e)
	}
	defer func() { _ = journal.close() }()
	authority := func() error { return nil }
	if e = mutateGraph(t.Context(), s, m, dir, journal, graphApproval{}, strings.Repeat("a", 64), false, authority, authority); e != nil {
		t.Fatal(e)
	}
	if _, exists := s.rows[objectID(guard.Object)]; !exists {
		t.Fatal("successful online replay guard was physically deleted")
	}
	reader := &graphFake{sqlRows: map[string][]record{"interpretation_attention_projection": {guard}}, mongoRows: map[string][]record{}, mongoRootScans: map[string]int{}}
	if e = graphNoRemainingReferences(t.Context(), reader, g, index, scope); e != nil {
		t.Fatal("original allowed retained reference falsely blocked clearance", e)
	}
	changed := guard
	changed.Columns = append(append([]cell{}, guard.Columns...), cell{Name: "last_error", Type: "TEXT", Raw: []byte("new state")})
	s.rows[objectID(guard.Object)] = changed
	reader.sqlRows["interpretation_attention_projection"] = []record{changed}
	if e = protectGraph(t.Context(), s, m, dir); e == nil {
		t.Fatal("retained online evidence drift escaped final raw verification")
	}
	if e = graphNoRemainingReferences(t.Context(), reader, g, index, scope); e == nil {
		t.Fatal("retained physical ID alone allowed a changed reference")
	}
}
