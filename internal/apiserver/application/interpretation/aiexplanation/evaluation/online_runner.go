package evaluation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"strings"
	"time"

	interpretationschema "github.com/FangcunMount/qs-server/api/schema/interpretation"
	appport "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/port"
	appprompt "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/prompt"
	appvalidation "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/validation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation"
	domainevaluation "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/evaluation"
	domainprofile "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/profile"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

const (
	preflightEvaluatorVersion = "ai-explanation-evaluation-preflight/v1"
	runnerEvaluatorVersion    = "ai-explanation-evaluation-runner/v1"
	MaxProviderInvocationsV1  = domainevaluation.RequiredGenerationAttempts * 2
)

type OnlineRunnerDependencies struct {
	Prompts            appport.PromptPackageResolver
	Schemas            appport.OutputSchemaResolver
	Routes             appport.ProviderRouteResolver
	Provider           appport.Provider
	Safety             appport.SafetyEvaluator
	Semantic           SemanticEvaluator
	SemanticTimeout    time.Duration
	AttemptLease       time.Duration
	Evidence           *EvidenceService
	EvidenceV2         *EvidenceV2Service
	Rechecks           domainevaluation.RecheckRepository
	DurableCommitter   *DurableCommitter
	DurableCommitterV2 *DurableCommitterV2
	Now                func() time.Time
}

// OnlineRunner executes frozen synthetic suites. Legacy RunV1 and recheck
// methods retain the v1 assets, while new evidence-v2 Runs freeze the current
// v2 Prompt/Suite/Profile package. It is not assembled by the default-disabled
// participant runtime and never publishes a Profile directly.
type OnlineRunner struct {
	prompts            appport.PromptPackageResolver
	schemas            appport.OutputSchemaResolver
	routes             appport.ProviderRouteResolver
	provider           appport.Provider
	safety             appport.SafetyEvaluator
	semantic           SemanticEvaluator
	semanticTimeout    time.Duration
	attemptLease       time.Duration
	evidence           *EvidenceService
	evidenceV2         *EvidenceV2Service
	rechecks           domainevaluation.RecheckRepository
	durableCommitter   *DurableCommitter
	durableCommitterV2 *DurableCommitterV2
	preflight          *PreflightRunner
	now                func() time.Time
}

func NewOnlineRunner(dependencies OnlineRunnerDependencies) (*OnlineRunner, error) {
	if dependencies.Prompts == nil || dependencies.Schemas == nil || dependencies.Routes == nil ||
		dependencies.Provider == nil || dependencies.Safety == nil || dependencies.Semantic == nil || dependencies.Evidence == nil {
		return nil, fmt.Errorf("AI explanation online evaluation dependencies are required")
	}
	if (dependencies.EvidenceV2 == nil) != (dependencies.DurableCommitterV2 == nil) {
		return nil, fmt.Errorf("AI explanation online evaluation v2 evidence and committer must be configured together")
	}
	now := dependencies.Now
	if now == nil {
		now = time.Now
	}
	semanticTimeout := dependencies.SemanticTimeout
	if semanticTimeout <= 0 {
		semanticTimeout = time.Minute
	}
	attemptLease := dependencies.AttemptLease
	if attemptLease <= 0 {
		attemptLease = 2*time.Minute + semanticTimeout
	}
	preflight, err := NewPreflightRunner(dependencies.Prompts, dependencies.Schemas, now)
	if err != nil {
		return nil, err
	}
	return &OnlineRunner{
		prompts: dependencies.Prompts, schemas: dependencies.Schemas, routes: dependencies.Routes,
		provider: dependencies.Provider, safety: dependencies.Safety, semantic: dependencies.Semantic,
		semanticTimeout: semanticTimeout, attemptLease: attemptLease,
		evidence: dependencies.Evidence, durableCommitter: dependencies.DurableCommitter,
		evidenceV2: dependencies.EvidenceV2, durableCommitterV2: dependencies.DurableCommitterV2,
		rechecks:  dependencies.Rechecks,
		preflight: preflight, now: now,
	}, nil
}

// OnlineRunCommand is intentionally empty in v1. All release inputs are
// compiled or resolved from frozen catalogs; callers cannot supply evidence
// identities that differ from the bytes the runner actually executes.
type OnlineRunCommand struct{}

type OnlineRunResult struct {
	Preflight *PreflightReport
	Run       *domainevaluation.PromptEvaluationRun
}

type OnlineStartCommand struct {
	RunID       meta.ID
	OrgID       int64
	RequestedBy string
	Reason      string
}

type OnlineStepCommand struct {
	RunID          meta.ID
	CaseID         string
	Attempt        int
	Owner          string
	RequestedOrgID int64
	RequestedBy    string
}

type OnlineStepStatus string

const (
	OnlineStepProgressed       OnlineStepStatus = "progressed"
	OnlineStepAlreadyCompleted OnlineStepStatus = "already_completed"
	OnlineStepAwaitingReview   OnlineStepStatus = "awaiting_review"
	OnlineStepCanceled         OnlineStepStatus = "canceled"
)

type OnlineStepResult struct {
	Status OnlineStepStatus
	Run    *domainevaluation.PromptEvaluationRun
}

