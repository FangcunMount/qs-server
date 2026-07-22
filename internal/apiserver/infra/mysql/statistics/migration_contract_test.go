package statistics

import (
	"os"
	"strings"
	"testing"
)

func TestStatisticsMigrationIsAdditiveAndDefinesContracts(t *testing.T) {
	data, err := os.ReadFile("../../../../pkg/migration/migrations/mysql/000052_add_statistics_v2.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, table := range []string{
		"statistics_access_fact", "statistics_assessment_fact", "statistics_plan_fact",
		"statistics_access_daily", "statistics_assessment_daily", "statistics_plan_activity_daily",
		"statistics_plan_fulfillment_daily", "statistics_v2_org_snapshot", "statistics_sync_run",
	} {
		if !strings.Contains(text, "CREATE TABLE `"+table+"`") {
			t.Fatalf("migration does not create %s", table)
		}
	}
	for _, token := range []string{"DATETIME(3)", "`stat_date` DATE", "`fact_key`", "`core_hash`", "uk_statistics_access_fact_key", "uk_statistics_assessment_fact_key", "uk_statistics_plan_fact_key", "data_committed_at"} {
		if !strings.Contains(text, token) {
			t.Fatalf("migration does not contain %q", token)
		}
	}
	for _, forbidden := range []string{"DROP TABLE", "ALTER TABLE `statistics_"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("additive up migration contains forbidden token %q", forbidden)
		}
	}
}

func TestStatisticsRetirementMigrationLeavesOnlyCanonicalSchema(t *testing.T) {
	up, err := os.ReadFile("../../../../pkg/migration/migrations/mysql/000056_retire_statistics_v1.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	text := string(up)
	for _, table := range []string{
		"behavior_footprint", "assessment_episode", "analytics_pending_event",
		"statistics_journey_daily", "statistics_content_daily", "statistics_plan_daily", "statistics_org_snapshot",
	} {
		if !strings.Contains(text, "DROP TABLE IF EXISTS `"+table+"`") {
			t.Fatalf("retirement migration does not drop %s", table)
		}
	}
	for _, token := range []string{
		"RENAME TABLE `statistics_v2_org_snapshot` TO `statistics_org_snapshot`",
		"WHERE `scope` = 'analytics_projector'",
		"idx_enrollment_collect_joined", "idx_task_collect_completed", "idx_assessment_collect_failed",
	} {
		if !strings.Contains(text, token) {
			t.Fatalf("retirement migration does not contain %q", token)
		}
	}

	down, err := os.ReadFile("../../../../pkg/migration/migrations/mysql/000056_retire_statistics_v1.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	downText := string(down)
	if !strings.Contains(downText, "RENAME TABLE `statistics_org_snapshot` TO `statistics_v2_org_snapshot`") {
		t.Fatal("down migration does not restore the historical snapshot table name")
	}
	for _, table := range []string{"behavior_footprint", "assessment_episode", "analytics_pending_event", "statistics_journey_daily", "statistics_content_daily", "statistics_plan_daily", "statistics_org_snapshot"} {
		if !strings.Contains(downText, "CREATE TABLE `"+table+"`") {
			t.Fatalf("down migration does not restore empty %s", table)
		}
	}
}

func TestStatisticsRunStrengtheningMigrationIsAdditive(t *testing.T) {
	data, err := os.ReadFile("../../../../pkg/migration/migrations/mysql/000054_strengthen_statistics_v2_runs.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, token := range []string{
		"`run_mode`", "`cache_resume_count`", "`last_cache_resume_operator_id`",
		"`last_cache_resume_reason`", "`last_cache_resume_at`", "`last_cache_resume_status`",
		"idx_statistics_sync_run_org_status_started",
	} {
		if !strings.Contains(text, token) {
			t.Fatalf("run migration does not contain %q", token)
		}
	}
	for _, forbidden := range []string{"DROP TABLE", "DROP COLUMN"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("run strengthening migration contains forbidden token %q", forbidden)
		}
	}
}

func TestStatisticsPublicationMigrationPersistsGenerationAndResumeAudit(t *testing.T) {
	data, err := os.ReadFile("../../../../pkg/migration/migrations/mysql/000055_finalize_statistics_v2_publication.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, token := range []string{
		"`cache_generation`", "`cache_published_at`", "`cache_resume_audit_json`",
		"idx_statistics_sync_run_org_publication",
	} {
		if !strings.Contains(text, token) {
			t.Fatalf("publication migration does not contain %q", token)
		}
	}
	for _, forbidden := range []string{"DROP TABLE", "DROP COLUMN"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("publication migration contains forbidden token %q", forbidden)
		}
	}
}

func TestStatisticsCollectorIndexMigrationMatchesStableSourceScans(t *testing.T) {
	up, err := os.ReadFile("../../../../pkg/migration/migrations/mysql/000057_add_statistics_collector_indexes.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	text := string(up)
	for _, token := range []string{
		"idx_entry_resolve_collect", "(`org_id`, `deleted_at`, `resolved_at`, `id`)",
		"idx_entry_intake_collect", "(`org_id`, `deleted_at`, `intake_at`, `id`)",
		"idx_evaluation_outcome_collect", "(`org_id`, `evaluated_at`, `id`)",
	} {
		if !strings.Contains(text, token) {
			t.Fatalf("collector index migration does not contain %q", token)
		}
	}

	down, err := os.ReadFile("../../../../pkg/migration/migrations/mysql/000057_add_statistics_collector_indexes.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, index := range []string{"idx_entry_resolve_collect", "idx_entry_intake_collect", "idx_evaluation_outcome_collect"} {
		if !strings.Contains(string(down), "DROP KEY `"+index+"`") {
			t.Fatalf("collector index down migration does not remove %s", index)
		}
	}
}
