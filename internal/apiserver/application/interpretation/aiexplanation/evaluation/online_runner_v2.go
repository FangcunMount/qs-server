package evaluation

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	appport "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/port"
	appprompt "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/prompt"
	appvalidation "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/validation"
	domainai "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation"
	domainevaluation "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/evaluation"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

const (
	evidenceV2InputSchemaID          = "ai-explanation-input"
	evidenceV2OutputSchemaID         = "ai-explanation-output"
	evidenceV2SemanticOutputSchemaID = "ai-explanation-semantic-output"
)

type OnlineStartV2Command struct {
	RunID           meta.ID
	OrgID           int64
	RequestedBy     string
	Reason          string
	ExecutionPolicy domainevaluation.EvaluationExecutionPolicy
	GatePolicy      domainevaluation.ReleaseGatePolicy
}

type OnlineRunV2Result struct {
	Preflight *PreflightReport
	Evidence  *domainevaluation.PromptEvaluationEvidenceV2
}

// PrepareRequestedV2 freezes the currently executable assets and supplied
// versioned policies in memory. The durable committer owns capacity reservation,
// creation and the first Outbox event.
func (r *OnlineRunner) PrepareRequestedV2(ctx context.Context, command OnlineStartV2Command) (*OnlineRunV2Result, error) {
	if r == nil || command.RunID.IsZero() || command.OrgID <= 0 || strings.TrimSpace(command.RequestedBy) == "" || strings.TrimSpace(command.Reason) == "" {
		return nil, fmt.Errorf("AI explanation online evaluation v2 start command is invalid")
	}
	if err := command.ExecutionPolicy.Validate(); err != nil {
		return nil, err
	}
	if err := command.GatePolicy.Validate(); err != nil {
		return nil, err
	}
	_, preflightReport, prepared, err := r.prepareV2(ctx)
	result := &OnlineRunV2Result{Preflight: preflightReport}
	if err != nil {
		return result, err
	}
	release, err := preparedEvidenceReleaseV2(prepared, command.ExecutionPolicy, command.GatePolicy)
	if err != nil {
		return result, err
	}
	caseIDs := make([]string, 0, len(prepared.generationCases))
	for _, testCase := range prepared.generationCases {
		caseIDs = append(caseIDs, testCase.CaseID)
	}
	createdAt := r.now().UTC()
	value, err := domainevaluation.NewPromptEvaluationEvidenceV2(
		command.RunID, release, command.ExecutionPolicy, command.GatePolicy, caseIDs, prepared.preflightCase.CaseID,
		command.OrgID, strings.TrimSpace(command.RequestedBy), strings.TrimSpace(command.Reason), createdAt,
	)
	if err != nil {
		return result, err
	}
	if err := value.Transition(domainevaluation.EvidenceStatusCollecting, "capacity_reserved", "system:runner", nil, createdAt); err != nil {
		return result, err
	}
	preflight := r.preflightAttempt(prepared.preflightCase, prepared.release.PreflightRejectionReason)
	evaluatedAt := preflight.FinishedAt
	if err := value.CompletePreflight(domainevaluation.PreflightCaseEvidence{
		CaseID: preflight.CaseID, Status: domainevaluation.PreflightEvidencePassed, EvaluatedAt: &evaluatedAt,
		ProviderCallCount: preflight.ProviderCallCount, RejectionReason: preflight.RejectionReason,
		Assertions: append([]domainevaluation.AssertionReceipt(nil), preflight.Assertions...),
	}); err != nil {
		return result, err
	}
	result.Evidence = value
	return result, nil
}

func (r *OnlineRunner) StartRequestedV2(ctx context.Context, command OnlineStartV2Command) (*OnlineRunV2Result, error) {
	result, err := r.PrepareRequestedV2(ctx, command)
	if err != nil {
		return result, err
	}
	if r.durableCommitterV2 == nil {
		return result, fmt.Errorf("AI explanation Prompt evaluation v2 durable committer is not configured")
	}
	if err := r.durableCommitterV2.CommitStartV2(ctx, result.Evidence); err != nil {
		return result, err
	}
	return result, nil
}

