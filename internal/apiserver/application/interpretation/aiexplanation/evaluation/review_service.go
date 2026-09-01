package evaluation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	interpretationschema "github.com/FangcunMount/qs-server/api/schema/interpretation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation"
	domainevaluation "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/evaluation"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

var ErrReviewCatalogCursor = errors.New("AI explanation Prompt review catalog cursor is invalid")

// ReviewService exposes the synthetic evidence packet needed by release
// reviewers and delegates all writes to EvidenceService's aggregate-version
// CAS. Authentication and authorization must provide a trusted Reviewer
// subject and authorize the requested role before this boundary is called.
// This service deliberately has no participant-facing transport.
type ReviewService struct {
	evidence         *EvidenceService
	catalog          ReviewRunCatalog
	suiteID          string
	suiteVersion     string
	suiteFingerprint aiexplanation.Fingerprint
	assessmentInputs map[string]json.RawMessage
}

const (
	DefaultReviewRunPageSize = 20
	MaxReviewRunPageSize     = 100
)

// ReviewRunCatalog is the bounded, organization-scoped read contract used by
// operator review workbenches. Cursor semantics are owned by the persistence
// adapter so transports never learn Mongo keyset details.
type ReviewRunCatalog interface {
	ListForReview(context.Context, int64, *domainevaluation.Status, string, int) ([]ReviewRunCatalogRecord, string, error)
}

// ReviewRunCatalogRecord is the deliberately bounded queue projection. It
// contains only the identities needed to derive review progress; raw Provider
// output, normalized output, receipts, assertions and semantic rationale are
// available only through the explicit single-run evidence query.
type ReviewRunCatalogRecord struct {
	RunID          meta.ID
	Version        int64
	Status         domainevaluation.Status
	Release        domainevaluation.ReleaseIdentity
	RequestedOrgID int64
	RequestedBy    string
	RequestReason  string
	CreatedAt      time.Time
	Attempts       []ReviewRunCatalogAttempt
	Reviews        []ReviewRunCatalogReview
	ExecutionPhase *domainevaluation.AttemptExecutionPhase
	Gate           *domainevaluation.GateResult
}

type ReviewRunCatalogAttempt struct {
	CaseID  string
	Attempt int
	Stage   domainevaluation.AttemptStage
	Failed  bool
}

type ReviewRunCatalogReview struct {
	CaseID   string
	Attempt  int
	Role     domainevaluation.ReviewRole
	Decision domainevaluation.ReviewDecision
}

type ReviewRunListQuery struct {
	OrgID  int64
	Status *domainevaluation.Status
	Cursor string
	Limit  int
}

type ReviewRunPage struct {
	Items      []*ReviewRun
	NextCursor string
}

type ReviewRun struct {
	RunID                          meta.ID
	Version                        int64
	Status                         domainevaluation.Status
	Release                        domainevaluation.ReleaseIdentity
	RequestedOrgID                 int64
	RequestedBy                    string
	RequestReason                  string
	CreatedAt                      time.Time
	Execution                      *ReviewExecution
	Recoveries                     []ReviewRecovery
	Progress                       ReviewProgress
	Attempts                       []ReviewAttempt
	Finalized                      *ReviewFinalization
	Canceled                       *ReviewCancellation
	Gate                           *domainevaluation.GateResult
	CanReview                      bool
	CanFinalize                    bool
	CanCancel                      bool
	RecoveryMaxProviderInvocations int
}

// ReviewExecution is the operator-safe projection of the current durable
// attempt checkpoint. Provider invocation and worker-owner identities remain
// internal because they are recovery controls, not review evidence.
type ReviewExecution struct {
	CaseID            string
	Attempt           int
	Phase             domainevaluation.AttemptExecutionPhase
	ClaimedAt         time.Time
	LeaseExpiresAt    time.Time
	DispatchStartedAt *time.Time
}

type ReviewRecovery struct {
	ID          string
	CaseID      string
	Attempt     int
	Actor       string
	Reason      string
	RequestedAt time.Time
}

type ReviewCancellation struct {
	At     time.Time
	Actor  string
	Reason string
}

type ReviewProgress struct {
	GenerationAttempts         int
	FailedAttempts             int
	RequiredReviews            int
	RecordedReviews            int
	MissingReviews             int
	FullyReviewedAttempts      int
	RejectedReviews            int
	AllRequiredReviewsRecorded bool
}