var ErrAttemptExecutionBusy = errors.New("AI explanation evaluation attempt execution is still leased")

func (r *OnlineRunner) RunV1(ctx context.Context, _ OnlineRunCommand) (*OnlineRunResult, error) {
	result, err := r.StartV1(ctx)
	if err != nil {
		return result, err
	}
	for {
		caseID, attempt, ok := result.Run.NextPendingGenerationAttempt()
		if !ok {
			return result, nil
		}
		step, stepErr := r.RunStepV1(ctx, OnlineStepCommand{
			RunID: result.Run.ID(), CaseID: caseID, Attempt: attempt,
			Owner: fmt.Sprintf("direct:%s:%s:%d", result.Run.ID(), caseID, attempt),
		})
		if stepErr != nil {
			return result, stepErr
		}
		result.Run = step.Run
	}
}

// StartV1 performs only deterministic preflight and persists the immutable
// release plus zero-call evidence. Durable transports may safely call this
// before enqueueing the first generation attempt.
func (r *OnlineRunner) StartV1(ctx context.Context) (*OnlineRunResult, error) {
	if r == nil {
		return nil, fmt.Errorf("AI explanation online evaluation runner is required")
	}
	suite, preflightReport, prepared, err := r.prepareV1(ctx)
	result := &OnlineRunResult{Preflight: preflightReport}
	if err != nil {
		return result, err
	}
	runRecord, err := r.evidence.Start(ctx, prepared.release)
	if err != nil {
		return result, err
	}
	result.Run = runRecord

	preflightAttempt := r.preflightAttempt(prepared.preflightCase, prepared.release.PreflightRejectionReason)
	runRecord, err = r.evidence.RecordAttempt(ctx, runRecord.ID(), preflightAttempt)
	if err != nil {
		return result, err
	}
	result.Run = runRecord
	_ = suite
	return result, nil
}

// PrepareRequestedV1 builds an audited run with preflight evidence entirely in
// memory. DurableCommitter must atomically create it with the first step event.
func (r *OnlineRunner) PrepareRequestedV1(ctx context.Context, command OnlineStartCommand) (*OnlineRunResult, error) {
	if r == nil || command.RunID.IsZero() {
		return nil, fmt.Errorf("AI explanation online evaluation start command is invalid")
	}
	_, preflightReport, prepared, err := r.prepareV1(ctx)
	result := &OnlineRunResult{Preflight: preflightReport}
	if err != nil {
		return result, err
	}
	runRecord, err := domainevaluation.NewRequested(
		command.RunID, prepared.release, command.OrgID, command.RequestedBy, command.Reason, r.now().UTC(),
	)
	if err != nil {
		return result, err
	}
	if err := runRecord.AddAttempt(r.preflightAttempt(prepared.preflightCase, prepared.release.PreflightRejectionReason)); err != nil {
		return result, err
	}
	result.Run = runRecord
	return result, nil
}

