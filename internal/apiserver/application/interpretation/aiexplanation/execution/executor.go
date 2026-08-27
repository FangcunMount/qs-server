// Package execution coordinates one-shot AI explanation provider attempts.
// It deliberately has no planning loop, tool calling or conversational state.
package execution

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	appinput "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/input"
	appport "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/port"
	appprompt "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/prompt"
	appvalidation "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/validation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation"
	domainartifact "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/artifact"
	domaingeneration "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/generation"
	domainprofile "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/profile"
	domainrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/run"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
)

type Status string

const (
	StatusGenerated  Status = "generated"
	StatusFailed     Status = "failed"
	StatusProcessing Status = "processing"
)

type Command struct {
	GenerationID            meta.ID
	TraceID                 string
	EventID                 string
	ExpectedAttempt         int
	AttemptOrigin           retrygovernance.AttemptOrigin
	ActionRequestID         string
	ExpectedRunID           meta.ID
	ExpectedLeaseExpiresAt  time.Time
	ExpectedInvocationPhase domainrun.InvocationPhase
}

type Result struct {
	Status     Status
	Generation *domaingeneration.AIExplanationGeneration
	Run        *domainrun.AIExplanationRun
	Artifact   *domainartifact.AIExplanationArtifact
	Failure    *domainrun.Failure
}

type Executor interface {
	Execute(context.Context, Command) (*Result, error)
}

type executor struct {
	generations   domaingeneration.Repository
	runs          domainrun.Repository
	artifacts     domainartifact.Repository
	profiles      domainprofile.Repository
	prompts       appport.PromptPackageResolver
	routes        appport.FrozenProviderRouteResolver
	schemas       appport.OutputSchemaResolver
	provider      appport.Provider
	safety        appport.SafetyEvaluator
	committer     appport.ExecutionCommitter
	leaseDuration time.Duration
	now           func() time.Time
	newID         func() meta.ID
}

func NewExecutor(
	generations domaingeneration.Repository,
	runs domainrun.Repository,
	artifacts domainartifact.Repository,
	profiles domainprofile.Repository,
	prompts appport.PromptPackageResolver,
	routes appport.FrozenProviderRouteResolver,
	schemas appport.OutputSchemaResolver,
	provider appport.Provider,
	safety appport.SafetyEvaluator,
	committer appport.ExecutionCommitter,
	leaseDuration time.Duration,
) (Executor, error) {
	if generations == nil || runs == nil || artifacts == nil || profiles == nil || prompts == nil || routes == nil || schemas == nil || provider == nil || safety == nil || committer == nil {
		return nil, fmt.Errorf("AI explanation executor dependencies are required")
	}
	if leaseDuration <= 0 {
		return nil, fmt.Errorf("AI explanation execution lease must be positive")
	}
	return &executor{
		generations: generations, runs: runs, artifacts: artifacts, profiles: profiles, prompts: prompts,
		routes: routes, schemas: schemas, provider: provider, safety: safety, committer: committer,
		leaseDuration: leaseDuration, now: time.Now, newID: meta.New,
	}, nil
}