// RunStepV2 executes exactly one durable Generation or Semantic address. It
// never performs both calls in one step and never automatically repeats a
// dispatching checkpoint.
func (r *OnlineRunner) RunStepV2(ctx context.Context, command OnlineStepV2Command) (*OnlineStepV2Result, error) {
	if r == nil || r.evidenceV2 == nil || r.durableCommitterV2 == nil || command.RunID.IsZero() ||
		!command.ExecutionKind.IsValid() || strings.TrimSpace(command.CaseID) == "" || command.SlotOrdinal < 1 ||
		command.ExecutionOrdinal < 1 || strings.TrimSpace(command.Owner) == "" {
		return nil, fmt.Errorf("AI explanation online evaluation v2 step command is invalid")
	}
	value, err := r.evidenceV2.Find(ctx, command.RunID)
	if err != nil {
		return nil, err
	}
	if command.RequestedOrgID != 0 && (value.Audit.OrganizationID != command.RequestedOrgID || value.Audit.RequestedBy != strings.TrimSpace(command.RequestedBy)) {
		return nil, fmt.Errorf("AI explanation evaluation v2 request audit does not match the durable event")
	}
	requestedAction := command.Action()
	if value.Status == domainevaluation.EvidenceStatusCanceled {
		return &OnlineStepV2Result{Status: OnlineStepV2Canceled, Evidence: value}, nil
	}
	if value.HasTerminalExecution(requestedAction) {
		return &OnlineStepV2Result{Status: OnlineStepV2AlreadyCompleted, Evidence: value}, nil
	}
	if value.Status != domainevaluation.EvidenceStatusCollecting {
		return nil, fmt.Errorf("AI explanation evaluation v2 Run is not collecting")
	}
	prepared, err := r.prepareExecutableV2(ctx, value)
	if err != nil {
		return nil, err
	}
	next, err := value.NextAction()
	if err != nil {
		return nil, err
	}
	if !sameOnlineStepV2Address(next, requestedAction) {
		return nil, fmt.Errorf("AI explanation evaluation v2 step is not the next frozen action")
	}

	now := r.now().UTC()
	checkpoint := value.Execution()
	if checkpoint != nil {
		if !sameOnlineCheckpointV2Address(*checkpoint, command) {
			return nil, ErrAttemptExecutionBusy
		}
		switch checkpoint.Phase {
		case domainevaluation.AttemptExecutionPrepared:
			if checkpoint.LeaseExpired(now) {
				value, err = r.evidenceV2.ReleaseExpiredPreparation(ctx, command.RunID, now)
				if err != nil {
					return nil, err
				}
				checkpoint = nil
			} else if checkpoint.Owner != strings.TrimSpace(command.Owner) {
				return nil, ErrAttemptExecutionBusy
			}
		case domainevaluation.AttemptExecutionDispatching:
			if checkpoint.LeaseExpired(now) {
				return r.completeUnknownV2(ctx, value, *checkpoint, now)
			}
			return nil, ErrAttemptExecutionBusy
		default:
			return nil, fmt.Errorf("AI explanation evaluation v2 checkpoint phase is invalid")
		}
	}
	if checkpoint == nil {
		executionID, invocationID := onlineV2ExecutionIDs(command)
		lease := r.onlineV2Lease(prepared, command.ExecutionKind)
		value, err = r.evidenceV2.ClaimNextExecution(ctx, command.RunID, ClaimEvidenceV2ExecutionCommand{
			ExecutionID: executionID, Owner: strings.TrimSpace(command.Owner), InvocationID: invocationID,
			ClaimedAt: now, LeaseExpiresAt: now.Add(lease),
		})
		if err != nil {
			return nil, err
		}
		checkpoint = value.Execution()
	}
	if checkpoint == nil || checkpoint.Owner != strings.TrimSpace(command.Owner) {
		return nil, ErrAttemptExecutionBusy
	}

	if command.ExecutionKind == domainevaluation.EvidenceExecutionGeneration {
		return r.runGenerationStepV2(ctx, prepared, value, command, *checkpoint)
	}
	return r.runSemanticStepV2(ctx, prepared, value, command, *checkpoint)
}

