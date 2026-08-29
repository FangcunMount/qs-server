package evaluation

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	domainevaluation "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/evaluation"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

type PrepareRecheckCommand struct {
	RecheckID   meta.ID
	SourceRunID meta.ID
	CaseID      string
	Attempt     int
	OrgID       int64
	RequestedBy string
	Reason      string
}

type RunRecheckCommand struct {
	RecheckID    meta.ID
	SourceRunID  meta.ID
	CaseID       string
	Attempt      int
	Owner        string
	RequestedOrg int64
	RequestedBy  string
}

type OnlineRecheckStatus string

const (
	OnlineRecheckCompleted        OnlineRecheckStatus = "recheck_completed"
	OnlineRecheckFailed           OnlineRecheckStatus = "recheck_failed"
	OnlineRecheckResultUnknown    OnlineRecheckStatus = "recheck_result_unknown"
	OnlineRecheckAlreadyCompleted OnlineRecheckStatus = "recheck_already_completed"
)

type OnlineRecheckResult struct {
	Status  OnlineRecheckStatus
	Recheck *domainevaluation.PromptEvaluationRecheck
}

// PrepareRequestedRecheckV1 validates that the immutable source evidence
// exists, then freezes the currently executable candidate release. A recheck
// therefore compares one source input with current code/config; it is not a
// replay of an obsolete Provider wire protocol.
func (r *OnlineRunner) PrepareRequestedRecheckV1(ctx context.Context, command PrepareRecheckCommand) (*domainevaluation.PromptEvaluationRecheck, error) {
	command.CaseID = strings.TrimSpace(command.CaseID)
	command.RequestedBy = strings.TrimSpace(command.RequestedBy)
	command.Reason = strings.TrimSpace(command.Reason)
	if r == nil || r.rechecks == nil || command.RecheckID.IsZero() || command.SourceRunID.IsZero() ||
		command.CaseID == "" || command.Attempt < 1 || command.OrgID <= 0 || command.RequestedBy == "" || command.Reason == "" {
		return nil, fmt.Errorf("AI explanation attempt recheck request is invalid")
	}
	source, err := r.evidence.Find(ctx, command.SourceRunID)
	if err != nil {
		return nil, err
	}
	if source.RequestedOrgID() != command.OrgID {
		return nil, fmt.Errorf("AI explanation attempt recheck source organization does not match")
	}
	found := false
	for _, attempt := range source.Attempts() {
		if attempt.Stage == domainevaluation.AttemptStageGeneration && attempt.CaseID == command.CaseID && attempt.Attempt == command.Attempt {
			found = true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("AI explanation attempt recheck source evidence does not exist")
	}
	_, _, prepared, err := r.prepareV1(ctx)
	if err != nil {
		return nil, err
	}
	if _, exists := prepared.caseByID(command.CaseID); !exists {
		return nil, fmt.Errorf("AI explanation attempt recheck case is absent from the current candidate suite")
	}
	return domainevaluation.NewPromptEvaluationRecheck(
		command.RecheckID, command.SourceRunID, command.CaseID, command.Attempt, prepared.release,
		command.OrgID, command.RequestedBy, command.Reason, r.now().UTC(),
	)
}

func (r *OnlineRunner) RunRecheckV1(ctx context.Context, command RunRecheckCommand) (*OnlineRecheckResult, error) {
	command.CaseID = strings.TrimSpace(command.CaseID)
	command.Owner = strings.TrimSpace(command.Owner)
	command.RequestedBy = strings.TrimSpace(command.RequestedBy)
	if r == nil || r.rechecks == nil || command.RecheckID.IsZero() || command.SourceRunID.IsZero() ||
		command.CaseID == "" || command.Attempt < 1 || command.Owner == "" || command.RequestedOrg <= 0 || command.RequestedBy == "" {
		return nil, fmt.Errorf("AI explanation attempt recheck execution address is invalid")
	}
	value, err := r.rechecks.FindRecheckByID(ctx, command.RecheckID)
	if err != nil {
		return nil, err
	}
	if value.SourceRunID() != command.SourceRunID || value.SourceCaseID() != command.CaseID || value.SourceAttempt() != command.Attempt ||
		value.RequestedOrgID() != command.RequestedOrg || value.RequestedBy() != command.RequestedBy {
		return nil, fmt.Errorf("AI explanation attempt recheck durable event does not match its aggregate")
	}
	if value.Status().IsTerminal() {
		return &OnlineRecheckResult{Status: OnlineRecheckAlreadyCompleted, Recheck: value}, nil
	}
	_, _, prepared, err := r.prepareV1(ctx)
	if err != nil {
		return nil, err
	}
	if !reflect.DeepEqual(value.Release(), prepared.release) {
		return nil, fmt.Errorf("AI explanation attempt recheck candidate release no longer matches executable assets")
	}
	testCase, exists := prepared.caseByID(command.CaseID)
	if !exists {
		return nil, fmt.Errorf("AI explanation attempt recheck case is not executable")
	}
	now := r.now().UTC()
	if execution := value.Execution(); execution != nil {
		if !execution.LeaseExpired(now) {
			return nil, ErrAttemptExecutionBusy
		}
		startedAt := execution.ClaimedAt
		if execution.DispatchStartedAt != nil {
			startedAt = *execution.DispatchStartedAt
		}
		record := r.failedAttempt(domainevaluation.AttemptRecord{
			CaseID: command.CaseID, Attempt: command.Attempt, Stage: domainevaluation.AttemptStageGeneration,
			StartedAt: startedAt, ProviderCallCount: 1,
		}, "provider_execution", "provider_result_unknown", "Provider result is unknown after an expired recheck dispatch lease", false, true)
		if record.FinishedAt.Before(now) {
			record.FinishedAt = now
		}
		expectedVersion := value.Version()
		if err := value.Complete(execution.Owner, record); err != nil {
			return nil, err
		}
		if err := r.rechecks.SaveRecheck(ctx, value, expectedVersion); err != nil {
			return nil, err
		}
		observePromptEvaluationAttemptFailure(record.Failure)
		return &OnlineRecheckResult{Status: OnlineRecheckResultUnknown, Recheck: value}, nil
	}

	expectedVersion := value.Version()
	leaseDuration := r.attemptLease
	minimumLease := prepared.route.Timeout + r.semanticTimeout + 30*time.Second
	if leaseDuration < minimumLease {
		leaseDuration = minimumLease
	}
	if err := value.BeginDispatch(command.Owner, now, leaseDuration); err != nil {
		return nil, err
	}
	if err := r.rechecks.SaveRecheck(ctx, value, expectedVersion); err != nil {
		return nil, err
	}
	record := r.executeAttempt(ctx, prepared, testCase, command.Attempt, "recheck:"+command.RecheckID.String())
	persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	expectedVersion = value.Version()
	if err := value.Complete(command.Owner, record); err != nil {
		return nil, err
	}
	if err := r.rechecks.SaveRecheck(persistCtx, value, expectedVersion); err != nil {
		return nil, err
	}
	observePromptEvaluationAttemptFailure(record.Failure)
	status := OnlineRecheckCompleted
	if record.Failure != nil {
		status = OnlineRecheckFailed
		if record.Failure.ResultUnknown {
			status = OnlineRecheckResultUnknown
		}
	}
	return &OnlineRecheckResult{Status: status, Recheck: value}, nil
}
