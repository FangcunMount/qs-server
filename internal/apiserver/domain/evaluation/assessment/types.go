package assessment

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// ==================== ID 类型定义 ====================

// ID 测评ID类型（使用 meta.ID 作为底层类型）
type ID = meta.ID

// NewID 创建测评ID
func NewID(id uint64) ID {
	return meta.FromUint64(id)
}

// ParseID 解析测评ID
func ParseID(s string) (ID, error) {
	return meta.ParseID(s)
}

// ==================== 测评状态枚举 ====================

// Status 测评状态
type Status string

const (
	// StatusPending 待提交：已创建，但答卷尚未提交
	StatusPending Status = "pending"

	// StatusSubmitted 已提交：答卷已提交，等待评估
	StatusSubmitted Status = "submitted"

	// StatusInterpreted 已解读：评估完成，报告已生成
	StatusInterpreted Status = "interpreted"

	// StatusFailed 评估失败
	StatusFailed Status = "failed"
)

// String 返回状态的字符串表示
func (s Status) String() string {
	return string(s)
}

// IsValid 检查状态是否有效
func (s Status) IsValid() bool {
	switch s {
	case StatusPending, StatusSubmitted, StatusInterpreted, StatusFailed:
		return true
	default:
		return false
	}
}

// IsPending 是否待提交状态
func (s Status) IsPending() bool {
	return s == StatusPending
}

// IsSubmitted 是否已提交状态
func (s Status) IsSubmitted() bool {
	return s == StatusSubmitted
}

// IsInterpreted 是否已解读状态
func (s Status) IsInterpreted() bool {
	return s == StatusInterpreted
}

// IsFailed 是否失败状态
func (s Status) IsFailed() bool {
	return s == StatusFailed
}

// IsTerminal 是否终态（不可再迁移）
func (s Status) IsTerminal() bool {
	return s == StatusInterpreted || s == StatusFailed
}

// ==================== 测评来源类型枚举 ====================

// OriginType 测评来源类型
type OriginType string

const (
	// OriginAdhoc 一次性测评：手动创建，不属于任何计划或筛查
	OriginAdhoc OriginType = "adhoc"

	// OriginPlan 测评计划：由 AssessmentPlan 生成的 AssessmentTask 创建
	OriginPlan OriginType = "plan"

	// OriginScreening 入校筛查：由 ScreeningProject 创建
	OriginScreening OriginType = "screening"
)

// String 返回来源类型的字符串表示
func (o OriginType) String() string {
	return string(o)
}

// IsValid 检查来源类型是否有效
func (o OriginType) IsValid() bool {
	switch o {
	case OriginAdhoc, OriginPlan, OriginScreening:
		return true
	default:
		return false
	}
}

// ==================== 风险等级（复用 scale 子域定义）====================

// RiskLevel 风险等级（复用 scale 子域的定义，保持一致性）
type RiskLevel = scale.RiskLevel

// 风险等级常量（复用 scale 子域定义）
const (
	RiskLevelNone   = scale.RiskLevelNone
	RiskLevelLow    = scale.RiskLevelLow
	RiskLevelMedium = scale.RiskLevelMedium
	RiskLevelHigh   = scale.RiskLevelHigh
	RiskLevelSevere = scale.RiskLevelSevere
)

// RiskLevelFromString 从字符串解析风险等级
func RiskLevelFromString(s string) RiskLevel {
	return RiskLevel(s)
}

// IsHighRisk 是否高风险（包含 high 和 severe）
func IsHighRisk(r RiskLevel) bool {
	return r == RiskLevelHigh || r == RiskLevelSevere
}

// ==================== 引用值对象 ====================

// QuestionnaireRef 问卷引用值对象
type QuestionnaireRef struct {
	id      meta.ID
	code    meta.Code
	version string
}

// NewQuestionnaireRef 创建问卷引用
func NewQuestionnaireRef(id meta.ID, code meta.Code, version string) QuestionnaireRef {
	return QuestionnaireRef{
		id:      id,
		code:    code,
		version: version,
	}
}

// ID 获取问卷ID
func (r QuestionnaireRef) ID() meta.ID {
	return r.id
}

// Code 获取问卷编码
func (r QuestionnaireRef) Code() meta.Code {
	return r.code
}

// Version 获取问卷版本
func (r QuestionnaireRef) Version() string {
	return r.version
}

// IsEmpty 是否为空引用
func (r QuestionnaireRef) IsEmpty() bool {
	return r.id.IsZero() && r.code.IsEmpty()
}

// AnswerSheetRef 答卷引用值对象
type AnswerSheetRef struct {
	id meta.ID
}

// NewAnswerSheetRef 创建答卷引用
func NewAnswerSheetRef(id meta.ID) AnswerSheetRef {
	return AnswerSheetRef{id: id}
}

// ID 获取答卷ID
func (r AnswerSheetRef) ID() meta.ID {
	return r.id
}

// IsEmpty 是否为空引用
func (r AnswerSheetRef) IsEmpty() bool {
	return r.id.IsZero()
}

// MedicalScaleRef 量表引用值对象
type MedicalScaleRef struct {
	id   meta.ID
	code meta.Code
	name string
}

// NewMedicalScaleRef 创建量表引用
func NewMedicalScaleRef(id meta.ID, code meta.Code, name string) MedicalScaleRef {
	return MedicalScaleRef{
		id:   id,
		code: code,
		name: name,
	}
}