func (r *OnlineRunner) runGenerationStepV2(
	ctx context.Context,
	prepared *preparedOnlineRun,
	value *domainevaluation.PromptEvaluationEvidenceV2,
	command OnlineStepV2Command,
	checkpoint domainevaluation.EvidenceExecutionCheckpoint,
) (*OnlineStepV2Result, error) {
	testCase, ok := prepared.caseByID(command.CaseID)
	if !ok {
		return nil, fmt.Errorf("AI explanation evaluation v2 generation case is not executable")
	}
	assembled, err := syntheticInput(testCase.ProviderPayload, prepared.profile, testCase.CaseID)
	if err != nil {
		return nil, err
	}
	messages, err := appprompt.Render(prepared.prompt, prepared.profile.Definition(), assembled)
	if err != nil {
		return nil, err
	}
	dispatchAt := r.now().UTC()
	if _, err := r.evidenceV2.MarkExecutionDispatching(ctx, command.RunID, checkpoint.Owner, dispatchAt); err != nil {
		return nil, err
	}
	execution := domainevaluation.CandidateGenerationExecution{
		ID: checkpoint.ID, CaseID: command.CaseID, SlotOrdinal: command.SlotOrdinal,
		ExecutionOrdinal: command.ExecutionOrdinal, InvocationID: checkpoint.InvocationID,
		Status: domainevaluation.ExecutionStatusFailed, StartedAt: dispatchAt, ProviderCallCount: 1,
	}
	request := appport.ProviderRequest{
		InvocationID: checkpoint.InvocationID, Route: prepared.route,
		SystemMessage: messages.SystemMessage, TaskMessage: messages.TaskMessage,
		DataPreamble: messages.DataPreamble, DataJSON: messages.DataJSON, OutputSchema: prepared.outputSchema,
	}
	providerContext, cancel := context.WithTimeout(ctx, prepared.route.Timeout)
	response, providerErr := r.provider.Generate(providerContext, request)
	cancel()
	finishedAt := r.finishTime(dispatchAt)
	execution.FinishedAt = &finishedAt
	if providerErr != nil {
		execution.Status, execution.Failure = classifyGenerationFailureV2(checkpoint.ID, providerErr)
		return r.commitGenerationV2(ctx, command, execution, "", nil)
	}
	if response == nil {
		execution.Failure = classifiedFailureV2(
			domainevaluation.FailureStageGenerationExecution, domainevaluation.FailureKindProviderProtocol,
			"provider_response_missing", false, false, domainevaluation.FailureDispositionNoAction,
			"Provider returned no response", checkpoint.ID,
		)
		return r.commitGenerationV2(ctx, command, execution, "", nil)
	}
	execution.RawOutput = append([]byte(nil), response.RawOutput...)
	receipt := response.Receipt
	execution.ProviderReceipt = &receipt
	if len(execution.RawOutput) > domainevaluation.MaxStoredOutputBytes {
		execution.RawOutput = nil
		execution.Failure = outputContractFailureV2("provider_output_too_large", "Provider output exceeded the evaluation evidence limit", checkpoint.ID)
		return r.commitGenerationV2(ctx, command, execution, "", nil)
	}
	if err := validateProviderReceipt(receipt, checkpoint.InvocationID, prepared.route.ExecutionSpec); err != nil {
		if receipt.Validate() != nil {
			execution.ProviderReceipt = nil
		}
		execution.Failure = classifiedFailureV2(
			domainevaluation.FailureStageGenerationExecution, domainevaluation.FailureKindProviderProtocol,
			"provider_receipt_invalid", false, false, domainevaluation.FailureDispositionNoAction,
			"Provider receipt did not match the frozen evaluation request", checkpoint.ID,
		)
		return r.commitGenerationV2(ctx, command, execution, "", nil)
	}

	validationOutput := response.OutputForValidation()
	typed, err := appvalidation.ParseTypedContent(validationOutput)
	if err != nil {
		code, message := providerOutputSchemaFailure(err)
		execution.Failure = outputContractFailureV2(code, message, checkpoint.ID)
		return r.commitGenerationV2(ctx, command, execution, "", nil)
	}
	execution.NormalizedOutput, err = json.Marshal(typed)
	if err != nil {
		return nil, err
	}
	execution.NormalizedOutputFingerprint = domainai.NewFingerprint(execution.NormalizedOutput)
	allAssertions := append(cloneAssertions(prepared.defaultAssertions), cloneAssertions(testCase.Expected.Assertions)...)
	candidate, err := EvaluateCandidate(ctx, validationOutput, assembled.Document, prepared.profile.Definition(), allAssertions, r.safety)
	if err != nil {
		return nil, err
	}
	receipts, _, err := candidateReceiptsV2(candidate, allAssertions, len(prepared.defaultAssertions))
	if err != nil {
		return nil, err
	}
	execution.Status = domainevaluation.ExecutionStatusSucceeded
	candidateID := onlineV2CandidateID(command, checkpoint.ID)
	return r.commitGenerationV2(ctx, command, execution, candidateID, receipts)
}

