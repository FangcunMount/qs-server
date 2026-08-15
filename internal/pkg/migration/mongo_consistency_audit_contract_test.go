package migration

import (
	"strings"
	"testing"
)

func TestMongoConsistencyAuditCheckpointMigrationContract(t *testing.T) {
	up := readMySQLMigration(t, "000069_add_mongo_consistency_audit_checkpoint.up.sql")
	down := readMySQLMigration(t, "000069_add_mongo_consistency_audit_checkpoint.down.sql")
	for _, token := range []string{
		"CREATE TABLE `mongo_consistency_audit_checkpoint`",
		"`phase` varchar(64) NOT NULL",
		"`cursor` bigint unsigned NOT NULL",
		"`cycle_upper_bound` bigint unsigned NOT NULL",
		"`statistics_json` json NOT NULL",
		"PRIMARY KEY (`checkpoint_key`)",
	} {
		if !strings.Contains(up, token) {
			t.Fatalf("up migration is missing %q", token)
		}
	}
	if !strings.Contains(down, "DROP TABLE IF EXISTS `mongo_consistency_audit_checkpoint`") {
		t.Fatal("down migration does not drop mongo consistency checkpoint")
	}
}
