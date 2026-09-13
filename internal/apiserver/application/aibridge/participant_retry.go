package aibridge

import (
	"context"
	"math"
	"strings"

	authz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
)

type ParticipantExecution struct {
	OrganizationID           int64    `json:"organization_id"`
	SessionID                string   `json:"session_id"`
	RequestID                string   `json:"request_id"`
	RunID                    string   `json:"run_id"`
	Version                  int64    `json:"version"`
	Status                   string   `json:"status"`
	SubjectID                string   `json:"subject_id"`
	TesteeID                 string   `json:"testee_id"`
	AssessmentIDs            []string `json:"assessment_ids"`
	FailureCode              string   `json:"failure_code"`
	ModelCallStatus          string   `json:"model_call_status"`
	InvocationID             string   `json:"invocation_id"`
	SourceRunID              string   `json:"source_run_id"`
	CanRetry                 bool     `json:"can_retry"`
	UnknownResultRisk        bool     `json:"unknown_result_risk"`
	RetryProviderInvocations int64    `json:"retry_provider_invocations"`
}
type ParticipantRetry struct {
	CommandID                   string `json:"command_id"`
	ExpectedRunID               string `json:"expected_run_id"`
	ExpectedVersion             int64  `json:"expected_version"`
	Reason                      string `json:"reason"`
	Confirm                     bool   `json:"confirm"`
	ExpectedProviderInvocations int64  `json:"expected_provider_invocations"`
	AcceptResultUnknownRisk     bool   `json:"accept_result_unknown_risk"`
}

func (s *ParticipantAdministration) authorize(ctx context.Context, scope DraftScope) error {
	if scope.OrganizationID <= 0 || scope.OperatorUserID <= 0 {
		return ErrInvalid
	}
	snapshot, ok := authz.FromContext(ctx)
	if !ok || !authz.DecideCapability(snapshot, authz.CapabilityOrgAdmin).Allowed {
		return ErrGovernanceDenied
	}
	if s == nil || s.Gateway == nil {
		return ErrManagementUnavailable
	}
	return nil
}
func (s *ParticipantAdministration) Get(ctx context.Context, scope DraftScope, sessionID string) (ParticipantExecution, error) {
	if err := s.authorize(ctx, scope); err != nil {
		return ParticipantExecution{}, err
	}
	if !validID(sessionID) || sessionID == "00000000-0000-0000-0000-000000000000" {
		return ParticipantExecution{}, ErrInvalid
	}
	return s.Gateway.GetParticipantExecution(ctx, scope, sessionID)
}
func (s *ParticipantAdministration) Retry(ctx context.Context, scope DraftScope, sessionID string, command ParticipantRetry) (Receipt, error) {
	if err := s.authorize(ctx, scope); err != nil {
		return Receipt{}, err
	}
	for _, id := range []string{sessionID, command.CommandID, command.ExpectedRunID} {
		if !validID(id) || id == "00000000-0000-0000-0000-000000000000" {
			return Receipt{}, ErrInvalid
		}
	}
	command.Reason = strings.TrimSpace(command.Reason)
	if command.ExpectedVersion < 1 || command.ExpectedVersion == math.MaxInt64 || !command.Confirm || command.ExpectedProviderInvocations != 1 || command.Reason == "" || len(command.Reason) > 1000 || strings.ContainsAny(command.Reason, "<>\x00") {
		return Receipt{}, ErrInvalid
	}
	return s.Gateway.RetryParticipant(ctx, scope, sessionID, command)
}
func (s *ParticipantAdministration) RetryReceipt(ctx context.Context, scope DraftScope, commandID string) (Receipt, error) {
	if err := s.authorize(ctx, scope); err != nil {
		return Receipt{}, err
	}
	if !validID(commandID) || commandID == "00000000-0000-0000-0000-000000000000" {
		return Receipt{}, ErrInvalid
	}
	return s.Gateway.GetParticipantRetryReceipt(ctx, scope, commandID)
}
