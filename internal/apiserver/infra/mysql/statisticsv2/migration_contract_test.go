package statisticsv2

import (
	"os"
	"strings"
	"testing"
)

func TestStatisticsV2MigrationIsAdditiveAndDefinesContracts(t *testing.T) {
	data, err := os.ReadFile("../../../../pkg/migration/migrations/mysql/000052_add_statistics_v2.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, table := range []string{
		"statistics_access_fact", "statistics_assessment_fact", "statistics_plan_fact",
		"statistics_access_daily", "statistics_assessment_daily", "statistics_plan_activity_daily",
		"statistics_plan_fulfillment_daily", "statistics_org_snapshot", "statistics_sync_run",
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
