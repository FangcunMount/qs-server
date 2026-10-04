package aibridge

import (
	"context"
	"errors"
	"testing"
)

type submissionCommands struct {
	calls  int
	scope  EvaluationScope
	cancel EvaluationCancel
	err    error
}

func (s *submissionCommands) SubmitEvaluationStart(_ context.Context, scope EvaluationScope, _ string, _ EvaluationStart) error {
	s.calls++
	s.scope = scope
	return s.err
}
func (s *submissionCommands) SubmitEvaluationCancel(_ context.Context, scope EvaluationScope, _ string, command EvaluationCancel) error {
	s.calls++
	s.scope, s.cancel = scope, command
	return s.err
}
func (s *submissionCommands) SubmitParticipantRetry(context.Context, DraftScope, string, ParticipantRetry) error {
	s.calls++
	return s.err
}

func TestMQManagementSubmissionNeverRetriesUncertainCommit(t *testing.T) {
	const id = "00000000-0000-4000-8000-000000000005"
	for _, family := range []string{"start", "cancel", "retry"} {
		t.Run(family, func(t *testing.T) {
			gateway := &managementStub{}
			messages := &submissionCommands{err: context.DeadlineExceeded}
			evaluation := &EvaluationAdministration{Gateway: gateway, Messages: messages}
			participant := &ParticipantAdministration{Gateway: &submissionParticipantQueries{}, Messages: messages}
			discard := false
			var err error
			switch family {
			case "start":
				err = evaluation.SubmitStart(adminContext(), managementScope(), EvaluationStart{CommandID: id, ExpectedVersion: 1, Reason: "已核对", Confirm: true})
			case "cancel":
				err = evaluation.SubmitCancel(adminContext(), managementScope(), EvaluationCancel{CommandID: id, ExpectedVersion: 1, Reason: "已核对", Confirm: true, Discard: &discard})
			case "retry":
				err = participant.SubmitRetry(adminContext(), DraftScope{OrganizationID: 7, OperatorUserID: 42}, managementScope().RunID, ParticipantRetry{CommandID: id, ExpectedRunID: "00000000-0000-4000-8000-000000000003", ExpectedVersion: 1, Reason: "已核对", Confirm: true, ExpectedProviderInvocations: 1})
			}
			if !errors.Is(err, context.DeadlineExceeded) || messages.calls != 1 || gateway.calls != 0 {
				t.Fatalf("uncertain commit must remain uncertain, without retry: err=%v commits=%d rpc=%d", err, messages.calls, gateway.calls)
			}
		})
	}
}

type submissionParticipantQueries struct{ ParticipantManagementGateway }
