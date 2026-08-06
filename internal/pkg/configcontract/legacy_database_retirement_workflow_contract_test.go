package configcontract

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLegacyDatabaseRetirementWorkflowStartsReadOnly(t *testing.T) {
	t.Parallel()

	workflow, err := os.ReadFile(filepath.Join(repoRoot(t), ".github", "workflows", "legacy-database-retirement.yml"))
	if err != nil {
		t.Fatalf("read legacy database retirement workflow: %v", err)
	}
	content := string(workflow)

	for _, required := range []string{
		"preflight",
		"backup",
		"legacy-db-retirement-20260806-v1",
		"expected exactly 5 MySQL retirement targets",
		"expected exactly 11 MongoDB retirement targets",
		"mysqldump --protocol=tcp",
		"--no-data",
		"mongodump",
		"mongorestore",
		"--dryRun",
		"--file=/tmp/retirement-preflight.js",
		"retirement preflight completed; no data was changed",
	} {
		if !strings.Contains(content, required) {
			t.Errorf("read-only retirement workflow must contain %q", required)
		}
	}

	for _, forbidden := range []string{
		"DROP TABLE",
		"dropCollection",
		".drop()",
	} {
		if strings.Contains(content, forbidden) {
			t.Errorf("read-only retirement workflow must not contain %q", forbidden)
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