func (r *OnlineRunner) runSemanticStepV2(
	ctx context.Context,
	prepared *preparedOnlineRun,
	value *domainevaluation.PromptEvaluationEvidenceV2,
	command OnlineStepV2Command,
	checkpoint domainevaluation.EvidenceExecutionCheckpoint,
) (*OnlineStepV2Result, error) {
	testCase, ok := prepared.caseByID(command.CaseID)
	if !ok {
		return nil, fmt.Errorf("AI explanation evaluation v2 semantic case is not executable")
	}
	slot, generation, err := semanticSourceV2(value, command)
	if err != nil {
		return nil, err
	}
	assembled, err := syntheticInput(testCase.ProviderPayload, prepared.profile, testCase.CaseID)
	if err != nil {
		return nil, err
	}
	messages, err := appprompt.Render(prepared.prompt, prepared.profile.Definition(), assembled)
	if err != nil {
		return nil, err
	}
	allAssertions := append(cloneAssertions(prepared.defaultAssertions), cloneAssertions(testCase.Expected.Assertions)...)
	obligations, err := frozenSemanticObligationsV2(slot.Candidate.Assertions, allAssertions, len(prepared.defaultAssertions))
	if err != nil {
		return nil, err
	}
	dispatchAt := r.now().UTC()
	if _, err := r.evidenceV2.MarkExecutionDispatching(ctx, command.RunID, checkpoint.Owner, dispatchAt); err != nil {
		return nil, err
	}
	semanticContext, cancel := context.WithTimeout(ctx, r.semanticTimeout)
	outcome, evaluateErr := r.semantic.Evaluate(semanticContext, SemanticEvaluationRequest{
		InvocationID: checkpoint.InvocationID, SuiteID: prepared.release.Suite.ID, CaseID: command.CaseID,
		Attempt: command.SlotOrdinal, InputJSON: append([]byte(nil), messages.DataJSON...),
		OutputJSON: append([]byte(nil), generation.NormalizedOutput...), Assertions: cloneSemanticAssertions(obligations),
	})
	cancel()
	execution := semanticExecutionV2(checkpoint, dispatchAt, outcome, evaluateErr, obligations, prepared.release.SemanticEvaluator)
	persistCtx, persistCancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer persistCancel()
	updated, err := r.durableCommitterV2.CommitSemanticV2(persistCtx, command.RunID, checkpoint.Owner, execution)
	if err != nil {
		return nil, err
	}
	return onlineStepV2Result(updated), nil
}

func (r *OnlineRunner) commitGenerationV2(
	ctx context.Context,
	command OnlineStepV2Command,
	execution domainevaluation.CandidateGenerationExecution,
	candidateID string,
	assertions []domainevaluation.AssertionReceipt,
) (*OnlineStepV2Result, error) {
	persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	updated, err := r.durableCommitterV2.CommitGenerationV2(persistCtx, command.RunID, CompleteGenerationV2Command{
		Owner: strings.TrimSpace(command.Owner), CandidateID: candidateID, Assertions: assertions, Execution: execution,
	})
	if err != nil {
		return nil, err
	}
	return onlineStepV2Result(updated), nil
}

func (r *OnlineRunner) completeUnknownV2(
	ctx context.Context,
	value *domainevaluation.PromptEvaluationEvidenceV2,
	checkpoint domainevaluation.EvidenceExecutionCheckpoint,
	at time.Time,
) (*OnlineStepV2Result, error) {
	startedAt := checkpoint.ClaimedAt
	if checkpoint.DispatchStartedAt != nil {
		startedAt = *checkpoint.DispatchStartedAt
	}
	failure := classifiedFailureV2(
		domainevaluation.FailureStageGenerationExecution, domainevaluation.FailureKindResultUnknown,
		"provider_result_unknown", false, true, domainevaluation.FailureDispositionManualAcknowledgement,
		"Provider result is unknown after an expired dispatch lease", checkpoint.ID,
	)
	if checkpoint.Kind == domainevaluation.EvidenceExecutionSemantic {
		failure.Stage = domainevaluation.FailureStageSemanticEvaluation
		failure.Code = domainevaluation.SemanticResultUnknown
	}
	if checkpoint.Kind == domainevaluation.EvidenceExecutionGeneration {
		execution := domainevaluation.CandidateGenerationExecution{
			ID: checkpoint.ID, CaseID: checkpoint.CaseID, SlotOrdinal: checkpoint.SlotOrdinal,
			ExecutionOrdinal: checkpoint.ExecutionOrdinal, InvocationID: checkpoint.InvocationID,
			Status: domainevaluation.ExecutionStatusResultUnknown, StartedAt: startedAt, FinishedAt: &at,
			ProviderCallCount: 1, Failure: failure,
		}
		return r.commitGenerationV2(ctx, OnlineStepV2Command{RunID: value.RunID, Owner: checkpoint.Owner}, execution, "", nil)
	}
	execution := domainevaluation.SemanticEvaluationExecution{
		ID: checkpoint.ID, CandidateID: checkpoint.CandidateID, ExecutionOrdinal: checkpoint.ExecutionOrdinal,
		InvocationID: checkpoint.InvocationID, Status: domainevaluation.ExecutionStatusResultUnknown,
		StartedAt: startedAt, FinishedAt: &at, ProviderCallCount: 1, Failure: failure,
	}
	persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	updated, err := r.durableCommitterV2.CommitSemanticV2(persistCtx, value.RunID, checkpoint.Owner, execution)
	if err != nil {
		return nil, err
	}
	return onlineStepV2Result(updated), nil
}

func (r *OnlineRunner) prepareExecutableV2(ctx context.Context, value *domainevaluation.PromptEvaluationEvidenceV2) (*preparedOnlineRun, error) {
	_, _, prepared, err := r.prepareV2(ctx)
	if err != nil {
		return nil, err
	}
	expected, err := preparedEvidenceReleaseV2(prepared, value.ExecutionPolicy, value.GatePolicy)
	if err != nil {
		return nil, err
	}
	if !reflect.DeepEqual(expected, value.Release) {
		return nil, fmt.Errorf("AI explanation evaluation v2 release no longer matches executable assets")
	}
	return prepared, nil
}