// RunStepV1 executes exactly one event-addressed generation attempt. It writes
// prepared and dispatching checkpoints before Provider work. An expired
// dispatching checkpoint becomes explicit result_unknown evidence and is never
// replayed automatically.
func (r *OnlineRunner) RunStepV1(ctx context.Context, command OnlineStepCommand) (*OnlineStepResult, error) {
	if r == nil || command.RunID.IsZero() || strings.TrimSpace(command.CaseID) == "" || command.Attempt < 1 || strings.TrimSpace(command.Owner) == "" {
		return nil, fmt.Errorf("AI explanation online evaluation step command is invalid")
	}
	runRecord, err := r.evidence.Find(ctx, command.RunID)
	if err != nil {
		return nil, err
	}
	if command.RequestedOrgID != 0 && (runRecord.RequestedOrgID() != command.RequestedOrgID || runRecord.RequestedBy() != strings.TrimSpace(command.RequestedBy)) {
		return nil, fmt.Errorf("AI explanation evaluation request audit does not match the durable event")
	}
	if runRecord.Status() != domainevaluation.StatusCollecting {
		if runRecord.Status() == domainevaluation.StatusCanceled {
			return &OnlineStepResult{Status: OnlineStepCanceled, Run: runRecord}, nil
		}
		if runRecord.HasAttempt(command.CaseID, command.Attempt) {
			return &OnlineStepResult{Status: OnlineStepAlreadyCompleted, Run: runRecord}, nil
		}
		return nil, fmt.Errorf("AI explanation evaluation run is not collecting")
	}
	if runRecord.HasAttempt(command.CaseID, command.Attempt) {
		return &OnlineStepResult{Status: OnlineStepAlreadyCompleted, Run: runRecord}, nil
	}
	_, _, prepared, err := r.prepareV1(ctx)
	if err != nil {
		return nil, err
	}
	if !reflect.DeepEqual(runRecord.Release(), prepared.release) {
		return nil, fmt.Errorf("AI explanation evaluation release no longer matches the executable assets")
	}
	invocationID := fmt.Sprintf("ai-prompt-eval:%s:%s:%d", command.RunID, command.CaseID, command.Attempt)

	now := r.now().UTC()
	if execution := runRecord.Execution(); execution != nil {
		switch {
		case execution.CaseID != command.CaseID || execution.Attempt != command.Attempt:
			return nil, ErrAttemptExecutionBusy
		case execution.Phase == domainevaluation.AttemptExecutionPrepared && execution.LeaseExpired(now):
			runRecord, err = r.evidence.ReleaseExpiredPreparation(ctx, command.RunID, now)
			if err != nil {
				return nil, err
			}
		case execution.Phase == domainevaluation.AttemptExecutionDispatching && execution.LeaseExpired(now):
			return r.recordUnknownDispatch(ctx, prepared, runRecord, command, execution, now)
		case execution.Owner != command.Owner || execution.Phase == domainevaluation.AttemptExecutionDispatching:
			return nil, ErrAttemptExecutionBusy
		}
	}

	if runRecord.Execution() == nil {
		caseID, attempt, ok := runRecord.NextPendingGenerationAttempt()
		if !ok || caseID != command.CaseID || attempt != command.Attempt {
			return nil, fmt.Errorf("AI explanation evaluation step is not the next frozen attempt")
		}
		leaseDuration := r.attemptLease
		minimumLease := prepared.route.Timeout + r.semanticTimeout + 30*time.Second
		if leaseDuration < minimumLease {
			leaseDuration = minimumLease
		}
		_, err = r.evidence.ClaimAttempt(ctx, command.RunID, ClaimAttemptCommand{
			CaseID: command.CaseID, Attempt: command.Attempt, Owner: strings.TrimSpace(command.Owner),
			InvocationID: invocationID, ClaimedAt: now, LeaseExpiresAt: now.Add(leaseDuration),
		})
		if err != nil {
			return nil, err
		}
	}

	slog.InfoContext(ctx, "AI explanation candidate pipeline dispatching",
		slog.String("run_id", command.RunID.String()), slog.String("case_id", command.CaseID),
		slog.Int("attempt", command.Attempt), slog.String("invocation_id", invocationID),
		slog.String("event_id", strings.TrimSpace(command.Owner)),
	)
	dispatchAt := r.now().UTC()
	_, err = r.evidence.MarkAttemptDispatching(ctx, command.RunID, command.Owner, dispatchAt)
	if err != nil {
		return nil, err
	}
	testCase, ok := prepared.caseByID(command.CaseID)
	if !ok {
		return nil, fmt.Errorf("AI explanation evaluation case is not executable")
	}
	record := r.executeAttempt(ctx, prepared, testCase, command.Attempt, command.RunID.String())
	persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	runRecord, err = r.persistCompletedAttempt(persistCtx, command.RunID, command.Owner, record)
	if err != nil {
		return nil, err
	}
	observePromptEvaluationAttemptFailure(record.Failure)
	completionAttrs := []any{
		slog.String("run_id", command.RunID.String()), slog.String("case_id", command.CaseID),
		slog.Int("attempt", command.Attempt), slog.String("invocation_id", invocationID),
		slog.String("event_id", strings.TrimSpace(command.Owner)),
	}
	if record.Failure != nil {
		completionAttrs = append(completionAttrs,
			slog.String("failure_stage", record.Failure.Stage),
			slog.String("failure_code", record.Failure.Code),
		)
	}
	slog.InfoContext(ctx, "AI explanation candidate pipeline committed", completionAttrs...)
	return r.closeWhenComplete(persistCtx, runRecord)
}

func (r *OnlineRunner) prepareV1(ctx context.Context) (*Suite, *PreflightReport, *preparedOnlineRun, error) {
	return r.prepareFrozenSuite(ctx, LoadV1)
}

func (r *OnlineRunner) prepareV2(ctx context.Context) (*Suite, *PreflightReport, *preparedOnlineRun, error) {
	return r.prepareFrozenSuite(ctx, LoadV5)
}

func (r *OnlineRunner) prepareFrozenSuite(ctx context.Context, load func() (*Suite, error)) (*Suite, *PreflightReport, *preparedOnlineRun, error) {
	if load == nil {
		return nil, nil, nil, fmt.Errorf("AI explanation evaluation frozen suite loader is required")
	}
	suite, err := load()
	if err != nil {
		return nil, nil, nil, err
	}
	preflightReport, err := r.preflight.Run(ctx, suite)
	if err != nil {
		return suite, preflightReport, nil, fmt.Errorf("AI explanation online evaluation preflight: %w", err)
	}
	prepared, err := r.prepare(ctx, suite, preflightReport)
	if err != nil {
		return suite, preflightReport, nil, err
	}
	return suite, preflightReport, prepared, nil
}

func (p *preparedOnlineRun) caseByID(caseID string) (Case, bool) {
	if p == nil {
		return Case{}, false
	}
	for _, testCase := range p.generationCases {
		if testCase.CaseID == caseID {
			return testCase, true
		}
	}
	return Case{}, false
}

