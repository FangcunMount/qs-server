//go:build integration

package migration

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/pkg/mongodbtest"
)

func TestMongoTransactionAuditIndexesUpDown(t *testing.T) {
	_, db := mongodbtest.ReplicaSetDatabase(t)
	collections := []string{"answersheets", "report_generations", "interpretation_runs", "domain_event_outbox"}
	for _, name := range collections {
		if err := db.CreateCollection(t.Context(), name); err != nil {
			t.Fatal(err)
		}
	}

	execMongoMigration(t, db, "000024_add_mongo_transaction_audit_indexes.up.json")
	for collection, indexes := range map[string][]string{
		"answersheets":        {"uk_answersheet_durable_event_id", "idx_answersheet_durable_audit"},
		"report_generations":  {"idx_generation_tx_schema_status_domain"},
		"interpretation_runs": {"idx_interpretation_run_retry_audit"},
		"domain_event_outbox": {"idx_outbox_consistency_audit"},
	} {
		for _, index := range indexes {
			assertMongoIndex(t, db.Collection(collection), index, true)
		}
	}

	execMongoMigration(t, db, "000024_add_mongo_transaction_audit_indexes.down.json")
	for collection, indexes := range map[string][]string{
		"answersheets":        {"uk_answersheet_durable_event_id", "idx_answersheet_durable_audit"},
		"report_generations":  {"idx_generation_tx_schema_status_domain"},
		"interpretation_runs": {"idx_interpretation_run_retry_audit"},
		"domain_event_outbox": {"idx_outbox_consistency_audit"},
	} {
		for _, index := range indexes {
			assertMongoIndex(t, db.Collection(collection), index, false)
		}
	}
}
