package outcome

// DecisionResult is the family-neutral Decision contract projected from an Execution.
// Not every assessment populates OutcomeCode; typology may use Profile instead.
type DecisionResult struct {
	OutcomeCode  string
	LevelCode    string
	LevelLabel   string
	PrimaryScore *ScoreValue
	Profile      *ProfileResult
	Dimensions   []DimensionResult
	Validity     []ValidityResult
}

// DecisionResultFromExecution maps the mutable Execution into the DecisionResult contract.
func DecisionResultFromExecution(execution *Execution) DecisionResult {
	if execution == nil {
		return DecisionResult{}
	}
	result := DecisionResult{
		PrimaryScore: cloneScoreValue(execution.Primary),
		Profile:      cloneProfile(execution.Profile),
		Dimensions:   append([]DimensionResult(nil), execution.Dimensions...),
		Validity:     append([]ValidityResult(nil), execution.Validity...),
	}
	if execution.Level != nil {
		result.OutcomeCode = execution.Level.Code
		result.LevelCode = execution.Level.Code
		result.LevelLabel = execution.Level.Label
	}
	if result.OutcomeCode == "" && execution.Profile != nil {
		result.OutcomeCode = execution.Profile.Code
	}
	if result.LevelLabel == "" && execution.Summary.PrimaryLabel != "" {
		result.LevelLabel = execution.Summary.PrimaryLabel
	}
	if result.LevelCode == "" && execution.Summary.Level != nil {
		result.LevelCode = *execution.Summary.Level
		if result.OutcomeCode == "" {
			result.OutcomeCode = result.LevelCode
		}
	}
	return result
}

func cloneScoreValue(value *ScoreValue) *ScoreValue {
	if value == nil {
		return nil
	}
	cloned := *value
	if value.Max != nil {
		max := *value.Max
		cloned.Max = &max
	}
	return &cloned
}

func cloneProfile(profile *ProfileResult) *ProfileResult {
	if profile == nil {
		return nil
	}
	cloned := *profile
	cloned.Traits = append([]string(nil), profile.Traits...)
	return &cloned
}