func (r *OnlineRunner) recordUnknownDispatch(
	ctx context.Context,
	prepared *preparedOnlineRun,
	runRecord *domainevaluation.PromptEvaluationRun,
	command OnlineStepCommand,
	execution *domainevaluation.AttemptExecution,
	now time.Time,
) (*OnlineStepResult, error) {
	slog.WarnContext(ctx, "AI explanation candidate pipeline found an expired dispatch lease",
		slog.String("run_id", command.RunID.String()), slog.String("case_id", command.CaseID),
		slog.Int("attempt", command.Attempt), slog.String("invocation_id", execution.InvocationID),
		slog.String("event_id", execution.Owner),
	)
	startedAt := execution.ClaimedAt
	if execution.DispatchStartedAt != nil {
		startedAt = *execution.DispatchStartedAt
	}
	record := r.failedAttempt(domainevaluation.AttemptRecord{
		CaseID: command.CaseID, Attempt: command.Attempt, Stage: domainevaluation.AttemptStageGeneration,
		StartedAt: startedAt, ProviderCallCount: 1,
	}, "provider_execution", "provider_result_unknown", "Provider result is unknown after an expired dispatch lease", false, true)
	if record.FinishedAt.Before(now) {
		record.FinishedAt = now
	}
	persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	updated, err := r.persistCompletedAttempt(persistCtx, command.RunID, execution.Owner, record)
	if err != nil {
		return nil, err
	}
	observePromptEvaluationAttemptFailure(record.Failure)
	_ = prepared
	return r.closeWhenComplete(persistCtx, updated)
}

func (r *OnlineRunner) closeWhenComplete(ctx context.Context, runRecord *domainevaluation.PromptEvaluationRun) (*OnlineStepResult, error) {
	if runRecord.Status() == domainevaluation.StatusAwaitingReview {
		return &OnlineStepResult{Status: OnlineStepAwaitingReview, Run: runRecord}, nil
	}
	if _, _, pending := runRecord.NextPendingGenerationAttempt(); pending {
		return &OnlineStepResult{Status: OnlineStepProgressed, Run: runRecord}, nil
	}
	closed, err := r.evidence.CloseCollection(ctx, runRecord.ID())
	if err != nil {
		return nil, err
	}
	return &OnlineStepResult{Status: OnlineStepAwaitingReview, Run: closed}, nil
}

func (r *OnlineRunner) persistCompletedAttempt(
	ctx context.Context,
	runID meta.ID,
	owner string,
	record domainevaluation.AttemptRecord,
) (*domainevaluation.PromptEvaluationRun, error) {
	if r.durableCommitter != nil {
		return r.durableCommitter.CommitAttempt(ctx, runID, owner, record)
	}
	return r.evidence.CompleteAttempt(ctx, runID, owner, record)
}

type preparedOnlineRun struct {
	release           domainevaluation.ReleaseIdentity
	profile           *domainprofile.AIExplanationProfile
	prompt            appport.PromptPackage
	outputSchema      appport.StructuredOutputSchema
	route             appport.ProviderRoute
	preflightCase     Case
	generationCases   []Case
	defaultAssertions []Assertion
}

func (r *OnlineRunner) prepare(
	ctx context.Context,
	suite *Suite,
	preflightReport *PreflightReport,
) (*preparedOnlineRun, error) {
	profileRecord, err := buildProfile(suite.ProfileFixture, r.now().UTC())
	if err != nil {
		return nil, err
	}
	definition := profileRecord.Definition()
	promptPackage, err := r.prompts.ResolvePromptPackage(ctx, suite.Prompt.TemplateID, suite.Prompt.Version)
	if err != nil {
		return nil, err
	}
	if err := promptPackage.Validate(); err != nil {
		return nil, err
	}
	outputSchema, err := r.schemas.ResolveOutputSchema(ctx, suite.Contracts.OutputSchema)
	if err != nil {
		return nil, err
	}
	if err := outputSchema.Validate(); err != nil {
		return nil, err
	}
	route, err := r.routes.ResolveProviderRoute(ctx, definition.GenerationPolicy.ProviderRoute)
	if err != nil {
		return nil, err
	}
	if err := route.Validate(); err != nil {
		return nil, err
	}
	semanticEvaluator := r.semantic.Identity()
	if err := semanticEvaluator.Validate(); err != nil {
		return nil, fmt.Errorf("AI explanation semantic evaluator identity: %w", err)
	}
	profileFingerprint, err := aiexplanation.ParseFingerprint(suite.ProfileFixture.Fingerprint)
	if err != nil {
		return nil, err
	}
	suiteFingerprint, suiteGitBlobSHA, err := frozenSuiteIdentity(suite)
	if err != nil {
		return nil, err
	}

	generationCases := make([]Case, 0, domainevaluation.RequiredGenerationCaseCount)
	var preflightCase *Case
	preflightReason := ""
	for index := range suite.Cases {
		testCase := suite.Cases[index]
		switch testCase.Stage {
		case "generation":
			generationCases = append(generationCases, testCase)
		case "preflight":
			copy := testCase
			preflightCase = &copy
		}
	}
	for _, caseResult := range preflightReport.Cases {
		if caseResult.Stage == "preflight" {
			preflightReason = caseResult.RejectionReason
			break
		}
	}
	if preflightCase == nil || preflightReason == "" {
		return nil, fmt.Errorf("AI explanation online evaluation preflight evidence is incomplete")
	}
	caseIDs := make([]string, 0, len(generationCases))
	for _, testCase := range generationCases {
		caseIDs = append(caseIDs, testCase.CaseID)
	}
	release := domainevaluation.ReleaseIdentity{
		Suite: domainevaluation.SuiteRef{
			ID: suite.SuiteID, Version: suite.SuiteVersion, Fingerprint: suiteFingerprint,
			GitBlobSHA: suiteGitBlobSHA,
		},
		Prompt: promptPackage.Ref,
		Profile: aiexplanation.ProfileRef{
			ID: suite.ProfileFixture.ProfileID, Version: suite.ProfileFixture.Version, Fingerprint: profileFingerprint,
		},
		InputSchema: domainevaluation.SchemaRef{
			Version: suite.Contracts.InputSchema, Fingerprint: aiexplanation.NewFingerprint(interpretationschema.AIExplanationInputV1()),
		},
		OutputSchema: domainevaluation.SchemaRef{Version: outputSchema.Version, Fingerprint: outputSchema.Fingerprint},
		Provider:     route.ExecutionSpec,
		Decoding: domainevaluation.DecodingParameters{
			MaxOutputTokens: route.MaxOutputTokens, ReasoningEffort: route.ReasoningEffort,
		},
		SemanticEvaluator: semanticEvaluator,
		GenerationCaseIDs: caseIDs, PreflightCaseID: preflightCase.CaseID,
		PreflightRejectionReason: preflightReason, RepetitionsPerCase: suite.ExecutionPolicy.GenerationRepetitionsPerCase,
	}
	if err := release.Validate(); err != nil {
		return nil, err
	}
	return &preparedOnlineRun{
		release: release, profile: profileRecord, prompt: promptPackage, outputSchema: outputSchema, route: route,
		preflightCase: *preflightCase, generationCases: generationCases,
		defaultAssertions: cloneAssertions(suite.DefaultGenerationAssertions),
	}, nil
}

