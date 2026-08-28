package evaluation

import (
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

var (
	ErrRecoveryNotAllowed     = errors.New("AI explanation evaluation recovery is not allowed")
	ErrCancellationNotAllowed = errors.New("AI explanation evaluation cancellation is not allowed")
)

type PromptEvaluationRun struct {
	id             meta.ID
	release        ReleaseIdentity
	status         Status
	version        int64
	attempts       []AttemptRecord
	reviews        []HumanReview
	execution      *AttemptExecution
	recoveries     []RecoveryRequest
	requestedOrgID int64
	requestedBy    string
	requestReason  string
	createdAt      time.Time
	closedAt       *time.Time
	finalizedAt    *time.Time
	finalizedBy    string
	finalReason    string
	gate           *GateResult
	canceledAt     *time.Time
	canceledBy     string
	cancelReason   string
}

func NewRequested(id meta.ID, release ReleaseIdentity, orgID int64, requestedBy, reason string, createdAt time.Time) (*PromptEvaluationRun, error) {
	requestedBy = strings.TrimSpace(requestedBy)
	reason = strings.TrimSpace(reason)
	if orgID <= 0 || requestedBy == "" || len(requestedBy) > 256 || reason == "" || len(reason) > 1000 {
		return nil, fmt.Errorf("AI explanation evaluation request audit is required")
	}
	runRecord, err := New(id, release, createdAt)
	if err != nil {
		return nil, err
	}
	runRecord.requestedOrgID = orgID
	runRecord.requestedBy = requestedBy
	runRecord.requestReason = reason
	return runRecord, nil
}

func New(id meta.ID, release ReleaseIdentity, createdAt time.Time) (*PromptEvaluationRun, error) {
	if id.IsZero() || createdAt.IsZero() {
		return nil, fmt.Errorf("AI explanation evaluation run id and created time are required")
	}
	if err := release.Validate(); err != nil {
		return nil, err
	}
	return &PromptEvaluationRun{
		id: id, release: cloneRelease(release), status: StatusCollecting, version: 1,
		createdAt: createdAt,
	}, nil
}

func (r *PromptEvaluationRun) AddAttempt(value AttemptRecord) error {
	if r == nil || r.status != StatusCollecting {
		return fmt.Errorf("collecting AI explanation evaluation run is required")
	}
	if r.execution != nil {
		return fmt.Errorf("AI explanation evaluation attempt execution must complete through its checkpoint")
	}
	return r.addAttempt(value)
}

func (r *PromptEvaluationRun) addAttempt(value AttemptRecord) error {
	if err := value.Validate(); err != nil {
		return err
	}
	if value.StartedAt.Before(r.createdAt) {
		return fmt.Errorf("AI explanation evaluation attempt predates the run")
	}
	switch value.Stage {
	case AttemptStageGeneration:
		if !r.release.IsGenerationCase(value.CaseID) || value.Attempt > r.release.RepetitionsPerCase {
			return fmt.Errorf("AI explanation generation attempt is outside the frozen suite plan")
		}
		if value.ProviderReceipt != nil && (value.ProviderReceipt.Provider != r.release.Provider.ResolvedProvider || value.ProviderReceipt.Model != r.release.Provider.ResolvedModel) {
			return fmt.Errorf("AI explanation provider receipt does not match the frozen release")
		}
		if value.Semantic != nil && (value.Semantic.EvaluatorVersion != r.release.SemanticEvaluator.Version ||
			value.Semantic.ProviderReceipt.Provider != r.release.SemanticEvaluator.Provider.ResolvedProvider ||
			value.Semantic.ProviderReceipt.Model != r.release.SemanticEvaluator.Provider.ResolvedModel) {
			return fmt.Errorf("AI explanation semantic evaluator receipt does not match the frozen release")
		}
	case AttemptStagePreflight:
		if value.CaseID != r.release.PreflightCaseID || value.RejectionReason != r.release.PreflightRejectionReason {
			return fmt.Errorf("AI explanation preflight attempt does not match the frozen suite plan")
		}
	}
	for _, existing := range r.attempts {
		if existing.CaseID == value.CaseID && existing.Attempt == value.Attempt {
			return fmt.Errorf("AI explanation evaluation attempt is duplicated")
		}
	}
	r.attempts = append(r.attempts, cloneAttempt(value))
	r.version++
	return nil
}

// NextPendingGenerationAttempt returns the only target that may be claimed.
// Keeping suite order in the aggregate prevents duplicate or out-of-order
// durable events from advancing unrelated attempts.
func (r *PromptEvaluationRun) NextPendingGenerationAttempt() (string, int, bool) {
	if r == nil || r.status != StatusCollecting || r.findAttempt(r.release.PreflightCaseID, 1) == nil {
		return "", 0, false
	}
	for _, caseID := range r.release.GenerationCaseIDs {
		for attempt := 1; attempt <= r.release.RepetitionsPerCase; attempt++ {
			if !r.hasGenerationAttempt(caseID, attempt) {
				return caseID, attempt, true
			}
		}
	}
	return "", 0, false
}

func (r *PromptEvaluationRun) HasAttempt(caseID string, attempt int) bool {
	return r != nil && r.findAttempt(caseID, attempt) != nil
}

func (r *PromptEvaluationRun) BeginAttemptExecution(value AttemptExecution) error {
	if r == nil || r.status != StatusCollecting || r.execution != nil {
		return fmt.Errorf("collecting AI explanation evaluation run without an active execution is required")
	}
	if err := value.Validate(); err != nil {
		return err
	}
	caseID, attempt, ok := r.NextPendingGenerationAttempt()
	if !ok || value.CaseID != caseID || value.Attempt != attempt || value.ClaimedAt.Before(r.createdAt) {
		return fmt.Errorf("AI explanation evaluation execution target is not the next frozen attempt")
	}
	copy := value
	r.execution = &copy
	r.version++
	return nil
}

func (r *PromptEvaluationRun) MarkAttemptDispatching(owner string, at time.Time) error {
	owner = strings.TrimSpace(owner)
	if r == nil || r.status != StatusCollecting || r.execution == nil || r.execution.Phase != AttemptExecutionPrepared ||
		owner == "" || owner != r.execution.Owner || at.IsZero() || at.Before(r.execution.ClaimedAt) || at.After(r.execution.LeaseExpiresAt) {
		return fmt.Errorf("prepared AI explanation evaluation execution and matching owner are required")
	}
	r.execution.Phase = AttemptExecutionDispatching
	r.execution.DispatchStartedAt = copyTime(at)
	r.version++
	return nil
}

func (r *PromptEvaluationRun) ReleaseExpiredPreparation(at time.Time) error {
	if r == nil || r.status != StatusCollecting || r.execution == nil || r.execution.Phase != AttemptExecutionPrepared || !r.execution.LeaseExpired(at) {
		return fmt.Errorf("expired prepared AI explanation evaluation execution is required")
	}
	r.execution = nil
	r.version++
	return nil
}

// RequestRecovery records an audited manual request and returns the only
// attempt that may be reawakened. A live lease cannot be bypassed.
func (r *PromptEvaluationRun) RequestRecovery(id, actor, reason string, at time.Time) (string, int, error) {
	if r == nil || r.status != StatusCollecting || len(r.recoveries) >= MaxRecoveryRequests {
		return "", 0, fmt.Errorf("%w: collecting run with recovery capacity is required", ErrRecoveryNotAllowed)
	}
	var caseID string
	var attempt int
	if r.execution != nil {
		if !r.execution.LeaseExpired(at) {
			return "", 0, fmt.Errorf("%w: execution lease has not expired", ErrRecoveryNotAllowed)
		}
		caseID, attempt = r.execution.CaseID, r.execution.Attempt
	} else {
		var ok bool
		caseID, attempt, ok = r.NextPendingGenerationAttempt()
		if !ok {
			return "", 0, fmt.Errorf("%w: no pending target", ErrRecoveryNotAllowed)
		}
	}
	return r.appendRecovery(id, caseID, attempt, actor, reason, at)
}

// RequestExpiredPreparationRecovery is the automatic recovery boundary. It
// fail-closes unless the aggregate still owns the exact prepared invocation
// observed by the scanner and its lease is expired. Dispatching executions are
// therefore never reawakened automatically, including under scan/commit races.
func (r *PromptEvaluationRun) RequestExpiredPreparationRecovery(id, invocationID string, observedLeaseExpiresAt time.Time, actor, reason string, at time.Time) (string, int, error) {
	invocationID = strings.TrimSpace(invocationID)
	if r == nil || r.status != StatusCollecting || len(r.recoveries) >= MaxRecoveryRequests || r.execution == nil ||
		r.execution.Phase != AttemptExecutionPrepared || invocationID == "" || r.execution.InvocationID != invocationID ||
		observedLeaseExpiresAt.IsZero() || !r.execution.LeaseExpiresAt.Equal(observedLeaseExpiresAt) || !r.execution.LeaseExpired(at) {
		return "", 0, fmt.Errorf("%w: exact expired prepared execution is required", ErrRecoveryNotAllowed)
	}
	return r.appendRecovery(id, r.execution.CaseID, r.execution.Attempt, actor, reason, at)
}

func (r *PromptEvaluationRun) appendRecovery(id, caseID string, attempt int, actor, reason string, at time.Time) (string, int, error) {
	id, actor, reason = strings.TrimSpace(id), strings.TrimSpace(actor), strings.TrimSpace(reason)
	request := RecoveryRequest{ID: id, CaseID: caseID, Attempt: attempt, Actor: actor, Reason: reason, RequestedAt: at}
	if err := request.Validate(); err != nil || at.Before(r.createdAt) {
		return "", 0, fmt.Errorf("%w: audit is invalid", ErrRecoveryNotAllowed)
	}
	for _, existing := range r.recoveries {
		if existing.ID == request.ID {
			return "", 0, fmt.Errorf("%w: request is duplicated", ErrRecoveryNotAllowed)
		}
	}
	r.recoveries = append(r.recoveries, request)
	r.version++
	return caseID, attempt, nil
}

// CanCancel reports whether cancellation is a valid governance action. A
// collecting run may be canceled only while no Provider call can be in flight.
// An inventory-complete run may be canceled only when immutable attempt
// evidence proves a technical failure; this prevents cancellation from being
// used to bypass human review of a successful release candidate.
func (r *PromptEvaluationRun) CanCancel() bool {
	if r == nil {
		return false
	}
	switch r.status {
	case StatusCollecting:
		return r.execution == nil || r.execution.Phase != AttemptExecutionDispatching
	case StatusAwaitingReview:
		return r.FailedAttemptCount() > 0
	default:
		return false
	}
}

// Cancel records an audited terminal decision while preserving all completed
// attempt and review evidence.
func (r *PromptEvaluationRun) Cancel(actor, reason string, at time.Time) error {
	actor, reason = strings.TrimSpace(actor), strings.TrimSpace(reason)
	if r == nil || !r.CanCancel() || actor == "" || len(actor) > 256 || reason == "" || len(reason) > 1000 ||
		at.IsZero() || at.Before(r.createdAt) || r.closedAt != nil && at.Before(*r.closedAt) {
		return fmt.Errorf("%w: cancellation requires a safe collecting run or immutable technical failure evidence", ErrCancellationNotAllowed)
	}
	r.status = StatusCanceled
	r.execution = nil
	r.canceledAt = copyTime(at)
	r.canceledBy = actor
	r.cancelReason = reason
	r.version++
	return nil
}

func (r *PromptEvaluationRun) CompleteAttemptExecution(owner string, value AttemptRecord) error {
	owner = strings.TrimSpace(owner)
	if r == nil || r.status != StatusCollecting || r.execution == nil || r.execution.Phase != AttemptExecutionDispatching ||
		owner == "" || owner != r.execution.Owner || value.CaseID != r.execution.CaseID || value.Attempt != r.execution.Attempt ||
		value.Stage != AttemptStageGeneration || r.execution.DispatchStartedAt == nil || value.StartedAt.Before(*r.execution.DispatchStartedAt) {
		return fmt.Errorf("dispatching AI explanation evaluation execution and matching attempt evidence are required")
	}
	execution := r.execution
	r.execution = nil
	if err := r.addAttempt(value); err != nil {
		r.execution = execution
		return err
	}
	return nil
}

func (r *PromptEvaluationRun) CloseCollection(at time.Time) error {
	if r == nil || r.status != StatusCollecting || at.IsZero() || at.Before(r.createdAt) {
		return fmt.Errorf("collecting AI explanation evaluation run and close time are required")
	}
	if !r.inventoryComplete() {
		return fmt.Errorf("AI explanation evaluation run inventory is incomplete")
	}
	for _, attempt := range r.attempts {
		if attempt.FinishedAt.After(at) {
			return fmt.Errorf("AI explanation evaluation collection cannot close before an attempt finishes")
		}
	}
	r.status = StatusAwaitingReview
	r.closedAt = copyTime(at)
	r.version++
	return nil
}

func (r *PromptEvaluationRun) AddHumanReview(value HumanReview) error {
	if r == nil || r.status != StatusAwaitingReview || r.FailedAttemptCount() > 0 {
		return fmt.Errorf("AI explanation evaluation run awaiting review is required")
	}
	if err := value.Validate(); err != nil {
		return err
	}
	if value.ReviewedAt.Before(*r.closedAt) {
		return fmt.Errorf("AI explanation human review predates collection close")
	}
	if !r.hasGenerationAttempt(value.CaseID, value.Attempt) {
		return fmt.Errorf("AI explanation human review target does not exist")
	}
	value.Reviewer = strings.TrimSpace(value.Reviewer)
	value.Reason = strings.TrimSpace(value.Reason)
	for _, existing := range r.reviews {
		if existing.CaseID != value.CaseID || existing.Attempt != value.Attempt {
			continue
		}
		if existing.Role == value.Role {
			return fmt.Errorf("AI explanation human review role is duplicated for an attempt")
		}
		if strings.TrimSpace(existing.Reviewer) == value.Reviewer {
			return fmt.Errorf("AI explanation human review roles require distinct reviewers")
		}
	}
	r.reviews = append(r.reviews, value)
	r.version++
	return nil
}

// Finalize makes the run immutable. A failed or incomplete gate becomes a
// durable rejected result; callers must start a new run rather than rewriting
// release evidence.
func (r *PromptEvaluationRun) Finalize(actor, reason string, at time.Time) error {
	if r == nil || r.status != StatusAwaitingReview || r.FailedAttemptCount() > 0 || strings.TrimSpace(actor) == "" || strings.TrimSpace(reason) == "" || at.IsZero() || at.Before(*r.closedAt) {
		return fmt.Errorf("AI explanation evaluation finalization audit is required")
	}
	for _, review := range r.reviews {
		if review.ReviewedAt.After(at) {
			return fmt.Errorf("AI explanation evaluation cannot finalize before a review is recorded")
		}
	}
	gate := r.evaluateGate()
	if gate.Passed {
		r.status = StatusApproved
	} else {
		r.status = StatusRejected
	}
	r.gate = &gate
	r.finalizedAt = copyTime(at)
	r.finalizedBy = strings.TrimSpace(actor)
	r.finalReason = strings.TrimSpace(reason)
	r.version++
	return nil
}

func (r *PromptEvaluationRun) evaluateGate() GateResult {
	result := GateResult{Passed: true}
	if !r.inventoryComplete() {
		result.addReason("inventory_incomplete", "", 0, "expected 35 generation attempts and one preflight")
	}

	attempts := r.sortedGenerationAttempts()
	result.Metrics.GenerationAttempts = len(attempts)
	result.Metrics.HumanReviews = len(r.reviews)
	var scoreTotals SemanticScores
	semanticCount := 0
	casePasses := make(map[string]int, len(r.release.GenerationCaseIDs))
	for _, attempt := range attempts {
		if attempt.Failure != nil {
			result.addReason("attempt_execution_failed", attempt.CaseID, attempt.Attempt, attempt.Failure.Stage+":"+attempt.Failure.Code)
		}
		caseAssertionPresent, caseAssertionPassed := evaluateAssertionGroups(&result, attempt)
		if !caseAssertionPresent {
			caseAssertionPassed = false
			result.addReason("case_assertion_missing", attempt.CaseID, attempt.Attempt, "no case-specific assertion receipt")
		}
		if caseAssertionPassed {
			casePasses[attempt.CaseID]++
			result.Metrics.CaseAssertionPasses++
		}
		if attempt.Semantic == nil {
			result.addReason("semantic_review_missing", attempt.CaseID, attempt.Attempt, "semantic rubric receipt is required")
		} else {
			semanticCount++
			scores := attempt.Semantic.Scores
			scoreTotals.Faithfulness += scores.Faithfulness
			scoreTotals.CrossDimensionQuality += scores.CrossDimensionQuality
			scoreTotals.SuggestionActionability += scores.SuggestionActionability
			scoreTotals.AudienceClarity += scores.AudienceClarity
			scoreTotals.Concision += scores.Concision
			if scores.Faithfulness < 4 || scores.CrossDimensionQuality < 3 || scores.SuggestionActionability < 3 || scores.AudienceClarity < 3 || scores.Concision < 3 {
				result.addReason("semantic_score_below_minimum", attempt.CaseID, attempt.Attempt, "one or more rubric scores are below the per-output minimum")
			}
		}
		r.evaluateReviews(&result, attempt)
	}

	for _, caseID := range r.release.GenerationCaseIDs {
		if casePasses[caseID] < 4 {
			result.addReason("case_assertion_stability_failed", caseID, 0, "fewer than four of five attempts passed case assertions")
		}
	}
	if result.Metrics.CaseAssertionPasses < 32 {
		result.addReason("case_assertion_overall_failed", "", 0, "fewer than 90 percent of generation attempts passed case assertions")
	}
	if semanticCount > 0 {
		denominator := float64(semanticCount)
		result.Metrics.FaithfulnessAverage = float64(scoreTotals.Faithfulness) / denominator
		result.Metrics.CrossDimensionAverage = float64(scoreTotals.CrossDimensionQuality) / denominator
		result.Metrics.ActionabilityAverage = float64(scoreTotals.SuggestionActionability) / denominator
		result.Metrics.AudienceClarityAverage = float64(scoreTotals.AudienceClarity) / denominator
		result.Metrics.ConcisionAverage = float64(scoreTotals.Concision) / denominator
	}
	if semanticCount != RequiredGenerationAttempts || result.Metrics.FaithfulnessAverage < 4.5 || result.Metrics.CrossDimensionAverage < 4 || result.Metrics.ActionabilityAverage < 4 || result.Metrics.AudienceClarityAverage < 4 || result.Metrics.ConcisionAverage < 4 {
		result.addReason("semantic_average_below_threshold", "", 0, "rubric averages do not meet the release threshold")
	}

	preflight := r.findAttempt(r.release.PreflightCaseID, 1)
	if preflight == nil || preflight.Stage != AttemptStagePreflight || preflight.ProviderCallCount != 0 || preflight.RejectionReason != r.release.PreflightRejectionReason {
		result.addReason("preflight_failed", r.release.PreflightCaseID, 1, "preflight must reject before any provider call")
	} else {
		requiredTypes := []string{"provider_call_count", "rejection_reason"}
		required := map[string]bool{"provider_call_count": false, "rejection_reason": false}
		for _, assertion := range preflight.Assertions {
			if assertion.Status != AssertionPassed {
				result.addReason("preflight_assertion_not_passed", preflight.CaseID, preflight.Attempt, assertion.Type+":"+string(assertion.Status))
			}
			if _, expected := required[assertion.Type]; expected && assertion.Status == AssertionPassed {
				required[assertion.Type] = true
			}
		}
		for _, assertionType := range requiredTypes {
			if !required[assertionType] {
				result.addReason("preflight_assertion_missing", preflight.CaseID, preflight.Attempt, assertionType)
			}
		}
	}
	result.Passed = len(result.Reasons) == 0
	return result
}

type assertionGroup struct {
	typeName  string
	scope     AssertionScope
	ordinal   int
	hard      bool
	passed    bool
	failed    bool
	lastState AssertionStatus
}

// Multiple evaluators may contribute to one assertion. A deterministic
// pending_semantic receipt is resolved by a later semantic receipt; a failed
// or blocked receipt is never hidden by another evaluator's pass.
func evaluateAssertionGroups(result *GateResult, attempt AttemptRecord) (bool, bool) {
	groups := make(map[string]*assertionGroup, len(attempt.Assertions))
	order := make([]string, 0, len(attempt.Assertions))
	for _, assertion := range attempt.Assertions {
		key := string(assertion.Scope) + "\x00" + assertion.Type + "\x00" + fmt.Sprint(assertion.Ordinal)
		group, exists := groups[key]
		if !exists {
			group = &assertionGroup{typeName: assertion.Type, scope: assertion.Scope, ordinal: assertion.Ordinal}
			groups[key] = group
			order = append(order, key)
		}
		group.hard = group.hard || assertion.Hard || assertion.Scope == AssertionScopeDefault
		group.passed = group.passed || assertion.Status == AssertionPassed
		group.failed = group.failed || assertion.Status == AssertionFailed || assertion.Status == AssertionBlocked
		group.lastState = assertion.Status
	}
	casePresent := false
	casePassed := true
	for _, key := range order {
		group := groups[key]
		effectivePassed := group.passed && !group.failed
		if group.scope == AssertionScopeCase {
			casePresent = true
			casePassed = casePassed && effectivePassed
		}
		if group.hard && !effectivePassed {
			result.addReason("hard_assertion_not_passed", attempt.CaseID, attempt.Attempt, fmt.Sprintf("%s[%d]:%s", group.typeName, group.ordinal, group.lastState))
		}
	}
	return casePresent, casePassed
}

func (r *PromptEvaluationRun) evaluateReviews(result *GateResult, attempt AttemptRecord) {
	found := map[ReviewRole]bool{}
	for _, review := range r.reviews {
		if review.CaseID != attempt.CaseID || review.Attempt != attempt.Attempt {
			continue
		}
		found[review.Role] = true
		if review.Decision == ReviewDecisionReject {
			result.addReason("human_review_rejected", attempt.CaseID, attempt.Attempt, string(review.Role)+":"+review.Reason)
		}
	}
	for _, role := range []ReviewRole{ReviewRoleAssessmentSemantics, ReviewRoleSafetyProduct} {
		if !found[role] {
			result.addReason("human_review_incomplete", attempt.CaseID, attempt.Attempt, string(role)+" review is required")
		}
	}
}

func (r *PromptEvaluationRun) inventoryComplete() bool {
	if len(r.attempts) != RequiredGenerationAttempts+1 {
		return false
	}
	for _, caseID := range r.release.GenerationCaseIDs {
		for attempt := 1; attempt <= r.release.RepetitionsPerCase; attempt++ {
			found := r.findAttempt(caseID, attempt)
			if found == nil || found.Stage != AttemptStageGeneration {
				return false
			}
		}
	}
	preflight := r.findAttempt(r.release.PreflightCaseID, 1)
	return preflight != nil && preflight.Stage == AttemptStagePreflight
}

func (r *PromptEvaluationRun) findAttempt(caseID string, attempt int) *AttemptRecord {
	for index := range r.attempts {
		if r.attempts[index].CaseID == caseID && r.attempts[index].Attempt == attempt {
			return &r.attempts[index]
		}
	}
	return nil
}

func (r *PromptEvaluationRun) hasGenerationAttempt(caseID string, attempt int) bool {
	found := r.findAttempt(caseID, attempt)
	return found != nil && found.Stage == AttemptStageGeneration
}

func (r *PromptEvaluationRun) sortedGenerationAttempts() []AttemptRecord {
	values := make([]AttemptRecord, 0, RequiredGenerationAttempts)
	for _, value := range r.attempts {
		if value.Stage == AttemptStageGeneration {
			values = append(values, cloneAttempt(value))
		}
	}
	caseOrder := make(map[string]int, len(r.release.GenerationCaseIDs))
	for index, caseID := range r.release.GenerationCaseIDs {
		caseOrder[caseID] = index
	}
	sort.Slice(values, func(i, j int) bool {
		if caseOrder[values[i].CaseID] == caseOrder[values[j].CaseID] {
			return values[i].Attempt < values[j].Attempt
		}
		return caseOrder[values[i].CaseID] < caseOrder[values[j].CaseID]
	})
	return values
}

func (g *GateResult) addReason(code, caseID string, attempt int, detail string) {
	g.Reasons = append(g.Reasons, GateReason{Code: code, CaseID: caseID, Attempt: attempt, Detail: detail})
}

type PersistedInput struct {
	ID             meta.ID
	Release        ReleaseIdentity
	Status         Status
	Version        int64
	Attempts       []AttemptRecord
	Reviews        []HumanReview
	Execution      *AttemptExecution
	Recoveries     []RecoveryRequest
	RequestedOrgID int64
	RequestedBy    string
	RequestReason  string
	CreatedAt      time.Time
	ClosedAt       *time.Time
	FinalizedAt    *time.Time
	FinalizedBy    string
	FinalReason    string
	Gate           *GateResult
	CanceledAt     *time.Time
	CanceledBy     string
	CancelReason   string
}

func Restore(input PersistedInput) (*PromptEvaluationRun, error) {
	run, err := New(input.ID, input.Release, input.CreatedAt)
	if err != nil {
		return nil, err
	}
	if input.Version < 1 || !input.Status.IsValid() {
		return nil, fmt.Errorf("AI explanation evaluation persistence state is invalid")
	}
	for _, attempt := range input.Attempts {
		if err := run.AddAttempt(attempt); err != nil {
			return nil, err
		}
	}
	run.status = input.Status
	run.reviews = append([]HumanReview(nil), input.Reviews...)
	run.execution = cloneAttemptExecution(input.Execution)
	run.recoveries = append([]RecoveryRequest(nil), input.Recoveries...)
	run.requestedOrgID = input.RequestedOrgID
	run.requestedBy = strings.TrimSpace(input.RequestedBy)
	run.requestReason = strings.TrimSpace(input.RequestReason)
	emptyAudit := run.requestedOrgID == 0 && run.requestedBy == "" && run.requestReason == ""
	completeAudit := run.requestedOrgID > 0 && run.requestedBy != "" && run.requestReason != ""
	if (!emptyAudit && !completeAudit) || len(run.requestedBy) > 256 || len(run.requestReason) > 1000 {
		return nil, fmt.Errorf("AI explanation evaluation persisted request audit is invalid")
	}
	seenReviews := make(map[string]struct{}, len(run.reviews))
	seenReviewers := make(map[string]struct{}, len(run.reviews))
	for _, review := range run.reviews {
		key := review.CaseID + "\x00" + fmt.Sprint(review.Attempt) + "\x00" + string(review.Role)
		_, duplicated := seenReviews[key]
		reviewerKey := review.CaseID + "\x00" + fmt.Sprint(review.Attempt) + "\x00" + strings.TrimSpace(review.Reviewer)
		_, reviewerDuplicated := seenReviewers[reviewerKey]
		if err := review.Validate(); err != nil || !run.hasGenerationAttempt(review.CaseID, review.Attempt) || duplicated || reviewerDuplicated {
			return nil, fmt.Errorf("AI explanation evaluation persisted human review is invalid")
		}
		seenReviews[key] = struct{}{}
		seenReviewers[reviewerKey] = struct{}{}
	}
	run.closedAt = copyTimePtr(input.ClosedAt)
	run.finalizedAt = copyTimePtr(input.FinalizedAt)
	run.finalizedBy = input.FinalizedBy
	run.finalReason = input.FinalReason
	run.gate = cloneGate(input.Gate)
	run.canceledAt = copyTimePtr(input.CanceledAt)
	run.canceledBy = strings.TrimSpace(input.CanceledBy)
	run.cancelReason = strings.TrimSpace(input.CancelReason)
	run.version = input.Version
	if err := run.validateLifecycle(); err != nil {
		return nil, err
	}
	return run, nil
}

func (r *PromptEvaluationRun) validateLifecycle() error {
	if err := r.validateRecoveryHistory(); err != nil {
		return err
	}
	switch r.status {
	case StatusCollecting:
		if r.closedAt != nil || r.finalizedAt != nil || r.canceledAt != nil || len(r.reviews) != 0 || r.gate != nil || r.finalizedBy != "" || r.finalReason != "" || r.canceledBy != "" || r.cancelReason != "" {
			return fmt.Errorf("collecting AI explanation evaluation lifecycle is invalid")
		}
		if r.execution != nil {
			caseID, attempt, ok := r.NextPendingGenerationAttempt()
			if err := r.execution.Validate(); err != nil || !ok || r.execution.CaseID != caseID || r.execution.Attempt != attempt || r.execution.ClaimedAt.Before(r.createdAt) {
				return fmt.Errorf("collecting AI explanation evaluation execution checkpoint is invalid")
			}
		}
	case StatusAwaitingReview:
		if r.execution != nil || r.canceledAt != nil || r.canceledBy != "" || r.cancelReason != "" || r.closedAt == nil || !r.inventoryComplete() || !r.reviewTimesValid() || r.finalizedAt != nil || r.gate != nil || r.finalizedBy != "" || r.finalReason != "" {
			return fmt.Errorf("AI explanation evaluation review lifecycle is invalid")
		}
	case StatusApproved, StatusRejected:
		if r.execution != nil || r.canceledAt != nil || r.canceledBy != "" || r.cancelReason != "" || r.closedAt == nil || !r.inventoryComplete() || !r.reviewTimesValid() || r.finalizedAt == nil || r.finalizedAt.Before(*r.closedAt) || strings.TrimSpace(r.finalizedBy) == "" || strings.TrimSpace(r.finalReason) == "" || r.gate == nil {
			return fmt.Errorf("AI explanation evaluation terminal lifecycle is invalid")
		}
		expected := r.evaluateGate()
		if !reflect.DeepEqual(*r.gate, expected) || (r.status == StatusApproved) != expected.Passed {
			return fmt.Errorf("AI explanation evaluation persisted gate is invalid")
		}
	case StatusCanceled:
		if r.execution != nil || r.finalizedAt != nil || r.gate != nil ||
			r.canceledAt == nil || r.canceledAt.Before(r.createdAt) || r.canceledBy == "" || r.cancelReason == "" ||
			r.finalizedBy != "" || r.finalReason != "" {
			return fmt.Errorf("AI explanation evaluation canceled lifecycle is invalid")
		}
		if r.closedAt == nil {
			if len(r.reviews) != 0 {
				return fmt.Errorf("AI explanation evaluation collecting cancellation cannot contain reviews")
			}
		} else if !r.inventoryComplete() || r.FailedAttemptCount() == 0 || r.canceledAt.Before(*r.closedAt) || !r.reviewTimesValid() {
			return fmt.Errorf("AI explanation evaluation post-collection cancellation requires technical failure evidence")
		}
	}
	return nil
}

func (r *PromptEvaluationRun) validateRecoveryHistory() error {
	if len(r.recoveries) > MaxRecoveryRequests {
		return fmt.Errorf("AI explanation evaluation recovery history exceeds its bound")
	}
	seen := make(map[string]struct{}, len(r.recoveries))
	for _, recovery := range r.recoveries {
		if err := recovery.Validate(); err != nil || recovery.RequestedAt.Before(r.createdAt) ||
			!r.release.IsGenerationCase(recovery.CaseID) || recovery.Attempt > r.release.RepetitionsPerCase {
			return fmt.Errorf("AI explanation evaluation persisted recovery audit is invalid")
		}
		if _, exists := seen[recovery.ID]; exists {
			return fmt.Errorf("AI explanation evaluation persisted recovery audit is duplicated")
		}
		seen[recovery.ID] = struct{}{}
		if r.closedAt != nil && recovery.RequestedAt.After(*r.closedAt) || r.canceledAt != nil && recovery.RequestedAt.After(*r.canceledAt) {
			return fmt.Errorf("AI explanation evaluation recovery audit postdates terminal collection")
		}
	}
	return nil
}

func (r *PromptEvaluationRun) reviewTimesValid() bool {
	if r.closedAt == nil {
		return false
	}
	for _, attempt := range r.attempts {
		if attempt.FinishedAt.After(*r.closedAt) {
			return false
		}
	}
	for _, review := range r.reviews {
		if review.ReviewedAt.Before(*r.closedAt) || r.finalizedAt != nil && review.ReviewedAt.After(*r.finalizedAt) ||
			r.canceledAt != nil && review.ReviewedAt.After(*r.canceledAt) {
			return false
		}
	}
	return true
}

func (r *PromptEvaluationRun) ID() meta.ID              { return r.id }
func (r *PromptEvaluationRun) Status() Status           { return r.status }
func (r *PromptEvaluationRun) Version() int64           { return r.version }
func (r *PromptEvaluationRun) Release() ReleaseIdentity { return cloneRelease(r.release) }
func (r *PromptEvaluationRun) CreatedAt() time.Time     { return r.createdAt }
func (r *PromptEvaluationRun) ClosedAt() *time.Time     { return copyTimePtr(r.closedAt) }
func (r *PromptEvaluationRun) FinalizedAt() *time.Time  { return copyTimePtr(r.finalizedAt) }
func (r *PromptEvaluationRun) FinalizedBy() string      { return r.finalizedBy }
func (r *PromptEvaluationRun) FinalReason() string      { return r.finalReason }
func (r *PromptEvaluationRun) Gate() *GateResult        { return cloneGate(r.gate) }
func (r *PromptEvaluationRun) CanceledAt() *time.Time   { return copyTimePtr(r.canceledAt) }
func (r *PromptEvaluationRun) CanceledBy() string       { return r.canceledBy }
func (r *PromptEvaluationRun) CancelReason() string     { return r.cancelReason }
func (r *PromptEvaluationRun) RequestedOrgID() int64    { return r.requestedOrgID }
func (r *PromptEvaluationRun) RequestedBy() string      { return r.requestedBy }
func (r *PromptEvaluationRun) RequestReason() string    { return r.requestReason }
func (r *PromptEvaluationRun) Execution() *AttemptExecution {
	return cloneAttemptExecution(r.execution)
}
func (r *PromptEvaluationRun) FailedAttemptCount() int {
	if r == nil {
		return 0
	}
	count := 0
	for _, attempt := range r.attempts {
		if attempt.Stage == AttemptStageGeneration && attempt.Failure != nil {
			count++
		}
	}
	return count
}
func (r *PromptEvaluationRun) IsPublishEvidence() bool {
	return r.status == StatusApproved && r.gate != nil && r.gate.Passed
}
func (r *PromptEvaluationRun) Attempts() []AttemptRecord {
	values := make([]AttemptRecord, len(r.attempts))
	for index := range r.attempts {
		values[index] = cloneAttempt(r.attempts[index])
	}
	return values
}
func (r *PromptEvaluationRun) Reviews() []HumanReview {
	return append([]HumanReview(nil), r.reviews...)
}
func (r *PromptEvaluationRun) Recoveries() []RecoveryRequest {
	return append([]RecoveryRequest(nil), r.recoveries...)
}

func cloneAttemptExecution(value *AttemptExecution) *AttemptExecution {
	if value == nil {
		return nil
	}
	cloned := *value
	cloned.DispatchStartedAt = copyTimePtr(value.DispatchStartedAt)
	return &cloned
}

func copyTime(value time.Time) *time.Time { copy := value; return &copy }
func copyTimePtr(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	return copyTime(*value)
}
