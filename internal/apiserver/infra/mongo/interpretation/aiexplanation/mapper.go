package aiexplanation

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation"
	domainartifact "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/artifact"
	domainevaluation "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/evaluation"
	domaingeneration "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/generation"
	domaininput "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/input"
	domainoutput "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/output"
	domainprofile "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/profile"
	domainrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/run"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	base "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
)

type Mapper struct{}

func NewMapper() *Mapper { return &Mapper{} }

func (*Mapper) GenerationToPO(value *domaingeneration.AIExplanationGeneration) (*GenerationPO, error) {
	if value == nil {
		return nil, fmt.Errorf("AI explanation generation is required")
	}
	key := value.Key()
	return &GenerationPO{
		BaseDocument:   base.BaseDocument{DomainID: value.ID(), CreatedAt: value.CreatedAt(), UpdatedAt: value.UpdatedAt()},
		SourceReportID: key.SourceReportID.Uint64(), Audience: string(key.Audience), Profile: profileRefToPO(key.Profile),
		InputSchema: value.Input().SchemaVersion(), InputJSON: value.Input().CanonicalJSON(), InputFingerprint: key.InputFingerprint.String(),
		ExecutionSpecFingerprint: key.ExecutionSpecFingerprint.String(), Association: associationToPO(value.Association()),
		RequestedBy: actorToPO(value.RequestedBy()), Prompt: promptToPO(value.Prompt()), ExecutionSpec: executionToPO(value.ExecutionSpec()),
		Status: string(value.Status()), LatestRunID: value.LatestRunID().Uint64(), ArtifactID: value.ArtifactID().Uint64(), Version: value.Version(),
	}, nil
}

func (*Mapper) GenerationToDomain(po *GenerationPO) (*domaingeneration.AIExplanationGeneration, error) {
	if po == nil {
		return nil, fmt.Errorf("AI explanation generation document is required")
	}
	profileRef, err := profileRefFromPO(po.Profile)
	if err != nil {
		return nil, err
	}
	inputFingerprint, err := aiexplanation.ParseFingerprint(po.InputFingerprint)
	if err != nil {
		return nil, err
	}
	executionFingerprint, err := aiexplanation.ParseFingerprint(po.ExecutionSpecFingerprint)
	if err != nil {
		return nil, err
	}
	snapshot, err := domaininput.RestoreSnapshot(po.InputSchema, po.InputJSON, inputFingerprint)
	if err != nil {
		return nil, err
	}
	promptRef, err := promptFromPO(po.Prompt)
	if err != nil {
		return nil, err
	}
	executionSpec, err := executionFromPO(po.ExecutionSpec)
	if err != nil {
		return nil, err
	}
	return domaingeneration.Restore(domaingeneration.RestoreInput{
		NewInput: domaingeneration.NewInput{
			ID:          po.DomainID,
			Key:         domaingeneration.Key{SourceReportID: meta.FromUint64(po.SourceReportID), Audience: policy.Audience(po.Audience), Profile: profileRef, InputFingerprint: inputFingerprint, ExecutionSpecFingerprint: executionFingerprint},
			Association: associationFromPO(po.Association), RequestedBy: actorFromPO(po.RequestedBy), Input: snapshot, Prompt: promptRef, ExecutionSpec: executionSpec, CreatedAt: po.CreatedAt,
		},
		Status: domaingeneration.Status(po.Status), LatestRunID: meta.FromUint64(po.LatestRunID), ArtifactID: meta.FromUint64(po.ArtifactID), Version: po.Version, UpdatedAt: po.UpdatedAt,
	})
}

func (*Mapper) RunToPO(value *domainrun.AIExplanationRun) (*RunPO, error) {
	if value == nil {
		return nil, fmt.Errorf("AI explanation run is required")
	}
	po := &RunPO{
		BaseDocument: base.BaseDocument{DomainID: value.ID()}, GenerationID: value.GenerationID().Uint64(), Attempt: value.Attempt(), Status: string(value.Status()),
		TraceID: value.TraceID(), StartedAt: value.StartedAt(), LeaseExpiresAt: value.LeaseExpiresAt(), FinishedAt: value.FinishedAt(), Origin: string(value.Origin()),
		InvocationID: value.InvocationID(), InvocationPhase: string(value.InvocationPhase()), DispatchStartedAt: value.DispatchStartedAt(), RecoveryCount: value.RecoveryCount(), LastReclaimedAt: value.LastReclaimedAt(),
	}
	if failure := value.Failure(); failure != nil {
		po.Failure = &FailurePO{Kind: string(failure.Kind), Code: failure.Code, SafeMessage: failure.SafeMessage, Retryable: failure.Retryable}
	}
	if receipt := value.ProviderReceipt(); receipt != nil {
		converted := receiptToPO(*receipt)
		po.Receipt = &converted
	}
	if authorization := value.RetryAuthorization(); authorization != nil {
		po.RetryAuthorization = &RetryAuthorizationPO{
			ExpectedAttempt: authorization.ExpectedAttempt, NextAttempt: authorization.NextAttempt,
			Origin: string(authorization.Origin), RequestID: authorization.RequestID, EventID: authorization.EventID,
			Actor: authorization.Actor, Reason: authorization.Reason,
			AcceptedResultUnknownRisk: authorization.AcceptedResultUnknownRisk, AuthorizedAt: authorization.AuthorizedAt,
		}
	}
	if wakeup := value.RecoveryWakeup(); wakeup != nil {
		po.RecoveryWakeup = &RecoveryWakeupPO{
			EventID: wakeup.EventID, ExpectedLeaseExpiresAt: wakeup.ExpectedLeaseExpiresAt,
			InvocationPhase: string(wakeup.InvocationPhase), RequestedAt: wakeup.RequestedAt,
		}
	}
	for _, claim := range value.ClaimHistory() {
		po.ClaimHistory = append(po.ClaimHistory, ClaimRecordPO{ReclaimedAt: claim.ReclaimedAt, TraceID: claim.TraceID})
	}
	return po, nil
}