func (e *executor) Execute(ctx context.Context, command Command) (*Result, error) {
	if command.GenerationID.IsZero() || strings.TrimSpace(command.TraceID) == "" {
		return nil, fmt.Errorf("AI explanation generation and trace are required")
	}
	hasRetryProof := command.ExpectedAttempt != 0 || command.AttemptOrigin != "" || strings.TrimSpace(command.ActionRequestID) != ""
	hasRecoveryProof := !command.ExpectedRunID.IsZero() || !command.ExpectedLeaseExpiresAt.IsZero() || command.ExpectedInvocationPhase != ""
	if hasRetryProof && hasRecoveryProof {
		return nil, domainrun.ErrRecoveryNotAllowed
	}
	if hasRecoveryProof && (command.ExpectedRunID.IsZero() || command.ExpectedLeaseExpiresAt.IsZero() ||
		(command.ExpectedInvocationPhase != domainrun.InvocationPhasePrepared && command.ExpectedInvocationPhase != domainrun.InvocationPhaseDispatching) ||
		strings.TrimSpace(command.EventID) == "") {
		return nil, domainrun.ErrRecoveryNotAllowed
	}
	generationRecord, err := e.generations.FindByID(ctx, command.GenerationID)
	if err != nil {
		return nil, err
	}
	switch generationRecord.Status() {
	case domaingeneration.StatusGenerated:
		artifactRecord, loadErr := e.artifacts.FindByID(ctx, generationRecord.ArtifactID())
		if loadErr != nil {
			return nil, loadErr
		}
		return &Result{Status: StatusGenerated, Generation: generationRecord, Artifact: artifactRecord}, nil
	case domaingeneration.StatusGenerating:
		return e.resumeOrResolveExpired(ctx, generationRecord, command)
	case domaingeneration.StatusFailed:
		runRecord, loadErr := e.runs.FindLatestByGenerationID(ctx, generationRecord.ID())
		if loadErr != nil {
			return nil, loadErr
		}
		if command.ExpectedAttempt == 0 && command.AttemptOrigin == "" && strings.TrimSpace(command.ActionRequestID) == "" {
			return &Result{Status: StatusFailed, Generation: generationRecord, Run: runRecord, Failure: runRecord.Failure()}, nil
		}
		return e.startFailedIfAuthorized(ctx, generationRecord, runRecord, command)
	case domaingeneration.StatusPending:
		if hasRecoveryProof {
			return nil, domainrun.ErrRecoveryNotAllowed
		}
		if hasRetryProof {
			return nil, domainrun.ErrRetryNotAllowed
		}
	default:
		return nil, fmt.Errorf("unsupported AI explanation generation status %s", generationRecord.Status())
	}

	startedAt := e.now()
	runRecord, err := domainrun.NewPending(e.newID(), generationRecord.ID(), 1, retrygovernance.AttemptOriginInitial)
	if err != nil {
		return nil, err
	}
	invocationID := fmt.Sprintf("generation-%s/attempt-%d", generationRecord.ID(), runRecord.Attempt())
	if err := runRecord.StartWithLease(startedAt, command.TraceID, startedAt.Add(e.leaseDuration), invocationID); err != nil {
		return nil, err
	}
	expectedVersion := generationRecord.Version()
	if err := generationRecord.Begin(runRecord.ID(), startedAt); err != nil {
		return nil, err
	}
	if err := e.committer.CommitStart(ctx, generationRecord, runRecord, expectedVersion); err != nil {
		return nil, err
	}
	return e.executeRunning(ctx, generationRecord, runRecord)
}

func (e *executor) startFailedIfAuthorized(
	ctx context.Context,
	generationRecord *domaingeneration.AIExplanationGeneration,
	latest *domainrun.AIExplanationRun,
	command Command,
) (*Result, error) {
	if latest == nil || latest.Status() != domainrun.StatusFailed || latest.ID() != generationRecord.LatestRunID() {
		return nil, fmt.Errorf("failed AI explanation Generation has invalid latest Run")
	}
	authorization := latest.RetryAuthorization()
	if authorization == nil || command.ExpectedAttempt != latest.Attempt() || command.ExpectedAttempt != authorization.ExpectedAttempt ||
		command.AttemptOrigin != authorization.Origin || strings.TrimSpace(command.ActionRequestID) != authorization.RequestID ||
		strings.TrimSpace(command.EventID) == "" || strings.TrimSpace(command.EventID) != authorization.EventID ||
		strings.TrimSpace(command.TraceID) != authorization.EventID {
		return nil, domainrun.ErrRetryNotAllowed
	}
	runRecord, err := domainrun.Next(e.newID(), latest, authorization.Origin)
	if err != nil {
		return nil, err
	}
	startedAt := e.now()
	invocationID := fmt.Sprintf("generation-%s/attempt-%d", generationRecord.ID(), runRecord.Attempt())
	if err := runRecord.StartWithLease(startedAt, command.TraceID, startedAt.Add(e.leaseDuration), invocationID); err != nil {
		return nil, err
	}
	expectedVersion := generationRecord.Version()
	if err := generationRecord.Begin(runRecord.ID(), startedAt); err != nil {
		return nil, err
	}
	if err := e.committer.CommitStart(ctx, generationRecord, runRecord, expectedVersion); err != nil {
		return nil, err
	}
	return e.executeRunning(ctx, generationRecord, runRecord)
}

