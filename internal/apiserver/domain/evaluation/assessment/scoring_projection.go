package assessment

// ScoringProjection is the minimal Evaluation fact needed to finalize an
// Assessment. Detailed dimensions, profiles, validity and report prose remain
// owned by the canonical Outcome and must not enter the Assessment aggregate.
type ScoringProjection struct {
	ModelRef EvaluationModelRef
	Summary  ResultSummary
	Score    *float64
	Level    string
}