func (*Mapper) RunToDomain(po *RunPO) (*domainrun.AIExplanationRun, error) {
	if po == nil {
		return nil, fmt.Errorf("AI explanation run document is required")
	}
	input := domainrun.RestoreInput{
		ID: po.DomainID, GenerationID: meta.FromUint64(po.GenerationID), Attempt: po.Attempt, Status: domainrun.Status(po.Status), TraceID: po.TraceID,
		StartedAt: po.StartedAt, LeaseExpiresAt: po.LeaseExpiresAt, FinishedAt: po.FinishedAt, Origin: retrygovernance.AttemptOrigin(po.Origin),
		InvocationID: po.InvocationID, InvocationPhase: domainrun.InvocationPhase(po.InvocationPhase), DispatchStartedAt: po.DispatchStartedAt,
		RecoveryCount: po.RecoveryCount, LastReclaimedAt: po.LastReclaimedAt,
	}
	if po.Failure != nil {
		input.Failure = &domainrun.Failure{Kind: domainrun.FailureKind(po.Failure.Kind), Code: po.Failure.Code, SafeMessage: po.Failure.SafeMessage, Retryable: po.Failure.Retryable}
	}
	if po.Receipt != nil {
		receipt := receiptFromPO(*po.Receipt)
		input.Receipt = &receipt
	}
	if po.RetryAuthorization != nil {
		input.RetryAuthorization = &domainrun.RetryAuthorization{
			ExpectedAttempt: po.RetryAuthorization.ExpectedAttempt, NextAttempt: po.RetryAuthorization.NextAttempt,
			Origin: retrygovernance.AttemptOrigin(po.RetryAuthorization.Origin), RequestID: po.RetryAuthorization.RequestID,
			EventID: po.RetryAuthorization.EventID, Actor: po.RetryAuthorization.Actor, Reason: po.RetryAuthorization.Reason,
			AcceptedResultUnknownRisk: po.RetryAuthorization.AcceptedResultUnknownRisk, AuthorizedAt: po.RetryAuthorization.AuthorizedAt,
		}
	}
	if po.RecoveryWakeup != nil {
		input.RecoveryWakeup = &domainrun.RecoveryWakeup{
			EventID: po.RecoveryWakeup.EventID, ExpectedLeaseExpiresAt: po.RecoveryWakeup.ExpectedLeaseExpiresAt,
			InvocationPhase: domainrun.InvocationPhase(po.RecoveryWakeup.InvocationPhase), RequestedAt: po.RecoveryWakeup.RequestedAt,
		}
	}
	for _, claim := range po.ClaimHistory {
		input.ClaimHistory = append(input.ClaimHistory, domainrun.ClaimRecord{ReclaimedAt: claim.ReclaimedAt, TraceID: claim.TraceID})
	}
	return domainrun.Restore(input)
}

func (*Mapper) ArtifactToPO(value *domainartifact.AIExplanationArtifact) (*ArtifactPO, error) {
	if value == nil {
		return nil, fmt.Errorf("AI explanation artifact is required")
	}
	source := value.Source()
	validation := value.Validation()
	return &ArtifactPO{
		BaseDocument: base.BaseDocument{DomainID: value.ID(), CreatedAt: value.GeneratedAt(), UpdatedAt: value.GeneratedAt()},
		GenerationID: value.GenerationID().Uint64(), RunID: value.RunID().Uint64(),
		Source:   SourceRefPO{ReportID: source.ReportID.Uint64(), OutcomeID: source.OutcomeID.Uint64(), Association: associationToPO(source.Association), ReportType: source.ReportType, TemplateVersion: source.TemplateVersion, ContentSchemaVersion: source.ContentSchemaVersion, BuilderIdentity: source.BuilderIdentity, ReportGeneratedAt: source.ReportGeneratedAt},
		Audience: string(value.Audience()), Profile: profileRefToPO(value.Profile()), Prompt: promptToPO(value.Prompt()), ExecutionSpec: executionToPO(value.ExecutionSpec()),
		InputSchema: value.InputSchema(), InputFingerprint: value.InputFingerprint().String(), OutputSchema: value.OutputSchema(), SafetyPolicy: value.SafetyPolicy(),
		ProviderReceipt: receiptToPO(value.ProviderReceipt()), Validation: ValidationReceiptPO{SchemaValidatorVersion: validation.SchemaValidatorVersion, ReferenceValidatorVersion: validation.ReferenceValidatorVersion, ProfileValidatorVersion: validation.ProfileValidatorVersion, SafetyValidatorVersion: validation.SafetyValidatorVersion, ValidatedAt: validation.ValidatedAt},
		Content: func() *domainoutput.Content { content := value.Content(); return &content }(), GeneratedAt: value.GeneratedAt(),
	}, nil
}