func (e *executor) executeRunning(
	ctx context.Context,
	generationRecord *domaingeneration.AIExplanationGeneration,
	runRecord *domainrun.AIExplanationRun,
) (*Result, error) {
	if generationRecord == nil || runRecord == nil ||
		generationRecord.Status() != domaingeneration.StatusGenerating ||
		runRecord.Status() != domainrun.StatusRunning ||
		generationRecord.LatestRunID() != runRecord.ID() ||
		runRecord.GenerationID() != generationRecord.ID() {
		return nil, fmt.Errorf("generating AI explanation Generation and running Run are required")
	}
	prepared, failure := e.prepare(ctx, generationRecord)
	if failure != nil {
		return e.commitFailure(ctx, generationRecord, runRecord, *failure)
	}
	switch runRecord.InvocationPhase() {
	case domainrun.InvocationPhasePrepared:
		if err := runRecord.BeginProviderDispatch(e.now()); err != nil {
			return nil, err
		}
		if err := e.committer.SaveDispatching(ctx, runRecord); err != nil {
			// No external side effect has happened. Leave the persisted Prepared
			// Run available for safe lease recovery.
			return nil, err
		}
	case domainrun.InvocationPhaseDispatching:
		if !prepared.route.Capabilities.IdempotentRedispatch {
			return nil, domainrun.ErrUnsafeLeaseReclaim
		}
	default:
		return nil, fmt.Errorf("AI explanation Run invocation phase %s cannot execute Provider", runRecord.InvocationPhase())
	}

	providerContext, cancel := context.WithTimeout(ctx, prepared.route.Timeout)
	response, providerErr := e.provider.Generate(providerContext, appport.ProviderRequest{
		InvocationID: runRecord.InvocationID(), Route: prepared.route,
		SystemMessage: prepared.messages.SystemMessage, TaskMessage: prepared.messages.TaskMessage,
		DataPreamble: prepared.messages.DataPreamble, DataJSON: prepared.messages.DataJSON,
		OutputSchema: prepared.outputSchema,
	})
	cancel()
	if providerErr != nil {
		failure, resultUnknown := providerFailure(providerErr, prepared.route)
		if resultUnknown {
			if err := runRecord.MarkProviderResultUnknown(); err != nil {
				return nil, err
			}
		}
		return e.commitFailure(ctx, generationRecord, runRecord, failure)
	}
	if response == nil {
		if err := runRecord.MarkProviderResultUnknown(); err != nil {
			return nil, err
		}
		retryable := prepared.route.Capabilities.IdempotentRedispatch || prepared.route.Capabilities.RetrieveByInvocationID
		return e.commitFailure(ctx, generationRecord, runRecord, domainrun.Failure{
			Kind: domainrun.FailureKindProviderTransport, Code: "provider_empty_response", SafeMessage: "AI 解读暂时不可用，请稍后再试", Retryable: retryable,
		})
	}
	if response.Receipt.Provider != generationRecord.ExecutionSpec().ResolvedProvider || response.Receipt.Model != generationRecord.ExecutionSpec().ResolvedModel {
		return e.commitFailure(ctx, generationRecord, runRecord, domainrun.Failure{
			Kind: domainrun.FailureKindProviderTransport, Code: "provider_receipt_mismatch", SafeMessage: "AI 解读暂时不可用，请稍后再试", Retryable: false,
		})
	}
	if err := runRecord.RecordProviderResponse(response.Receipt); err != nil {
		return e.commitFailure(ctx, generationRecord, runRecord, domainrun.Failure{
			Kind: domainrun.FailureKindProviderTransport, Code: "provider_receipt_invalid", SafeMessage: "AI 解读暂时不可用，请稍后再试", Retryable: false,
		})
	}
	validationStartedAt := time.Now()
	validated, err := appvalidation.Validate(response.RawOutput, prepared.input.Document, prepared.profile.Definition())
	if err != nil {
		observeOutputValidation(validationResultOutputRejected, time.Since(validationStartedAt))
		return e.commitFailure(ctx, generationRecord, runRecord, domainrun.Failure{
			Kind: domainrun.FailureKindOutputValidation, Code: "output_validation_failed", SafeMessage: "AI 解读结果未通过质量校验，请稍后再试", Retryable: false,
		})
	}
	safetyResult, err := e.safety.Evaluate(ctx, appport.SafetyRequest{
		Content: validated.Content, Input: prepared.input.ProviderPayload, Policy: prepared.profile.Definition().SafetyPolicy,
	})
	if err != nil {
		observeOutputValidation(validationResultSafetyError, time.Since(validationStartedAt))
		return e.commitFailure(ctx, generationRecord, runRecord, domainrun.Failure{
			Kind: domainrun.FailureKindSafety, Code: "safety_evaluator_unavailable", SafeMessage: "AI 解读暂时不可用，请稍后再试", Retryable: true,
		})
	}
	if !safetyResult.Allowed || strings.TrimSpace(safetyResult.ValidatorVersion) == "" {
		observeOutputValidation(validationResultSafetyRejected, time.Since(validationStartedAt))
		code := strings.TrimSpace(safetyResult.FailureCode)
		if code == "" {
			code = "safety_validation_failed"
		}
		message := strings.TrimSpace(safetyResult.SafeMessage)
		if message == "" {
			message = "AI 解读结果未通过安全校验"
		}
		return e.commitFailure(ctx, generationRecord, runRecord, domainrun.Failure{
			Kind: domainrun.FailureKindSafety, Code: code, SafeMessage: message, Retryable: false,
		})
	}
	validatedAt := e.now()
	artifactRecord, err := buildArtifact(e.newID(), generationRecord, runRecord, response.Receipt, prepared.input.Document, prepared.profile, validated, safetyResult.ValidatorVersion, validatedAt)
	if err != nil {
		observeOutputValidation(validationResultArtifactError, time.Since(validationStartedAt))
		return e.commitFailure(ctx, generationRecord, runRecord, domainrun.Failure{
			Kind: domainrun.FailureKindOutputValidation, Code: "artifact_construction_failed", SafeMessage: "AI 解读结果未通过质量校验，请稍后再试", Retryable: false,
		})
	}
	observeOutputValidation(validationResultAccepted, time.Since(validationStartedAt))
	terminalVersion := generationRecord.Version()
	if err := runRecord.Succeed(validatedAt); err != nil {
		return nil, err
	}
	if err := generationRecord.Succeed(runRecord.ID(), artifactRecord.ID(), validatedAt); err != nil {
		return nil, err
	}
	if err := e.committer.CommitSuccess(ctx, generationRecord, runRecord, artifactRecord, terminalVersion); err != nil {
		return nil, err
	}
	return &Result{Status: StatusGenerated, Generation: generationRecord, Run: runRecord, Artifact: artifactRecord}, nil
}

