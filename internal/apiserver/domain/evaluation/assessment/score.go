package assessment

// ScaleScoreProjection is the legacy scale-compatible score projection stored in MySQL.
// New model families should write AssessmentOutcome instead.
type ScaleScoreProjection struct {
	assessmentID ID
	totalScore   float64
	riskLevel    RiskLevel
	factorScores []ScaleFactorScore
}

// NewScaleScoreProjection creates a scale score projection.
func NewScaleScoreProjection(
	assessmentID ID,
	totalScore float64,
	riskLevel RiskLevel,
	factorScores []ScaleFactorScore,
) *ScaleScoreProjection {
	return &ScaleScoreProjection{
		assessmentID: assessmentID,
		totalScore:   totalScore,
		riskLevel:    riskLevel,
		factorScores: factorScores,
	}
}

// ReconstructScaleScoreProjection rebuilds a scale score projection from persistence.
func ReconstructScaleScoreProjection(
	assessmentID ID,
	totalScore float64,
	riskLevel RiskLevel,
	factorScores []ScaleFactorScore,
) *ScaleScoreProjection {
	return &ScaleScoreProjection{
		assessmentID: assessmentID,
		totalScore:   totalScore,
		riskLevel:    riskLevel,
		factorScores: factorScores,
	}
}

// ScaleScoreProjectionFromOutcome projects a canonical outcome into scale storage.
func ScaleScoreProjectionFromOutcome(assessmentID ID, outcome *AssessmentOutcome) *ScaleScoreProjection {
	if outcome == nil || !outcome.ModelRef.IsScale() {
		return nil
	}

	var totalScore float64
	var riskLevel RiskLevel
	if outcome.Primary != nil {
		totalScore = outcome.Primary.Value
	}
	if outcome.Level != nil && IsRiskLevelCode(outcome.Level.Code) {
		riskLevel = RiskLevel(outcome.Level.Code)
	}

	factorScores := factorScoresForScaleProjection(outcome)
	scaleFactors := make([]ScaleFactorScore, 0, len(factorScores))
	for _, fs := range factorScores {
		scaleFactors = append(scaleFactors, NewScaleFactorScore(
			fs.FactorCode,
			fs.FactorName,
			fs.RawScore,
			fs.RiskLevel,
			fs.IsTotalScore,
		))
	}

	return NewScaleScoreProjection(assessmentID, totalScore, riskLevel, scaleFactors)
}

func factorScoresForScaleProjection(outcome *AssessmentOutcome) []FactorScoreResult {
	if scores, ok := outcome.Detail.Payload.([]FactorScoreResult); ok && len(scores) > 0 {
		return scores
	}
	if len(outcome.Dimensions) > 0 {
		return factorScoreResultsFromDimensions(outcome.Dimensions)
	}
	return nil
}

// ScaleScoreProjectionFromEvaluationResult projects a legacy evaluation result into scale storage.
//
// Deprecated: characterization boundary only; persistence should use ScaleScoreProjectionFromOutcome.
func ScaleScoreProjectionFromEvaluationResult(assessmentID ID, result *EvaluationResult) *ScaleScoreProjection {
	if result == nil {
		return nil
	}

	factorScores := make([]ScaleFactorScore, 0, len(result.FactorScores))
	for _, fs := range result.FactorScores {
		factorScores = append(factorScores, NewScaleFactorScore(
			fs.FactorCode,
			fs.FactorName,
			fs.RawScore,
			fs.RiskLevel,
			fs.IsTotalScore,
		))
	}

	return NewScaleScoreProjection(
		assessmentID,
		result.TotalScore,
		result.RiskLevel,
		factorScores,
	)
}

func (s *ScaleScoreProjection) AssessmentID() ID {
	return s.assessmentID
}

func (s *ScaleScoreProjection) TotalScore() float64 {
	return s.totalScore
}

func (s *ScaleScoreProjection) RiskLevel() RiskLevel {
	return s.riskLevel
}

func (s *ScaleScoreProjection) FactorScores() []ScaleFactorScore {
	return s.factorScores
}

func (s *ScaleScoreProjection) IsHighRisk() bool {
	return IsHighRisk(s.riskLevel)
}

func (s *ScaleScoreProjection) GetFactorScore(factorCode FactorCode) *ScaleFactorScore {
	for i := range s.factorScores {
		if s.factorScores[i].FactorCode().Equals(factorCode) {
			return &s.factorScores[i]
		}
	}
	return nil
}

func (s *ScaleScoreProjection) GetHighRiskFactors() []ScaleFactorScore {
	var highRiskFactors []ScaleFactorScore
	for _, fs := range s.factorScores {
		if fs.IsHighRisk() {
			highRiskFactors = append(highRiskFactors, fs)
		}
	}
	return highRiskFactors
}

func (s *ScaleScoreProjection) GetTotalScoreFactor() *ScaleFactorScore {
	for i := range s.factorScores {
		if s.factorScores[i].IsTotalScore() {
			return &s.factorScores[i]
		}
	}
	return nil
}

func (s *ScaleScoreProjection) FactorCount() int {
	return len(s.factorScores)
}

// ScaleFactorScore records one factor row in the scale score projection.
type ScaleFactorScore struct {
	factorCode   FactorCode
	factorName   string
	rawScore     float64
	riskLevel    RiskLevel
	isTotalScore bool
}

func NewScaleFactorScore(
	factorCode FactorCode,
	factorName string,
	rawScore float64,
	riskLevel RiskLevel,
	isTotalScore bool,
) ScaleFactorScore {
	return ScaleFactorScore{
		factorCode:   factorCode,
		factorName:   factorName,
		rawScore:     rawScore,
		riskLevel:    riskLevel,
		isTotalScore: isTotalScore,
	}
}

func (f ScaleFactorScore) FactorCode() FactorCode {
	return f.factorCode
}

func (f ScaleFactorScore) FactorName() string {
	return f.factorName
}

func (f ScaleFactorScore) RawScore() float64 {
	return f.rawScore
}

func (f ScaleFactorScore) RiskLevel() RiskLevel {
	return f.riskLevel
}

func (f ScaleFactorScore) IsTotalScore() bool {
	return f.isTotalScore
}

func (f ScaleFactorScore) IsHighRisk() bool {
	return IsHighRisk(f.riskLevel)
}
