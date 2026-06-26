package assessment

// DimensionKind classifies a dimension result independent of scale factor semantics.
type DimensionKind string

const (
	DimensionKindFactor  DimensionKind = "factor"
	DimensionKindPole    DimensionKind = "pole"
	DimensionKindTrait   DimensionKind = "trait"
	DimensionKindIndex   DimensionKind = "index"
	DimensionKindAbility DimensionKind = "ability"
)

// OutcomeScoreKind classifies a primary or dimension score value.
type OutcomeScoreKind string

const (
	OutcomeScoreKindRawTotal     OutcomeScoreKind = "raw_total"
	OutcomeScoreKindMatchPercent OutcomeScoreKind = "match_percent"
	OutcomeScoreKindTypeCode     OutcomeScoreKind = "type_code"
)

// OutcomeScoreValue is the canonical score representation on an assessment outcome.
type OutcomeScoreValue struct {
	Kind  OutcomeScoreKind
	Value float64
	Label string
	Max   *float64
}

// OutcomeResultLevel is the canonical level representation on an assessment outcome.
type OutcomeResultLevel struct {
	Code     string
	Label    string
	Severity string
}

// DimensionResult records one scored dimension on an assessment outcome.
type DimensionResult struct {
	Code        string
	Name        string
	Kind        DimensionKind
	Score       *OutcomeScoreValue
	Level       *OutcomeResultLevel
	Description string
	Suggestion  string
}

// ValidityResult records optional validity checks for an assessment outcome.
type ValidityResult struct {
	Code    string
	Label   string
	Passed  bool
	Message string
}

// AssessmentOutcome is the canonical execution result for all model families.
type AssessmentOutcome struct {
	ModelRef   EvaluationModelRef
	Summary    ResultSummary
	Detail     EvaluationDetail
	Primary    *OutcomeScoreValue
	Level      *OutcomeResultLevel
	Dimensions []DimensionResult
	Validity   []ValidityResult
}

// NewAssessmentOutcome creates a canonical assessment outcome.
func NewAssessmentOutcome(
	modelRef EvaluationModelRef,
	summary ResultSummary,
	detail EvaluationDetail,
) *AssessmentOutcome {
	if detail.Kind == "" {
		detail.Kind = modelRef.Kind()
	}
	return &AssessmentOutcome{
		ModelRef:   modelRef,
		Summary:    summary,
		Detail:     detail,
		Dimensions: make([]DimensionResult, 0),
		Validity:   make([]ValidityResult, 0),
	}
}

// AssessmentOutcomeFromEvaluationResult adapts a legacy evaluation result.
//
// Deprecated: characterization and ApplyEvaluation adapter only; application write paths must use AssessmentOutcome directly.
func AssessmentOutcomeFromEvaluationResult(result *EvaluationResult) *AssessmentOutcome {
	if result == nil {
		return nil
	}
	outcome := NewAssessmentOutcome(result.ModelRef, result.Summary, result.Detail)
	if result.Summary.Score != nil {
		outcome.Primary = &OutcomeScoreValue{
			Kind:  OutcomeScoreKindRawTotal,
			Value: *result.Summary.Score,
		}
	} else if result.TotalScore != 0 {
		outcome.Primary = &OutcomeScoreValue{
			Kind:  OutcomeScoreKindRawTotal,
			Value: result.TotalScore,
		}
	}
	if result.Summary.Level != nil && *result.Summary.Level != "" {
		outcome.Level = &OutcomeResultLevel{Code: *result.Summary.Level}
	} else if result.RiskLevel != "" {
		outcome.Level = &OutcomeResultLevel{Code: string(result.RiskLevel)}
	}
	if result.Summary.PrimaryLabel != "" && outcome.Level != nil && outcome.Level.Label == "" {
		outcome.Level.Label = result.Summary.PrimaryLabel
	}
	outcome.Dimensions = dimensionResultsFromFactorScores(result.FactorScores)
	return outcome
}

// ToEvaluationResult projects the outcome into the legacy write model.
func (o *AssessmentOutcome) ToEvaluationResult() *EvaluationResult {
	if o == nil {
		return nil
	}
	result := NewModelEvaluationResult(o.ModelRef, o.Summary, o.Detail)
	if o.Primary != nil {
		result.TotalScore = o.Primary.Value
		if result.Summary.Score == nil {
			score := o.Primary.Value
			result.Summary.Score = &score
		}
	}
	if o.Level != nil && o.Level.Code != "" {
		result.RiskLevel = RiskLevel(o.Level.Code)
		level := o.Level.Code
		if result.Summary.Level == nil {
			result.Summary.Level = &level
		}
		if result.Summary.PrimaryLabel == "" && o.Level.Label != "" {
			result.Summary.PrimaryLabel = o.Level.Label
		}
	}
	if scores, ok := o.Detail.Payload.([]FactorScoreResult); ok && len(scores) > 0 {
		result.FactorScores = scores
	} else if len(o.Dimensions) > 0 {
		result.FactorScores = factorScoreResultsFromDimensions(o.Dimensions)
	}
	if result.Conclusion == "" && o.Summary.PrimaryLabel != "" {
		result.Conclusion = o.Summary.PrimaryLabel
	}
	return result
}

func dimensionResultsFromFactorScores(scores []FactorScoreResult) []DimensionResult {
	results := make([]DimensionResult, 0, len(scores))
	for _, score := range scores {
		dim := DimensionResult{
			Code: score.FactorCode.String(),
			Name: score.FactorName,
			Kind: DimensionKindFactor,
			Score: &OutcomeScoreValue{
				Kind:  OutcomeScoreKindRawTotal,
				Value: score.RawScore,
			},
			Description: score.Conclusion,
			Suggestion:  score.Suggestion,
		}
		if score.RiskLevel != "" {
			dim.Level = &OutcomeResultLevel{Code: string(score.RiskLevel)}
		}
		results = append(results, dim)
	}
	return results
}

func factorScoreResultsFromDimensions(dimensions []DimensionResult) []FactorScoreResult {
	results := make([]FactorScoreResult, 0, len(dimensions))
	for _, dim := range dimensions {
		if dim.Score == nil {
			continue
		}
		risk := RiskLevelNone
		if dim.Level != nil {
			risk = RiskLevel(dim.Level.Code)
		}
		results = append(results, FactorScoreResult{
			FactorCode: NewFactorCode(dim.Code),
			FactorName: dim.Name,
			RawScore:   dim.Score.Value,
			RiskLevel:  risk,
			Conclusion: dim.Description,
			Suggestion: dim.Suggestion,
		})
	}
	return results
}