func preparedEvidenceReleaseV2(
	prepared *preparedOnlineRun,
	executionPolicy domainevaluation.EvaluationExecutionPolicy,
	gatePolicy domainevaluation.ReleaseGatePolicy,
) (domainevaluation.EvidenceReleaseIdentity, error) {
	if prepared == nil {
		return domainevaluation.EvidenceReleaseIdentity{}, fmt.Errorf("AI explanation evaluation v2 prepared release is required")
	}
	executionFingerprint, err := executionPolicy.Fingerprint()
	if err != nil {
		return domainevaluation.EvidenceReleaseIdentity{}, err
	}
	gateFingerprint, err := gatePolicy.Fingerprint()
	if err != nil {
		return domainevaluation.EvidenceReleaseIdentity{}, err
	}
	ref := func(id, version string, fingerprint domainai.Fingerprint) domainevaluation.FrozenContractRef {
		return domainevaluation.FrozenContractRef{ID: id, Version: version, Fingerprint: fingerprint}
	}
	release := domainevaluation.EvidenceReleaseIdentity{
		Suite:                ref(prepared.release.Suite.ID, prepared.release.Suite.Version, prepared.release.Suite.Fingerprint),
		Prompt:               ref(prepared.release.Prompt.TemplateID, prepared.release.Prompt.Version, prepared.release.Prompt.Fingerprint),
		Profile:              ref(prepared.release.Profile.ID, prepared.release.Profile.Version, prepared.release.Profile.Fingerprint),
		InputSchema:          ref(evidenceV2InputSchemaID, prepared.release.InputSchema.Version, prepared.release.InputSchema.Fingerprint),
		OutputSchema:         ref(evidenceV2OutputSchemaID, prepared.release.OutputSchema.Version, prepared.release.OutputSchema.Fingerprint),
		GenerationRoute:      ref(prepared.release.Provider.Route, prepared.release.Provider.RouteRevision, prepared.release.Provider.Fingerprint),
		SemanticPrompt:       ref(prepared.release.SemanticEvaluator.Prompt.TemplateID, prepared.release.SemanticEvaluator.Prompt.Version, prepared.release.SemanticEvaluator.Prompt.Fingerprint),
		SemanticOutputSchema: ref(evidenceV2SemanticOutputSchemaID, prepared.release.SemanticEvaluator.OutputSchema.Version, prepared.release.SemanticEvaluator.OutputSchema.Fingerprint),
		SemanticRoute:        ref(prepared.release.SemanticEvaluator.Provider.Route, prepared.release.SemanticEvaluator.Provider.RouteRevision, prepared.release.SemanticEvaluator.Provider.Fingerprint),
		ExecutionPolicy:      ref(executionPolicy.PolicyID, executionPolicy.Version, executionFingerprint),
		GatePolicy:           ref(gatePolicy.PolicyID, gatePolicy.Version, gateFingerprint),
	}
	release.Fingerprint, err = release.ExpectedFingerprint()
	if err != nil {
		return domainevaluation.EvidenceReleaseIdentity{}, err
	}
	return release, release.Validate(executionPolicy, gatePolicy)
}

// frozenSemanticObligationsV2 consumes the generation-time receipts. Revalidating
// NormalizedOutput would evaluate different bytes from the original provider
// response and could change schema, policy, or safety evidence after acceptance.
// Rebuild only the suite-defined inventory to verify scope, order, ordinal, and
// hard-gate identity; preserve every recorded status, evaluator, and detail.
func frozenSemanticObligationsV2(
	frozen []domainevaluation.AssertionReceipt,
	assertions []Assertion,
	defaultCount int,
) ([]SemanticAssertion, error) {
	candidate := &CandidateEvaluation{Assertions: make([]CandidateAssertionResult, len(frozen))}
	for index, receipt := range frozen {
		candidate.Assertions[index] = CandidateAssertionResult{
			Type: receipt.Type, Evaluator: receipt.Evaluator,
			Status: AssertionStatus(receipt.Status), Detail: receipt.Detail,
		}
	}
	receipts, obligations, err := candidateReceiptsV2(candidate, assertions, defaultCount)
	if err != nil {
		return nil, err
	}
	if !reflect.DeepEqual(receipts, frozen) || len(obligations) == 0 {
		return nil, fmt.Errorf("AI explanation evaluation v2 Candidate no longer matches frozen deterministic evidence")
	}
	return obligations, nil
}