type ReviewAttempt struct {
	CaseID            string
	Attempt           int
	AssessmentInput   json.RawMessage
	RawProviderOutput []byte
	NormalizedOutput  json.RawMessage
	ProviderReceipt   *aiexplanation.ProviderReceipt
	Failure           *domainevaluation.AttemptFailure
	Assertions        []domainevaluation.AssertionReceipt
	Semantic          *domainevaluation.SemanticReceipt
	SemanticExecution *domainevaluation.SemanticExecutionRecord
	Reviews           []domainevaluation.HumanReview
	MissingRoles      []domainevaluation.ReviewRole
}

type ReviewFinalization struct {
	At     time.Time
	Actor  string
	Reason string
}

func NewReviewService(evidence *EvidenceService) (*ReviewService, error) {
	if evidence == nil {
		return nil, fmt.Errorf("AI explanation Prompt review evidence service is required")
	}
	raw := interpretationschema.AIExplanationPromptEvaluationCasesV1()
	suite, err := Parse(raw)
	if err != nil {
		return nil, err
	}
	inputs := make(map[string]json.RawMessage, domainevaluation.RequiredGenerationCaseCount)
	for _, testCase := range suite.Cases {
		if testCase.Stage != "generation" {
			continue
		}
		encoded, err := json.Marshal(testCase.ProviderPayload)
		if err != nil {
			return nil, fmt.Errorf("marshal AI explanation review fixture %s: %w", testCase.CaseID, err)
		}
		inputs[testCase.CaseID] = encoded
	}
	if len(inputs) != domainevaluation.RequiredGenerationCaseCount {
		return nil, fmt.Errorf("AI explanation Prompt review suite generation inventory is invalid")
	}
	return &ReviewService{
		evidence:         evidence,
		catalog:          reviewRunCatalog(evidence.repository),
		suiteID:          suite.SuiteID,
		suiteVersion:     suite.SuiteVersion,
		suiteFingerprint: aiexplanation.NewFingerprint(raw),
		assessmentInputs: inputs,
	}, nil
}

func reviewRunCatalog(repository domainevaluation.Repository) ReviewRunCatalog {
	reader, _ := repository.(ReviewRunCatalog)
	return reader
}

// List returns review-safe projections only for the trusted organization.
// Supplying awaiting_review is the v1 review-queue query; no separate mutable
// queue is introduced because missing roles are derived from immutable evidence.
func (s *ReviewService) List(ctx context.Context, query ReviewRunListQuery) (*ReviewRunPage, error) {
	if s == nil || s.catalog == nil {
		return nil, fmt.Errorf("AI explanation Prompt review catalog is not configured")
	}
	if query.OrgID <= 0 || query.Limit < 0 || query.Limit > MaxReviewRunPageSize ||
		(query.Status != nil && !query.Status.IsValid()) {
		return nil, fmt.Errorf("AI explanation Prompt review catalog query is invalid")
	}
	limit := query.Limit
	if limit == 0 {
		limit = DefaultReviewRunPageSize
	}
	values, nextCursor, err := s.catalog.ListForReview(ctx, query.OrgID, query.Status, query.Cursor, limit)
	if err != nil {
		return nil, err
	}
	if len(values) > limit {
		return nil, fmt.Errorf("AI explanation Prompt review catalog exceeded requested limit")
	}
	page := &ReviewRunPage{Items: make([]*ReviewRun, 0, len(values)), NextCursor: nextCursor}
	for _, value := range values {
		if value.RequestedOrgID != query.OrgID {
			return nil, fmt.Errorf("AI explanation Prompt review catalog crossed organization boundary")
		}
		if query.Status != nil && value.Status != *query.Status {
			return nil, fmt.Errorf("AI explanation Prompt review catalog returned inconsistent status")
		}
		projected, err := s.projectCatalogRecord(value)
		if err != nil {
			return nil, err
		}
		page.Items = append(page.Items, projected)
	}
	return page, nil
}

