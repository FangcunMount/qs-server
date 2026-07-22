//go:build integration

package migration

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/pkg/mongodbtest"
)

func TestStatisticsMongoCollectorIndexesUpDown(t *testing.T) {
	_, db := mongodbtest.ReplicaSetDatabase(t)
	for _, name := range []string{"answersheets", "interpret_report_artifacts", "interpretation_runs"} {
		if err := db.CreateCollection(t.Context(), name); err != nil {
			t.Fatal(err)
		}
	}

	execMongoMigration(t, db, "000018_add_statistics_collector_indexes.up.json")
	assertMongoIndex(t, db.Collection("answersheets"), "idx_answersheets_statistics_org_filled", true)
	assertMongoIndex(t, db.Collection("interpret_report_artifacts"), "idx_artifacts_statistics_org_generated", true)
	assertMongoIndex(t, db.Collection("interpretation_runs"), "idx_interpretation_runs_statistics_org_failed", true)

	execMongoMigration(t, db, "000018_add_statistics_collector_indexes.down.json")
	assertMongoIndex(t, db.Collection("answersheets"), "idx_answersheets_statistics_org_filled", false)
	assertMongoIndex(t, db.Collection("interpret_report_artifacts"), "idx_artifacts_statistics_org_generated", false)
	assertMongoIndex(t, db.Collection("interpretation_runs"), "idx_interpretation_runs_statistics_org_failed", false)
}
