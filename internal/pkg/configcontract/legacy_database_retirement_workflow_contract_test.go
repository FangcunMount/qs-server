package configcontract

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLegacyDatabaseRetirementWorkflowProtectsDestructiveApply(t *testing.T) {
	t.Parallel()

	workflow, err := os.ReadFile(filepath.Join(repoRoot(t), ".github", "workflows", "legacy-database-retirement.yml"))
	if err != nil {
		t.Fatalf("read legacy database retirement workflow: %v", err)
	}
	content := string(workflow)

	for _, required := range []string{
		"preflight",
		"backup",
		"apply",
		"verify",
		"legacy-db-retirement-20260806-v1",
		"source_run_id=31070267072",
		"DROP-LEGACY-DB-OBJECTS-20260806-V1",
		"apply requires the exact retirement confirmation",
		"expected exactly 5 MySQL retirement targets",
		"expected exactly 11 MongoDB retirement targets",
		"mysqldump --protocol=tcp",
		"--no-data",
		"mongodump",
		"mongorestore",
		"--dryRun",
		"--file=/tmp/retirement-preflight.js",
		"chmod 0444 \"$MONGO_CHECK_SCRIPT\"",
		"MongoDB retirement preflight failed after $ATTEMPT attempts",
		"MongoDB backup failed after $ATTEMPT attempts",
		"MongoDB backup validation failed after $ATTEMPT attempts",
		"MongoDB backup revalidation failed after $ATTEMPT attempts",
		"--user 0:0",
		"--entrypoint /usr/bin/mongodump",
		"--entrypoint /usr/bin/mongorestore",
		"/backup/.legacy-db-retirement-20260806-v1.partial",
		"/backup/legacy-db-retirement-20260806-v1",
		"retirement preflight completed; no data was changed",
		"verified retirement backup is restorable",
		"DROP TABLE IF EXISTS analytics_scan_watermarks",
		"targetDB.getCollection(name).drop()",
		"remaining_retirement_targets",
		"remaining_mysql_retirement_targets",
	} {
		if !strings.Contains(content, required) {
			t.Errorf("protected retirement workflow must contain %q", required)
		}
	}
}

func TestLegacyDatabaseRetirementWorkflowUsesExactAllowlist(t *testing.T) {
	t.Parallel()

	workflow, err := os.ReadFile(filepath.Join(repoRoot(t), ".github", "workflows", "legacy-database-retirement.yml"))
	if err != nil {
		t.Fatalf("read legacy database retirement workflow: %v", err)
	}
	content := string(workflow)

	for _, target := range []string{
		"analytics_scan_watermarks",
		"seed_orphan_outbox_bak_hist_v1_20260802_r1",
		"seed_orphan_outbox_bak_hist_v1_orphans_20260802",
		"seed_orphan_stats_bak_hist_v1_20260802_r1",
		"seed_orphan_stats_bak_hist_v1_orphans_20260802",
		"assessment_models_legacy_20260717_084636",
		"domain_event_outbox__seed_orphan_backup_hist_v1_20260802_r1",
		"domain_event_outbox__seed_orphan_backup_hist_v1_orphans_20260802",
		"interpret_report_artifacts__seed_orphan_backup_hist_v1_20260802_r1",
		"interpret_report_artifacts__seed_orphan_backup_hist_v1_orphans_20260802",
		"interpretation_runs__seed_orphan_backup_hist_v1_20260802_r1",
		"interpretation_runs__seed_orphan_backup_hist_v1_orphans_20260802",
		"report_generations__seed_orphan_backup_hist_v1_20260802_r1",
		"report_generations__seed_orphan_backup_hist_v1_orphans_20260802",
		"report_query_catalog__seed_orphan_backup_hist_v1_20260802_r1",
		"report_query_catalog__seed_orphan_backup_hist_v1_orphans_20260802",
	} {
		if !strings.Contains(content, target) {
			t.Errorf("retirement workflow allowlist must contain %q", target)
		}
	}
}
