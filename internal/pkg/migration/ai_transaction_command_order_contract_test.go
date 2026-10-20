package migration

import (
	"strings"
	"testing"
)

func TestAITransactionCommandOrderIsExplicitAdditiveAndRetainsHistory(t *testing.T) {
	r, err := compiledMigrationResources()
	if err != nil {
		t.Fatal(err)
	}
	if !r.accepts(108, 42) || !r.accepts(107, 42) || r.accepts(108, 41) {
		t.Fatal("command order installation requires the declared full schema pair")
	}
	up := readMySQLMigration(t, "000108_ai_transaction_command_order.up.sql")
	if !strings.Contains(up, "ADD UNIQUE KEY uk_ai_messaging_command_sequence (aggregate_key,aggregate_sequence)") {
		t.Fatal("missing exact command sequence uniqueness")
	}
	for _, forbidden := range []string{"DELETE FROM", "UPDATE AI_", "DROP TABLE", "DROP COLUMN"} {
		if strings.Contains(strings.ToUpper(up), forbidden) {
			t.Fatal("installation must reject conflicts, never rewrite history", forbidden)
		}
	}
	down := readMySQLMigration(t, "000108_ai_transaction_command_order.down.sql")
	if !strings.Contains(down, "SIGNAL") || strings.Contains(strings.ToUpper(down), "DROP ") {
		t.Fatal("automatic downgrade cannot remove command order protection")
	}
}
