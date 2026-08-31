//go:build integration

package aiexplanation

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	domainai "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation"
	domainevaluation "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/evaluation"
	"github.com/FangcunMount/qs-server/internal/pkg/mongodbtest"
	"github.com/stretchr/testify/require"
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

func TestPromptEvaluationEvidenceV2RepositoryRoundTripsNearMongoLimit(t *testing.T) {
	_, db := mongodbtest.ReplicaSetDatabase(t)
	repository, err := NewPromptEvaluationRepository(db, RetentionPolicy{
		Version: "integration-v2-near-limit", ParticipantRecordRetention: 24 * time.Hour,
		PromptEvaluationRetention: 24 * time.Hour, CapacityLedgerRetention: 24 * time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	evidence := completeMapperEvidenceV2(t)
	inflateMapperEvidenceV2Outputs(t, evidence, 108<<10)
	po, err := NewMapper().PromptEvaluationEvidenceV2ToPO(evidence)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := bson.Marshal(po)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("complete v2 evidence BSON size: %d bytes", len(raw))
	if len(raw) < 14<<20 || len(raw) >= 16<<20 {
		t.Fatalf("near-limit v2 BSON size = %d, want [14 MiB, 16 MiB)", len(raw))
	}
	if err := repository.CreateEvidenceV2(t.Context(), evidence); err != nil {
		t.Fatal(err)
	}
	restored, err := repository.FindEvidenceV2ByID(t.Context(), evidence.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if restored.Status != domainevaluation.EvidenceStatusAwaitingReview ||
		len(restored.GenerationExecutions) != domainevaluation.RequiredGenerationAttempts ||
		len(restored.SemanticExecutions) != domainevaluation.RequiredGenerationAttempts ||
		!bytes.Equal(restored.GenerationExecutions[0].NormalizedOutput, evidence.GenerationExecutions[0].NormalizedOutput) ||
		!bytes.Equal(restored.SemanticExecutions[len(restored.SemanticExecutions)-1].NormalizedOutput, evidence.SemanticExecutions[len(evidence.SemanticExecutions)-1].NormalizedOutput) {
		t.Fatalf("near-limit v2 evidence did not round-trip: status=%s generation=%d semantic=%d",
			restored.Status, len(restored.GenerationExecutions), len(restored.SemanticExecutions))
	}
}

func inflateMapperEvidenceV2Outputs(t *testing.T, evidence *domainevaluation.PromptEvaluationEvidenceV2, outputBytes int) {
	t.Helper()
	generationOutput := mapperSizedJSON(t, "generation", outputBytes)
	semanticOutput := mapperSizedJSON(t, "semantic", outputBytes)
	generationFingerprint := domainai.NewFingerprint(generationOutput)
	semanticFingerprint := domainai.NewFingerprint(semanticOutput)
	for index := range evidence.GenerationExecutions {
		execution := &evidence.GenerationExecutions[index]
		execution.RawOutput = append([]byte(nil), generationOutput...)
		execution.NormalizedOutput = append([]byte(nil), generationOutput...)
		execution.NormalizedOutputFingerprint = generationFingerprint
		for slotIndex := range evidence.Slots {
			candidate := evidence.Slots[slotIndex].Candidate
			if candidate != nil && candidate.GenerationExecutionID == execution.ID {
				candidate.NormalizedOutputFingerprint = generationFingerprint
				break
			}
		}
	}
	for index := range evidence.SemanticExecutions {
		execution := &evidence.SemanticExecutions[index]
		execution.RawOutput = append([]byte(nil), semanticOutput...)
		execution.NormalizedOutput = append([]byte(nil), semanticOutput...)
		execution.Result.OutputFingerprint = semanticFingerprint
	}
	require.NoError(t, evidence.Validate())
}

func mapperSizedJSON(t *testing.T, label string, size int) []byte {
	t.Helper()
	prefix, suffix := fmt.Sprintf(`{"payload":"%s:`, label), `"}`
	require.GreaterOrEqual(t, size, len(prefix)+len(suffix))
	return []byte(prefix + strings.Repeat("x", size-len(prefix)-len(suffix)) + suffix)
}