func (*Mapper) ArtifactToDomain(po *ArtifactPO) (*domainartifact.AIExplanationArtifact, error) {
	if po == nil {
		return nil, fmt.Errorf("AI explanation artifact document is required")
	}
	profileRef, err := profileRefFromPO(po.Profile)
	if err != nil {
		return nil, err
	}
	promptRef, err := promptFromPO(po.Prompt)
	if err != nil {
		return nil, err
	}
	executionSpec, err := executionFromPO(po.ExecutionSpec)
	if err != nil {
		return nil, err
	}
	inputFingerprint, err := aiexplanation.ParseFingerprint(po.InputFingerprint)
	if err != nil {
		return nil, err
	}
	return domainartifact.New(domainartifact.NewInput{
		ID: po.DomainID, GenerationID: meta.FromUint64(po.GenerationID), RunID: meta.FromUint64(po.RunID),
		Source:   domainartifact.SourceRef{ReportID: meta.FromUint64(po.Source.ReportID), OutcomeID: meta.FromUint64(po.Source.OutcomeID), Association: associationFromPO(po.Source.Association), ReportType: po.Source.ReportType, TemplateVersion: po.Source.TemplateVersion, ContentSchemaVersion: po.Source.ContentSchemaVersion, BuilderIdentity: po.Source.BuilderIdentity, ReportGeneratedAt: po.Source.ReportGeneratedAt},
		Audience: policy.Audience(po.Audience), Profile: profileRef, Prompt: promptRef, ExecutionSpec: executionSpec,
		InputSchema: po.InputSchema, InputFingerprint: inputFingerprint, OutputSchema: po.OutputSchema, SafetyPolicy: po.SafetyPolicy, ProviderReceipt: receiptFromPO(po.ProviderReceipt),
		Validation: domainartifact.ValidationReceipt{SchemaValidatorVersion: po.Validation.SchemaValidatorVersion, ReferenceValidatorVersion: po.Validation.ReferenceValidatorVersion, ProfileValidatorVersion: po.Validation.ProfileValidatorVersion, SafetyValidatorVersion: po.Validation.SafetyValidatorVersion, ValidatedAt: po.Validation.ValidatedAt},
		Content:    artifactContentFromPO(po), GeneratedAt: po.GeneratedAt,
	})
}

func artifactContentFromPO(po *ArtifactPO) domainoutput.Content {
	if po == nil || po.Content == nil {
		return domainoutput.Content{}
	}
	return po.Content.Clone()
}

func (*Mapper) ProfileToPO(value *domainprofile.AIExplanationProfile) (*ProfilePO, error) {
	if value == nil {
		return nil, fmt.Errorf("AI explanation Profile is required")
	}
	return &ProfilePO{
		BaseDocument: base.BaseDocument{DomainID: value.ID(), CreatedAt: value.CreatedAt(), UpdatedAt: value.UpdatedAt()}, Definition: value.Definition(), Fingerprint: value.Fingerprint().String(), SelectorSlotKey: profileSelectorSlotKey(value.Selector()), Status: string(value.Status()),
		CreatedBy: value.CreatedBy(), CreatedReason: value.CreatedReason(),
		PublishedAt: value.PublishedAt(), PublishedBy: value.PublishedBy(), PublishedReason: value.PublishedReason(), PublishedEvidenceRunID: value.PublishedEvidenceRunID().Uint64(),
		DisabledAt: value.DisabledAt(), DisabledBy: value.DisabledBy(), DisabledReason: value.DisabledReason(),
	}, nil
}

func profileSelectorSlotKey(selector domainprofile.Selector) string {
	var modelCode, modelVersion any
	if selector.ModelCode != nil {
		modelCode = *selector.ModelCode
	}
	if selector.ModelVersion != nil {
		modelVersion = *selector.ModelVersion
	}
	raw, err := json.Marshal([]any{selector.Audience, selector.ModelKind, selector.DecisionKind, selector.Specificity(), modelCode, modelVersion})
	if err != nil {
		panic(fmt.Errorf("marshal validated AI explanation Profile selector slot: %w", err))
	}
	return string(raw)
}

