package mongoconsistency

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	"github.com/FangcunMount/qs-server/internal/apiserver/eventing/eventevidencebinding"
	repository "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/interpretation"
	catalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	outcome "github.com/FangcunMount/qs-server/internal/pkg/eventing/outcome"
	"go.mongodb.org/mongo-driver/bson"
)

// The original failure proof is immutable even after a manual authorization
// changes the Run's current retry decision. Verify immutable failure facts
// against the wire, and the complete first decision against the first proof.
func (s *Scanner) checkFailedTransaction(ctx context.Context, generationID, runID uint64) (bool, error) {
	var r repository.InterpretationRunPO
	if err := s.db.Collection("interpretation_runs").FindOne(ctx, bson.M{"domain_id": runID, "deleted_at": nil}).Decode(&r); err != nil {
		return false, err
	}
	proof := r.FailedTransactionEvidence
	if proof == nil {
		return false, nil
	}
	var g repository.ReportGenerationPO
	if err := s.db.Collection("report_generations").FindOne(ctx, bson.M{"domain_id": generationID, "deleted_at": nil}).Decode(&g); err != nil {
		return true, err
	}
	if g.Status != "failed" || g.LatestRunID != runID || g.TransactionSchemaVersion != 1 || r.GenerationID != generationID || r.Status != "failed" || r.Failure == nil || r.FinishedAt == nil || !r.FinishedAt.Equal(proof.RecordedAt) || !g.UpdatedAt.Equal(*r.FinishedAt) {
		return true, driftf("failed transaction business graph conflict")
	}
	fact, err := s.outcome(ctx, g.OutcomeID)
	if err != nil {
		return true, err
	}
	expected := outcome.ReportFailedPayload{OrgID: fact.OrgID(), GenerationID: g.DomainID.String(), RunID: r.DomainID.String(), AssessmentID: fact.AssessmentID().String(), OutcomeID: strconv.FormatUint(g.OutcomeID, 10), TesteeID: fact.TesteeID(), Attempt: uint(r.Attempt), ReportType: g.ReportType, TemplateVersion: g.TemplateVersion,
		FailureKind: r.Failure.Kind, FailureCode: r.Failure.Code, Retryable: r.Failure.Retryable, SafeReason: r.Failure.SafeMessage, FailedAt: *r.FinishedAt}
	binding, err := eventevidencebinding.Failed(expected)
	if err != nil {
		return true, conflict(err)
	}
	err = s.checkTransactionEvidence(ctx, proof, proof.Reference.EventID, catalog.InterpretationReportFailed, domain.AggregateType, g.DomainID.String(), fmt.Sprintf("org:%d", fact.OrgID()), proof.BusinessBindingSHA256, func(raw []byte) (string, error) {
		var body outcome.ReportFailedPayload
		if err := json.Unmarshal(raw, &body); err != nil {
			return "", err
		}
		complete, err := eventevidencebinding.Failed(body)
		if err != nil {
			return "", err
		}
		body.RetryDecision = nil
		immutable, err := eventevidencebinding.Failed(body)
		if err != nil || immutable != binding {
			return "", driftf("original failure immutable fields conflict")
		}
		return complete, nil
	})
	return true, err
}
