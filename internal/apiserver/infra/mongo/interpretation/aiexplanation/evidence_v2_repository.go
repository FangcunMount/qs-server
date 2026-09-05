package aiexplanation

import (
	"context"
	"errors"
	"fmt"
	"strings"

	domainevaluation "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/evaluation"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

var _ domainevaluation.EvidenceV2Repository = (*PromptEvaluationRepository)(nil)

func (r *PromptEvaluationRepository) CreateEvidenceV2(ctx context.Context, value *domainevaluation.PromptEvaluationEvidenceV2) error {
	po, err := r.mapper.PromptEvaluationEvidenceV2ToPO(value)
	if err != nil {
		return err
	}
	if _, err := r.InsertOne(ctx, po); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			if isActiveOrgExecutionDuplicate(err) {
				return fmt.Errorf("create AI explanation Prompt evaluation evidence v2: %w", domainevaluation.ErrOrgConcurrencyExceeded)
			}
			return fmt.Errorf("create AI explanation Prompt evaluation evidence v2: %w", domainevaluation.ErrAlreadyExists)
		}
		return fmt.Errorf("create AI explanation Prompt evaluation evidence v2: %w", err)
	}
	return nil
}

func (r *PromptEvaluationRepository) SaveEvidenceV2(ctx context.Context, value *domainevaluation.PromptEvaluationEvidenceV2, expectedVersion int64) error {
	po, err := r.mapper.PromptEvaluationEvidenceV2ToPO(value)
	if err != nil {
		return err
	}
	if expectedVersion < 1 || po.Version <= expectedVersion {
		return domainevaluation.ErrConflict
	}
	setFields := bson.M{
		"status": po.Status, "version": po.Version, "preflight_evidence": po.PreflightEvidence,
		"slots": po.Slots, "generation_executions": po.GenerationExecutions, "semantic_executions": po.SemanticExecutions,
		"review_reopenings": po.ReviewReopenings, "human_reviews": po.HumanReviews, "unresolved_result_unknown_count": po.UnresolvedResultUnknownCount,
		"result_unknown_resolutions": po.ResultUnknownResolutions, "state_transitions": po.StateTransitions,
		"gate_result": po.GateResult, "audit": po.Audit, "execution": po.Execution,
		"closed_at": po.ClosedAt, "finalized_at": po.FinalizedAt, "canceled_at": po.CanceledAt,
		"updated_at": po.UpdatedAt,
	}
	update := bson.M{"$set": setFields}
	if value.Status.IsTerminal() {
		terminalAt := value.Audit.FinalizedAt
		if value.Status == domainevaluation.EvidenceStatusCanceled {
			terminalAt = value.Audit.CanceledAt
		}
		if terminalAt == nil {
			return fmt.Errorf("terminal AI explanation Prompt evaluation evidence v2 has no terminal time")
		}
		expiresAt, expiryErr := expiresAfter(*terminalAt, r.retention.PromptEvaluationRetention)
		if expiryErr != nil {
			return expiryErr
		}
		setFields["expires_at"] = expiresAt
		setFields["retention_policy_version"] = strings.TrimSpace(r.retention.Version)
	}
	setOrUnsetPromptEvaluationActiveKeys(update, setFields, po.ActiveReleaseKey, po.ActiveExecutionOrgKey)
	result, err := r.UpdateOne(ctx, bson.M{
		"domain_id": po.DomainID, "evidence_version": PromptEvaluationEvidenceVersionV2, "version": expectedVersion,
	}, update)
	if err != nil {
		return fmt.Errorf("save AI explanation Prompt evaluation evidence v2: %w", err)
	}
	if result.MatchedCount != 1 {
		return domainevaluation.ErrConflict
	}
	return nil
}

func (r *PromptEvaluationRepository) FindEvidenceV2ByID(ctx context.Context, id meta.ID) (*domainevaluation.PromptEvaluationEvidenceV2, error) {
	var po PromptEvaluationEvidenceV2PO
	if err := r.FindOne(ctx, bson.M{"domain_id": id, "evidence_version": PromptEvaluationEvidenceVersionV2}, &po); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, domainevaluation.ErrNotFound
		}
		return nil, fmt.Errorf("find AI explanation Prompt evaluation evidence v2: %w", err)
	}
	return r.mapper.PromptEvaluationEvidenceV2ToDomain(&po)
}

func setOrUnsetPromptEvaluationActiveKeys(update, setFields bson.M, releaseKey, orgKey string) {
	unset := bson.M{}
	if releaseKey != "" {
		setFields["active_release_key"] = releaseKey
	} else {
		unset["active_release_key"] = ""
	}
	if orgKey != "" {
		setFields["active_execution_org_key"] = orgKey
	} else {
		unset["active_execution_org_key"] = ""
	}
	if len(unset) > 0 {
		update["$unset"] = unset
	}
}