func (*Mapper) ProfileToDomain(po *ProfilePO) (*domainprofile.AIExplanationProfile, error) {
	if po == nil {
		return nil, fmt.Errorf("AI explanation Profile document is required")
	}
	fingerprint, err := aiexplanation.ParseFingerprint(po.Fingerprint)
	if err != nil {
		return nil, err
	}
	return domainprofile.Restore(domainprofile.PersistedInput{
		ID: po.DomainID, Definition: po.Definition, Fingerprint: fingerprint, Status: domainprofile.Status(po.Status), CreatedAt: po.CreatedAt, UpdatedAt: po.UpdatedAt,
		CreatedBy: po.CreatedBy, CreatedReason: po.CreatedReason,
		PublishedAt: po.PublishedAt, PublishedBy: po.PublishedBy, PublishedReason: po.PublishedReason, PublishedEvidenceRunID: meta.FromUint64(po.PublishedEvidenceRunID),
		DisabledAt: po.DisabledAt, DisabledBy: po.DisabledBy, DisabledReason: po.DisabledReason,
	})
}

func (*Mapper) PromptEvaluationRunToPO(value *domainevaluation.PromptEvaluationRun) (*PromptEvaluationRunPO, error) {
	if value == nil {
		return nil, fmt.Errorf("AI explanation Prompt evaluation run is required")
	}
	release := value.Release()
	releaseFingerprint, err := release.Fingerprint()
	if err != nil {
		return nil, err
	}
	activeReleaseKey := ""
	if value.Status() == domainevaluation.StatusCollecting || value.Status() == domainevaluation.StatusAwaitingReview {
		activeReleaseKey = releaseFingerprint.String()
	}
	activeExecutionOrgKey := ""
	if value.Status() == domainevaluation.StatusCollecting && value.RequestedOrgID() > 0 {
		activeExecutionOrgKey = strconv.FormatInt(value.RequestedOrgID(), 10)
	}
	po := &PromptEvaluationRunPO{
		BaseDocument: base.BaseDocument{DomainID: value.ID(), CreatedAt: value.CreatedAt(), UpdatedAt: evaluationUpdatedAt(value)},
		Release: EvaluationReleasePO{
			Suite:  EvaluationSuiteRefPO{ID: release.Suite.ID, Version: release.Suite.Version, Fingerprint: release.Suite.Fingerprint.String(), GitBlobSHA: release.Suite.GitBlobSHA},
			Prompt: promptToPO(release.Prompt), Profile: profileRefToPO(release.Profile),
			InputSchema: schemaRefToPO(release.InputSchema), OutputSchema: schemaRefToPO(release.OutputSchema),
			Provider: executionToPO(release.Provider), Decoding: decodingToPO(release.Decoding),
			SemanticEvaluator: semanticEvaluatorSpecToPO(release.SemanticEvaluator),
			GenerationCaseIDs: append([]string(nil), release.GenerationCaseIDs...), PreflightCaseID: release.PreflightCaseID,
			PreflightRejectionReason: release.PreflightRejectionReason, RepetitionsPerCase: release.RepetitionsPerCase,
		},
		Status: string(value.Status()), ActiveReleaseKey: activeReleaseKey, ActiveExecutionOrgKey: activeExecutionOrgKey,
		Version: value.Version(), ClosedAt: value.ClosedAt(), FinalizedAt: value.FinalizedAt(),
		FinalizedBy: value.FinalizedBy(), FinalReason: value.FinalReason(), Gate: gateToPO(value.Gate()),
		Execution:      evaluationExecutionToPO(value.Execution()),
		RequestedOrgID: value.RequestedOrgID(), RequestedBy: value.RequestedBy(), RequestReason: value.RequestReason(),
		CanceledAt: value.CanceledAt(), CanceledBy: value.CanceledBy(), CancelReason: value.CancelReason(),
	}
	for _, recovery := range value.Recoveries() {
		po.Recoveries = append(po.Recoveries, EvaluationRecoveryRequestPO{
			ID: recovery.ID, CaseID: recovery.CaseID, Attempt: recovery.Attempt, Actor: recovery.Actor,
			Reason: recovery.Reason, RequestedAt: recovery.RequestedAt,
		})
	}
	for _, attempt := range value.Attempts() {
		po.Attempts = append(po.Attempts, attemptToPO(attempt))
	}
	for _, review := range value.Reviews() {
		po.Reviews = append(po.Reviews, EvaluationHumanReviewPO{
			CaseID: review.CaseID, Attempt: review.Attempt, Role: string(review.Role), Reviewer: review.Reviewer,
			Decision: string(review.Decision), ReviewedAt: review.ReviewedAt, Reason: review.Reason,
		})
	}
	return po, nil
}

