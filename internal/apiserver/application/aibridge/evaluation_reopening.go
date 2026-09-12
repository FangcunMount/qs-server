package aibridge

import (
	"context"
	"strings"
)

type EvaluationReopen struct {
	ExpectedVersion int64  `json:"expected_version"`
	Reason          string `json:"reason"`
	Confirm         bool   `json:"confirm"`
}

func (s *EvaluationAdministration) ReopenReview(ctx context.Context, scope EvaluationScope, command EvaluationReopen) (EvaluationState, error) {
	if err := s.authorize(ctx, scope); err != nil {
		return EvaluationState{}, err
	}
	command.Reason = strings.TrimSpace(command.Reason)
	if command.ExpectedVersion < 1 || !command.Confirm || command.Reason == "" || len(command.Reason) > 1000 || strings.ContainsAny(command.Reason, "<>") {
		return EvaluationState{}, ErrInvalid
	}
	return s.Gateway.ReopenEvaluationReview(ctx, scope, command)
}