func semanticSourceV2(
	value *domainevaluation.PromptEvaluationEvidenceV2,
	command OnlineStepV2Command,
) (*domainevaluation.CandidateSlot, *domainevaluation.CandidateGenerationExecution, error) {
	for slotIndex := range value.Slots {
		slot := &value.Slots[slotIndex]
		if slot.CaseID != command.CaseID || slot.Ordinal != command.SlotOrdinal || slot.Candidate == nil || slot.Candidate.ID != command.CandidateID {
			continue
		}
		for executionIndex := range value.GenerationExecutions {
			execution := &value.GenerationExecutions[executionIndex]
			if execution.ID == slot.Candidate.GenerationExecutionID && execution.Status == domainevaluation.ExecutionStatusSucceeded &&
				execution.NormalizedOutputFingerprint == slot.Candidate.NormalizedOutputFingerprint {
				return slot, execution, nil
			}
		}
		return nil, nil, fmt.Errorf("AI explanation evaluation v2 Candidate generation evidence is missing")
	}
	return nil, nil, fmt.Errorf("AI explanation evaluation v2 Candidate address is invalid")
}

func semanticExecutionV2(
	checkpoint domainevaluation.EvidenceExecutionCheckpoint,
	dispatchAt time.Time,
	outcome SemanticEvaluationOutcome,
	evaluateErr error,
	obligations []SemanticAssertion,
	expected domainevaluation.SemanticEvaluatorSpec,
) domainevaluation.SemanticEvaluationExecution {
	finishedAt := outcome.FinishedAt
	if finishedAt.IsZero() || finishedAt.Before(dispatchAt) {
		finishedAt = dispatchAt
	}
	execution := domainevaluation.SemanticEvaluationExecution{
		ID: checkpoint.ID, CandidateID: checkpoint.CandidateID, ExecutionOrdinal: checkpoint.ExecutionOrdinal,
		InvocationID: checkpoint.InvocationID, Status: domainevaluation.ExecutionStatusFailed,
		StartedAt: dispatchAt, FinishedAt: &finishedAt, ProviderCallCount: outcome.ProviderCallCount,
		RawOutput: append([]byte(nil), outcome.RawOutput...), NormalizedOutput: append([]byte(nil), outcome.NormalizedOutput...),
	}
	if outcome.ProviderReceipt != nil {
		receipt := *outcome.ProviderReceipt
		if receipt.Validate() == nil {
			execution.ProviderReceipt = &receipt
		}
	}
	if len(execution.RawOutput) > domainevaluation.MaxStoredOutputBytes || len(execution.NormalizedOutput) > domainevaluation.MaxStoredOutputBytes {
		execution.RawOutput = nil
		execution.NormalizedOutput = nil
		execution.Failure = classifiedFailureV2(
			domainevaluation.FailureStageSemanticEvaluation, domainevaluation.FailureKindSemanticExecution,
			domainevaluation.SemanticOutputMissingOrTooLarge, true, false, domainevaluation.FailureDispositionRetrySemantic,
			"semantic output was missing or exceeded the evidence limit", checkpoint.ID,
		)
		return execution
	}
	if evaluateErr != nil {
		execution.Failure = classifiedFailureV2(
			domainevaluation.FailureStageSemanticEvaluation, domainevaluation.FailureKindInfrastructureExecution,
			"semantic_evaluator_failed", false, false, domainevaluation.FailureDispositionNoAction,
			"semantic evaluator could not execute", checkpoint.ID,
		)
		return execution
	}
	if outcome.Failure != nil {
		execution.Status, execution.Failure = classifySemanticFailureV2(checkpoint.ID, *outcome.Failure)
		if outcome.ProviderDiagnostics != nil {
			diagnostics := *outcome.ProviderDiagnostics
			execution.Failure.ProviderDiagnostics = &diagnostics
			if !execution.Failure.ResultUnknown {
				switch {
				case diagnostics.Code == "provider_rate_limited":
					execution.Failure.Code = domainevaluation.SemanticProviderRateLimited
				case diagnostics.Code == "provider_output_cardinality_invalid" && diagnostics.ResponseStatus == "completed" && diagnostics.ResponseShape == "no_message":
					execution.Failure.Code = domainevaluation.SemanticProviderNoMessage
					execution.Failure.Retryable = true
				}
			}
		}
		return execution
	}
	if len(execution.RawOutput) == 0 || len(execution.NormalizedOutput) == 0 {
		execution.Failure = classifiedFailureV2(
			domainevaluation.FailureStageSemanticEvaluation, domainevaluation.FailureKindSemanticExecution,
			domainevaluation.SemanticOutputMissingOrTooLarge, true, false, domainevaluation.FailureDispositionRetrySemantic,
			"semantic evaluator returned no output evidence", checkpoint.ID,
		)
		return execution
	}
	if !json.Valid(execution.NormalizedOutput) {
		execution.Failure = classifiedFailureV2(
			domainevaluation.FailureStageSemanticEvaluation, domainevaluation.FailureKindSemanticExecution,
			domainevaluation.SemanticOutputDecodeInvalid, true, false, domainevaluation.FailureDispositionRetrySemantic,
			"semantic normalized output is not valid JSON", checkpoint.ID,
		)
		return execution
	}
	if outcome.Result == nil {
		execution.Failure = classifiedFailureV2(
			domainevaluation.FailureStageSemanticEvaluation, domainevaluation.FailureKindSemanticExecution,
			domainevaluation.SemanticDecisionContractInvalid, true, false, domainevaluation.FailureDispositionRetrySemantic,
			"semantic evaluator returned incomplete terminal evidence", checkpoint.ID,
		)
		return execution
	}
	if execution.ProviderReceipt == nil || validateProviderReceipt(*execution.ProviderReceipt, checkpoint.InvocationID, expected.Provider) != nil {
		execution.Failure = classifiedFailureV2(
			domainevaluation.FailureStageSemanticEvaluation, domainevaluation.FailureKindSemanticExecution,
			domainevaluation.SemanticReceiptInvalid, true, false, domainevaluation.FailureDispositionRetrySemantic,
			"semantic receipt did not match the frozen execution", checkpoint.ID,
		)
		return execution
	}
	if _, _, err := semanticReceipts(*outcome.Result, execution.ProviderReceipt, obligations, expected, checkpoint.InvocationID); err != nil {
		execution.Failure = classifiedFailureV2(
			domainevaluation.FailureStageSemanticEvaluation, domainevaluation.FailureKindSemanticExecution,
			domainevaluation.SemanticDecisionContractInvalid, true, false, domainevaluation.FailureDispositionRetrySemantic,
			"semantic evaluator returned invalid decision evidence", checkpoint.ID,
		)
		return execution
	}
	decisions := make([]domainevaluation.SemanticDecision, 0, len(outcome.Result.Decisions))
	for _, decision := range outcome.Result.Decisions {
		decisions = append(decisions, domainevaluation.SemanticDecision{
			Type: decision.Type, Scope: decision.Scope, Ordinal: decision.Ordinal,
			Status: decision.Status, Detail: strings.TrimSpace(decision.Detail),
		})
	}
	execution.Result = &domainevaluation.SemanticEvaluationResult{
		EvaluatorVersion: outcome.Result.EvaluatorVersion, Scores: outcome.Result.Scores,
		Rationale: strings.TrimSpace(outcome.Result.Rationale), Decisions: decisions,
		OutputFingerprint: domainai.NewFingerprint(execution.NormalizedOutput),
	}
	if err := execution.Result.Validate(); err != nil {
		execution.Result = nil
		execution.Failure = classifiedFailureV2(
			domainevaluation.FailureStageSemanticEvaluation, domainevaluation.FailureKindSemanticExecution,
			domainevaluation.SemanticDecisionContractInvalid, true, false, domainevaluation.FailureDispositionRetrySemantic,
			"semantic evaluator returned invalid decision evidence", checkpoint.ID,
		)
		return execution
	}
	execution.Status = domainevaluation.ExecutionStatusSucceeded
	return execution
}