func (e *executor) resumeOrResolveExpired(
	ctx context.Context,
	generationRecord *domaingeneration.AIExplanationGeneration,
	command Command,
) (*Result, error) {
	runRecord, err := e.runs.FindByID(ctx, generationRecord.LatestRunID())
	if err != nil {
		return nil, err
	}
	if runRecord.Status() != domainrun.StatusRunning || runRecord.GenerationID() != generationRecord.ID() {
		return nil, fmt.Errorf("generating AI explanation has invalid latest Run")
	}
	if !command.ExpectedRunID.IsZero() {
		// A recovery event is scoped to the exact lease snapshot persisted by
		// the scheduler. If another owner already advanced the same Run, this
		// delivery is an idempotent no-op. It may never reclaim the newer lease.
		if runRecord.ID() != command.ExpectedRunID {
			return &Result{Status: StatusProcessing, Generation: generationRecord, Run: runRecord}, nil
		}
		currentLease := runRecord.LeaseExpiresAt()
		if currentLease != nil && currentLease.After(command.ExpectedLeaseExpiresAt) {
			return &Result{Status: StatusProcessing, Generation: generationRecord, Run: runRecord}, nil
		}
		wakeup := runRecord.RecoveryWakeup()
		if currentLease == nil || !currentLease.Equal(command.ExpectedLeaseExpiresAt) ||
			runRecord.InvocationPhase() != command.ExpectedInvocationPhase || wakeup == nil ||
			wakeup.EventID != strings.TrimSpace(command.EventID) ||
			!wakeup.ExpectedLeaseExpiresAt.Equal(command.ExpectedLeaseExpiresAt) ||
			wakeup.InvocationPhase != command.ExpectedInvocationPhase {
			return nil, domainrun.ErrRecoveryNotAllowed
		}
	}
	now := e.now()
	leaseExpiresAt := runRecord.LeaseExpiresAt()
	if leaseExpiresAt == nil {
		return nil, fmt.Errorf("running AI explanation Run has no lease")
	}
	if leaseExpiresAt.After(now) {
		return &Result{Status: StatusProcessing, Generation: generationRecord, Run: runRecord}, nil
	}

	route, err := e.routes.ResolveFrozenProviderRoute(ctx, generationRecord.ExecutionSpec())
	if err != nil || route.ExecutionSpec != generationRecord.ExecutionSpec() || route.Validate() != nil {
		return e.commitFailure(ctx, generationRecord, runRecord, domainrun.Failure{
			Kind: domainrun.FailureKindProfile, Code: "provider_route_release_unavailable",
			SafeMessage: "AI 解读配置暂时不可用", Retryable: false,
		})
	}
	if runRecord.InvocationPhase() == domainrun.InvocationPhaseResultUnknown ||
		(runRecord.InvocationPhase() != domainrun.InvocationPhasePrepared && !route.Capabilities.IdempotentRedispatch) {
		if runRecord.InvocationPhase() == domainrun.InvocationPhaseDispatching {
			if err := runRecord.MarkProviderResultUnknown(); err != nil {
				return nil, err
			}
		}
		return e.commitFailure(ctx, generationRecord, runRecord, domainrun.Failure{
			Kind: domainrun.FailureKindProviderTransport, Code: "provider_result_unknown",
			SafeMessage: "AI 解读调用结果无法确认，请稍后重试", Retryable: false,
		})
	}

	reclaimer, ok := e.runs.(domainrun.LeaseReclaimer)
	if !ok {
		return nil, fmt.Errorf("AI explanation Run repository does not support atomic lease reclaim")
	}
	reclaimed, claimed, err := reclaimer.ReclaimExpiredLease(
		ctx, runRecord.ID(), now, command.TraceID, now.Add(e.leaseDuration),
		route.Capabilities.IdempotentRedispatch,
	)
	if err != nil {
		return nil, err
	}
	if !claimed {
		winner, loadErr := e.runs.FindByID(ctx, runRecord.ID())
		if loadErr != nil {
			return nil, loadErr
		}
		return &Result{Status: StatusProcessing, Generation: generationRecord, Run: winner}, nil
	}
	return e.executeRunning(ctx, generationRecord, reclaimed)
}