func (s *ReviewService) projectCatalogRecord(value ReviewRunCatalogRecord) (*ReviewRun, error) {
	if value.RunID.IsZero() || !value.Status.IsValid() || value.RequestedOrgID <= 0 || value.CreatedAt.IsZero() {
		return nil, fmt.Errorf("AI explanation Prompt review catalog record is invalid")
	}
	if value.Release.Suite.ID != s.suiteID || value.Release.Suite.Version != s.suiteVersion ||
		value.Release.Suite.Fingerprint != s.suiteFingerprint {
		return nil, fmt.Errorf("AI explanation Prompt review suite does not match frozen release evidence")
	}
	if err := value.Release.Validate(); err != nil {
		return nil, err
	}

	progress := ReviewProgress{}
	generationAttempts := make(map[string]bool, len(value.Attempts))
	for _, attempt := range value.Attempts {
		if attempt.Stage == domainevaluation.AttemptStagePreflight {
			if attempt.CaseID != value.Release.PreflightCaseID || attempt.Attempt != 1 {
				return nil, fmt.Errorf("AI explanation Prompt review catalog preflight attempt is invalid")
			}
			continue
		}
		if attempt.Stage != domainevaluation.AttemptStageGeneration || !value.Release.IsGenerationCase(attempt.CaseID) ||
			attempt.Attempt < 1 || attempt.Attempt > value.Release.RepetitionsPerCase {
			return nil, fmt.Errorf("AI explanation Prompt review catalog attempt is invalid")
		}
		key := reviewAttemptKey(attempt.CaseID, attempt.Attempt)
		if _, exists := generationAttempts[key]; exists {
			return nil, fmt.Errorf("AI explanation Prompt review catalog attempt is duplicated")
		}
		generationAttempts[key] = attempt.Failed
		if attempt.Failed {
			progress.FailedAttempts++
		}
	}

	reviewsByAttempt := make(map[string]map[domainevaluation.ReviewRole]domainevaluation.ReviewDecision, len(value.Reviews))
	progress.GenerationAttempts = len(generationAttempts)
	progress.RequiredReviews = len(generationAttempts) * 2
	for _, review := range value.Reviews {
		key := reviewAttemptKey(review.CaseID, review.Attempt)
		if _, exists := generationAttempts[key]; !exists ||
			(review.Role != domainevaluation.ReviewRoleAssessmentSemantics && review.Role != domainevaluation.ReviewRoleSafetyProduct) ||
			(review.Decision != domainevaluation.ReviewDecisionApprove && review.Decision != domainevaluation.ReviewDecisionReject) {
			return nil, fmt.Errorf("AI explanation Prompt review catalog review is invalid")
		}
		if reviewsByAttempt[key] == nil {
			reviewsByAttempt[key] = make(map[domainevaluation.ReviewRole]domainevaluation.ReviewDecision, 2)
		}
		if _, exists := reviewsByAttempt[key][review.Role]; exists {
			return nil, fmt.Errorf("AI explanation Prompt review catalog review role is duplicated")
		}
		reviewsByAttempt[key][review.Role] = review.Decision
		progress.RecordedReviews++
		if review.Decision == domainevaluation.ReviewDecisionReject {
			progress.RejectedReviews++
		}
	}
	for key := range generationAttempts {
		recorded := len(reviewsByAttempt[key])
		progress.MissingReviews += 2 - recorded
		if recorded == 2 {
			progress.FullyReviewedAttempts++
		}
	}
	progress.AllRequiredReviewsRecorded = progress.GenerationAttempts == domainevaluation.RequiredGenerationAttempts && progress.MissingReviews == 0
	if value.ExecutionPhase != nil &&
		(value.Status != domainevaluation.StatusCollecting ||
			(*value.ExecutionPhase != domainevaluation.AttemptExecutionPrepared && *value.ExecutionPhase != domainevaluation.AttemptExecutionDispatching)) {
		return nil, fmt.Errorf("AI explanation Prompt review catalog execution checkpoint is invalid")
	}

	result := &ReviewRun{
		RunID: value.RunID, Version: value.Version, Status: value.Status, Release: value.Release,
		RequestedOrgID: value.RequestedOrgID, RequestedBy: value.RequestedBy,
		RequestReason: value.RequestReason, CreatedAt: value.CreatedAt,
		Progress: progress, Gate: value.Gate,
		CanReview:   value.Status == domainevaluation.StatusAwaitingReview && progress.FailedAttempts == 0,
		CanFinalize: value.Status == domainevaluation.StatusAwaitingReview && progress.FailedAttempts == 0 && progress.AllRequiredReviewsRecorded,
		CanCancel: value.Status == domainevaluation.StatusCollecting && (value.ExecutionPhase == nil || *value.ExecutionPhase != domainevaluation.AttemptExecutionDispatching) ||
			value.Status == domainevaluation.StatusAwaitingReview && progress.FailedAttempts > 0,
	}
	if value.Status == domainevaluation.StatusCollecting {
		result.RecoveryMaxProviderInvocations = (domainevaluation.RequiredGenerationAttempts - progress.GenerationAttempts) * 2
		if value.ExecutionPhase != nil && *value.ExecutionPhase == domainevaluation.AttemptExecutionDispatching {
			result.RecoveryMaxProviderInvocations -= 2
		}
		if result.RecoveryMaxProviderInvocations < 0 {
			result.RecoveryMaxProviderInvocations = 0
		}
	}
	return result, nil
}

