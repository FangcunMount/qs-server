//go:build integration

package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
)

// The seed uses the original owned SQL 1..101/Mongo 1..39 providers and the
// existing complete mixed graph, before its separate discovery-negative tests
// introduce aliases or foreign links. No application/consumer process is run.
func TestMockCohortGraphCLINativeOwnedCycle(t *testing.T) {
	f := seedCohortGraphNative(t)
	h := f.h
	ctx, cancel := context.WithTimeout(t.Context(), 6*time.Minute)
	defer cancel()
	dir, scopeDir, archiveDir := privateTestDir(t), privateTestDir(t), privateTestDir(t)
	cfg, err := mysqldriver.ParseDSN(os.Getenv("QS_MOCK_NATIVE_MYSQL_DSN"))
	require.NoError(t, err)
	cfg.DBName, cfg.MultiStatements = h.name, false
	write := func(t *testing.T, name string, raw []byte) string {
		t.Helper()
		path := filepath.Join(dir, name)
		require.NoError(t, writeNew(path, raw))
		return path
	}
	dsnFile := write(t, "mysql-dsn", []byte(cfg.FormatDSN()))
	uriFile := write(t, "mongo-uri", []byte(os.Getenv("QS_MOCK_NATIVE_MONGO_URI")))
	cli := graphCLINative{ctx: ctx, dir: dir, mysql: dsnFile, mongo: uriFile}
	protectionRaw, selectorRaw := encode(f.protected), encode(f.selector)
	protectionPath := write(t, "protection.json", protectionRaw)
	selectorPath := write(t, "selector.json", selectorRaw)
	before := graphNativeSnapshot(t, h)
	guards, err := nativeProtectionSnapshot(ctx, h.store, f.protected)
	require.NoError(t, err)
	var index graphIndex
	var g group
	var common []string
	if !t.Run("PlanCohortAndOwnerBoundGraphPlan", func(t *testing.T) {
		raw, code := cli.invoke(t, []string{"--action=plan-cohort", "--timeout=60s", "--selector-file=" + selectorPath, "--selector-sha256=" + digest(selectorRaw), "--protection-file=" + protectionPath, "--protection-sha256=" + digest(protectionRaw), "--scope-dir=" + scopeDir})
		require.Equal(t, 0, code)
		var planned actionReceipt
		require.NoError(t, strictDecode(raw, &planned))
		require.Equal(t, "plan-cohort", planned.Action)
		require.True(t, planned.Complete && planned.DryRun && planned.Pending && planned.Released)
		require.False(t, planned.Whole)
		indexRaw, er := privateRead(filepath.Join(scopeDir, "graph-index.json"), 16<<20)
		require.NoError(t, er)
		require.NoError(t, strictDecode(indexRaw, &index))
		require.Equal(t, digest(indexRaw), planned.MaterialSHA)
		require.True(t, index.DiscoveryComplete && index.ReadOnly && index.Released)
		require.True(t, index.WritePending, "discovery alone is not successful whole-graph deletion")
		require.Empty(t, index.Holds)
		require.EqualValues(t, 1001, index.Counts["mysql/assessment_score"])
		require.EqualValues(t, 501, index.Counts["mongo/answersheets"])
		now := time.Now().UTC()
		g = group{Format: 1, Kind: "mock_cohort", Operation: f.selector.Operation, ProtectedSHA: digest(protectionRaw), IAM: json.RawMessage(`{"user_ids":["800"],"profile_ids":["9000"],"login_identity_ids":[]}`), QS: qsScope{Graph: &graphBinding{ArchiveDirectory: archiveDir, IndexSHA: digest(indexRaw), IndexSource: toolSource, SelectorSHA: digest(selectorRaw), RootIDs: []string{"44"}, WindowStarted: now.Add(-time.Second), WindowExpires: now.Add(20 * time.Minute)}, Selector: f.selector.Selection, Binding: h.binding, Objects: []object{}, Exclusions: []string{}}}
		ownerRaw := encode(ownerConfirmation{1, "mock_cohort_owner_confirmation", g.Operation, "10001", digest(g.IAM), digest(encode(g.QS)), g.ProtectedSHA, true, true, []string{}})
		g.OwnerSHA = digest(ownerRaw)
		groupRaw := encode(g)
		groupPath := write(t, "group.json", groupRaw)
		ownerPath := write(t, "owner.json", ownerRaw)
		freezeRaw := encode(freezeReceipt{1, "mock_cohort_frozen", g.Operation, digest(groupRaw), true, true, true, true})
		freezePath := write(t, "freeze.json", freezeRaw)
		common = []string{"--timeout=60s", "--selector-file=" + selectorPath, "--selector-sha256=" + digest(selectorRaw), "--scope-dir=" + scopeDir, "--protection-file=" + protectionPath, "--protection-sha256=" + digest(protectionRaw), "--group-file=" + groupPath, "--group-sha256=" + digest(groupRaw), "--owner-confirmation-file=" + ownerPath, "--frozen-receipt-file=" + freezePath, "--frozen-receipt-sha256=" + digest(freezeRaw), "--archive-dir=" + archiveDir}
		r := cli.action(t, "plan", common)
		require.True(t, r.DryRun)
		require.False(t, r.Whole)
		require.Equal(t, before, graphNativeSnapshot(t, h), "both CLI planning paths must leave all original facts unchanged")
	}) {
		return
	}
	var m graphArchive
	var materialSHA string
	if !t.Run("CaptureAndActualSuccessfulExitProof", func(t *testing.T) {
		args := append(append([]string{}, common...), "--action=capture")
		raw, code := cli.invoke(t, args)
		require.Equal(t, 0, code, "failed capture must never receive success evidence")
		var r actionReceipt
		require.NoError(t, strictDecode(raw, &r))
		require.Equal(t, "capture", r.Action)
		require.True(t, r.Complete && r.Released && !r.Pending)
		require.False(t, r.DryRun || r.Whole)
		materialRaw, er := privateRead(filepath.Join(archiveDir, "material.json"), 16<<20)
		require.NoError(t, er)
		materialSHA = digest(materialRaw)
		require.Equal(t, materialSHA, r.MaterialSHA)
		require.NoError(t, strictDecode(materialRaw, &m))
		require.NoError(t, validateGraphArchive(m, g, r.GroupSHA))
		require.NoError(t, validateGraphCoverage(m, g, f.protected, index, scopeDir, archiveDir))
		stdoutPath := write(t, "capture-stdout.json", raw)
		// This is the independent test executor's record of the observed child
		// process exit. The tool does not generate or certify its own evidence.
		evidenceRaw := encode(graphCaptureExecution{1, "mock_cohort_capture_execution", stdoutPath, digest(raw), "exited", &code})
		evidencePath := write(t, "capture-execution.json", evidenceRaw)
		require.NoError(t, readGraphCaptureProof(evidencePath, digest(evidenceRaw), m, materialSHA))
		common = append(common, "--material-sha256="+materialSHA, "--capture-evidence-file="+evidencePath, "--capture-evidence-sha256="+digest(evidenceRaw))
		require.Equal(t, before, graphNativeSnapshot(t, h), "complete capture cannot change original facts")
	}) {
		return
	}
	approve := func(t *testing.T, action string) []string {
		t.Helper()
		freezeRaw, er := privateRead(filepath.Join(dir, "freeze.json"), 32<<10)
		require.NoError(t, er)
		now := time.Now().UTC()
		a := graphApproval{approval: approval{Format: 1, Kind: "mock_cohort_action_approval", Action: action, Operation: g.Operation, GroupSHA: m.GroupSHA, ArchiveSHA: materialSHA, ProtectedSHA: g.ProtectedSHA, ToolSource: toolSource, FreezeSHA: digest(freezeRaw), SettlementSHA: digest(freezeRaw), Issued: now.Add(-100 * time.Millisecond), Expires: g.QS.Graph.WindowExpires}}
		if action == "restore" {
			a.BeforeReopen, a.RecoveryStarted, a.RecoveryExpires = true, now.Add(-time.Second), now.Add(10*time.Minute)
			a.Expires = a.RecoveryExpires
		}
		raw := encode(a)
		path := write(t, action+"-approval.json", raw)
		return append(append([]string{}, common...), "--approval-file="+path, "--approval-sha256="+digest(raw), "--execute")
	}
	if !t.Run("ApplyAndWholeGraphVerifyViaCLI", func(t *testing.T) {
		r := cli.action(t, "apply", approve(t, "apply"))
		require.True(t, r.Whole)
		require.False(t, r.DryRun)
		r = cli.action(t, "verify", common)
		require.True(t, r.Whole)
		// A real/shared subject's explicit parent link cannot hide behind its own
		// source_ref or Testee. Exercise the real SQL predicates and CLI handoff,
		// including equal numeric values from a different domain that must pass.
		for n, link := range []struct {
			table, column string
			parent        int
			blocked       bool
		}{
			{"statistics_assessment_fact", "report_id", 700, false},      // sheet ID, not a report
			{"statistics_assessment_fact", "answersheet_id", 101, false}, // assessment ID, not a sheet
			{"statistics_assessment_fact", "answersheet_id", 700, true},
			{"statistics_assessment_fact", "assessment_id", 101, true},
			{"statistics_assessment_fact", "outcome_id", 201, true},
			{"statistics_assessment_fact", "report_id", 501, true},
			{"statistics_assessment_fact", "enrollment_id", 501, true},
			{"statistics_assessment_fact", "task_id", 601, true},
			{"statistics_plan_fact", "enrollment_id", 501, true},
			{"statistics_plan_fact", "task_id", 601, true},
		} {
			id := 900000 + n
			columns, values := "", ""
			if link.table == "statistics_plan_fact" {
				columns, values = ",plan_id", ",500"
			}
			h.exec(fmt.Sprintf("INSERT INTO %s(id,org_id,fact_key,core_hash,fact_type,occurred_at,stat_date,source_type,source_ref,testee_id,`%s`%s) VALUES(?,1,?,REPEAT('f',64),'assessment_completed','2026-01-01','2026-01-01','assessment','102',45,?%s)", link.table, link.column, columns, values), id, fmt.Sprintf("foreign-native-parent-%d", n), link.parent)
			args := append(append([]string{}, common...), "--action=verify")
			raw, code := cli.invoke(t, args)
			if link.blocked {
				require.NotEqual(t, 0, code, "%s.%s must block a whole-cohort handoff", link.table, link.column)
				require.Empty(t, raw, "failed clearance emitted a success receipt")
			} else {
				require.Equal(t, 0, code, "equal numeric IDs in different domains widened clearance")
				var verified actionReceipt
				require.NoError(t, strictDecode(raw, &verified))
				require.True(t, verified.Whole)
			}
			h.exec("DELETE FROM "+link.table+" WHERE id=?", id)
		}
		r = cli.action(t, "verify", common)
		require.True(t, r.Whole, "removing only the synthetic exception must restore clearance")
		j, er := openGraphJournal(archiveDir, false)
		require.NoError(t, er)
		require.NotEmpty(t, j.latest)
		for _, e := range j.latest {
			require.Equal(t, "apply", e.Action)
			require.Equal(t, "committed", e.State)
		}
		require.NoError(t, j.close())
		afterGuards, er := nativeProtectionSnapshot(ctx, h.store, f.protected)
		require.NoError(t, er)
		require.True(t, reflect.DeepEqual(guards, afterGuards), "shared Plan/Entry, retained root and system/clack originals changed")
	}) {
		return
	}
	t.Run("RestoreAndVerifyOriginalPhysicalFacts", func(t *testing.T) {
		r := cli.action(t, "restore", approve(t, "restore"))
		require.False(t, r.Whole, "restoration is not an IAM deletion handoff")
		args := append(append([]string{}, common...), "--expect-original")
		r = cli.action(t, "verify", args)
		require.False(t, r.Whole)
		require.Equal(t, before, graphNativeSnapshot(t, h), "every original SQL value and physical BSON byte/type must be restored")
		afterGuards, er := nativeProtectionSnapshot(ctx, h.store, f.protected)
		require.NoError(t, er)
		require.Equal(t, guards, afterGuards)
		require.NoError(t, h.store.identity(ctx, h.binding))
		t.Logf("whole_graph_synthetic_cli_cycle=true sql_score_count=%d independent_answersheets_count=%d no_business_runtime_started=true production_qualified=false", m.Counts["mysql/assessment_score"], m.Counts["mongo/answersheets"])
	})
}