type prepared struct {
	input        *appinput.Result
	profile      *domainprofile.AIExplanationProfile
	messages     appprompt.Messages
	route        appport.ProviderRoute
	outputSchema appport.StructuredOutputSchema
}

func (e *executor) prepare(ctx context.Context, generationRecord *domaingeneration.AIExplanationGeneration) (*prepared, *domainrun.Failure) {
	input, err := appinput.Restore(generationRecord.Input())
	if err != nil {
		return nil, executionFailure(domainrun.FailureKindInput, "input_snapshot_invalid", "AI 解读所需的测评结果不可用", false)
	}
	if err := validateFrozenInput(generationRecord, input.Document); err != nil {
		return nil, executionFailure(domainrun.FailureKindInput, "input_snapshot_mismatch", "AI 解读所需的测评结果不可用", false)
	}
	profileRecord, err := e.profiles.FindByKey(ctx, generationRecord.Key().Profile.ID, generationRecord.Key().Profile.Version)
	if err != nil || profileRecord == nil || profileRecord.Fingerprint() != generationRecord.Key().Profile.Fingerprint {
		return nil, executionFailure(domainrun.FailureKindProfile, "profile_release_unavailable", "AI 解读配置暂时不可用", false)
	}
	pkg, err := e.prompts.ResolvePromptPackage(ctx, generationRecord.Prompt().TemplateID, generationRecord.Prompt().Version)
	if err != nil || pkg.Ref != generationRecord.Prompt() {
		return nil, executionFailure(domainrun.FailureKindPrompt, "prompt_release_unavailable", "AI 解读配置暂时不可用", false)
	}
	route, err := e.routes.ResolveFrozenProviderRoute(ctx, generationRecord.ExecutionSpec())
	if err != nil || route.ExecutionSpec != generationRecord.ExecutionSpec() {
		return nil, executionFailure(domainrun.FailureKindProfile, "provider_route_release_unavailable", "AI 解读配置暂时不可用", false)
	}
	outputSchema, err := e.schemas.ResolveOutputSchema(ctx, profileRecord.Definition().GenerationPolicy.OutputSchemaVersion)
	if err != nil || outputSchema.Version != aiexplanation.OutputSchemaVersionV1 || outputSchema.Validate() != nil {
		return nil, executionFailure(domainrun.FailureKindProfile, "output_schema_release_unavailable", "AI 解读配置暂时不可用", false)
	}
	messages, err := appprompt.Render(pkg, profileRecord.Definition(), input)
	if err != nil {
		return nil, executionFailure(domainrun.FailureKindPrompt, "prompt_render_failed", "AI 解读配置暂时不可用", false)
	}
	return &prepared{input: input, profile: profileRecord, messages: messages, route: route, outputSchema: outputSchema}, nil
}