func (*Mapper) PromptEvaluationRunToDomain(po *PromptEvaluationRunPO) (*domainevaluation.PromptEvaluationRun, error) {
	if po == nil {
		return nil, fmt.Errorf("AI explanation Prompt evaluation document is required")
	}
	suiteFingerprint, err := aiexplanation.ParseFingerprint(po.Release.Suite.Fingerprint)
	if err != nil {
		return nil, err
	}
	promptRef, err := promptFromPO(po.Release.Prompt)
	if err != nil {
		return nil, err
	}
	profileRef, err := profileRefFromPO(po.Release.Profile)
	if err != nil {
		return nil, err
	}
	inputSchema, err := schemaRefFromPO(po.Release.InputSchema)
	if err != nil {
		return nil, err
	}
	outputSchema, err := schemaRefFromPO(po.Release.OutputSchema)
	if err != nil {
		return nil, err
	}
	provider, err := executionFromPO(po.Release.Provider)
	if err != nil {
		return nil, err
	}
	semanticEvaluator, err := semanticEvaluatorSpecFromPO(po.Release.SemanticEvaluator)
	if err != nil {
		return nil, err
	}
	attempts := make([]domainevaluation.AttemptRecord, 0, len(po.Attempts))
	for _, value := range po.Attempts {
		attempt, err := attemptFromPO(value)
		if err != nil {
			return nil, err
		}
		attempts = append(attempts, attempt)
	}
	reviews := make([]domainevaluation.HumanReview, 0, len(po.Reviews))
	for _, value := range po.Reviews {
		reviews = append(reviews, domainevaluation.HumanReview{
			CaseID: value.CaseID, Attempt: value.Attempt, Role: domainevaluation.ReviewRole(value.Role), Reviewer: value.Reviewer,
			Decision: domainevaluation.ReviewDecision(value.Decision), ReviewedAt: value.ReviewedAt, Reason: value.Reason,
		})
	}
	recoveries := make([]domainevaluation.RecoveryRequest, 0, len(po.Recoveries))
	for _, value := range po.Recoveries {
		recoveries = append(recoveries, domainevaluation.RecoveryRequest{
			ID: value.ID, CaseID: value.CaseID, Attempt: value.Attempt, Actor: value.Actor,
			Reason: value.Reason, RequestedAt: value.RequestedAt,
		})
	}
	release := domainevaluation.ReleaseIdentity{
		Suite:  domainevaluation.SuiteRef{ID: po.Release.Suite.ID, Version: po.Release.Suite.Version, Fingerprint: suiteFingerprint, GitBlobSHA: po.Release.Suite.GitBlobSHA},
		Prompt: promptRef, Profile: profileRef, InputSchema: inputSchema, OutputSchema: outputSchema, Provider: provider,
		SemanticEvaluator: semanticEvaluator,
		Decoding:          decodingFromPO(po.Release.Decoding), GenerationCaseIDs: append([]string(nil), po.Release.GenerationCaseIDs...),
		PreflightCaseID: po.Release.PreflightCaseID, PreflightRejectionReason: po.Release.PreflightRejectionReason,
		RepetitionsPerCase: po.Release.RepetitionsPerCase,
	}
	releaseFingerprint, err := release.Fingerprint()
	if err != nil {
		return nil, err
	}
	status := domainevaluation.Status(po.Status)
	active := status == domainevaluation.StatusCollecting || status == domainevaluation.StatusAwaitingReview
	if active && po.ActiveReleaseKey != releaseFingerprint.String() || !active && po.ActiveReleaseKey != "" {
		return nil, fmt.Errorf("AI explanation Prompt evaluation active release key is invalid")
	}
	expectedActiveExecutionOrgKey := ""
	if status == domainevaluation.StatusCollecting && po.RequestedOrgID > 0 {
		expectedActiveExecutionOrgKey = strconv.FormatInt(po.RequestedOrgID, 10)
	}
	if po.ActiveExecutionOrgKey != expectedActiveExecutionOrgKey {
		return nil, fmt.Errorf("AI explanation Prompt evaluation active organization execution key is invalid")
	}
	return domainevaluation.Restore(domainevaluation.PersistedInput{
		ID:      po.DomainID,
		Release: release,
		Status:  status, Version: po.Version, Attempts: attempts, Reviews: reviews,
		Execution: evaluationExecutionFromPO(po.Execution), Recoveries: recoveries,
		RequestedOrgID: po.RequestedOrgID, RequestedBy: po.RequestedBy, RequestReason: po.RequestReason,
		CreatedAt: po.CreatedAt, ClosedAt: po.ClosedAt, FinalizedAt: po.FinalizedAt, FinalizedBy: po.FinalizedBy,
		FinalReason: po.FinalReason, Gate: gateFromPO(po.Gate),
		CanceledAt: po.CanceledAt, CanceledBy: po.CanceledBy, CancelReason: po.CancelReason,
	})
}

func schemaRefToPO(value domainevaluation.SchemaRef) EvaluationSchemaRefPO {
	return EvaluationSchemaRefPO{Version: value.Version, Fingerprint: value.Fingerprint.String()}
}

func schemaRefFromPO(value EvaluationSchemaRefPO) (domainevaluation.SchemaRef, error) {
	fingerprint, err := aiexplanation.ParseFingerprint(value.Fingerprint)
	return domainevaluation.SchemaRef{Version: value.Version, Fingerprint: fingerprint}, err
}