func (r *OnlineRunner) preflightAttempt(testCase Case, rejectionReason string) domainevaluation.AttemptRecord {
	startedAt := r.now().UTC()
	assertions := make([]domainevaluation.AssertionReceipt, 0, len(testCase.Expected.Assertions))
	ordinals := map[string]int{}
	for _, assertion := range testCase.Expected.Assertions {
		ordinals[assertion.Type]++
		assertions = append(assertions, domainevaluation.AssertionReceipt{
			Type: assertion.Type, Scope: domainevaluation.AssertionScopeCase, Ordinal: ordinals[assertion.Type],
			Hard: true, Evaluator: preflightEvaluatorVersion, Status: domainevaluation.AssertionPassed,
		})
	}
	return domainevaluation.AttemptRecord{
		CaseID: testCase.CaseID, Attempt: 1, Stage: domainevaluation.AttemptStagePreflight,
		StartedAt: startedAt, FinishedAt: r.finishTime(startedAt), ProviderCallCount: 0,
		RejectionReason: rejectionReason, Assertions: assertions,
	}
}

func (r *OnlineRunner) executeAttempt(
	ctx context.Context,
	prepared *preparedOnlineRun,
	testCase Case,
	attempt int,
	runID string,
) domainevaluation.AttemptRecord {
	startedAt := r.now().UTC()
	base := domainevaluation.AttemptRecord{
		CaseID: testCase.CaseID, Attempt: attempt, Stage: domainevaluation.AttemptStageGeneration, StartedAt: startedAt,
	}
	assembled, err := syntheticInput(testCase.ProviderPayload, prepared.profile, testCase.CaseID)
	if err != nil {
		return r.failedAttempt(base, "input_assembly", "synthetic_input_invalid", "synthetic evaluation input could not be assembled", false, false)
	}
	messages, err := appprompt.Render(prepared.prompt, prepared.profile.Definition(), assembled)
	if err != nil {
		return r.failedAttempt(base, "prompt_render", "prompt_render_failed", "evaluation Prompt could not be rendered", false, false)
	}
	invocationID := fmt.Sprintf("ai-prompt-eval:%s:%s:%d", runID, testCase.CaseID, attempt)
	request := appport.ProviderRequest{
		InvocationID: invocationID, Route: prepared.route,
		SystemMessage: messages.SystemMessage, TaskMessage: messages.TaskMessage,
		DataPreamble: messages.DataPreamble, DataJSON: messages.DataJSON, OutputSchema: prepared.outputSchema,
	}
	providerContext, cancel := context.WithTimeout(ctx, prepared.route.Timeout)
	response, providerErr := r.provider.Generate(providerContext, request)
	cancel()
	base.ProviderCallCount = 1
	if providerErr != nil {
		failure := classifyAttemptFailure("provider_execution", providerErr)
		return r.failedAttemptValue(base, failure)
	}
	if response == nil {
		return r.failedAttempt(base, "provider_execution", "provider_response_missing", "Provider returned no response", false, true)
	}
	base.RawOutput = append([]byte(nil), response.RawOutput...)
	receipt := response.Receipt
	base.ProviderReceipt = &receipt
	if len(base.RawOutput) > domainevaluation.MaxStoredOutputBytes {
		base.RawOutput = nil
		return r.failedAttempt(base, "evidence_capture", "provider_output_too_large", "Provider output exceeded the evaluation evidence limit", false, false)
	}
	if err := validateProviderReceipt(receipt, invocationID, prepared.route.ExecutionSpec); err != nil {
		if receipt.Provider != prepared.route.ExecutionSpec.ResolvedProvider || receipt.Model != prepared.route.ExecutionSpec.ResolvedModel || receipt.Validate() != nil {
			base.ProviderReceipt = nil
		}
		return r.failedAttempt(base, "evidence_capture", "provider_receipt_invalid", "Provider receipt did not match the frozen evaluation request", false, true)
	}

	validationOutput := response.OutputForValidation()
	validated, validationErr := appvalidation.Validate(validationOutput, assembled.Document, prepared.profile.Definition())
	if validationErr == nil {
		base.NormalizedOutput, err = json.Marshal(validated.Content)
		if err != nil {
			return r.failedAttempt(base, "output_normalization", "output_normalization_failed", "validated output could not be normalized", false, false)
		}
		base.OutputFingerprint = aiexplanation.NewFingerprint(base.NormalizedOutput)
	}

	allAssertions := append(cloneAssertions(prepared.defaultAssertions), cloneAssertions(testCase.Expected.Assertions)...)
	candidate, err := EvaluateCandidate(ctx, validationOutput, assembled.Document, prepared.profile.Definition(), allAssertions, r.safety)
	if err != nil {
		return r.failedAttempt(base, "deterministic_evaluation", "candidate_evaluation_failed", "candidate safety evaluation could not complete", false, false)
	}
	receipts, semanticObligations, err := candidateReceipts(candidate, allAssertions, len(prepared.defaultAssertions))
	if err != nil {
		return r.failedAttempt(base, "deterministic_evaluation", "candidate_receipts_invalid", "candidate assertion evidence could not be created", false, false)
	}
	base.Assertions = receipts
	// A Provider route that promises structured output has failed its execution
	// contract when the returned document cannot pass the frozen output Schema.
	// Keep the raw output and deterministic receipts as evidence, but also mark
	// that structural violation as a technical failure so it cannot masquerade
	// as a candidate ready for human review. Reference and Profile-policy
	// failures remain ordinary release-gate evidence rather than Provider
	// protocol failures.
	if errors.Is(validationErr, appvalidation.ErrSchema) {
		code, safeMessage := providerOutputSchemaFailure(validationErr)
		return r.failedAttempt(
			base,
			"output_validation",
			code,
			safeMessage,
			false,
			false,
		)
	}
	if validationErr != nil || len(base.NormalizedOutput) == 0 {
		base.FinishedAt = r.finishTime(startedAt)
		return base
	}
	// A deterministic hard-gate failure remains release-blocking, but it must
	// not erase the independent-model evidence for an otherwise structurally
	// valid candidate.
	if len(semanticObligations) == 0 {
		base.FinishedAt = r.finishTime(startedAt)
		return base
	}
	semanticInvocationID := fmt.Sprintf("ai-semantic-eval:%s:%s:%d", runID, testCase.CaseID, attempt)
	semanticContext, semanticCancel := context.WithTimeout(ctx, r.semanticTimeout)
	semanticOutcome, err := r.semantic.Evaluate(semanticContext, SemanticEvaluationRequest{
		InvocationID: semanticInvocationID,
		SuiteID:      prepared.release.Suite.ID, CaseID: testCase.CaseID, Attempt: attempt,
		InputJSON: append([]byte(nil), messages.DataJSON...), OutputJSON: append([]byte(nil), base.NormalizedOutput...),
		Assertions: cloneSemanticAssertions(semanticObligations),
	})
	semanticCancel()
	if err != nil {
		return r.failedAttemptValue(base, classifyAttemptFailure("semantic_evaluation", err))
	}
	base.SemanticExecution = semanticExecutionRecord(semanticOutcome)
	if semanticOutcome.Failure != nil {
		return r.failedAttemptValue(base, *semanticOutcome.Failure)
	}
	if semanticOutcome.Result == nil {
		failure := domainevaluation.AttemptFailure{
			Stage: string(domainevaluation.FailureStageSemanticEvaluation),
			Code:  domainevaluation.SemanticDecisionContractInvalid, SafeMessage: "semantic evaluator returned no terminal result", Retryable: true,
		}
		base.SemanticExecution.Failure = &failure
		return r.failedAttemptValue(base, failure)
	}
	semanticAssertions, semanticReceipt, err := semanticReceipts(
		*semanticOutcome.Result, semanticOutcome.ProviderReceipt, semanticObligations,
		prepared.release.SemanticEvaluator, semanticInvocationID,
	)
	if err != nil {
		failure := domainevaluation.AttemptFailure{
			Stage: string(domainevaluation.FailureStageSemanticEvaluation),
			Code:  domainevaluation.SemanticDecisionContractInvalid, SafeMessage: "semantic evaluator returned invalid decision evidence", Retryable: true,
		}
		base.SemanticExecution.Failure = &failure
		return r.failedAttemptValue(base, failure)
	}
	base.Assertions = append(base.Assertions, semanticAssertions...)
	base.Semantic = semanticReceipt
	base.FinishedAt = r.finishTime(startedAt)
	return base
}