func validateFrozenInput(generationRecord *domaingeneration.AIExplanationGeneration, document appinput.Document) error {
	reportID, err := meta.ParseID(document.Source.ReportID)
	if err != nil || reportID != generationRecord.Key().SourceReportID || document.Context.Audience != string(generationRecord.Key().Audience) {
		return fmt.Errorf("AI explanation frozen input source or audience mismatch")
	}
	profile := generationRecord.Key().Profile
	if document.Profile.ProfileID != profile.ID || document.Profile.ProfileVersion != profile.Version || document.Profile.ProfileFingerprint != profile.Fingerprint.String() {
		return fmt.Errorf("AI explanation frozen input Profile mismatch")
	}
	return nil
}

func buildArtifact(
	id meta.ID,
	generationRecord *domaingeneration.AIExplanationGeneration,
	runRecord *domainrun.AIExplanationRun,
	receipt aiexplanation.ProviderReceipt,
	document appinput.Document,
	profileRecord *domainprofile.AIExplanationProfile,
	validated *appvalidation.Result,
	safetyValidatorVersion string,
	validatedAt time.Time,
) (*domainartifact.AIExplanationArtifact, error) {
	reportID, err := meta.ParseID(document.Source.ReportID)
	if err != nil {
		return nil, err
	}
	outcomeID, err := meta.ParseID(document.Source.OutcomeID)
	if err != nil {
		return nil, err
	}
	reportGeneratedAt, err := time.Parse(time.RFC3339Nano, document.Source.GeneratedAt)
	if err != nil {
		return nil, err
	}
	return domainartifact.New(domainartifact.NewInput{
		ID: id, GenerationID: generationRecord.ID(), RunID: runRecord.ID(),
		Source: domainartifact.SourceRef{
			ReportID: reportID, OutcomeID: outcomeID, Association: generationRecord.Association(), ReportType: document.Source.ReportType,
			TemplateVersion: document.Source.TemplateVersion, ContentSchemaVersion: document.Source.ContentSchemaVersion,
			BuilderIdentity: document.Source.BuilderIdentity, ReportGeneratedAt: reportGeneratedAt,
		},
		Audience: generationRecord.Key().Audience, Profile: generationRecord.Key().Profile, Prompt: generationRecord.Prompt(), ExecutionSpec: generationRecord.ExecutionSpec(),
		InputSchema: generationRecord.Input().SchemaVersion(), InputFingerprint: generationRecord.Input().Fingerprint(), OutputSchema: aiexplanation.OutputSchemaVersionV1,
		SafetyPolicy: profileRecord.Definition().SafetyPolicy.PolicyVersion, ProviderReceipt: receipt,
		Validation: domainartifact.ValidationReceipt{
			SchemaValidatorVersion: validated.SchemaValidatorVersion, ReferenceValidatorVersion: validated.ReferenceValidatorVersion,
			ProfileValidatorVersion: validated.ProfileValidatorVersion, SafetyValidatorVersion: safetyValidatorVersion, ValidatedAt: validatedAt,
		},
		Content: validated.Content, GeneratedAt: validatedAt,
	})
}