func decodingToPO(value domainevaluation.DecodingParameters) EvaluationDecodingPO {
	return EvaluationDecodingPO{MaxOutputTokens: value.MaxOutputTokens, Temperature: cloneFloatPtr(value.Temperature), TopP: cloneFloatPtr(value.TopP), Seed: cloneInt64Ptr(value.Seed)}
}

func decodingFromPO(value EvaluationDecodingPO) domainevaluation.DecodingParameters {
	return domainevaluation.DecodingParameters{MaxOutputTokens: value.MaxOutputTokens, Temperature: cloneFloatPtr(value.Temperature), TopP: cloneFloatPtr(value.TopP), Seed: cloneInt64Ptr(value.Seed)}
}

func semanticEvaluatorSpecToPO(value domainevaluation.SemanticEvaluatorSpec) EvaluationSemanticEvaluatorSpecPO {
	return EvaluationSemanticEvaluatorSpecPO{
		Version: value.Version, Prompt: promptToPO(value.Prompt), OutputSchema: schemaRefToPO(value.OutputSchema),
		Provider: executionToPO(value.Provider), Decoding: decodingToPO(value.Decoding),
	}
}

func semanticEvaluatorSpecFromPO(value EvaluationSemanticEvaluatorSpecPO) (domainevaluation.SemanticEvaluatorSpec, error) {
	prompt, err := promptFromPO(value.Prompt)
	if err != nil {
		return domainevaluation.SemanticEvaluatorSpec{}, err
	}
	outputSchema, err := schemaRefFromPO(value.OutputSchema)
	if err != nil {
		return domainevaluation.SemanticEvaluatorSpec{}, err
	}
	provider, err := executionFromPO(value.Provider)
	if err != nil {
		return domainevaluation.SemanticEvaluatorSpec{}, err
	}
	return domainevaluation.SemanticEvaluatorSpec{
		Version: value.Version, Prompt: prompt, OutputSchema: outputSchema, Provider: provider, Decoding: decodingFromPO(value.Decoding),
	}, nil
}

func attemptToPO(value domainevaluation.AttemptRecord) EvaluationAttemptPO {
	po := EvaluationAttemptPO{
		CaseID: value.CaseID, Attempt: value.Attempt, Stage: string(value.Stage), StartedAt: value.StartedAt, FinishedAt: value.FinishedAt,
		ProviderCallCount: value.ProviderCallCount, RawOutput: append([]byte(nil), value.RawOutput...), NormalizedOutput: append([]byte(nil), value.NormalizedOutput...),
		OutputFingerprint: value.OutputFingerprint.String(), RejectionReason: value.RejectionReason,
	}
	if value.ProviderReceipt != nil {
		receipt := receiptToPO(*value.ProviderReceipt)
		po.ProviderReceipt = &receipt
	}
	if value.Failure != nil {
		po.Failure = &EvaluationAttemptFailurePO{
			Stage: value.Failure.Stage, Code: value.Failure.Code, SafeMessage: value.Failure.SafeMessage,
			Retryable: value.Failure.Retryable, ResultUnknown: value.Failure.ResultUnknown,
		}
	}
	for _, assertion := range value.Assertions {
		po.Assertions = append(po.Assertions, EvaluationAssertionPO{
			Type: assertion.Type, Scope: string(assertion.Scope), Ordinal: assertion.Ordinal, Hard: assertion.Hard, Evaluator: assertion.Evaluator,
			Status: string(assertion.Status), Detail: assertion.Detail,
		})
	}
	if value.Semantic != nil {
		po.Semantic = &EvaluationSemanticPO{
			EvaluatorVersion: value.Semantic.EvaluatorVersion, ProviderReceipt: receiptToPO(value.Semantic.ProviderReceipt),
			Scores: scoresToPO(value.Semantic.Scores), Rationale: value.Semantic.Rationale,
		}
	}
	return po
}

func attemptFromPO(value EvaluationAttemptPO) (domainevaluation.AttemptRecord, error) {
	var outputFingerprint aiexplanation.Fingerprint
	var err error
	if value.OutputFingerprint != "" {
		outputFingerprint, err = aiexplanation.ParseFingerprint(value.OutputFingerprint)
		if err != nil {
			return domainevaluation.AttemptRecord{}, err
		}
	}
	result := domainevaluation.AttemptRecord{
		CaseID: value.CaseID, Attempt: value.Attempt, Stage: domainevaluation.AttemptStage(value.Stage), StartedAt: value.StartedAt, FinishedAt: value.FinishedAt,
		ProviderCallCount: value.ProviderCallCount, RawOutput: append([]byte(nil), value.RawOutput...), NormalizedOutput: append([]byte(nil), value.NormalizedOutput...),
		OutputFingerprint: outputFingerprint, RejectionReason: value.RejectionReason,
	}
	if value.ProviderReceipt != nil {
		receipt := receiptFromPO(*value.ProviderReceipt)
		result.ProviderReceipt = &receipt
	}
	if value.Failure != nil {
		result.Failure = &domainevaluation.AttemptFailure{
			Stage: value.Failure.Stage, Code: value.Failure.Code, SafeMessage: value.Failure.SafeMessage,
			Retryable: value.Failure.Retryable, ResultUnknown: value.Failure.ResultUnknown,
		}
	}
	for _, assertion := range value.Assertions {
		result.Assertions = append(result.Assertions, domainevaluation.AssertionReceipt{
			Type: assertion.Type, Scope: domainevaluation.AssertionScope(assertion.Scope), Ordinal: assertion.Ordinal, Hard: assertion.Hard,
			Evaluator: assertion.Evaluator, Status: domainevaluation.AssertionStatus(assertion.Status), Detail: assertion.Detail,
		})
	}
	if value.Semantic != nil {
		result.Semantic = &domainevaluation.SemanticReceipt{
			EvaluatorVersion: value.Semantic.EvaluatorVersion, ProviderReceipt: receiptFromPO(value.Semantic.ProviderReceipt),
			Scores: scoresFromPO(value.Semantic.Scores), Rationale: value.Semantic.Rationale,
		}
	}
	return result, nil
}

