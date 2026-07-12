package assessment

// ScaleScoreProjection is the Evaluation-owned scale query projection stored in MySQL.
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
