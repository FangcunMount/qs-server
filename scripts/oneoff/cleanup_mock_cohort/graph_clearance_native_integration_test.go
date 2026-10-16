//go:build integration

package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
)

// A separate real-process lane for explicit direct deletion and readonly
// clearance. Existing default and rule-batch native selectors are unchanged.
func TestMockCohortFrozenRuleClearanceCLINativeOwnedCycle(t *testing.T) {
	f := seedCohortGraphNative(t)
	h := f.h
	ctx, cancel := context.WithTimeout(t.Context(), 6*time.Minute)
	defer cancel()
	dir, scope, archive := privateTestDir(t), privateTestDir(t), privateTestDir(t)
	write := func(name string, raw []byte) string {
		path := filepath.Join(dir, name)
		require.NoError(t, writeNew(path, raw))
		return path
	}
	cfg, e := mysqldriver.ParseDSN(os.Getenv("QS_MOCK_NATIVE_MYSQL_DSN"))
	require.NoError(t, e)
	cfg.DBName, cfg.MultiStatements = h.name, false
	cli := graphCLINative{ctx: ctx, dir: dir, mysql: write("mysql-dsn", []byte(cfg.FormatDSN())), mongo: write("mongo-uri", []byte(os.Getenv("QS_MOCK_NATIVE_MONGO_URI")))}
	selectorRaw, protectionRaw := encode(f.selector), encode(f.protected)
	selectorPath, protectionPath := write("selector.json", selectorRaw), write("protection.json", protectionRaw)
	windowStarted := time.Now().UTC()
	call := func(action string, args []string, partial bool) (actionReceipt, []byte, int) {
		raw, code := cli.invoke(t, append(append([]string{}, args...), "--action="+action))
		require.Equal(t, 0, code)
		var receipt actionReceipt
		require.NoError(t, strictDecode(raw, &receipt))
		require.Equal(t, action, receipt.Action)
		require.Equal(t, toolSource, receipt.Source)
		require.True(t, receipt.Complete && receipt.Released)
		require.Equal(t, partial, receipt.Pending)
		return receipt, raw, code
	}
	readArgs := []string{"--timeout=60s", "--selector-file=" + selectorPath, "--selector-sha256=" + digest(selectorRaw), "--scope-dir=" + scope, "--protection-file=" + protectionPath, "--protection-sha256=" + digest(protectionRaw)}
	planned, _, _ := call("plan-cohort", append(append([]string{}, readArgs...), "--rule-batch"), true)
	indexRaw, e := privateRead(filepath.Join(scope, "graph-index.json"), 16<<20)
	require.NoError(t, e)
	var index graphIndex
	require.NoError(t, strictDecode(indexRaw, &index))
	require.Equal(t, digest(indexRaw), planned.MaterialSHA)
	require.NotNil(t, index.RuleBatch)
	require.False(t, index.RuleBatch.Started.Before(windowStarted))
	require.EqualValues(t, 501, index.Counts["mongo/answersheets"])
	require.EqualValues(t, 1001, index.Counts["mysql/assessment_score"])
	g := group{Format: 1, Kind: "mock_cohort", Operation: f.selector.Operation, ProtectedSHA: digest(protectionRaw), IAM: json.RawMessage(`{"user_ids":["800"],"profile_ids":["9000"],"login_identity_ids":[]}`), QS: qsScope{Graph: &graphBinding{ArchiveDirectory: archive, IndexSHA: digest(indexRaw), IndexSource: toolSource, SelectorSHA: digest(selectorRaw), RootIDs: []string{"44"}, WindowStarted: windowStarted, WindowExpires: windowStarted.Add(20 * time.Minute)}, Selector: f.selector.Selection, Binding: h.binding, Objects: []object{}, Exclusions: []string{}}}
	ownerRaw := encode(ownerConfirmation{1, "mock_cohort_owner_confirmation", g.Operation, "10001", digest(g.IAM), digest(encode(g.QS)), g.ProtectedSHA, true, true, []string{}})
	g.OwnerSHA = digest(ownerRaw)
	groupRaw := encode(g)
	freezeRaw := encode(freezeReceipt{1, "mock_cohort_frozen", g.Operation, digest(groupRaw), true, true, true, true})
	common := append(append([]string{}, readArgs...), "--group-file="+write("group.json", groupRaw), "--group-sha256="+digest(groupRaw), "--owner-confirmation-file="+write("owner.json", ownerRaw), "--frozen-receipt-file="+write("freeze.json", freezeRaw), "--frozen-receipt-sha256="+digest(freezeRaw), "--archive-dir="+archive)
	before := graphNativeSnapshot(t, h)
	guards, e := nativeProtectionSnapshot(ctx, h.store, f.protected)
	require.NoError(t, e)
	captured, raw, code := call("capture", append(append([]string{}, common...), "--rule-batch", "--frozen-rule-delete"), false)
	require.False(t, captured.Whole || captured.DryRun)
	entries, e := os.ReadDir(archive)
	require.NoError(t, e)
	for _, entry := range entries {
		require.False(t, entry.IsDir(), "same-window reuse must not create/re-run frozen-scope discovery")
	}
	require.Equal(t, before, graphNativeSnapshot(t, h))
	materialRaw, e := privateRead(filepath.Join(archive, "material.json"), 16<<20)
	require.NoError(t, e)
	require.Equal(t, digest(materialRaw), captured.MaterialSHA)
	var material graphArchive
	require.NoError(t, strictDecode(materialRaw, &material))
	proofRaw := encode(graphCaptureExecution{1, "mock_cohort_capture_execution", write("capture-stdout.json", raw), digest(raw), "exited", &code})
	common = append(common, "--material-sha256="+digest(materialRaw), "--capture-evidence-file="+write("capture-execution.json", proofRaw), "--capture-evidence-sha256="+digest(proofRaw))
	now := time.Now().UTC()
	approvalRaw := encode(graphApproval{approval: approval{Format: 1, Kind: "mock_cohort_action_approval", Action: "apply", Operation: g.Operation, GroupSHA: digest(groupRaw), ArchiveSHA: digest(materialRaw), ProtectedSHA: g.ProtectedSHA, ToolSource: toolSource, FreezeSHA: digest(freezeRaw), SettlementSHA: digest(freezeRaw), Issued: now.Add(-time.Millisecond), Expires: g.QS.Graph.WindowExpires}})
	// This explicit rule mode intentionally deletes the same frozen physical keys
	// after a business body changed; default full-preimage CAS remains separate.
	h.exec("UPDATE assessment_score SET raw_score=999 WHERE id=1")
	changed, e := h.client.Database(h.name).Collection("answersheets").UpdateOne(ctx, bson.D{{Key: "domain_id", Value: int64(700)}}, bson.D{{Key: "$set", Value: bson.D{{Key: "synthetic_body_changed_after_archive", Value: true}}}})
	require.NoError(t, e)
	require.EqualValues(t, 1, changed.MatchedCount)
	// The ordinary provider still refuses changed complete preimages without
	// deleting an earlier namespace or consuming any CLI action approval.
	ordinary := newGraphStore(h.store)
	defaultConflicts := 0
	for _, ref := range material.Targets {
		if ref.Name != "assessment_score" && ref.Name != "answersheets" {
			continue
		}
		rows, err := readGraphArchivePage(archive, material, ref)
		require.NoError(t, err)
		for _, row := range rows {
			match := ref.Name == "assessment_score" && string(row.Object.Key[0].Raw) == "1"
			if ref.Name == "answersheets" {
				v := bson.Raw(row.BSON).Lookup("domain_id")
				match = v.Type == bson.TypeInt64 && v.Int64() == 700
			}
			if match {
				require.Error(t, ordinary.mutateExactBatch(ctx, []record{row}, false))
				defaultConflicts++
			}
		}
	}
	require.Equal(t, 2, defaultConflicts)
	applied, applyRaw, applyCode := call("apply", append(append([]string{}, common...), "--rule-batch", "--frozen-rule-delete", "--approval-file="+write("apply-approval.json", approvalRaw), "--approval-sha256="+digest(approvalRaw), "--execute"), true)
	require.False(t, applied.Whole || applied.DryRun)
	verified, _, _ := call("verify", append(append([]string{}, common...), "--rule-batch", "--frozen-rule-delete"), true)
	require.False(t, verified.Whole || verified.DryRun)
	journal, e := openGraphJournal(archive, false)
	require.NoError(t, e)
	require.NoError(t, graphVerify(ctx, newGraphStore(h.store), material, archive, journal, false))
	require.NoError(t, journal.close())
	afterGuards, e := nativeProtectionSnapshot(ctx, h.store, f.protected)
	require.NoError(t, e)
	require.Equal(t, guards, afterGuards)
	// The ordinary default path independently performs global clearance. Its
	// whole receipt is distinct from the earlier true-but-partial batch receipts.
	cleared, _, _ := call("verify", common, false)
	require.True(t, cleared.Whole)
	require.False(t, cleared.DryRun)
	// Preserve the historical forward boundary. New clearance is read-only and
	// must not manufacture a new historical Whole apply/verify receipt.
	markerPath := filepath.Join(archive, "forward-only.json")
	markerRaw := []byte(`{"fixture_business_reopened":true}`)
	require.NoError(t, writeNew(markerPath, markerRaw))
	rejected, code := cli.invoke(t, append(append([]string{}, common...), "--action=verify"))
	require.NotZero(t, code)
	require.Empty(t, rejected)
	journalRaw, e := privateRead(filepath.Join(archive, "graph-journal.jsonl"), 128<<20)
	require.NoError(t, e)
	applyPath := write("partial-apply-stdout.json", applyRaw)
	applyProofRaw := encode(graphCaptureExecution{1, "mock_cohort_apply_execution", applyPath, digest(applyRaw), "exited", &applyCode})
	applyProofPath := write("partial-apply-execution.json", applyProofRaw)
	currentFreezePath := write("current-freeze.json", freezeRaw)
	manifest := graphClearanceManifest{1, "mock_cohort_qs_clearance_inputs", g.Operation, []graphClearanceBatchInput{{
		Group:    graphClearanceFile{filepath.Join(dir, "group.json"), digest(groupRaw)},
		Owner:    graphClearanceFile{filepath.Join(dir, "owner.json"), digest(ownerRaw)},
		Selector: graphClearanceFile{selectorPath, digest(selectorRaw)}, ScopeDirectory: scope,
		MaterialSHA: digest(materialRaw), CaptureEvidence: graphClearanceFile{filepath.Join(dir, "capture-execution.json"), digest(proofRaw)},
		PartialApply: graphClearanceFile{applyPath, digest(applyRaw)}, PartialApplyExecution: graphClearanceFile{applyProofPath, digest(applyProofRaw)},
		JournalSHA: digest(journalRaw), CurrentFreeze: graphClearanceFile{currentFreezePath, digest(freezeRaw)},
	}}}
	manifestRaw := encode(manifest)
	clearanceRaw, exit := cli.invoke(t, []string{"--action=clearance", "--timeout=60s", "--clearance-manifest-file=" + write("clearance-inputs.json", manifestRaw), "--clearance-manifest-sha256=" + digest(manifestRaw), "--protection-file=" + protectionPath, "--protection-sha256=" + digest(protectionRaw)})
	require.Zero(t, exit)
	var result graphClearanceReceipt
	require.NoError(t, strictDecode(clearanceRaw, &result))
	require.True(t, result.Complete && result.Released && result.Absent && result.Protected && result.Clear)
	require.Equal(t, toolSource, result.Source)
	require.Len(t, result.Batches, 1)
	require.Equal(t, digest(applyRaw), result.Batches[0].PartialApplySHA)
	journalAfter, e := privateRead(filepath.Join(archive, "graph-journal.jsonl"), 128<<20)
	require.NoError(t, e)
	require.Equal(t, journalRaw, journalAfter)
	markerAfter, e := privateRead(markerPath, 32<<10)
	require.NoError(t, e)
	require.Equal(t, markerRaw, markerAfter)
	finalGuards, e := nativeProtectionSnapshot(ctx, h.store, f.protected)
	require.NoError(t, e)
	require.Equal(t, guards, finalGuards)
	t.Log("frozen_rule_actual_cli=true archived_raw_originals_once=true changed_body_same_physical_keys_deleted=true true_partial_apply=true later_readonly_clearance=true journal_and_forward_bytes_unchanged=true protected_unchanged=true synthetic_only=true")
}