func scoresToPO(value domainevaluation.SemanticScores) EvaluationSemanticScoresPO {
	return EvaluationSemanticScoresPO{
		Faithfulness: value.Faithfulness, CrossDimensionQuality: value.CrossDimensionQuality,
		SuggestionActionability: value.SuggestionActionability, AudienceClarity: value.AudienceClarity, Concision: value.Concision,
	}
}

func scoresFromPO(value EvaluationSemanticScoresPO) domainevaluation.SemanticScores {
	return domainevaluation.SemanticScores{
		Faithfulness: value.Faithfulness, CrossDimensionQuality: value.CrossDimensionQuality,
		SuggestionActionability: value.SuggestionActionability, AudienceClarity: value.AudienceClarity, Concision: value.Concision,
	}
}

func gateToPO(value *domainevaluation.GateResult) *EvaluationGatePO {
	if value == nil {
		return nil
	}
	po := &EvaluationGatePO{Passed: value.Passed, Metrics: metricsToPO(value.Metrics)}
	for _, reason := range value.Reasons {
		po.Reasons = append(po.Reasons, EvaluationGateReasonPO{Code: reason.Code, CaseID: reason.CaseID, Attempt: reason.Attempt, Detail: reason.Detail})
	}
	return po
}

func gateFromPO(value *EvaluationGatePO) *domainevaluation.GateResult {
	if value == nil {
		return nil
	}
	result := &domainevaluation.GateResult{Passed: value.Passed, Metrics: metricsFromPO(value.Metrics)}
	for _, reason := range value.Reasons {
		result.Reasons = append(result.Reasons, domainevaluation.GateReason{Code: reason.Code, CaseID: reason.CaseID, Attempt: reason.Attempt, Detail: reason.Detail})
	}
	return result
}

func metricsToPO(value domainevaluation.QualityMetrics) EvaluationQualityMetricsPO {
	return EvaluationQualityMetricsPO{
		GenerationAttempts: value.GenerationAttempts, CaseAssertionPasses: value.CaseAssertionPasses,
		FaithfulnessAverage: value.FaithfulnessAverage, CrossDimensionAverage: value.CrossDimensionAverage,
		ActionabilityAverage: value.ActionabilityAverage, AudienceClarityAverage: value.AudienceClarityAverage,
		ConcisionAverage: value.ConcisionAverage, HumanReviews: value.HumanReviews,
	}
}

func metricsFromPO(value EvaluationQualityMetricsPO) domainevaluation.QualityMetrics {
	return domainevaluation.QualityMetrics{
		GenerationAttempts: value.GenerationAttempts, CaseAssertionPasses: value.CaseAssertionPasses,
		FaithfulnessAverage: value.FaithfulnessAverage, CrossDimensionAverage: value.CrossDimensionAverage,
		ActionabilityAverage: value.ActionabilityAverage, AudienceClarityAverage: value.AudienceClarityAverage,
		ConcisionAverage: value.ConcisionAverage, HumanReviews: value.HumanReviews,
	}
}

func evaluationUpdatedAt(value *domainevaluation.PromptEvaluationRun) time.Time {
	updated := value.CreatedAt()
	for _, attempt := range value.Attempts() {
		if attempt.FinishedAt.After(updated) {
			updated = attempt.FinishedAt
		}
	}
	for _, review := range value.Reviews() {
		if review.ReviewedAt.After(updated) {
			updated = review.ReviewedAt
		}
	}
	for _, recovery := range value.Recoveries() {
		if recovery.RequestedAt.After(updated) {
			updated = recovery.RequestedAt
		}
	}
	if execution := value.Execution(); execution != nil {
		if execution.ClaimedAt.After(updated) {
			updated = execution.ClaimedAt
		}
		if execution.DispatchStartedAt != nil && execution.DispatchStartedAt.After(updated) {
			updated = *execution.DispatchStartedAt
		}
	}
	if closed := value.ClosedAt(); closed != nil && closed.After(updated) {
		updated = *closed
	}
	if finalized := value.FinalizedAt(); finalized != nil && finalized.After(updated) {
		updated = *finalized
	}
	if canceled := value.CanceledAt(); canceled != nil && canceled.After(updated) {
		updated = *canceled
	}
	return updated
}

