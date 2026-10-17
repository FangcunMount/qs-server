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
)

// Fresh explicit capture is followed by ordinary complete-preimage consumers
// and a real restore, proving the archived SQL/BSON originals are unchanged.
func TestMockCohortFreshFrozenCaptureRestoreCLINativeOwnedCycle(t *testing.T) {
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
	applied, _, _ := call("apply", append(append([]string{}, common...), "--rule-batch", "--approval-file="+write("apply-approval.json", approvalRaw), "--approval-sha256="+digest(approvalRaw), "--execute"), true)
	require.False(t, applied.Whole || applied.DryRun)
	verified, _, _ := call("verify", append(append([]string{}, common...), "--rule-batch"), true)
	require.False(t, verified.Whole || verified.DryRun)
	journal, e := openGraphJournal(archive, false)
	require.NoError(t, e)
	require.NoError(t, graphVerify(ctx, newGraphStore(h.store), material, archive, journal, false))
	require.NoError(t, journal.close())
	afterGuards, e := nativeProtectionSnapshot(ctx, h.store, f.protected)
	require.NoError(t, e)
	require.Equal(t, guards, afterGuards)
	// Restore intentionally omits the explicit fast flags. The original full
	// coverage/fingerprint/CAS/unique checks must accept every archived value.
	now = time.Now().UTC()
	restoreApproval := graphApproval{approval: approval{Format: 1, Kind: "mock_cohort_action_approval", Action: "restore", Operation: g.Operation, GroupSHA: digest(groupRaw), ArchiveSHA: digest(materialRaw), ProtectedSHA: g.ProtectedSHA, ToolSource: toolSource, FreezeSHA: digest(freezeRaw), SettlementSHA: digest(freezeRaw), Issued: now.Add(-time.Millisecond), Expires: now.Add(10 * time.Minute)}, BeforeReopen: true, RecoveryStarted: now.Add(-time.Second), RecoveryExpires: now.Add(10 * time.Minute)}
	restoreRaw := encode(restoreApproval)
	restored, _, _ := call("restore", append(append([]string{}, common...), "--approval-file="+write("restore-approval.json", restoreRaw), "--approval-sha256="+digest(restoreRaw), "--execute"), false)
	require.False(t, restored.Whole || restored.DryRun)
	original, _, _ := call("verify", append(append([]string{}, common...), "--expect-original"), false)
	require.False(t, original.Whole || original.DryRun)
	require.Equal(t, before, graphNativeSnapshot(t, h), "fresh explicit capture must preserve every exact SQL value and BSON byte/type for default restoration")
	finalGuards, e := nativeProtectionSnapshot(ctx, h.store, f.protected)
	require.NoError(t, e)
	require.Equal(t, guards, finalGuards)
	t.Log("fresh_frozen_capture_actual_cli=true raw_sql_bson_restored=true default_consumers_unchanged=true protected_unchanged=true synthetic_only=true")
}