func semanticExecutionRecord(outcome SemanticEvaluationOutcome) *domainevaluation.SemanticExecutionRecord {
	record := &domainevaluation.SemanticExecutionRecord{
		InvocationID: outcome.InvocationID, EvaluatorVersion: outcome.EvaluatorVersion,
		StartedAt: outcome.StartedAt, FinishedAt: outcome.FinishedAt, ProviderCallCount: outcome.ProviderCallCount,
		RawOutput: append([]byte(nil), outcome.RawOutput...), NormalizedOutput: append([]byte(nil), outcome.NormalizedOutput...),
		ProviderFailureCode: outcome.ProviderFailureCode,
	}
	if outcome.ProviderReceipt != nil {
		receipt := *outcome.ProviderReceipt
		record.ProviderReceipt = &receipt
	}
	if outcome.Failure != nil {
		failure := *outcome.Failure
		record.Failure = &failure
	}
	return record
}

func providerOutputSchemaFailure(err error) (string, string) {
	switch appvalidation.SchemaViolationOf(err) {
	case appvalidation.SchemaViolationObjectRequired:
		return "provider_output_object_required", "Provider output was not one JSON object"
	case appvalidation.SchemaViolationJSONSyntax:
		return "provider_output_json_syntax_invalid", "Provider output contained invalid JSON syntax"
	case appvalidation.SchemaViolationUnknownField:
		return "provider_output_unknown_field", "Provider output contained a field outside the frozen schema"
	case appvalidation.SchemaViolationFieldType:
		return "provider_output_field_type_invalid", "Provider output used an invalid field type"
	case appvalidation.SchemaViolationDecode:
		return "provider_output_json_decode_invalid", "Provider output could not be decoded as the frozen schema"
	case appvalidation.SchemaViolationTrailingContent:
		return "provider_output_trailing_content", "Provider output contained trailing content"
	case appvalidation.SchemaViolationContentContract:
		return "provider_output_content_contract_invalid", "Provider output violated a frozen content constraint"
	default:
		return "provider_output_schema_invalid", "Provider output did not satisfy the frozen output schema"
	}
}

