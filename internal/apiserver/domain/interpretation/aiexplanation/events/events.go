// Package events translates AI explanation aggregate state into stable,
// privacy-minimal integration events.
package events

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/FangcunMount/component-base/pkg/event"
	domainartifact "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/artifact"
	domaingeneration "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/generation"
	domainrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/run"
	eventcatalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	eventpayload "github.com/FangcunMount/qs-server/internal/pkg/eventing/payload"
)

const AggregateType = "AIExplanationGeneration"

type RequestedEvent = event.Event[eventpayload.AIExplanationRequestedData]
type RetryRequestedEvent = event.Event[eventpayload.AIExplanationRetryRequestedData]
type LeaseRecoveryRequestedEvent = event.Event[eventpayload.AIExplanationLeaseRecoveryRequestedData]
type GeneratedEvent = event.Event[eventpayload.AIExplanationGeneratedData]
type FailedEvent = event.Event[eventpayload.AIExplanationFailedData]

// Factory implements the application persistence EventFactory port.
type Factory struct{}

func RetryEventID(generationID string, expectedAttempt int, requestID string) string {
	return fmt.Sprintf("ai-explanation-retry:%s:%d:manual:%s", generationID, expectedAttempt, requestID)
}

func LeaseRecoveryEventID(generationID, runID string, leaseExpiresAt time.Time, phase domainrun.InvocationPhase) string {
	digest := sha256.Sum256([]byte(generationID + "\x00" + runID + "\x00" + leaseExpiresAt.UTC().Format(time.RFC3339Nano) + "\x00" + string(phase)))
	return "ai-explanation-lease-recovery:" + generationID + ":" + runID + ":" + hex.EncodeToString(digest[:16])
}

func (Factory) Requested(generation *domaingeneration.AIExplanationGeneration) (event.DomainEvent, error) {
	if generation == nil || generation.Status() != domaingeneration.StatusPending {
		return nil, fmt.Errorf("pending AI explanation Generation is required for requested event")
	}
	association := generation.Association()
	return event.New(eventcatalog.AIExplanationRequested, AggregateType, generation.ID().String(), eventpayload.AIExplanationRequestedData{
		OrgID: association.OrgID, GenerationID: generation.ID().String(), AssessmentID: association.AssessmentID.String(),
		TesteeID: association.TesteeID, SourceReportID: generation.Key().SourceReportID.String(), Audience: string(generation.Key().Audience),
		RequestedAt: generation.CreatedAt(),
	}), nil
}

func (Factory) RetryRequested(generation *domaingeneration.AIExplanationGeneration, run *domainrun.AIExplanationRun, authorization domainrun.RetryAuthorization) (event.DomainEvent, error) {
	if generation == nil || run == nil || generation.Status() != domaingeneration.StatusFailed || run.Status() != domainrun.StatusFailed ||
		generation.LatestRunID() != run.ID() || run.GenerationID() != generation.ID() || authorization.ExpectedAttempt != run.Attempt() {
		return nil, fmt.Errorf("failed AI explanation Generation and authorized Run are required for retry event")
	}
	if err := authorization.Validate(); err != nil {
		return nil, err
	}
	wantEventID := RetryEventID(generation.ID().String(), authorization.ExpectedAttempt, authorization.RequestID)
	if authorization.EventID != wantEventID {
		return nil, fmt.Errorf("AI explanation retry event identity mismatch")
	}
	association := generation.Association()
	return RetryRequestedEvent{BaseEvent: event.BaseEvent{
		ID: authorization.EventID, EventTypeValue: eventcatalog.AIExplanationRetryRequested,
		OccurredAtValue: authorization.AuthorizedAt, AggregateTypeValue: AggregateType, AggregateIDValue: generation.ID().String(),
	}, Data: eventpayload.AIExplanationRetryRequestedData{
		OrgID: association.OrgID, GenerationID: generation.ID().String(), FailedRunID: run.ID().String(),
		AssessmentID: association.AssessmentID.String(), TesteeID: association.TesteeID,
		SourceReportID: generation.Key().SourceReportID.String(), Audience: string(generation.Key().Audience),
		ExpectedAttempt: authorization.ExpectedAttempt, NextAttempt: authorization.NextAttempt,
		AttemptOrigin: string(authorization.Origin), ActionRequestID: authorization.RequestID,
		RequestedAt: authorization.AuthorizedAt,
	}}, nil
}

