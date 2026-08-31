package aiexplanation

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"

	appevaluation "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/evaluation"
	appgovernance "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/governance"
	domainevaluation "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/evaluation"
	domainprofile "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/profile"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

const (
	administrationCatalogCursorVersion = "v1"
	evaluationCatalogCursorKind        = "prompt_evaluation"
	profileCatalogCursorKind           = "profile"
	maxAdministrationCatalogPageSize   = 100
)

type administrationCatalogCursor struct {
	Version   string    `json:"version"`
	Kind      string    `json:"kind"`
	OrgID     int64     `json:"org_id,omitempty"`
	Status    string    `json:"status,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	DomainID  meta.ID   `json:"domain_id"`
}

// promptEvaluationCatalogPO intentionally cannot represent Provider output or
// detailed evaluation evidence. Keep this projection separate from
// PromptEvaluationRunPO so a queue query cannot accidentally regress into a
// full aggregate read.
type promptEvaluationCatalogPO struct {
	DomainID       meta.ID                             `bson:"domain_id"`
	CreatedAt      time.Time                           `bson:"created_at"`
	Release        EvaluationReleasePO                 `bson:"release"`
	Status         string                              `bson:"status"`
	Version        int64                               `bson:"version"`
	Attempts       []promptEvaluationCatalogAttemptPO  `bson:"attempts"`
	Reviews        []promptEvaluationCatalogReviewPO   `bson:"reviews"`
	Execution      *promptEvaluationCatalogExecutionPO `bson:"execution,omitempty"`
	RequestedOrgID int64                               `bson:"requested_org_id,omitempty"`
	RequestedBy    string                              `bson:"requested_by,omitempty"`
	RequestReason  string                              `bson:"request_reason,omitempty"`
	Gate           *EvaluationGatePO                   `bson:"gate,omitempty"`
}

type promptEvaluationCatalogAttemptPO struct {
	CaseID  string   `bson:"case_id"`
	Attempt int      `bson:"attempt"`
	Stage   string   `bson:"stage"`
	Failure bson.Raw `bson:"failure,omitempty"`
}

type promptEvaluationCatalogReviewPO struct {
	CaseID   string `bson:"case_id"`
	Attempt  int    `bson:"attempt"`
	Role     string `bson:"role"`
	Decision string `bson:"decision"`
}

type promptEvaluationCatalogExecutionPO struct {
	Phase string `bson:"phase"`
}

func (r *PromptEvaluationRepository) ListForReview(
	ctx context.Context,
	orgID int64,
	status *domainevaluation.Status,
	cursor string,
	limit int,
) ([]appevaluation.ReviewRunCatalogRecord, string, error) {
	if r == nil || orgID <= 0 || limit < 1 || limit > maxAdministrationCatalogPageSize ||
		(status != nil && !status.IsValid()) {
		return nil, "", fmt.Errorf("list AI explanation Prompt evaluations: invalid query")
	}
	statusValue := catalogStatus(status)
	filter := bson.M{"requested_org_id": orgID}
	if statusValue != "" {
		filter["status"] = statusValue
	}
	if strings.TrimSpace(cursor) != "" {
		state, err := decodeAdministrationCatalogCursor(cursor, evaluationCatalogCursorKind, orgID, statusValue)
		if err != nil {
			return nil, "", fmt.Errorf("%w: %v", appevaluation.ErrReviewCatalogCursor, err)
		}
		applyAdministrationKeyset(filter, state)
	}
	values, err := r.listPromptEvaluationPOs(ctx, filter, limit+1)
	if err != nil {
		return nil, "", err
	}
	nextCursor := ""
	if len(values) > limit {
		values = values[:limit]
		last := values[len(values)-1]
		nextCursor, err = encodeAdministrationCatalogCursor(administrationCatalogCursor{
			Version: administrationCatalogCursorVersion, Kind: evaluationCatalogCursorKind,
			OrgID: orgID, Status: statusValue, CreatedAt: last.CreatedAt.UTC(), DomainID: last.DomainID,
		})
		if err != nil {
			return nil, "", err
		}
	}
	result := make([]appevaluation.ReviewRunCatalogRecord, 0, len(values))
	for index := range values {
		value := values[index]
		release, err := evaluationReleaseFromPO(value.Release)
		if err != nil {
			return nil, "", fmt.Errorf("decode AI explanation Prompt evaluation catalog: %w", err)
		}
		statusValue := domainevaluation.Status(value.Status)
		if !statusValue.IsValid() || value.RequestedOrgID != orgID || (status != nil && statusValue != *status) {
			return nil, "", fmt.Errorf("AI explanation Prompt evaluation catalog returned inconsistent data")
		}
		record := appevaluation.ReviewRunCatalogRecord{
			RunID: value.DomainID, Version: value.Version, Status: statusValue, Release: release,
			RequestedOrgID: value.RequestedOrgID, RequestedBy: value.RequestedBy,
			RequestReason: value.RequestReason, CreatedAt: value.CreatedAt, Gate: gateFromPO(value.Gate),
			Attempts: make([]appevaluation.ReviewRunCatalogAttempt, 0, len(value.Attempts)),
			Reviews:  make([]appevaluation.ReviewRunCatalogReview, 0, len(value.Reviews)),
		}
		for _, attempt := range value.Attempts {
			record.Attempts = append(record.Attempts, appevaluation.ReviewRunCatalogAttempt{
				CaseID: attempt.CaseID, Attempt: attempt.Attempt, Stage: domainevaluation.AttemptStage(attempt.Stage), Failed: len(attempt.Failure) > 0,
			})
		}
		for _, review := range value.Reviews {
			record.Reviews = append(record.Reviews, appevaluation.ReviewRunCatalogReview{
				CaseID: review.CaseID, Attempt: review.Attempt, Role: domainevaluation.ReviewRole(review.Role),
				Decision: domainevaluation.ReviewDecision(review.Decision),
			})
		}
		if value.Execution != nil {
			phase := domainevaluation.AttemptExecutionPhase(value.Execution.Phase)
			record.ExecutionPhase = &phase
		}
		result = append(result, record)
	}
	return result, nextCursor, nil
}

func (r *PromptEvaluationRepository) listPromptEvaluationPOs(ctx context.Context, filter bson.M, limit int) ([]promptEvaluationCatalogPO, error) {
	cursor, err := r.Find(ctx, legacyPromptEvaluationFilter(filter), options.Find().
		SetSort(bson.D{{Key: "created_at", Value: -1}, {Key: "domain_id", Value: -1}}).
		SetProjection(bson.M{
			"_id": 0, "domain_id": 1, "created_at": 1, "release": 1, "status": 1, "version": 1,
			"attempts.case_id": 1, "attempts.attempt": 1, "attempts.stage": 1, "attempts.failure": 1,
			"reviews.case_id": 1, "reviews.attempt": 1, "reviews.role": 1, "reviews.decision": 1,
			"execution.phase": 1, "requested_org_id": 1, "requested_by": 1, "request_reason": 1, "gate": 1,
		}).
		SetLimit(int64(limit)))
	if err != nil {
		return nil, fmt.Errorf("list AI explanation Prompt evaluations: %w", err)
	}
	defer func() { _ = cursor.Close(ctx) }()
	values := make([]promptEvaluationCatalogPO, 0, limit)
	for cursor.Next(ctx) {
		var value promptEvaluationCatalogPO
		if err := cursor.Decode(&value); err != nil {
			return nil, fmt.Errorf("decode AI explanation Prompt evaluation catalog: %w", err)
		}
		values = append(values, value)
	}
	if err := cursor.Err(); err != nil {
		return nil, fmt.Errorf("scan AI explanation Prompt evaluation catalog: %w", err)
	}
	return values, nil
}

func (r *ProfileRepository) ListProfiles(
	ctx context.Context,
	status *domainprofile.Status,
	cursor string,
	limit int,
) ([]*domainprofile.AIExplanationProfile, string, error) {
	if r == nil || limit < 1 || limit > maxAdministrationCatalogPageSize ||
		(status != nil && !status.IsValid()) {
		return nil, "", fmt.Errorf("list AI explanation Profiles: invalid query")
	}
	statusValue := catalogStatus(status)
	filter := bson.M{}
	if statusValue != "" {
		filter["status"] = statusValue
	}
	if strings.TrimSpace(cursor) != "" {
		state, err := decodeAdministrationCatalogCursor(cursor, profileCatalogCursorKind, 0, statusValue)
		if err != nil {
			return nil, "", fmt.Errorf("%w: %v", appgovernance.ErrProfileCatalogCursor, err)
		}
		applyAdministrationKeyset(filter, state)
	}
	cursorResult, err := r.Find(ctx, filter, options.Find().
		SetSort(bson.D{{Key: "created_at", Value: -1}, {Key: "domain_id", Value: -1}}).
		SetLimit(int64(limit+1)))
	if err != nil {
		return nil, "", fmt.Errorf("list AI explanation Profiles: %w", err)
	}
	defer func() { _ = cursorResult.Close(ctx) }()
	values := make([]ProfilePO, 0, limit+1)
	for cursorResult.Next(ctx) {
		var value ProfilePO
		if err := cursorResult.Decode(&value); err != nil {
			return nil, "", fmt.Errorf("decode AI explanation Profile catalog: %w", err)
		}
		values = append(values, value)
	}
	if err := cursorResult.Err(); err != nil {
		return nil, "", fmt.Errorf("scan AI explanation Profile catalog: %w", err)
	}
	nextCursor := ""
	if len(values) > limit {
		values = values[:limit]
		last := values[len(values)-1]
		nextCursor, err = encodeAdministrationCatalogCursor(administrationCatalogCursor{
			Version: administrationCatalogCursorVersion, Kind: profileCatalogCursorKind,
			Status: statusValue, CreatedAt: last.CreatedAt.UTC(), DomainID: last.DomainID,
		})
		if err != nil {
			return nil, "", err
		}
	}
	result := make([]*domainprofile.AIExplanationProfile, 0, len(values))
	for index := range values {
		value, err := r.mapper.ProfileToDomain(&values[index])
		if err != nil {
			return nil, "", fmt.Errorf("decode AI explanation Profile catalog: %w", err)
		}
		if status != nil && value.Status() != *status {
			return nil, "", fmt.Errorf("AI explanation Profile catalog returned inconsistent data")
		}
		result = append(result, value)
	}
	return result, nextCursor, nil
}

func catalogStatus[T ~string](status *T) string {
	if status == nil {
		return ""
	}
	return string(*status)
}

func applyAdministrationKeyset(filter bson.M, cursor administrationCatalogCursor) {
	filter["$or"] = bson.A{
		bson.M{"created_at": bson.M{"$lt": cursor.CreatedAt}},
		bson.M{"created_at": cursor.CreatedAt, "domain_id": bson.M{"$lt": cursor.DomainID}},
	}
}

func encodeAdministrationCatalogCursor(value administrationCatalogCursor) (string, error) {
	if err := value.validate(value.Kind, value.OrgID, value.Status); err != nil {
		return "", err
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("encode AI explanation administration cursor: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func decodeAdministrationCatalogCursor(raw, kind string, orgID int64, status string) (administrationCatalogCursor, error) {
	payload, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(raw))
	if err != nil {
		return administrationCatalogCursor{}, fmt.Errorf("AI explanation administration cursor is malformed")
	}
	var value administrationCatalogCursor
	if err := json.Unmarshal(payload, &value); err != nil {
		return administrationCatalogCursor{}, fmt.Errorf("AI explanation administration cursor is malformed")
	}
	if err := value.validate(kind, orgID, status); err != nil {
		return administrationCatalogCursor{}, err
	}
	return value, nil
}

func (c administrationCatalogCursor) validate(kind string, orgID int64, status string) error {
	if c.Version != administrationCatalogCursorVersion || c.Kind != kind || c.OrgID != orgID || c.Status != status ||
		c.CreatedAt.IsZero() || c.CreatedAt.Location() != time.UTC || c.DomainID.IsZero() {
		return fmt.Errorf("AI explanation administration cursor does not match the query")
	}
	return nil
}