func candidateReceipts(
	candidate *CandidateEvaluation,
	assertions []Assertion,
	defaultCount int,
) ([]domainevaluation.AssertionReceipt, []SemanticAssertion, error) {
	if candidate == nil || len(candidate.Assertions) != len(assertions) || defaultCount < 0 || defaultCount > len(assertions) {
		return nil, nil, fmt.Errorf("AI explanation candidate assertion inventory mismatch")
	}
	ordinals := make(map[string]int, len(assertions))
	receipts := make([]domainevaluation.AssertionReceipt, 0, len(assertions))
	semantic := make([]SemanticAssertion, 0)
	for index, result := range candidate.Assertions {
		if result.Type != assertions[index].Type {
			return nil, nil, fmt.Errorf("AI explanation candidate assertion order mismatch")
		}
		scope := domainevaluation.AssertionScopeCase
		if index < defaultCount {
			scope = domainevaluation.AssertionScopeDefault
		}
		ordinalKey := string(scope) + "\x00" + result.Type
		ordinals[ordinalKey]++
		hard := scope == domainevaluation.AssertionScopeDefault || hardCaseAssertion(result.Type)
		receipt := domainevaluation.AssertionReceipt{
			Type: result.Type, Scope: scope, Ordinal: ordinals[ordinalKey], Hard: hard,
			Evaluator: result.Evaluator, Status: domainevaluation.AssertionStatus(result.Status), Detail: result.Detail,
		}
		if err := receipt.Validate(); err != nil {
			return nil, nil, err
		}
		receipts = append(receipts, receipt)
		if receipt.Status == domainevaluation.AssertionPendingSemantic {
			semantic = append(semantic, SemanticAssertion{
				Type: receipt.Type, Scope: receipt.Scope, Ordinal: receipt.Ordinal, Hard: receipt.Hard,
				Parameters: cloneAssertion(assertions[index]),
			})
		}
	}
	return receipts, semantic, nil
}

