package migration

import (
	"strings"
	"testing"
)

func TestRetireAnalyticsScanWatermarksMigrationContract(t *testing.T) {
	t.Parallel()

	up := readMySQLMigration(t, "000067_retire_analytics_scan_watermarks.up.sql")
	down := readMySQLMigration(t, "000067_retire_analytics_scan_watermarks.down.sql")

	if strings.Count(up, "DROP TABLE IF EXISTS `analytics_scan_watermarks`") != 1 {
		t.Fatal("up migration must drop analytics_scan_watermarks exactly once")
	}
	for _, token := range []string{
		"CREATE TABLE IF NOT EXISTS `analytics_scan_watermarks`",
		"UNIQUE KEY `uk_source_org` (`source_name`, `org_id`)",
		"KEY `idx_status_updated_at` (`status`, `updated_at`)",
	} {
		if !strings.Contains(down, token) {
			t.Fatalf("down migration must preserve retired schema token %q", token)
		}
	}
}
