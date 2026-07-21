package plan

import (
	"os"
	"strings"
	"testing"
)

func TestPlanEnrollmentMigrationDefinesRoundAndActiveConstraints(t *testing.T) {
	data, err := os.ReadFile("../../../../pkg/migration/migrations/mysql/000051_add_plan_enrollment.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, token := range []string{
		"CREATE TABLE IF NOT EXISTS `plan_enrollment`",
		"`active_slot` TINYINT GENERATED ALWAYS AS",
		"UNIQUE KEY `uk_plan_enrollment_round`",
		"UNIQUE KEY `uk_plan_enrollment_active`",
		"ADD COLUMN `enrollment_id`",
		"ADD COLUMN `expired_at`",
		"ADD COLUMN `canceled_at`",
		"ADD UNIQUE KEY `uk_enrollment_seq` (`enrollment_id`, `seq`)",
		"'derived_legacy'",
	} {
		if !strings.Contains(text, token) {
			t.Fatalf("migration does not contain %q", token)
		}
	}
}
