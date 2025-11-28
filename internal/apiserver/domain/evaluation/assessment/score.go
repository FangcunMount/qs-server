package assessment

import "time"

// ==================== AssessmentScore 测评得分 ====================

// AssessmentScore 测评得分实体
// 记录一次测评的完整得分情况，包含总分和所有因子得分
// 核心职责：
// 1. 记录测评的总分和风险等级
// 2. 管理所有因子得分
// 3. 支持按维度查询和趋势分析
// 设计要点：
// - 与 Assessment 是 1:1 关系
// - 包含多个 FactorScore（1:N）
// - 存储在 MySQL：便于 SQL 聚合查询
type AssessmentScore struct {
	assessmentID ID
	totalScore   float64
	riskLevel    RiskLevel
	factorScores []FactorScore
	createdAt    time.Time
}

// NewAssessmentScore 创建测评得分
func NewAssessmentScore(
	assessmentID ID,
	totalScore float64,
	riskLevel RiskLevel,
	factorScores []FactorScore,
) *AssessmentScore {
	return &AssessmentScore{
		assessmentID: assessmentID,
		totalScore:   totalScore,
		riskLevel:    riskLevel,
		factorScores: factorScores,
		createdAt:    time.Now(),
	}
}

// ReconstructAssessmentScore 从持久化数据重建测评得分（用于仓储层）
func ReconstructAssessmentScore(
	assessmentID ID,
	totalScore float64,
	riskLevel RiskLevel,
	factorScores []FactorScore,
	createdAt time.Time,
) *AssessmentScore {
	return &AssessmentScore{
		assessmentID: assessmentID,
		totalScore:   totalScore,
		riskLevel:    riskLevel,
		factorScores: factorScores,
		createdAt:    createdAt,
	}
}

// FromEvaluationResult 从评估结果创建测评得分
func FromEvaluationResult(assessmentID ID, result *EvaluationResult) *AssessmentScore {
	if result == nil {
		return nil
	}

	// 转换因子得分
	factorScores := make([]FactorScore, 0, len(result.FactorScores))
	for _, fs := range result.FactorScores {
		factorScores = append(factorScores, NewFactorScore(
			fs.FactorCode,
			fs.FactorName,
			fs.RawScore,
			fs.RiskLevel,
			fs.IsTotalScore,
		))
	}

	return NewAssessmentScore(
		assessmentID,
		result.TotalScore,
		result.RiskLevel,
		factorScores,
	)
}

// ==================== AssessmentScore 查询方法 ====================

// AssessmentID 获取测评ID
func (s *AssessmentScore) AssessmentID() ID {
	return s.assessmentID
}

// TotalScore 获取总分
func (s *AssessmentScore) TotalScore() float64 {
	return s.totalScore
}

// RiskLevel 获取风险等级
func (s *AssessmentScore) RiskLevel() RiskLevel {
	return s.riskLevel
}

// FactorScores 获取所有因子得分
func (s *AssessmentScore) FactorScores() []FactorScore {
	return s.factorScores
}

// CreatedAt 获取创建时间
func (s *AssessmentScore) CreatedAt() time.Time {
	return s.createdAt
}

// ==================== AssessmentScore 业务方法 ====================

// IsHighRisk 是否高风险
func (s *AssessmentScore) IsHighRisk() bool {
	return IsHighRisk(s.riskLevel)
}

// GetFactorScore 根据因子编码获取因子得分
func (s *AssessmentScore) GetFactorScore(factorCode FactorCode) *FactorScore {
	for i := range s.factorScores {
		if s.factorScores[i].FactorCode().Equals(factorCode) {
			return &s.factorScores[i]
		}
	}
	return nil
}

// GetHighRiskFactors 获取高风险因子列表
func (s *AssessmentScore) GetHighRiskFactors() []FactorScore {
	var highRiskFactors []FactorScore
	for _, fs := range s.factorScores {
		if fs.IsHighRisk() {
			highRiskFactors = append(highRiskFactors, fs)
		}
	}
	return highRiskFactors
}