// ID 获取量表ID
func (r MedicalScaleRef) ID() meta.ID {
	return r.id
}

// Code 获取量表编码
func (r MedicalScaleRef) Code() meta.Code {
	return r.code
}

// Name 获取量表名称
func (r MedicalScaleRef) Name() string {
	return r.name
}

// IsEmpty 是否为空引用
func (r MedicalScaleRef) IsEmpty() bool {
	return r.id.IsZero() && r.code.IsEmpty()
}

// ==================== 业务来源值对象 ====================

// Origin 业务来源值对象
type Origin struct {
	originType OriginType
	originID   *string
}

// NewAdhocOrigin 创建一次性测评来源
func NewAdhocOrigin() Origin {
	return Origin{
		originType: OriginAdhoc,
		originID:   nil,
	}
}

// NewPlanOrigin 创建测评计划来源
func NewPlanOrigin(planID string) Origin {
	return Origin{
		originType: OriginPlan,
		originID:   &planID,
	}
}

// NewScreeningOrigin 创建入校筛查来源
func NewScreeningOrigin(screeningProjectID string) Origin {
	return Origin{
		originType: OriginScreening,
		originID:   &screeningProjectID,
	}
}

// Type 获取来源类型
func (o Origin) Type() OriginType {
	return o.originType
}

// ID 获取来源ID
func (o Origin) ID() *string {
	return o.originID
}

// IsAdhoc 是否一次性测评
func (o Origin) IsAdhoc() bool {
	return o.originType == OriginAdhoc
}

// IsPlan 是否来自测评计划
func (o Origin) IsPlan() bool {
	return o.originType == OriginPlan
}

// IsScreening 是否来自入校筛查
func (o Origin) IsScreening() bool {
	return o.originType == OriginScreening
}

// ==================== 因子编码（复用 scale 子域定义） ====================

// FactorCode 因子编码（复用 scale 子域的定义）
type FactorCode = scale.FactorCode

// NewFactorCode 创建因子编码
func NewFactorCode(code string) FactorCode {
	return scale.NewFactorCode(code)
}

// ==================== 评估结果值对象 ====================

// EvaluationResult 评估结果值对象
// 包含完整的量表评估结果
// 由应用服务层使用 calculation 和 interpretation 功能域组装
type EvaluationResult struct {
	// 总分
	TotalScore float64

	// 总体风险等级
	RiskLevel RiskLevel

	// 总结论
	Conclusion string

	// 建议
	Suggestion string

	// 各因子得分
	FactorScores []FactorScoreResult
}

// NewEvaluationResult 创建评估结果
// 由应用服务层调用，组装计算和解读结果
func NewEvaluationResult(
	totalScore float64,
	riskLevel RiskLevel,
	conclusion string,
	suggestion string,
	factorScores []FactorScoreResult,
) *EvaluationResult {
	if factorScores == nil {
		factorScores = make([]FactorScoreResult, 0)
	}
	return &EvaluationResult{
		TotalScore:   totalScore,
		RiskLevel:    riskLevel,
		Conclusion:   conclusion,
		Suggestion:   suggestion,
		FactorScores: factorScores,
	}
}

// GetFactorScore 获取指定因子的得分
func (r *EvaluationResult) GetFactorScore(factorCode FactorCode) (*FactorScoreResult, bool) {
	for i := range r.FactorScores {
		if r.FactorScores[i].FactorCode == factorCode {
			return &r.FactorScores[i], true
		}
	}
	return nil, false
}

// IsHighRiskResult 是否高风险
func (r *EvaluationResult) IsHighRiskResult() bool {
	return r.RiskLevel == RiskLevelHigh || r.RiskLevel == RiskLevelSevere
}

// HasHighRiskFactor 是否存在高风险因子
func (r *EvaluationResult) HasHighRiskFactor() bool {
	for _, fs := range r.FactorScores {
		if fs.IsHighRiskScore() {
			return true
		}
	}
	return false
}

// GetHighRiskFactors 获取高风险因子列表
func (r *EvaluationResult) GetHighRiskFactors() []FactorScoreResult {
	var result []FactorScoreResult
	for _, fs := range r.FactorScores {
		if fs.IsHighRiskScore() {
			result = append(result, fs)
		}
	}
	return result
}

// ==================== 因子得分结果值对象 ====================

// FactorScoreResult 因子得分结果
type FactorScoreResult struct {
	// 因子编码
	FactorCode FactorCode

	// 因子名称
	FactorName string

	// 原始得分
	RawScore float64

	// 风险等级
	RiskLevel RiskLevel

	// 结论
	Conclusion string

	// 建议
	Suggestion string

	// 是否为总分因子
	IsTotalScore bool
}

// NewFactorScoreResult 创建因子得分结果
func NewFactorScoreResult(
	factorCode FactorCode,
	factorName string,
	rawScore float64,
	riskLevel RiskLevel,
	conclusion string,
	suggestion string,
	isTotalScore bool,
) FactorScoreResult {
	return FactorScoreResult{
		FactorCode:   factorCode,
		FactorName:   factorName,
		RawScore:     rawScore,
		RiskLevel:    riskLevel,
		Conclusion:   conclusion,
		Suggestion:   suggestion,
		IsTotalScore: isTotalScore,
	}
}

// IsHighRiskScore 是否高风险
func (f FactorScoreResult) IsHighRiskScore() bool {
	return f.RiskLevel == RiskLevelHigh || f.RiskLevel == RiskLevelSevere
}
