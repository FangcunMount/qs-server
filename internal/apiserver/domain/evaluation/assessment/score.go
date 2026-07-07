package assessment

// ScaleScoreProjection 是旧量表兼容分数投影 stored in MySQL。
// New 建模家族 应该 write AssessmentOutcome instead。
type ScaleScoreProjection struct {
	assessmentID ID
	totalScore   float64
	riskLevel    RiskLevel
	factorScores []ScaleFactorScore
}

// NewScaleScoreProjection 创建scale score 投影。
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

// ReconstructScaleScoreProjection rebuilds scale score 投影 从 持久化。
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

// ScaleScoreProjectionFromOutcome 投影规范 结果 为 scale storage。
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

// ScaleScoreProjectionFromEvaluationResult 投影旧版 评估 结果 为 scale storage。
//
// Deprecated: 仅作为表征边界保留；持久化应使用 ScaleScoreProjectionFromOutcome。
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

// ScaleFactorScore 记录一个因子行 in scale score 投影。
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