func (s *ReviewService) Find(ctx context.Context, runID meta.ID) (*ReviewRun, error) {
	if s == nil || s.evidence == nil {
		return nil, fmt.Errorf("AI explanation Prompt review service is required")
	}
	runRecord, err := s.evidence.Find(ctx, runID)
	if err != nil {
		return nil, err
	}
	return s.project(runRecord)
}

func (s *ReviewService) RecordHumanReview(ctx context.Context, runID meta.ID, command HumanReviewCommand) (*ReviewRun, error) {
	if s == nil || s.evidence == nil {
		return nil, fmt.Errorf("AI explanation Prompt review service is required")
	}
	runRecord, err := s.evidence.RecordHumanReview(ctx, runID, command)
	if err != nil {
		return nil, err
	}
	return s.project(runRecord)
}

func (s *ReviewService) Finalize(ctx context.Context, runID meta.ID, actor, reason string) (*ReviewRun, error) {
	if s == nil || s.evidence == nil {
		return nil, fmt.Errorf("AI explanation Prompt review service is required")
	}
	current, err := s.Find(ctx, runID)
	if err != nil {
		return nil, err
	}
	if !current.CanFinalize {
		return nil, fmt.Errorf("AI explanation Prompt evaluation requires all dual-role reviews before finalization")
	}
	runRecord, err := s.evidence.Finalize(ctx, runID, actor, reason)
	if err != nil {
		return nil, err
	}
	return s.project(runRecord)
}

func (s *ReviewService) Cancel(ctx context.Context, runID meta.ID, actor, reason string) (*ReviewRun, error) {
	if s == nil || s.evidence == nil {
		return nil, fmt.Errorf("AI explanation Prompt review service is required")
	}
	runRecord, err := s.evidence.Cancel(ctx, runID, actor, reason)
	if err != nil {
		return nil, err
	}
	return s.project(runRecord)
}

