package aibridge

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	authz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	"github.com/google/uuid"
)

var ErrGovernanceDenied = errors.New("AI evaluation governance permission required")
var ErrManagementUnavailable = errors.New("AI evaluation management unavailable")

// EvaluationScope must originate from the authenticated QS request, never its JSON body.
type EvaluationScope struct {
	RunID                          string
	OrganizationID, OperatorUserID int64
}
type EvaluationStart struct {
	ExpectedVersion int64  `json:"expected_version"`
	Reason          string `json:"reason"`
	Confirm         bool   `json:"confirm"`
}
type UnknownResolution struct {
	ExpectedVersion                      int64  `json:"expected_version"`
	ExecutionID                          string `json:"execution_id"`
	Decision                             string `json:"decision"`
	Reason                               string `json:"reason"`
	Confirm                              bool   `json:"confirm"`
	AcknowledgedDuplicateCallAndCostRisk bool   `json:"acknowledged_duplicate_call_and_cost_risk"`
}
type EvaluationState struct {
	RunID                        string          `json:"run_id"`
	Version                      int64           `json:"version"`
	Status                       string          `json:"status"`
	UnresolvedResultUnknownCount int64           `json:"unresolved_result_unknown_count"`
	Resolutions                  json.RawMessage `json:"resolutions" swaggertype:"array,object"`
	Reviews                      json.RawMessage `json:"reviews" swaggertype:"array,object"`
}
type EvaluationGateway interface {
	PreviewEvaluationGates(context.Context, EvaluationScope, int64) (EvaluationGatePreview, error)
	ListEvaluationCandidates(context.Context, EvaluationScope) (EvaluationCandidateIndex, error)
	GetEvaluationCandidate(context.Context, EvaluationScope, CandidateQuery) (EvaluationCandidateEvidence, error)
	ReviewEvaluation(context.Context, EvaluationScope, EvaluationReview) (EvaluationState, error)
	CreateEvaluation(context.Context, EvaluationScope, EvaluationCreate) (EvaluationState, error)
	StartEvaluation(context.Context, EvaluationScope, EvaluationStart) (EvaluationState, error)
	GetEvaluation(context.Context, EvaluationScope) (EvaluationState, error)
	ResolveUnknown(context.Context, EvaluationScope, UnknownResolution) (EvaluationState, error)
}
type EvaluationAdministration struct{ Gateway EvaluationGateway }

func (s *EvaluationAdministration) authorize(ctx context.Context, scope EvaluationScope) error {
	return s.authorizeCapability(ctx, scope, authz.CapabilityOrgAdmin)
}
func (s *EvaluationAdministration) authorizeCapability(ctx context.Context, scope EvaluationScope, capability authz.Capability) error {
	id, err := uuid.Parse(scope.RunID)
	if err != nil || id == uuid.Nil || id.String() != scope.RunID || scope.OrganizationID <= 0 || scope.OperatorUserID <= 0 {
		return ErrInvalid
	}
	snapshot, ok := authz.FromContext(ctx)
	if !ok || !authz.DecideCapability(snapshot, capability).Allowed {
		return ErrGovernanceDenied
	}
	if s == nil || s.Gateway == nil {
		return ErrManagementUnavailable
	}
	return nil
}
func (s *EvaluationAdministration) Get(ctx context.Context, scope EvaluationScope) (EvaluationState, error) {
	if err := s.authorizeCapability(ctx, scope, authz.CapabilityAuditInterpretation); err != nil {
		return EvaluationState{}, err
	}
	return s.Gateway.GetEvaluation(ctx, scope)
}
func (s *EvaluationAdministration) Resolve(ctx context.Context, scope EvaluationScope, command UnknownResolution) (EvaluationState, error) {
	if err := s.authorize(ctx, scope); err != nil {
		return EvaluationState{}, err
	}
	command.Reason = strings.TrimSpace(command.Reason)
	if command.ExpectedVersion < 1 || command.ExecutionID == "" || len(command.ExecutionID) > 128 ||
		!command.Confirm || !command.AcknowledgedDuplicateCallAndCostRisk || command.Reason == "" || len(command.Reason) > 1000 ||
		(command.Decision != "authorize_replacement" && command.Decision != "cancel_run") {
		return EvaluationState{}, ErrInvalid
	}
	return s.Gateway.ResolveUnknown(ctx, scope, command)
}

func (s *EvaluationAdministration) Start(ctx context.Context, scope EvaluationScope, command EvaluationStart) (EvaluationState, error) {
	if err := s.authorize(ctx, scope); err != nil {
		return EvaluationState{}, err
	}
	command.Reason = strings.TrimSpace(command.Reason)
	if command.ExpectedVersion < 1 || !command.Confirm || command.Reason == "" || len(command.Reason) > 1000 || strings.ContainsAny(command.Reason, "<>") {
		return EvaluationState{}, ErrInvalid
	}
	return s.Gateway.StartEvaluation(ctx, scope, command)
}