func classifyGenerationFailureV2(executionID string, err error) (domainevaluation.ExecutionStatus, *domainevaluation.ClassifiedFailure) {
	status := domainevaluation.ExecutionStatusFailed
	code, message, retryable, resultUnknown := "provider_execution_failed", "Provider execution failed", false, false
	var providerErr *appport.ProviderError
	if errors.As(err, &providerErr) && providerErr != nil {
		code, message = strings.TrimSpace(providerErr.Code), strings.TrimSpace(providerErr.SafeMessage)
		retryable, resultUnknown = providerErr.Retryable, providerErr.ResultUnknown
	}
	if code == "" {
		code = "provider_execution_failed"
	}
	if message == "" {
		message = "Provider execution failed"
	}
	kind := domainevaluation.FailureKindInfrastructureExecution
	disposition := domainevaluation.FailureDispositionNoAction
	if resultUnknown {
		status, kind, retryable, disposition = domainevaluation.ExecutionStatusResultUnknown,
			domainevaluation.FailureKindResultUnknown, false, domainevaluation.FailureDispositionManualAcknowledgement
	} else if retryable {
		disposition = domainevaluation.FailureDispositionRetryGeneration
	}
	return status, classifiedFailureV2(
		domainevaluation.FailureStageGenerationExecution, kind, code, retryable, resultUnknown, disposition, message, executionID,
	)
}

