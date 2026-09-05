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
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
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

func TestEvidenceV2CatalogScopesPagesAndCancellationReleasesKeys(t *testing.T) {
	_, db := mongodbtest.ReplicaSetDatabase(t)
	repository, err := NewPromptEvaluationRepository(db, RetentionPolicy{Version: "test-catalog", ParticipantRecordRetention: 24 * time.Hour, PromptEvaluationRetention: 24 * time.Hour, CapacityLedgerRetention: 24 * time.Hour})
	require.NoError(t, err)
	original := newMapperEvidenceV2(t)
	for i := 0; i < 3; i++ {
		e := original.Clone()
		e.RunID += meta.ID(i)
		require.NoError(t, e.Cancel("user:42", "superseded", false, e.Audit.CreatedAt.Add(time.Hour)))
		require.NoError(t, repository.CreateEvidenceV2(t.Context(), &e))
	}
	// A second organization and legacy document never appear in this projection.
	other := original.Clone()
	other.RunID += 100
	other.Audit.OrganizationID = 99
	require.NoError(t, other.Cancel("user:42", "other org", false, other.Audit.CreatedAt.Add(time.Hour)))
	require.NoError(t, repository.CreateEvidenceV2(t.Context(), &other))
	_, err = db.Collection((PromptEvaluationRunPO{}).CollectionName()).InsertOne(t.Context(), bson.M{"domain_id": int64(99999999), "requested_org_id": original.Audit.OrganizationID, "status": "canceled", "created_at": original.Audit.CreatedAt})
	require.NoError(t, err)
	status := domainevaluation.EvidenceStatusCanceled
	items, next, err := repository.ListEvidenceV2(t.Context(), original.Audit.OrganizationID, &status, "", 2)
	require.NoError(t, err)
	require.Len(t, items, 2)
	require.NotEmpty(t, next)
	require.Equal(t, (original.RunID + 2).String(), items[0].RunID)
	require.Equal(t, 35, items[0].RequiredCandidates)
	require.False(t, items[0].CanCancel)
	require.Equal(t, "superseded", items[0].LastReason)
	tail, end, err := repository.ListEvidenceV2(t.Context(), original.Audit.OrganizationID, &status, next, 2)
	require.NoError(t, err)
	require.Len(t, tail, 1)
	require.Empty(t, end)
	require.Equal(t, original.RunID.String(), tail[0].RunID)
	_, _, err = repository.ListEvidenceV2(t.Context(), 99, &status, next, 2)
	require.Error(t, err)
	_, _, err = repository.ListEvidenceV2(t.Context(), original.Audit.OrganizationID, nil, next, 2)
	require.Error(t, err)
	active := original.Clone()
	active.RunID += 10
	require.NoError(t, repository.CreateEvidenceV2(t.Context(), &active))
	listed, _, err := repository.ListEvidenceV2(t.Context(), active.Audit.OrganizationID, nil, "", 20)
	require.NoError(t, err)
	require.Len(t, listed, 4)
	oldVersion := active.Version()
	stale := active.Clone()
	require.NoError(t, active.Cancel("user:42", "cancel before dispatch", false, active.Audit.CreatedAt.Add(time.Hour)))
	require.NoError(t, repository.SaveEvidenceV2(t.Context(), &active, oldVersion))
	require.NoError(t, stale.MarkExecutionDispatching(stale.Execution().Owner, stale.Execution().ClaimedAt.Add(time.Second)))
	require.ErrorIs(t, repository.SaveEvidenceV2(t.Context(), &stale, oldVersion), domainevaluation.ErrConflict)
	again := original.Clone()
	again.RunID += 11
	require.NoError(t, repository.CreateEvidenceV2(t.Context(), &again))
	var po PromptEvaluationEvidenceV2PO
	require.NoError(t, db.Collection(po.CollectionName()).FindOne(t.Context(), bson.M{"domain_id": active.RunID}).Decode(&po))
	require.Empty(t, po.ActiveReleaseKey)
	require.Empty(t, po.ActiveExecutionOrgKey)
	require.NotNil(t, po.CanceledAt)
}