// This fixture child executes the unmodified production main/run entrypoint.
// An anchored parent selector never counts this transport helper as a native case.
func TestMockCohortGraphCLINativeChild(t *testing.T) {
	if os.Getenv("QS_MOCK_NATIVE_GRAPH_CLI_CHILD") != "1" {
		return
	}
	raw := os.Getenv("QS_MOCK_NATIVE_GRAPH_CLI_ARGS")
	var args []string
	if len(raw) > 32<<10 || json.Unmarshal([]byte(raw), &args) != nil {
		os.Exit(2)
	}
	os.Args = append([]string{os.Args[0]}, args...)
	main()
	os.Exit(0)
}

type graphCLINative struct {
	ctx               context.Context
	dir, mysql, mongo string
	sequence          int
}

func (c *graphCLINative) invoke(t *testing.T, args []string) ([]byte, int) {
	t.Helper()
	exe, er := os.Executable()
	require.NoError(t, er)
	ctx, cancel := context.WithTimeout(c.ctx, 75*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, "-test.run=^TestMockCohortGraphCLINativeChild$", "-test.v=false", "-test.timeout=70s")
	keys := map[string]string{"QS_MOCK_NATIVE_GRAPH_CLI_CHILD": "1", "QS_MOCK_NATIVE_GRAPH_CLI_ARGS": string(encode(args)), "QS_MOCK_MYSQL_DSN_FILE": c.mysql, "QS_MOCK_MONGO_URI_FILE": c.mongo}
	for _, s := range os.Environ() {
		key, _, found := bytes.Cut([]byte(s), []byte("="))
		if found {
			if _, replace := keys[string(key)]; !replace {
				cmd.Env = append(cmd.Env, s)
			}
		}
	}
	for k, v := range keys {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	var stdout, stderr graphCLINativeOutput
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	if err != nil {
		var exit *exec.ExitError
		require.True(t, errors.As(err, &exit), "CLI did not return a real process exit")
	}
	require.NoError(t, ctx.Err(), "unknown process timeout cannot qualify capture or mutation")
	require.False(t, stdout.overflow || stderr.overflow)
	require.NotNil(t, cmd.ProcessState)
	require.True(t, cmd.ProcessState.Exited())
	c.sequence++
	require.NoError(t, writeNew(filepath.Join(c.dir, fmt.Sprintf("actual-%02d-stdout.json", c.sequence)), stdout.Bytes()))
	require.NoError(t, writeNew(filepath.Join(c.dir, fmt.Sprintf("actual-%02d-stderr.json", c.sequence)), stderr.Bytes()))
	t.Logf("real_graph_cli_exit=%d stdout_sha256=%s stdout_base64=%s stderr_sha256=%s stderr_base64=%s", cmd.ProcessState.ExitCode(), digest(stdout.Bytes()), base64.StdEncoding.EncodeToString(stdout.Bytes()), digest(stderr.Bytes()), base64.StdEncoding.EncodeToString(stderr.Bytes()))
	return append([]byte{}, stdout.Bytes()...), cmd.ProcessState.ExitCode()
}
func (c *graphCLINative) action(t *testing.T, action string, common []string) actionReceipt {
	t.Helper()
	raw, code := c.invoke(t, append(append([]string{}, common...), "--action="+action))
	require.Equal(t, 0, code)
	var r actionReceipt
	require.NoError(t, strictDecode(raw, &r))
	require.Equal(t, "mock_cohort_qs_action_result", r.Kind)
	require.Equal(t, action, r.Action)
	require.Equal(t, toolSource, r.Source)
	require.True(t, r.Complete && r.Released && !r.Pending)
	require.Zero(t, r.Exclusions)
	return r
}

type graphCLINativeOutput struct {
	bytes.Buffer
	overflow bool
}

func (w *graphCLINativeOutput) Write(p []byte) (int, error) {
	n := len(p)
	if remain := (64 << 10) - w.Len(); len(p) > remain {
		p, w.overflow = p[:remain], true
	}
	_, _ = w.Buffer.Write(p)
	return n, nil
}
