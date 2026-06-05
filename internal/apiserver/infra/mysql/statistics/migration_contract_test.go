package statistics

import (
	"os"
	"strings"
	"testing"
)

func TestPlanFulfillmentStatisticsMigrationAddsPlannedCohortIndex(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("../../../../pkg/migration/migrations/mysql/000031_add_plan_fulfillment_statistics_indexes.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, token := range []string{
		"idx_task_org_deleted_planned_status",
		"`org_id`, `deleted_at`, `planned_at`, `status`, `plan_id`",
		"statistics_plan_daily",
		"idx_task_org_deleted_expire_status",
	} {
		if !strings.Contains(text, token) {
			t.Fatalf("migration does not contain %q", token)
		}
	}
}
