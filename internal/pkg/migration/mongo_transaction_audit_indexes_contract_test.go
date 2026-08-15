package migration

import (
	"os"
	"strings"
	"testing"
)

func TestMongoTransactionAuditIndexesMigrationContract(t *testing.T) {
	t.Parallel()

	up, err := os.ReadFile("migrations/mongodb/000024_add_mongo_transaction_audit_indexes.up.json")
	if err != nil {
		t.Fatal(err)
	}
	body := string(up)
	for _, required := range []string{
		`"createIndexes": "answersheets"`,
		`"name": "uk_answersheet_durable_event_id"`,
		`"name": "idx_answersheet_durable_audit"`,
		`"createIndexes": "report_generations"`,
		`"name": "idx_generation_tx_schema_status_domain"`,
		`"createIndexes": "interpretation_runs"`,
		`"name": "idx_interpretation_run_retry_audit"`,
		`"createIndexes": "domain_event_outbox"`,
		`"name": "idx_outbox_consistency_audit"`,
	} {
		if !strings.Contains(body, required) {
			t.Fatalf("up migration missing %s", required)
		}
	}

	down, err := os.ReadFile("migrations/mongodb/000024_add_mongo_transaction_audit_indexes.down.json")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(down), `"drop":`) {
		t.Fatal("down migration must only remove compatible indexes, never business collections")
	}
}