func (Factory) LeaseRecoveryRequested(generation *domaingeneration.AIExplanationGeneration, run *domainrun.AIExplanationRun, wakeup domainrun.RecoveryWakeup) (event.DomainEvent, error) {
	if generation == nil || run == nil || generation.Status() != domaingeneration.StatusGenerating || run.Status() != domainrun.StatusRunning ||
		generation.LatestRunID() != run.ID() || run.GenerationID() != generation.ID() {
		return nil, fmt.Errorf("generating AI explanation and running Run are required for lease recovery event")
	}
	if err := wakeup.Validate(); err != nil {
		return nil, err
	}
	leaseExpiresAt := run.LeaseExpiresAt()
	if leaseExpiresAt == nil || !leaseExpiresAt.Equal(wakeup.ExpectedLeaseExpiresAt) || run.InvocationPhase() != wakeup.InvocationPhase {
		return nil, domainrun.ErrRecoveryNotAllowed
	}
	wantEventID := LeaseRecoveryEventID(generation.ID().String(), run.ID().String(), wakeup.ExpectedLeaseExpiresAt, wakeup.InvocationPhase)
	if wakeup.EventID != wantEventID {
		return nil, fmt.Errorf("AI explanation lease recovery event identity mismatch")
	}
	association := generation.Association()
	return LeaseRecoveryRequestedEvent{BaseEvent: event.BaseEvent{
		ID: wakeup.EventID, EventTypeValue: eventcatalog.AIExplanationLeaseRecoveryRequested,
		OccurredAtValue: wakeup.RequestedAt, AggregateTypeValue: AggregateType, AggregateIDValue: generation.ID().String(),
	}, Data: eventpayload.AIExplanationLeaseRecoveryRequestedData{
		OrgID: association.OrgID, GenerationID: generation.ID().String(), RunID: run.ID().String(), Attempt: run.Attempt(),
		ExpectedLeaseExpiresAt: wakeup.ExpectedLeaseExpiresAt, InvocationPhase: string(wakeup.InvocationPhase), RequestedAt: wakeup.RequestedAt,
	}}, nil
}

func (Factory) Generated(generation *domaingeneration.AIExplanationGeneration, run *domainrun.AIExplanationRun, artifact *domainartifact.AIExplanationArtifact) (event.DomainEvent, error) {
	if generation == nil || run == nil || artifact == nil || generation.Status() != domaingeneration.StatusGenerated || run.Status() != domainrun.StatusSucceeded {
		return nil, fmt.Errorf("generated AI explanation state is required for generated event")
	}
	if generation.LatestRunID() != run.ID() || generation.ArtifactID() != artifact.ID() || run.GenerationID() != generation.ID() || artifact.GenerationID() != generation.ID() || artifact.RunID() != run.ID() {
		return nil, fmt.Errorf("AI explanation generated event references do not match")
	}
	association := generation.Association()
	return event.New(eventcatalog.AIExplanationGenerated, AggregateType, generation.ID().String(), eventpayload.AIExplanationGeneratedData{
		OrgID: association.OrgID, GenerationID: generation.ID().String(), RunID: run.ID().String(), ArtifactID: artifact.ID().String(),
		AssessmentID: association.AssessmentID.String(), TesteeID: association.TesteeID, SourceReportID: generation.Key().SourceReportID.String(),
		Audience: string(generation.Key().Audience), GeneratedAt: artifact.GeneratedAt(),
	}), nil
}

func (Factory) Failed(generation *domaingeneration.AIExplanationGeneration, run *domainrun.AIExplanationRun) (event.DomainEvent, error) {
	if generation == nil || run == nil || generation.Status() != domaingeneration.StatusFailed || run.Status() != domainrun.StatusFailed || generation.LatestRunID() != run.ID() || run.GenerationID() != generation.ID() {
		return nil, fmt.Errorf("failed AI explanation state is required for failed event")
	}
	failure := run.Failure()
	finishedAt := run.FinishedAt()
	if failure == nil || finishedAt == nil {
		return nil, fmt.Errorf("AI explanation failed event requires failure and timestamp")
	}
	association := generation.Association()
	return event.New(eventcatalog.AIExplanationFailed, AggregateType, generation.ID().String(), eventpayload.AIExplanationFailedData{
		OrgID: association.OrgID, GenerationID: generation.ID().String(), RunID: run.ID().String(), AssessmentID: association.AssessmentID.String(),
		TesteeID: association.TesteeID, SourceReportID: generation.Key().SourceReportID.String(), Audience: string(generation.Key().Audience),
		Attempt: run.Attempt(), FailureKind: string(failure.Kind), FailureCode: failure.Code, Retryable: failure.Retryable,
		SafeReason: failure.SafeMessage, FailedAt: *finishedAt,
	}), nil
}
