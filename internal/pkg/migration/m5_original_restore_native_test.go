//go:build integration

package migration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"

	drivermysql "github.com/go-sql-driver/mysql"
)

// Every database is allocated by this test. The real installed CLI is exercised;
// no production database, approval, first-start receipt or acceptance is claimed.
func TestSchemaPairNativeM5OriginalRestoreCLI(t *testing.T) {
	repo, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	sourceRaw, err := exec.Command("git", "-C", repo, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	source := strings.TrimSpace(string(sourceRaw))
	bin := filepath.Join(t.TempDir(), "qs-migrate-offline")
	build := exec.CommandContext(t.Context(), "go", "build", "-ldflags", "-X github.com/FangcunMount/qs-server/pkg/version.GitCommit="+source, "-o", bin, "./scripts/maintenance/qs-migrate-offline")
	build.Dir = repo
	if raw, err := build.CombinedOutput(); err != nil {
		t.Fatalf("CLI build: %v %s", err, raw)
	}
	old := migrationResourcesThrough(t, 110, 44)
	current, err := compiledMigrationResources()
	if err != nil {
		t.Fatal(err)
	}
	writePrivate := func(t *testing.T, path string, value any) string {
		t.Helper()
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(path, raw, 0600); err != nil {
			t.Fatal(err)
		}
		return retirementHash(raw)
	}
	cases := []string{"dirty_before_ddl", "dirty_after_drop", "clean_before_runtime"}
	for _, stage := range cases {
		t.Run(stage, func(t *testing.T) {
			n := openNativeSchemaPairWithPrefix(t, "qs_m5_restore_cli_test")
			n.approve(t, old)
			n.runBoth(t, n.preflight(t, old))
			n.cfg.ExpectedSourceSHA = source
			if _, err := n.db.ExecContext(t.Context(), "ALTER TABLE schema_migrations DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci; CREATE TABLE m5_retained_cli_fixture(id INT PRIMARY KEY, original_bytes BLOB); INSERT INTO m5_retained_cli_fixture VALUES(1,X'0001FF')"); err != nil {
				t.Fatal(err)
			}
			var fixtureAccount string
			invoke := func(args ...string) (M5RestoreResult, []byte, bool) {
				cfg, err := drivermysql.ParseDSN(os.Getenv("QS_M3_TEST_MYSQL_DSN"))
				if err != nil {
					t.Fatal(err)
				}
				cfg.DBName = n.cfg.MySQLDatabase
				if fixtureAccount != "" {
					cfg.User, cfg.Passwd = fixtureAccount, "owned-restore-test-only"
				}
				base := []string{"--mysql-database", n.cfg.MySQLDatabase, "--mongo-database", n.cfg.MongoDatabase, "--timeout", "90s"}
				ctx, cancel := context.WithTimeout(t.Context(), 100*time.Second)
				defer cancel()
				cmd := exec.CommandContext(ctx, bin, append(base, args...)...)
				cmd.Dir = repo
				cmd.Env = append(os.Environ(), "MYSQL_TLS_MODE=DISABLED", "QS_OFFLINE_MYSQL_DSN="+cfg.FormatDSN(), "QS_OFFLINE_MONGO_URI="+os.Getenv("QS_SERVER_TEST_MONGO_URI"))
				cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
				cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
				cmd.WaitDelay = 5 * time.Second
				var raw bytes.Buffer
				cmd.Stdout = &raw
				cmd.Stderr = &bytes.Buffer{}
				err = cmd.Run()
				var out M5RestoreResult
				_ = json.Unmarshal(raw.Bytes(), &out)
				return out, raw.Bytes(), err == nil
			}
			_, raw, ok := invoke("--retained-baseline")
			var baseline RetainedBaseline
			if !ok || json.Unmarshal(raw, &baseline) != nil || !baseline.Complete {
				t.Fatal("actual CLI full baseline refused")
			}
			_, raw, ok = invoke("--capture-original-operation", fmt.Sprintf("%d-1", time.Now().UnixNano()))
			var material M5OriginalMaterial
			if !ok || json.Unmarshal(raw, &material) != nil || !reflect.DeepEqual(material.Baseline.Objects, baseline.Objects) {
				t.Fatal("actual original export failed")
			}
			dir := t.TempDir()
			original := filepath.Join(dir, "original.json")
			digest := writePrivate(t, original, material)
			if stage == "dirty_before_ddl" {
				if _, err := n.db.ExecContext(t.Context(), "UPDATE schema_migrations SET version=111,dirty=TRUE WHERE version=110 AND dirty=FALSE"); err != nil {
					t.Fatal(err)
				}
			} else {
				n.runBoth(t, n.preflight(t, current))
				if stage == "dirty_after_drop" {
					if _, err := n.db.ExecContext(t.Context(), "UPDATE schema_migrations SET dirty=TRUE WHERE version=111"); err != nil {
						t.Fatal(err)
					}
				}
			}
			base := []string{"--restore-original", original, "--restore-original-sha256", digest}
			inspect, _, ok := invoke(base...)
			if !ok || inspect.Stage != "inspection_complete" || inspect.AuthorizationConsumed || inspect.MutationAttempted || inspect.State == nil || inspect.State.SQLVersion != 111 {
				t.Fatal("actual interrupted inspection failed", inspect)
			}
			now := time.Now().UTC().Truncate(time.Second)
			proof := strings.Repeat("a", 64)
			approval := M5RestoreApproval{FormatVersion: 1, Kind: "qs_m5_original_empty_restore", OperationID: material.OperationID, ApprovedSourceSHA: source, OriginalSHA256: digest, ObservedStateSHA256: inspect.ObservedStateSHA256, FreezeReceiptSHA256: proof, BoundaryReceiptSHA256: proof, ReviewSHA256: proof, ExpiresAt: now.Add(10 * time.Minute).Format(time.RFC3339), RecoveryStartedAt: now.Format(time.RFC3339), DataOwner: "owned-fixture", RecoveryOwner: "owned-fixture", RuntimeFrozen: true, RuntimeNotStarted: true}
			approvePath := filepath.Join(dir, "restore-approval.json")
			for _, mutate := range []func(*M5RestoreApproval){func(a *M5RestoreApproval) { a.RuntimeNotStarted = false }, func(a *M5RestoreApproval) { a.RuntimeFrozen = false }, func(a *M5RestoreApproval) { a.ObservedStateSHA256 = strings.Repeat("f", 64) }, func(a *M5RestoreApproval) { a.ApprovedSourceSHA = strings.Repeat("f", 40) }} {
				a := approval
				mutate(&a)
				hash := writePrivate(t, approvePath, a)
				got, _, success := invoke(append(base, "--restore-apply", "--restore-approval-file", approvePath, "--restore-approval-sha256", hash)...)
				if success || got.AuthorizationConsumed || got.MutationAttempted {
					t.Fatal("pre-start negative consumed/mutated", got)
				}
			}
			// A new retained business fact refuses before approval consumption, even
			// with the previously observed state hash and intact target material.
			if _, err := n.db.ExecContext(t.Context(), "UPDATE m5_retained_cli_fixture SET original_bytes=X'02' WHERE id=1"); err != nil {
				t.Fatal(err)
			}
			approveHash := writePrivate(t, approvePath, approval)
			got, _, success := invoke(append(base, "--restore-apply", "--restore-approval-file", approvePath, "--restore-approval-sha256", approveHash)...)
			if success || got.AuthorizationConsumed || got.LedgerReplayAttempted {
				t.Fatal("changed retained fact overwritten", got)
			}
			if _, err := n.db.ExecContext(t.Context(), "UPDATE m5_retained_cli_fixture SET original_bytes=X'0001FF' WHERE id=1"); err != nil {
				t.Fatal(err)
			}
			if stage == "dirty_after_drop" {
				// The target DDL succeeds but the ledger INSERT is denied after
				// DELETE. The actual transaction must roll back the complete ledger.
				account := "m5_restore_" + strings.TrimPrefix(n.cfg.MySQLDatabase, "qs_m5_restore_cli_test_")
				if _, err := n.db.ExecContext(t.Context(), "CREATE USER '"+account+"'@'%' IDENTIFIED BY 'owned-restore-test-only'; GRANT SELECT,SHOW VIEW,TRIGGER,EVENT,CREATE ROUTINE,ALTER ROUTINE,EXECUTE,DROP,LOCK TABLES,CREATE,DELETE ON "+quotedSQLIdentifier(n.cfg.MySQLDatabase)+".* TO '"+account+"'@'%'"); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _, _ = n.db.ExecContext(context.Background(), "DROP USER IF EXISTS '"+account+"'@'%'") })
				fixtureAccount = account
				failed, _, success := invoke(append(base, "--restore-apply", "--restore-approval-file", approvePath, "--restore-approval-sha256", approveHash)...)
				fixtureAccount = ""
				if success || !failed.AuthorizationConsumed || !failed.LedgerReplayAttempted || failed.CommitOutcomeUnknown || failed.Verified {
					t.Fatal("actual ledger fault was not retained", failed)
				}
				version, dirty, mongoVersion, mongoDirty := n.heads(t)
				if version != 111 || !dirty || mongoVersion != 44 || mongoDirty {
					t.Fatal("failed replay cleared interrupted ledger")
				}
				inspection, _, success := invoke(base...)
				if !success || inspection.State == nil || !inspection.State.TargetPresent {
					t.Fatal("partial DDL not available for inspect-only recovery", inspection)
				}
				approval.ObservedStateSHA256 = inspection.ObservedStateSHA256
				approvePath = filepath.Join(dir, "independent-after-failure.json")
				approveHash = writePrivate(t, approvePath, approval)
			}
			got, _, success = invoke(append(base, "--restore-apply", "--restore-approval-file", approvePath, "--restore-approval-sha256", approveHash)...)
			if !success || !got.Verified || !got.AuthorizationConsumed || !got.LedgerReplayAttempted || got.CommitOutcomeUnknown || got.Stage != "complete" || got.State.SQLVersion != 110 || got.State.SQLDirty || !reflect.DeepEqual(got.State.Objects, baseline.Objects) {
				t.Fatal("actual complete original replay failed", got)
			}
			if _, _, success := invoke(append(base, "--restore-apply", "--restore-approval-file", approvePath, "--restore-approval-sha256", approveHash)...); success {
				t.Fatal("consumed recovery approval reused")
			}
			if err := n.preflight(t, old).VerifyComplete(t.Context()); err != nil {
				t.Fatal("original110/44 compatibility failed", err)
			}
		})
	}
	if t.Failed() {
		return
	}
	report := map[string]any{"source_sha": source, "fixture_only": true, "actual_cli": true, "cases": cases, "complete_retained_assets_verified": true, "pre_start_and_changed_fact_refusals": true, "actual_ledger_insert_denied_rollback_preserved": true, "separate_one_use_approval": true, "generic_force_or_down_used": false, "production_or_system_acceptance": false, "passed": true}
	if path := os.Getenv("QS_M5_RESTORE_CLI_PROOF"); path != "" {
		writePrivate(t, path, report)
	}
	raw, _ := json.Marshal(report)
	t.Log("M5_QS_ORIGINAL_RESTORE_CLI_RECEIPT " + string(raw))
}