func (s *ReviewService) project(runRecord *domainevaluation.PromptEvaluationRun) (*ReviewRun, error) {
	if runRecord == nil {
		return nil, fmt.Errorf("AI explanation Prompt evaluation run is required")
	}
	release := runRecord.Release()
	if release.Suite.ID != s.suiteID || release.Suite.Version != s.suiteVersion ||
		release.Suite.Fingerprint != s.suiteFingerprint {
		return nil, fmt.Errorf("AI explanation Prompt review suite does not match frozen release evidence")
	}

	reviewsByAttempt := make(map[string][]domainevaluation.HumanReview)
	for _, review := range runRecord.Reviews() {
		key := reviewAttemptKey(review.CaseID, review.Attempt)
		reviewsByAttempt[key] = append(reviewsByAttempt[key], review)
	}
	caseOrder := make(map[string]int, len(release.GenerationCaseIDs))
	for index, caseID := range release.GenerationCaseIDs {
		caseOrder[caseID] = index
	}
	attempts := make([]ReviewAttempt, 0, domainevaluation.RequiredGenerationAttempts)
	progress := ReviewProgress{}
	for _, attempt := range runRecord.Attempts() {
		if attempt.Stage != domainevaluation.AttemptStageGeneration {
			continue
		}
		input, exists := s.assessmentInputs[attempt.CaseID]
		if !exists {
			return nil, fmt.Errorf("AI explanation Prompt review case is absent from frozen suite")
		}
		key := reviewAttemptKey(attempt.CaseID, attempt.Attempt)
		reviews := append([]domainevaluation.HumanReview(nil), reviewsByAttempt[key]...)
		sort.Slice(reviews, func(i, j int) bool { return reviews[i].Role < reviews[j].Role })
		missing := missingReviewRoles(reviews)
		progress.GenerationAttempts++
		if attempt.Failure != nil {
			progress.FailedAttempts++
		}
		progress.RequiredReviews += 2
		progress.RecordedReviews += len(reviews)
		progress.MissingReviews += len(missing)
		if len(missing) == 0 {
			progress.FullyReviewedAttempts++
		}
		for _, review := range reviews {
			if review.Decision == domainevaluation.ReviewDecisionReject {
				progress.RejectedReviews++
			}
		}
		attempts = append(attempts, ReviewAttempt{
			CaseID: attempt.CaseID, Attempt: attempt.Attempt,
			AssessmentInput:   append(json.RawMessage(nil), input...),
			RawProviderOutput: append([]byte(nil), attempt.RawOutput...),
			NormalizedOutput:  append(json.RawMessage(nil), attempt.NormalizedOutput...),
			ProviderReceipt:   attempt.ProviderReceipt, Failure: attempt.Failure,
			Assertions: append([]domainevaluation.AssertionReceipt(nil), attempt.Assertions...),
			Semantic:   attempt.Semantic, SemanticExecution: attempt.SemanticExecution,
			Reviews: reviews, MissingRoles: missing,
		})
	}
	sort.Slice(attempts, func(i, j int) bool {
		if caseOrder[attempts[i].CaseID] == caseOrder[attempts[j].CaseID] {
			return attempts[i].Attempt < attempts[j].Attempt
		}
		return caseOrder[attempts[i].CaseID] < caseOrder[attempts[j].CaseID]
	})
	progress.AllRequiredReviewsRecorded = progress.GenerationAttempts == domainevaluation.RequiredGenerationAttempts && progress.MissingReviews == 0
	result := &ReviewRun{
		RunID: runRecord.ID(), Version: runRecord.Version(), Status: runRecord.Status(), Release: release,
		RequestedOrgID: runRecord.RequestedOrgID(), RequestedBy: runRecord.RequestedBy(),
		RequestReason: runRecord.RequestReason(), CreatedAt: runRecord.CreatedAt(),
		Progress: progress, Attempts: attempts, Gate: runRecord.Gate(),
		CanReview:   runRecord.Status() == domainevaluation.StatusAwaitingReview && progress.FailedAttempts == 0,
		CanFinalize: runRecord.Status() == domainevaluation.StatusAwaitingReview && progress.FailedAttempts == 0 && progress.AllRequiredReviewsRecorded,
		CanCancel:   runRecord.CanCancel(),
	}
	if runRecord.Status() == domainevaluation.StatusCollecting {
		pending := domainevaluation.RequiredGenerationAttempts - progress.GenerationAttempts
		result.RecoveryMaxProviderInvocations = pending * 2
		if execution := runRecord.Execution(); execution != nil && execution.Phase == domainevaluation.AttemptExecutionDispatching {
			result.RecoveryMaxProviderInvocations -= 2
		}
		if result.RecoveryMaxProviderInvocations < 0 {
			result.RecoveryMaxProviderInvocations = 0
		}
	}
	if execution := runRecord.Execution(); execution != nil {
		result.Execution = &ReviewExecution{
			CaseID: execution.CaseID, Attempt: execution.Attempt, Phase: execution.Phase,
			ClaimedAt: execution.ClaimedAt, LeaseExpiresAt: execution.LeaseExpiresAt,
			DispatchStartedAt: execution.DispatchStartedAt,
		}
	}
	for _, recovery := range runRecord.Recoveries() {
		result.Recoveries = append(result.Recoveries, ReviewRecovery{
			ID: recovery.ID, CaseID: recovery.CaseID, Attempt: recovery.Attempt, Actor: recovery.Actor,
			Reason: recovery.Reason, RequestedAt: recovery.RequestedAt,
		})
	}
	if finalizedAt := runRecord.FinalizedAt(); finalizedAt != nil {
		result.Finalized = &ReviewFinalization{At: *finalizedAt, Actor: runRecord.FinalizedBy(), Reason: runRecord.FinalReason()}
	}
	if canceledAt := runRecord.CanceledAt(); canceledAt != nil {
		result.Canceled = &ReviewCancellation{At: *canceledAt, Actor: runRecord.CanceledBy(), Reason: runRecord.CancelReason()}
	}
	return result, nil
}

func missingReviewRoles(reviews []domainevaluation.HumanReview) []domainevaluation.ReviewRole {
	found := make(map[domainevaluation.ReviewRole]bool, len(reviews))
	for _, review := range reviews {
		found[review.Role] = true
	}
	missing := make([]domainevaluation.ReviewRole, 0, 2)
	for _, role := range []domainevaluation.ReviewRole{
		domainevaluation.ReviewRoleAssessmentSemantics,
		domainevaluation.ReviewRoleSafetyProduct,
	} {
		if !found[role] {
			missing = append(missing, role)
		}
	}
	return missing
}

func reviewAttemptKey(caseID string, attempt int) string {
	return fmt.Sprintf("%s\x00%d", caseID, attempt)
}
