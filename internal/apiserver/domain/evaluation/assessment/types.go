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

	// StatusEvaluated Evaluation 成功终态：结构化评估事实已可靠提交。
	StatusEvaluated Status = "evaluated"

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
	case StatusFailed:
		return "失败"
	default:
		return string(s)
	}
}

// IsValid 检查状态是否有效
func (s Status) IsValid() bool {
	switch s {
	case StatusPending, StatusSubmitted, StatusEvaluated, StatusFailed:
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

// IsFailed 是否失败状态
func (s Status) IsFailed() bool {
	return s == StatusFailed
}

// CanApplyScoring 是否可应用计分结果
func (s Status) CanApplyScoring() bool {
	return s == StatusSubmitted
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

	// EvaluationModelKindTypology 类型学模型（trait 等子形态由 SubKind/Algorithm 区分）。
	EvaluationModelKindTypology EvaluationModelKind = modelcatalog.KindTypology

	// EvaluationModelKindPersonality is a deprecated alias for EvaluationModelKindTypology.
	EvaluationModelKindPersonality EvaluationModelKind = EvaluationModelKindTypology
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

// ResultSummary 是跨模型通用的结果摘要。
type ResultSummary struct {
	PrimaryLabel string
	Score        *float64
	Level        *string
	Tags         []string
}
