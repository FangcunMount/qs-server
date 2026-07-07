package assessment

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
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

	// StatusEvaluated 已计分：结构化计分完成，等待报告生成
	StatusEvaluated Status = "evaluated"

	// StatusInterpreted 已解读：评估完成，报告已生成
	StatusInterpreted Status = "interpreted"

	// StatusFailed 评估失败
	StatusFailed Status = "failed"
)

// String 返回状态的字符串表示
func (s Status) String() string {
	return string(s)
}

// DisplayName 返回状态的中文展示名称。
func (s Status) DisplayName() string {
	switch s {
	case StatusPending:
		return "待处理"
	case StatusSubmitted:
		return "已提交"
	case StatusEvaluated:
		return "已计分"
	case StatusInterpreted:
		return "已解读"
	case StatusFailed:
		return "失败"
	default:
		return string(s)
	}
}

// IsValid 检查状态是否有效
func (s Status) IsValid() bool {
	switch s {
	case StatusPending, StatusSubmitted, StatusEvaluated, StatusInterpreted, StatusFailed:
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

// IsEvaluated 是否已计分状态
func (s Status) IsEvaluated() bool {
	return s == StatusEvaluated
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

// CanApplyScoring 是否可应用计分结果
func (s Status) CanApplyScoring() bool {
	return s == StatusSubmitted
}

// CanApplyInterpretation 是否可应用解读结果并生成报告
func (s Status) CanApplyInterpretation() bool {
	return s == StatusSubmitted || s == StatusEvaluated
}

// ==================== 测评来源类型枚举 ====================

// OriginType 测评来源类型
type OriginType string

const (
	// OriginAdhoc 一次性测评：手动创建，不属于任何计划
	OriginAdhoc OriginType = "adhoc"

	// OriginPlan 测评计划：由 AssessmentPlan 生成的 AssessmentTask 创建
	OriginPlan OriginType = "plan"
)

// String 返回来源类型的字符串表示
func (o OriginType) String() string {
	return string(o)
}

// DisplayName 返回来源类型的中文展示名称。
func (o OriginType) DisplayName() string {
	switch o {
	case OriginAdhoc:
		return "临时测评"
	case OriginPlan:
		return "计划测评"
	default:
		return string(o)
	}
}

// IsValid 检查来源类型是否有效
func (o OriginType) IsValid() bool {
	switch o {
	case OriginAdhoc, OriginPlan:
		return true
	default:
		return false
	}
}

// ==================== 风险等级 ====================

// RiskLevel 风险等级
type RiskLevel string

const (
	RiskLevelNone   RiskLevel = "none"
	RiskLevelLow    RiskLevel = "low"
	RiskLevelMedium RiskLevel = "medium"
	RiskLevelHigh   RiskLevel = "high"
	RiskLevelSevere RiskLevel = "severe"
)

func (r RiskLevel) String() string {
	return string(r)
}

// RiskLevelFromString 从字符串解析风险等级
func RiskLevelFromString(s string) RiskLevel {
	return RiskLevel(s)
}

// IsHighRisk 是否高风险（包含 high 和 severe）
func IsHighRisk(r RiskLevel) bool {
	return r == RiskLevelHigh || r == RiskLevelSevere
}

// IsRiskLevelCode 报告是否 编码 是 旧量表风险等级值。
func IsRiskLevelCode(code string) bool {
	switch RiskLevel(code) {
	case RiskLevelNone, RiskLevelLow, RiskLevelMedium, RiskLevelHigh, RiskLevelSevere:
		return true
	default:
		return false
	}
}

// ==================== 解释模型引用 ====================

// EvaluationModelKind 测评模型类型。
type EvaluationModelKind = modelcatalog.Kind

const (
	// EvaluationModelKindScale 医学/心理量表模型。
	EvaluationModelKindScale EvaluationModelKind = modelcatalog.KindScale

	// EvaluationModelKindPersonality 人格类模型（typology/trait 等子形态由 SubKind/Algorithm 区分）。
	EvaluationModelKindPersonality EvaluationModelKind = modelcatalog.KindPersonality
)

// EvaluationModelRef 表示执行期模型引用（含计分与解读规则快照）。
type EvaluationModelRef struct {
	id        meta.ID
	kind      EvaluationModelKind
	subKind   modelcatalog.SubKind
	algorithm modelcatalog.Algorithm
	code      meta.Code
	version   string
	title     string
}

// NewEvaluationModelRef 创建通用测评模型引用。
func NewEvaluationModelRef(kind EvaluationModelKind, id meta.ID, code meta.Code, version, title string) EvaluationModelRef {
	return NewEvaluationModelRefWithIdentity(kind, modelcatalog.SubKindEmpty, "", id, code, version, title)
}

// NewEvaluationModelRefByCode 创建不带底层模型 ID 的测评模型引用。
func NewEvaluationModelRefByCode(kind EvaluationModelKind, code meta.Code, version, title string) EvaluationModelRef {
	return NewEvaluationModelRefWithIdentity(kind, modelcatalog.SubKindEmpty, "", meta.ID(0), code, version, title)
}

// NewEvaluationModelRefWithIdentity 创建带 v2 身份三元组的测评模型引用。
func NewEvaluationModelRefWithIdentity(
	kind EvaluationModelKind,
	subKind modelcatalog.SubKind,
	algorithm modelcatalog.Algorithm,
	id meta.ID,
	code meta.Code,
	version, title string,
) EvaluationModelRef {
	return EvaluationModelRef{
		id:        id,
		kind:      kind,
		subKind:   subKind,
		algorithm: algorithm,
		code:      code,
		version:   version,
		title:     title,
	}
}

// NewScaleEvaluationModelRef 创建 Scale 测评模型引用。
func NewScaleEvaluationModelRef(id meta.ID, code meta.Code, version, title string) EvaluationModelRef {
	return NewEvaluationModelRef(EvaluationModelKindScale, id, code, version, title)
}

func (r EvaluationModelRef) ID() meta.ID {
	return r.id
}

func (r EvaluationModelRef) Kind() EvaluationModelKind {
	return r.kind
}

func (r EvaluationModelRef) Code() meta.Code {
	return r.code
}

func (r EvaluationModelRef) Version() string {
	return r.version
}

func (r EvaluationModelRef) Title() string {
	return r.title
}

func (r EvaluationModelRef) SubKind() modelcatalog.SubKind {
	return r.subKind
}

func (r EvaluationModelRef) Algorithm() modelcatalog.Algorithm {
	return r.algorithm
}

func (r EvaluationModelRef) IsEmpty() bool {
	return r.kind == "" && r.code.IsEmpty()
}

func (r EvaluationModelRef) IsScale() bool {
	return r.kind == EvaluationModelKindScale
}

func (r EvaluationModelRef) SameIdentity(other EvaluationModelRef) bool {
	return r.ExecutionIdentity() == other.ExecutionIdentity() &&
		r.code == other.code &&
		r.version == other.version
}

// ==================== 引用值对象 ====================

// QuestionnaireRef 问卷引用值对象
type QuestionnaireRef struct {
	id      meta.ID
	code    meta.Code
	version string
}

// NewQuestionnaireRef 创建问卷引用（完整版，包含 ID）
func NewQuestionnaireRef(id meta.ID, code meta.Code, version string) QuestionnaireRef {
	return QuestionnaireRef{
		id:      id,
		code:    code,
		version: version,
	}
}

// NewQuestionnaireRefByCode 通过编码创建问卷引用（推荐，code 是唯一标识）
func NewQuestionnaireRefByCode(code meta.Code, version string) QuestionnaireRef {
	return QuestionnaireRef{
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

// IsEmpty 是否为空引用（code 为空即视为空引用）
func (r QuestionnaireRef) IsEmpty() bool {
	return r.code.IsEmpty()
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
	id      meta.ID
	code    meta.Code
	name    string
	version string
}

// NewMedicalScaleRef 创建量表引用
func NewMedicalScaleRef(id meta.ID, code meta.Code, name string) MedicalScaleRef {
	return MedicalScaleRef{
		id:   id,
		code: code,
		name: name,
	}
}

// NewMedicalScaleRefWithVersion 创建带解释模型版本的量表引用。
func NewMedicalScaleRefWithVersion(id meta.ID, code meta.Code, name, version string) MedicalScaleRef {
	return MedicalScaleRef{
		id:      id,
		code:    code,
		name:    name,
		version: version,
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

// Version 获取量表解释模型版本。
func (r MedicalScaleRef) Version() string {
	return r.version
}

// IsEmpty 是否为空引用
func (r MedicalScaleRef) IsEmpty() bool {
	return r.id.IsZero() && r.code.IsEmpty()
}

// ToEvaluationModelRef 将旧的 MedicalScaleRef 转换为通用解释模型引用。
func (r MedicalScaleRef) ToEvaluationModelRef() EvaluationModelRef {
	return NewScaleEvaluationModelRef(r.id, r.code, r.version, r.name)
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

// ReconstructOrigin 从持久化数据重建 Origin（用于仓储层）
func ReconstructOrigin(originType OriginType, originID *string) Origin {
	return Origin{
		originType: originType,
		originID:   originID,
	}
}

// ==================== 因子编码 ====================

// FactorCode 因子编码
type FactorCode string

// NewFactorCode 创建因子编码
func NewFactorCode(code string) FactorCode {
	return FactorCode(code)
}

func (c FactorCode) Value() string {
	return string(c)
}

func (c FactorCode) String() string {
	return string(c)
}

func (c FactorCode) IsEmpty() bool {
	return c == ""
}

func (c FactorCode) Equals(other FactorCode) bool {
	return c == other
}

// ==================== 评估结果值对象 ====================

// ResultSummary 是跨模型通用的结果摘要。
type ResultSummary struct {
	PrimaryLabel string
	Score        *float64
	Level        *string
	Tags         []string
}

// EvaluationDetail 承载具体模型的结构化结果。
type EvaluationDetail struct {
	Kind    EvaluationModelKind
	Payload any
}

// EvaluationResult 评估结果值对象
// 包含通用结果摘要和当前兼容保留的量表评估结果字段。
// 由应用服务层使用 calculation 和 interpretation 功能域组装
type EvaluationResult struct {
	// 解释模型引用
	ModelRef EvaluationModelRef

	// 通用结果摘要
	Summary ResultSummary

	// 具体模型结果明细
	Detail EvaluationDetail

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
	level := string(riskLevel)
	summaryScore := totalScore
	return &EvaluationResult{
		TotalScore:   totalScore,
		RiskLevel:    riskLevel,
		Conclusion:   conclusion,
		Suggestion:   suggestion,
		FactorScores: factorScores,
		Summary: ResultSummary{
			PrimaryLabel: level,
			Score:        &summaryScore,
			Level:        &level,
		},
		Detail: EvaluationDetail{
			Payload: factorScores,
		},
	}
}

// NewModelEvaluationResult 创建跨解释模型的通用结果。
func NewModelEvaluationResult(
	modelRef EvaluationModelRef,
	summary ResultSummary,
	detail EvaluationDetail,
) *EvaluationResult {
	totalScore := 0.0
	if summary.Score != nil {
		totalScore = *summary.Score
	}
	if detail.Kind == "" {
		detail.Kind = modelRef.Kind()
	}
	return &EvaluationResult{
		ModelRef:     modelRef,
		Summary:      summary,
		Detail:       detail,
		TotalScore:   totalScore,
		RiskLevel:    RiskLevelNone,
		Conclusion:   summary.PrimaryLabel,
		FactorScores: make([]FactorScoreResult, 0),
	}
}

// WithModelRef 绑定解释模型引用。
func (r *EvaluationResult) WithModelRef(modelRef EvaluationModelRef) *EvaluationResult {
	if r == nil {
		return nil
	}
	r.ModelRef = modelRef
	if r.Detail.Kind == "" {
		r.Detail.Kind = modelRef.Kind()
	}
	return r
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
