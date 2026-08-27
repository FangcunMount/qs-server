//go:build integration

package aiexplanation

import (
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/migration"
	"github.com/FangcunMount/qs-server/internal/pkg/mongodbtest"
)

func TestAIExplanationMigrationRuntimeIndexesConvergeOnReplicaSet(t *testing.T) {
	client, db := mongodbtest.ReplicaSetDatabase(t)
	if _, _, err := migration.NewMongoMigrator(client, &migration.Config{
		Enabled:  true,
		Database: db.Name(),
	}).Run(); err != nil {
		t.Fatalf("run MongoDB migrations: %v", err)
	}

	retention := RetentionPolicy{
		Version:                    "ai-explanation-retention-test-v1",
		ParticipantRecordRetention: 24 * time.Hour,
		PromptEvaluationRetention:  24 * time.Hour,
		CapacityLedgerRetention:    24 * time.Hour,
	}
	if _, err := NewPromptEvaluationRepository(db, retention); err != nil {
		t.Fatalf("initialize Prompt evaluation repository after migrations: %v", err)
	}
}