func classifySemanticFailureV2(executionID string, failure domainevaluation.AttemptFailure) (domainevaluation.ExecutionStatus, *domainevaluation.ClassifiedFailure) {
	if failure.ResultUnknown {
		return domainevaluation.ExecutionStatusResultUnknown, classifiedFailureV2(
			domainevaluation.FailureStageSemanticEvaluation, domainevaluation.FailureKindResultUnknown,
			domainevaluation.SemanticResultUnknown, false, true, domainevaluation.FailureDispositionManualAcknowledgement,
			failure.SafeMessage, executionID,
		)
	}
	code := strings.TrimSpace(failure.Code)
	if code == "" {
		code = domainevaluation.SemanticProviderFailed
	}
	return domainevaluation.ExecutionStatusFailed, classifiedFailureV2(
		domainevaluation.FailureStageSemanticEvaluation, domainevaluation.FailureKindSemanticExecution,
		code, failure.Retryable, false, domainevaluation.FailureDispositionRetrySemantic,
		failure.SafeMessage, executionID,
	)
}

func outputContractFailureV2(code, message, executionID string) *domainevaluation.ClassifiedFailure {
	return classifiedFailureV2(
		domainevaluation.FailureStageOutputValidation, domainevaluation.FailureKindOutputContractConformance,
		code, false, false, domainevaluation.FailureDispositionReplaceGeneration, message, executionID,
	)
}

func classifiedFailureV2(
	stage domainevaluation.FailureStage,
	kind domainevaluation.FailureKind,
	code string,
	retryable, resultUnknown bool,
	disposition domainevaluation.FailureDisposition,
	safeMessage, executionID string,
) *domainevaluation.ClassifiedFailure {
	if strings.TrimSpace(safeMessage) == "" {
		safeMessage = "evaluation execution could not complete"
	}
	return &domainevaluation.ClassifiedFailure{
		SchemaVersion: domainevaluation.FailureTaxonomySchemaVersionV1, Stage: stage, Kind: kind,
		Code: strings.TrimSpace(code), Retryable: retryable, ResultUnknown: resultUnknown,
		Disposition: disposition, SafeMessage: strings.TrimSpace(safeMessage), EvidenceRefs: []string{executionID},
	}
}

func onlineStepV2Result(value *domainevaluation.PromptEvaluationEvidenceV2) *OnlineStepV2Result {
	status := OnlineStepV2Progressed
	switch value.Status {
	case domainevaluation.EvidenceStatusAwaitingReview:
		status = OnlineStepV2AwaitingReview
	case domainevaluation.EvidenceStatusBlocked:
		status = OnlineStepV2Blocked
	case domainevaluation.EvidenceStatusCanceled:
		status = OnlineStepV2Canceled
	}
	return &OnlineStepV2Result{Status: status, Evidence: value}
}

func (r *OnlineRunner) onlineV2Lease(prepared *preparedOnlineRun, kind domainevaluation.EvidenceExecutionKind) time.Duration {
	minimum := prepared.route.Timeout + 30*time.Second
	if kind == domainevaluation.EvidenceExecutionSemantic {
		minimum = r.semanticTimeout + 30*time.Second
	}
	if r.attemptLease > minimum {
		return r.attemptLease
	}
	return minimum
}

func sameOnlineStepV2Address(left, right domainevaluation.EvidenceNextAction) bool {
	return left.Kind == right.Kind && left.CaseID == right.CaseID && left.SlotOrdinal == right.SlotOrdinal &&
		left.CandidateID == right.CandidateID && left.ExecutionOrdinal == right.ExecutionOrdinal
}

func sameOnlineCheckpointV2Address(checkpoint domainevaluation.EvidenceExecutionCheckpoint, command OnlineStepV2Command) bool {
	return checkpoint.Kind == command.ExecutionKind && checkpoint.CaseID == command.CaseID &&
		checkpoint.SlotOrdinal == command.SlotOrdinal && checkpoint.CandidateID == command.CandidateID &&
		checkpoint.ExecutionOrdinal == command.ExecutionOrdinal
}

func onlineV2ExecutionIDs(command OnlineStepV2Command) (string, string) {
	kind := "g"
	if command.ExecutionKind == domainevaluation.EvidenceExecutionSemantic {
		kind = "s"
	}
	address := fmt.Sprintf("%s:%s:%s:%d:%s:%d", kind, command.RunID, command.CaseID, command.SlotOrdinal, command.CandidateID, command.ExecutionOrdinal)
	return boundedOnlineV2ID("evalv2-exec", address), boundedOnlineV2ID("evalv2-inv", address)
}

func onlineV2CandidateID(command OnlineStepV2Command, executionID string) string {
	return boundedOnlineV2ID("evalv2-candidate", fmt.Sprintf("%s:%s:%d:%s", command.RunID, command.CaseID, command.SlotOrdinal, executionID))
}

func boundedOnlineV2ID(prefix, address string) string {
	readable := prefix + ":" + address
	if len(readable) <= 128 {
		return readable
	}
	digest := sha256.Sum256([]byte(address))
	return fmt.Sprintf("%s:%x", prefix, digest[:16])
}

var _ OnlineStepV2Runner = (*OnlineRunner)(nil)
