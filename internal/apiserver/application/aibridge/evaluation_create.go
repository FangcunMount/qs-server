package aibridge

import (
	"context"
	"regexp"
	"strings"
)

type FrozenEvaluationRef struct {
	ID          string `json:"id"`
	Version     string `json:"version"`
	Fingerprint string `json:"fingerprint"`
}

// EvaluationRelease carries immutable references; qs-ai resolves and verifies each asset.
type EvaluationRelease struct {
	Suite                FrozenEvaluationRef `json:"suite"`
	Prompt               FrozenEvaluationRef `json:"prompt"`
	Profile              FrozenEvaluationRef `json:"profile"`
	InputSchema          FrozenEvaluationRef `json:"input_schema"`
	OutputSchema         FrozenEvaluationRef `json:"output_schema"`
	GenerationRoute      FrozenEvaluationRef `json:"generation_route"`
	SemanticPrompt       FrozenEvaluationRef `json:"semantic_prompt"`
	SemanticOutputSchema FrozenEvaluationRef `json:"semantic_output_schema"`
	SemanticRoute        FrozenEvaluationRef `json:"semantic_route"`
	ExecutionPolicy      FrozenEvaluationRef `json:"execution_policy"`
	GatePolicy           FrozenEvaluationRef `json:"gate_policy"`
}
type EvaluationCreate struct {
	Release EvaluationRelease `json:"release"`
	Reason  string            `json:"reason"`
	Confirm bool              `json:"confirm"`
}

var frozenID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9:._/-]{0,127}$`)
var frozenVersion = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]{0,127}$`)
var frozenFingerprint = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

func (r EvaluationRelease) valid() bool {
	for _, ref := range []FrozenEvaluationRef{
		r.Suite,
		r.Prompt,
		r.Profile,
		r.InputSchema,
		r.OutputSchema,
		r.GenerationRoute,
		r.SemanticPrompt,
		r.SemanticOutputSchema,
		r.SemanticRoute,
		r.ExecutionPolicy,
		r.GatePolicy,
	} {
		if !frozenID.MatchString(ref.ID) || !frozenVersion.MatchString(ref.Version) || !frozenFingerprint.MatchString(ref.Fingerprint) {
			return false
		}
	}
	return true
}

func (s *EvaluationAdministration) Create(ctx context.Context, scope EvaluationScope, command EvaluationCreate) (EvaluationState, error) {
	if err := s.authorize(ctx, scope); err != nil {
		return EvaluationState{}, err
	}
	command.Reason = strings.TrimSpace(command.Reason)
	if !command.Confirm || command.Reason == "" || len(command.Reason) > 1000 || strings.ContainsAny(command.Reason, "<>") || !command.Release.valid() {
		return EvaluationState{}, ErrInvalid
	}
	return s.Gateway.CreateEvaluation(ctx, scope, command)
}