// GetTotalScoreFactor 获取总分因子（如果存在）
func (s *AssessmentScore) GetTotalScoreFactor() *FactorScore {
	for i := range s.factorScores {
		if s.factorScores[i].IsTotalScore() {
			return &s.factorScores[i]
		}
	}
	return nil
}

// FactorCount 获取因子数量
func (s *AssessmentScore) FactorCount() int {
	return len(s.factorScores)
}

// ==================== FactorScore 因子得分 ====================

// FactorScore 因子得分值对象
// 记录单个因子的得分情况
type FactorScore struct {
	factorCode   FactorCode
	factorName   string
	rawScore     float64
	riskLevel    RiskLevel
	isTotalScore bool // 是否为总分因子
}

// NewFactorScore 创建因子得分
func NewFactorScore(
	factorCode FactorCode,
	factorName string,
	rawScore float64,
	riskLevel RiskLevel,
	isTotalScore bool,
) FactorScore {
	return FactorScore{
		factorCode:   factorCode,
		factorName:   factorName,
		rawScore:     rawScore,
		riskLevel:    riskLevel,
		isTotalScore: isTotalScore,
	}
}

// ==================== FactorScore 查询方法 ====================

// FactorCode 获取因子编码
func (f FactorScore) FactorCode() FactorCode {
	return f.factorCode
}

// FactorName 获取因子名称
func (f FactorScore) FactorName() string {
	return f.factorName
}

// RawScore 获取原始分
func (f FactorScore) RawScore() float64 {
	return f.rawScore
}

// RiskLevel 获取风险等级
func (f FactorScore) RiskLevel() RiskLevel {
	return f.riskLevel
}

// IsTotalScore 是否为总分因子
func (f FactorScore) IsTotalScore() bool {
	return f.isTotalScore
}

// IsHighRisk 是否高风险
func (f FactorScore) IsHighRisk() bool {
	return IsHighRisk(f.riskLevel)
}

// ==================== 趋势分析辅助类型 ====================

// ScoreTrend 得分趋势（用于趋势分析）
type ScoreTrend struct {
	FactorCode FactorCode
	FactorName string
	DataPoints []ScoreDataPoint
	TrendType  TrendType
	ChangeRate float64 // 变化率（最新分数相对于首次分数的变化百分比）
}

// ScoreDataPoint 得分数据点
type ScoreDataPoint struct {
	AssessmentID ID
	RawScore     float64
	RiskLevel    RiskLevel
	RecordedAt   time.Time
}

// TrendType 趋势类型
type TrendType string

const (
	// TrendTypeImproving 改善趋势
	TrendTypeImproving TrendType = "improving"

	// TrendTypeStable 稳定趋势
	TrendTypeStable TrendType = "stable"

	// TrendTypeWorsening 恶化趋势
	TrendTypeWorsening TrendType = "worsening"

	// TrendTypeUnknown 未知趋势（数据不足）
	TrendTypeUnknown TrendType = "unknown"
)

// String 返回趋势类型的字符串表示
func (t TrendType) String() string {
	return string(t)
}

// IsImproving 是否改善趋势
func (t TrendType) IsImproving() bool {
	return t == TrendTypeImproving
}

// IsWorsening 是否恶化趋势
func (t TrendType) IsWorsening() bool {
	return t == TrendTypeWorsening
}

// CalculateTrend 计算趋势（基于多个数据点）
// 返回趋势类型和变化率
func CalculateTrend(dataPoints []ScoreDataPoint) (TrendType, float64) {
	if len(dataPoints) < 2 {
		return TrendTypeUnknown, 0
	}

	// 按时间排序（假设已排序，最旧在前）
	first := dataPoints[0]
	last := dataPoints[len(dataPoints)-1]

	if first.RawScore == 0 {
		return TrendTypeUnknown, 0
	}

	// 计算变化率
	changeRate := (last.RawScore - first.RawScore) / first.RawScore * 100

	// 阈值判断（变化超过 5% 才认为有明显趋势）
	const threshold = 5.0

	switch {
	case changeRate < -threshold:
		return TrendTypeImproving, changeRate // 分数降低表示改善（对于心理量表）
	case changeRate > threshold:
		return TrendTypeWorsening, changeRate // 分数升高表示恶化
	default:
		return TrendTypeStable, changeRate
	}
}