func providerFailure(err error, route appport.ProviderRoute) (domainrun.Failure, bool) {
	var classified *appport.ProviderError
	if errors.As(err, &classified) && classified != nil && classified.Kind.IsValid() && strings.TrimSpace(classified.Code) != "" && strings.TrimSpace(classified.SafeMessage) != "" {
		retryable := classified.Retryable
		if classified.ResultUnknown && !route.Capabilities.IdempotentRedispatch && !route.Capabilities.RetrieveByInvocationID {
			retryable = false
		}
		return domainrun.Failure{Kind: classified.Kind, Code: classified.Code, SafeMessage: classified.SafeMessage, Retryable: retryable}, classified.ResultUnknown
	}
	kind := domainrun.FailureKindProviderTransport
	code := "provider_unavailable"
	if errors.Is(err, context.DeadlineExceeded) {
		kind = domainrun.FailureKindProviderTimeout
		code = "provider_timeout"
	}
	return domainrun.Failure{Kind: kind, Code: code, SafeMessage: "AI 解读暂时不可用，请稍后再试", Retryable: true}, false
}

func executionFailure(kind domainrun.FailureKind, code, message string, retryable bool) *domainrun.Failure {
	return &domainrun.Failure{Kind: kind, Code: code, SafeMessage: message, Retryable: retryable}
}

func (e *executor) commitFailure(ctx context.Context, generationRecord *domaingeneration.AIExplanationGeneration, runRecord *domainrun.AIExplanationRun, failure domainrun.Failure) (*Result, error) {
	failedAt := e.now()
	expectedVersion := generationRecord.Version()
	if err := runRecord.Fail(failedAt, failure); err != nil {
		return nil, err
	}
	if err := generationRecord.Fail(runRecord.ID(), failedAt); err != nil {
		return nil, err
	}
	if err := e.committer.CommitFailure(ctx, generationRecord, runRecord, expectedVersion); err != nil {
		return nil, err
	}
	return &Result{Status: StatusFailed, Generation: generationRecord, Run: runRecord, Failure: &failure}, nil
}
