package aibridge

import (
	"context"
	"strings"
)

type EvaluationFinalize struct {
	ExpectedVersion int64  `json:"expected_version"`
	ExpectedPassed  *bool  `json:"expected_passed"`
	Reason          string `json:"reason"`
	Confirm         bool   `json:"confirm"`
}

func (s *EvaluationAdministration) Finalize(ctx context.Context, scope EvaluationScope, command EvaluationFinalize) (EvaluationState, error) {
	if err := s.authorize(ctx, scope); err != nil {
		return EvaluationState{}, err
	}
	command.Reason = strings.TrimSpace(command.Reason)
	if command.ExpectedVersion < 1 || command.ExpectedPassed == nil || !command.Confirm || command.Reason == "" || len(command.Reason) > 1000 || strings.ContainsAny(command.Reason, "<>") {
		return EvaluationState{}, ErrInvalid
	}
	return s.Gateway.FinalizeEvaluation(ctx, scope, command)
}
