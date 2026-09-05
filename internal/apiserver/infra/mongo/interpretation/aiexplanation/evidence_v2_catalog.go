package aiexplanation

import (
	"context"
	"fmt"
	"strings"
	"time"

	appevaluation "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/evaluation"
	domainevaluation "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/evaluation"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const evidenceV2CatalogKind = "prompt_evaluation_v2"

// This projection cannot decode raw outputs, semantic rationale or assertions.
type evidenceV2CatalogPO struct {
	DomainID  meta.ID                         `bson:"domain_id"`
	CreatedAt time.Time                       `bson:"created_at"`
	Status    domainevaluation.EvidenceStatus `bson:"status"`
	Version   int64                           `bson:"version"`
	OrgID     int64                           `bson:"requested_org_id"`
	Release   struct {
		Prompt  struct{ Version string }
		Profile struct{ Version string }
	} `bson:"release"`
	ReleaseFingerprint string `bson:"release_fingerprint"`
	Slots              []struct {
		Candidate *struct {
			ReviewReady bool `bson:"reviewready"`
		}
	} `bson:"slots"`
	Reviews []struct {
		CandidateID string `bson:"candidateid"`
	} `bson:"human_reviews"`
	Execution   *struct{ Phase string } `bson:"execution"`
	Transitions []struct {
		CauseCode string
		Reason    string
	} `bson:"state_transitions"`
	Unknown         int `bson:"unresolved_result_unknown_count"`
	ExecutionPolicy struct {
		SlotPolicy struct {
			RequiredGenerationCases   int
			RequiredCandidatesPerCase int
		}
	} `bson:"execution_policy"`
}

func evidenceV2CatalogProjection() bson.M {
	return bson.M{"_id": 0, "domain_id": 1, "created_at": 1, "status": 1, "version": 1, "requested_org_id": 1,
		"release.prompt.version": 1, "release.profile.version": 1, "release_fingerprint": 1,
		"slots.candidate.reviewready": 1, "human_reviews.candidateid": 1, "execution.phase": 1,
		"state_transitions.causecode": 1, "state_transitions.reason": 1, "unresolved_result_unknown_count": 1, "execution_policy.slotpolicy": 1}
}
func (r *PromptEvaluationRepository) ListEvidenceV2(ctx context.Context, orgID int64, status *domainevaluation.EvidenceStatus, cursor string, limit int) ([]appevaluation.EvidenceV2Summary, string, error) {
	if r == nil || orgID <= 0 || limit < 1 || limit > 100 || (status != nil && !status.IsValid()) {
		return nil, "", fmt.Errorf("invalid v2 catalog query")
	}
	statusValue := ""
	filter := bson.M{"requested_org_id": orgID, "evidence_version": PromptEvaluationEvidenceVersionV2}
	if status != nil {
		statusValue = string(*status)
		filter["status"] = statusValue
	}
	if strings.TrimSpace(cursor) != "" {
		state, err := decodeAdministrationCatalogCursor(cursor, evidenceV2CatalogKind, orgID, statusValue)
		if err != nil {
			return nil, "", fmt.Errorf("%w: %v", appevaluation.ErrReviewCatalogCursor, err)
		}
		applyAdministrationKeyset(filter, state)
	}
	cur, err := r.Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}, {Key: "domain_id", Value: -1}}).SetProjection(evidenceV2CatalogProjection()).SetLimit(int64(limit+1)))
	if err != nil {
		return nil, "", err
	}
	defer func() { _ = cur.Close(ctx) }()
	values := make([]evidenceV2CatalogPO, 0, limit+1)
	if err := cur.All(ctx, &values); err != nil {
		return nil, "", err
	}
	next := ""
	if len(values) > limit {
		values = values[:limit]
		last := values[len(values)-1]
		next, err = encodeAdministrationCatalogCursor(administrationCatalogCursor{Version: administrationCatalogCursorVersion, Kind: evidenceV2CatalogKind, OrgID: orgID, Status: statusValue, CreatedAt: last.CreatedAt.UTC(), DomainID: last.DomainID})
		if err != nil {
			return nil, "", err
		}
	}
	result := make([]appevaluation.EvidenceV2Summary, 0, len(values))
	for _, v := range values {
		if v.OrgID != orgID || v.DomainID.IsZero() || !v.Status.IsValid() || (status != nil && v.Status != *status) {
			return nil, "", fmt.Errorf("inconsistent v2 catalog row")
		}
		item := appevaluation.EvidenceV2Summary{
			RunID: v.DomainID.String(), OrganizationID: v.OrgID, Version: v.Version,
			Status: v.Status, CreatedAt: v.CreatedAt,
			PromptVersion: v.Release.Prompt.Version, ProfileVersion: v.Release.Profile.Version,
			ReleaseFingerprint: v.ReleaseFingerprint,
			RequiredCandidates: v.ExecutionPolicy.SlotPolicy.RequiredGenerationCases * v.ExecutionPolicy.SlotPolicy.RequiredCandidatesPerCase,
			ReviewCount:        len(v.Reviews), UnresolvedResultUnknownCount: v.Unknown,
		}
		for _, slot := range v.Slots {
			if slot.Candidate != nil {
				item.AcceptedCandidates++
				if slot.Candidate.ReviewReady {
					item.ReviewReadyCandidates++
				}
			}
		}
		if n := len(v.Transitions); n > 0 {
			item.LastCause = v.Transitions[n-1].CauseCode
			item.LastReason = v.Transitions[n-1].Reason
		}
		eligible := !v.Status.IsTerminal() && v.Unknown == 0 && (v.Execution == nil || v.Execution.Phase == string(domainevaluation.AttemptExecutionPrepared))
		item.CanDiscard = eligible && v.Status == domainevaluation.EvidenceStatusAwaitingReview
		item.CanCancel = eligible && !item.CanDiscard
		result = append(result, item)
	}
	return result, next, nil
}
