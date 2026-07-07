package patterns

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/typology/trait"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/personality/typology"
)

// ScoringResult is the domain-local output of a personality model adapter.
type ScoringResult struct {
	Runtime         *modeltypology.RuntimeSpec
	Vector          trait.ProfileVector
	Candidate       trait.OutcomeCandidate
	SelectedOutcome SelectedOutcome
	SpecialMatch    *ScoringSpecialMatch
	Detail          any
}

// SelectedOutcome captures the chosen model outcome before detail assembly.
type SelectedOutcome struct {
	Code       string
	Similarity float64
	Trigger    string
}

// ScoringSpecialMatch records a special rule that altered scoring.
type ScoringSpecialMatch struct {
	OutcomeCode string
	Trigger     string
	SkipScoring bool
}

// LegacyDetail returns the typed detail payload for backward-compatible callers.
func (r ScoringResult) LegacyDetail() any {
	return r.Detail
}
