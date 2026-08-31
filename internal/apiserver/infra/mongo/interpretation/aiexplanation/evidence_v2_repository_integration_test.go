//go:build integration

package aiexplanation

import (
	"errors"
	"testing"
	"time"

	domainevaluation "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/evaluation"
	"github.com/FangcunMount/qs-server/internal/pkg/mongodbtest"
	"go.mongodb.org/mongo-driver/bson"
)

func TestPromptEvaluationEvidenceV2RepositoryUsesSameCollectionWithVersionIsolationAndCAS(t *testing.T) {
	_, db := mongodbtest.ReplicaSetDatabase(t)
	repository, err := NewPromptEvaluationRepository(db, RetentionPolicy{
		Version: "integration-v2", ParticipantRecordRetention: 24 * time.Hour,
		PromptEvaluationRetention: 24 * time.Hour, CapacityLedgerRetention: 24 * time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	evidence := newMapperEvidenceV2(t)
	if err := repository.CreateEvidenceV2(t.Context(), evidence); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.FindByID(t.Context(), evidence.RunID); !errors.Is(err, domainevaluation.ErrNotFound) {
		t.Fatalf("v1 reader observed v2 evidence: %v", err)
	}
	persisted, err := repository.FindEvidenceV2ByID(t.Context(), evidence.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Version() != evidence.Version() || persisted.Execution() == nil {
		t.Fatalf("persisted v2 evidence = %#v", persisted)
	}
	var discriminator struct {
		EvidenceVersion string `bson:"evidence_version"`
	}
	if err := db.Collection((PromptEvaluationRunPO{}).CollectionName()).FindOne(
		t.Context(), bson.M{"domain_id": evidence.RunID},
	).Decode(&discriminator); err != nil || discriminator.EvidenceVersion != PromptEvaluationEvidenceVersionV2 {
		t.Fatalf("v2 discriminator = %#v, %v", discriminator, err)
	}

	expectedVersion := persisted.Version()
	if err := persisted.ReleaseExpiredPreparation(persisted.Execution().LeaseExpiresAt); err != nil {
		t.Fatal(err)
	}
	if err := repository.SaveEvidenceV2(t.Context(), persisted, expectedVersion); err != nil {
		t.Fatal(err)
	}
	if err := repository.SaveEvidenceV2(t.Context(), persisted, expectedVersion); !errors.Is(err, domainevaluation.ErrConflict) {
		t.Fatalf("stale v2 save error = %v", err)
	}
	updated, err := repository.FindEvidenceV2ByID(t.Context(), evidence.RunID)
	if err != nil || updated.Execution() != nil || updated.Version() != expectedVersion+1 {
		t.Fatalf("updated v2 evidence = %#v, %v", updated, err)
	}
}