func evaluationExecutionToPO(value *domainevaluation.AttemptExecution) *EvaluationAttemptExecutionPO {
	if value == nil {
		return nil
	}
	return &EvaluationAttemptExecutionPO{
		CaseID: value.CaseID, Attempt: value.Attempt, Owner: value.Owner, InvocationID: value.InvocationID,
		Phase: string(value.Phase), ClaimedAt: value.ClaimedAt, LeaseExpiresAt: value.LeaseExpiresAt,
		DispatchStartedAt: copyTimePointer(value.DispatchStartedAt),
	}
}

func evaluationExecutionFromPO(value *EvaluationAttemptExecutionPO) *domainevaluation.AttemptExecution {
	if value == nil {
		return nil
	}
	return &domainevaluation.AttemptExecution{
		CaseID: value.CaseID, Attempt: value.Attempt, Owner: value.Owner, InvocationID: value.InvocationID,
		Phase: domainevaluation.AttemptExecutionPhase(value.Phase), ClaimedAt: value.ClaimedAt, LeaseExpiresAt: value.LeaseExpiresAt,
		DispatchStartedAt: copyTimePointer(value.DispatchStartedAt),
	}
}

func copyTimePointer(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneFloatPtr(value *float64) *float64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneInt64Ptr(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func associationToPO(value aiexplanation.Association) AssociationPO {
	return AssociationPO{OrgID: value.OrgID, AssessmentID: value.AssessmentID.Uint64(), TesteeID: value.TesteeID}
}

func associationFromPO(value AssociationPO) aiexplanation.Association {
	return aiexplanation.Association{OrgID: value.OrgID, AssessmentID: meta.FromUint64(value.AssessmentID), TesteeID: value.TesteeID}
}

func actorToPO(value aiexplanation.ActorRef) ActorRefPO {
	return ActorRefPO{Kind: value.Kind, ID: value.ID}
}
func actorFromPO(value ActorRefPO) aiexplanation.ActorRef {
	return aiexplanation.ActorRef{Kind: value.Kind, ID: value.ID}
}

func profileRefToPO(value aiexplanation.ProfileRef) ProfileRefPO {
	return ProfileRefPO{ID: value.ID, Version: value.Version, Fingerprint: value.Fingerprint.String()}
}

func profileRefFromPO(value ProfileRefPO) (aiexplanation.ProfileRef, error) {
	fingerprint, err := aiexplanation.ParseFingerprint(value.Fingerprint)
	return aiexplanation.ProfileRef{ID: value.ID, Version: value.Version, Fingerprint: fingerprint}, err
}

func promptToPO(value aiexplanation.PromptRef) PromptRefPO {
	return PromptRefPO{TemplateID: value.TemplateID, Version: value.Version, Fingerprint: value.Fingerprint.String(), GitBlobSHA: value.GitBlobSHA}
}

func promptFromPO(value PromptRefPO) (aiexplanation.PromptRef, error) {
	fingerprint, err := aiexplanation.ParseFingerprint(value.Fingerprint)
	return aiexplanation.PromptRef{TemplateID: value.TemplateID, Version: value.Version, Fingerprint: fingerprint, GitBlobSHA: value.GitBlobSHA}, err
}

func executionToPO(value aiexplanation.ProviderExecutionSpec) ExecutionSpecPO {
	return ExecutionSpecPO{Route: value.Route, RouteRevision: value.RouteRevision, ResolvedProvider: value.ResolvedProvider, ResolvedModel: value.ResolvedModel, Fingerprint: value.Fingerprint.String()}
}

func executionFromPO(value ExecutionSpecPO) (aiexplanation.ProviderExecutionSpec, error) {
	fingerprint, err := aiexplanation.ParseFingerprint(value.Fingerprint)
	return aiexplanation.ProviderExecutionSpec{Route: value.Route, RouteRevision: value.RouteRevision, ResolvedProvider: value.ResolvedProvider, ResolvedModel: value.ResolvedModel, Fingerprint: fingerprint}, err
}

func receiptToPO(value aiexplanation.ProviderReceipt) ProviderReceiptPO {
	return ProviderReceiptPO{InvocationID: value.InvocationID, RequestID: value.RequestID, Provider: value.Provider, Model: value.Model, InputTokens: value.InputTokens, OutputTokens: value.OutputTokens, LatencyNanos: value.Latency.Nanoseconds()}
}

func receiptFromPO(value ProviderReceiptPO) aiexplanation.ProviderReceipt {
	return aiexplanation.ProviderReceipt{InvocationID: value.InvocationID, RequestID: value.RequestID, Provider: value.Provider, Model: value.Model, InputTokens: value.InputTokens, OutputTokens: value.OutputTokens, Latency: time.Duration(value.LatencyNanos)}
}