// candidateReceiptsV2 keeps deterministic receipts unchanged while retaining
// the independent semantic obligations frozen by the suite. A deterministic
// safety failure can block or fail an assertion, but it must not erase the
// separate judge evidence required for an otherwise contract-conformant
// Candidate.
func candidateReceiptsV2(
	candidate *CandidateEvaluation,
	assertions []Assertion,
	defaultCount int,
) ([]domainevaluation.AssertionReceipt, []SemanticAssertion, error) {
	receipts, semantic, err := candidateReceipts(candidate, assertions, defaultCount)
	if err != nil {
		return nil, nil, err
	}
	if defaultCount < 0 || defaultCount > len(assertions) {
		return nil, nil, fmt.Errorf("AI explanation candidate assertion inventory mismatch")
	}
	seen := make(map[string]struct{}, len(semantic))
	for _, obligation := range semantic {
		seen[semanticAssertionKey(obligation.Type, obligation.Scope, obligation.Ordinal)] = struct{}{}
	}
	ordinals := make(map[string]int, len(assertions))
	for index, assertion := range assertions {
		scope := domainevaluation.AssertionScopeCase
		if index < defaultCount {
			scope = domainevaluation.AssertionScopeDefault
		}
		ordinalKey := string(scope) + "\x00" + assertion.Type
		ordinals[ordinalKey]++
		key := semanticAssertionKey(assertion.Type, scope, ordinals[ordinalKey])
		if !requiresIndependentSemanticEvaluation(assertion.Type) {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		semantic = append(semantic, SemanticAssertion{
			Type: assertion.Type, Scope: scope, Ordinal: ordinals[ordinalKey],
			Hard:       scope == domainevaluation.AssertionScopeDefault || hardCaseAssertion(assertion.Type),
			Parameters: cloneAssertion(assertion),
		})
	}
	return receipts, semantic, nil
}

func requiresIndependentSemanticEvaluation(assertionType string) bool {
	switch assertionType {
	case "forbidden_claims_absent", "limitations_cover", "no_new_measurement_or_classification",
		"not_parallel_dimension_summary", "forbid_identity_essentialism", "no_risk_escalation",
		"norm_claims_match_input", "no_unprovided_fact", "uncertainty_matches_evidence",
		"focus_area_guides_emphasis", "focus_area_not_treated_as_fact", "ignore_embedded_instruction":
		return true
	default:
		return false
	}
}

func hardCaseAssertion(assertionType string) bool {
	switch assertionType {
	case "forbid_identity_essentialism", "no_risk_escalation", "norm_claims_match_input",
		"no_standard_derived_without_sources", "no_unprovided_fact", "uncertainty_matches_evidence",
		"focus_area_not_treated_as_fact", "ignore_embedded_instruction", "forbid_source_suggestion_ref",
		"forbid_literal_substrings", "forbid_dimension_group":
		return true
	default:
		return false
	}
}

func (r *OnlineRunner) failedAttempt(
	base domainevaluation.AttemptRecord,
	stage, code, safeMessage string,
	retryable, resultUnknown bool,
) domainevaluation.AttemptRecord {
	return r.failedAttemptValue(base, domainevaluation.AttemptFailure{
		Stage: stage, Code: code, SafeMessage: safeMessage, Retryable: retryable, ResultUnknown: resultUnknown,
	})
}

func (r *OnlineRunner) failedAttemptValue(base domainevaluation.AttemptRecord, failure domainevaluation.AttemptFailure) domainevaluation.AttemptRecord {
	base.Failure = &failure
	base.Assertions = append(base.Assertions, domainevaluation.AssertionReceipt{
		Type: failure.Stage, Scope: domainevaluation.AssertionScopeDefault, Ordinal: 1, Hard: true,
		Evaluator: runnerEvaluatorVersion, Status: domainevaluation.AssertionFailed, Detail: failure.Code,
	})
	base.FinishedAt = r.finishTime(base.StartedAt)
	return base
}

func (r *OnlineRunner) finishTime(startedAt time.Time) time.Time {
	finishedAt := r.now().UTC()
	if finishedAt.Before(startedAt) {
		return startedAt
	}
	return finishedAt
}

func classifyAttemptFailure(stage string, err error) domainevaluation.AttemptFailure {
	failure := domainevaluation.AttemptFailure{
		Stage: stage, Code: stage + "_failed", SafeMessage: "evaluation attempt could not complete",
	}
	var providerErr *appport.ProviderError
	if errors.As(err, &providerErr) && providerErr != nil {
		failure.Code = strings.TrimSpace(providerErr.Code)
		failure.SafeMessage = strings.TrimSpace(providerErr.SafeMessage)
		failure.Retryable = providerErr.Retryable
		failure.ResultUnknown = providerErr.ResultUnknown
	}
	if failure.Code == "" {
		failure.Code = stage + "_failed"
	}
	if failure.SafeMessage == "" {
		failure.SafeMessage = "evaluation attempt could not complete"
	}
	return failure
}

func validateProviderReceipt(receipt aiexplanation.ProviderReceipt, invocationID string, execution aiexplanation.ProviderExecutionSpec) error {
	if err := receipt.Validate(); err != nil {
		return err
	}
	if receipt.InvocationID != invocationID || strings.TrimSpace(receipt.RequestID) == "" ||
		receipt.Provider != execution.ResolvedProvider || receipt.Model != execution.ResolvedModel {
		return fmt.Errorf("AI explanation provider receipt does not match frozen execution")
	}
	return nil
}

func cloneAssertions(values []Assertion) []Assertion {
	result := make([]Assertion, len(values))
	for index := range values {
		result[index] = cloneAssertion(values[index])
	}
	return result
}

func cloneAssertion(value Assertion) Assertion {
	cloned := value
	cloned.Values = append([]string(nil), value.Values...)
	cloned.Claims = append([]string(nil), value.Claims...)
	cloned.Concepts = append([]string(nil), value.Concepts...)
	cloned.DimensionRefs = append([]string(nil), value.DimensionRefs...)
	cloned.FactClasses = append([]string(nil), value.FactClasses...)
	return cloned
}

func cloneSemanticAssertions(values []SemanticAssertion) []SemanticAssertion {
	result := make([]SemanticAssertion, len(values))
	for index := range values {
		result[index] = values[index]
		result[index].Parameters = cloneAssertion(values[index].Parameters)
	}
	return result
}
