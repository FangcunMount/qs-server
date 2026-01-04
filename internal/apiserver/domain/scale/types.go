package scale

import (
	"context"
	"encoding/json"
	"reflect"
	"regexp"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ===================== 量表状态 =================

// Status 量表状态
type Status string

const (
	StatusDraft     Status = "draft"     // 草稿
	StatusPublished Status = "published" // 已发布
	StatusArchived  Status = "archived"  // 已归档
)

// Value 获取状态值
func (s Status) Value() string {
	return string(s)
}

// String 获取状态字符串
func (s Status) String() string {
	return string(s)
}

// IsDraft 是否草稿状态
func (s Status) IsDraft() bool {
	return s == StatusDraft
}

// IsPublished 是否已发布状态
func (s Status) IsPublished() bool {
	return s == StatusPublished
}

// IsArchived 是否已归档状态
func (s Status) IsArchived() bool {
	return s == StatusArchived
}

// ParseStatus 解析状态字符串
func ParseStatus(value string) (Status, bool) {
	switch Status(value) {
	case StatusDraft, StatusPublished, StatusArchived:
		return Status(value), true
	default:
		return "", false
	}
}

// ===================== 因子编码 =================

// FactorCode 因子编码
type FactorCode string

// NewFactorCode 创建因子编码
func NewFactorCode(value string) FactorCode {
	return FactorCode(value)
}

// Value 获取编码值
func (c FactorCode) Value() string {
	return string(c)
}

// String 获取编码字符串
func (c FactorCode) String() string {
	return string(c)
}

// IsEmpty 判断编码是否为空
func (c FactorCode) IsEmpty() bool {
	return c == ""
}

// Equals 判断编码是否相等
func (c FactorCode) Equals(other FactorCode) bool {
	return c == other
}

// ===================== 因子类型 =================

// FactorType 因子类型
type FactorType string

const (
	// FactorTypePrimary 一级因子
	FactorTypePrimary FactorType = "primary"
	// FactorTypeMultilevel 多级因子
	FactorTypeMultilevel FactorType = "multilevel"
)

// String 返回因子类型的字符串表示
func (ft FactorType) String() string {
	return string(ft)
}

// IsValid 检查因子类型是否有效
func (ft FactorType) IsValid() bool {
	return ft == FactorTypePrimary || ft == FactorTypeMultilevel
}

// ===================== 计分策略 =================

// ScoringStrategyCode 因子计分策略编码
type ScoringStrategyCode string

const (
	// ScoringStrategySum 求和策略
	ScoringStrategySum ScoringStrategyCode = "sum"
	// ScoringStrategyAvg 平均策略
	ScoringStrategyAvg ScoringStrategyCode = "avg"
	// ScoringStrategyCnt 计数策略（统计匹配特定选项的题目数量）
	ScoringStrategyCnt ScoringStrategyCode = "cnt"
)

// String 返回计分策略的字符串表示
func (s ScoringStrategyCode) String() string {
	return string(s)
}

// IsValid 检查计分策略是否有效
func (s ScoringStrategyCode) IsValid() bool {
	return s == ScoringStrategySum || s == ScoringStrategyAvg || s == ScoringStrategyCnt
}

// ===================== 风险等级 =================

// RiskLevel 风险等级
type RiskLevel string

const (
	// RiskLevelNone 无风险
	RiskLevelNone RiskLevel = "none"
	// RiskLevelLow 低风险
	RiskLevelLow RiskLevel = "low"
	// RiskLevelMedium 中风险
	RiskLevelMedium RiskLevel = "medium"
	// RiskLevelHigh 高风险
	RiskLevelHigh RiskLevel = "high"
	// RiskLevelSevere 严重风险
	RiskLevelSevere RiskLevel = "severe"
)

// String 返回风险等级的字符串表示
func (r RiskLevel) String() string {
	return string(r)
}

// IsValid 检查风险等级是否有效
func (r RiskLevel) IsValid() bool {
	switch r {
	case RiskLevelNone, RiskLevelLow, RiskLevelMedium, RiskLevelHigh, RiskLevelSevere:
		return true
	default:
		return false
	}
}

// ===================== 分数区间 =================

// ScoreRange 分数区间 [Min, Max)
// 采用左闭右开区间，避免边界值重叠问题
// 例如：[0, 10) 表示 0 到 10 分（不包括 10 分）都适用该规则
// 对于 float 值如 29.52，可以设置 [20, 30) 和 [30, 40)，避免边界值重叠
type ScoreRange struct {
	min float64
	max float64
}

// NewScoreRange 创建分数区间
func NewScoreRange(min, max float64) ScoreRange {
	return ScoreRange{min: min, max: max}
}

// Min 获取最小值
func (r ScoreRange) Min() float64 {
	return r.min
}

// Max 获取最大值
func (r ScoreRange) Max() float64 {
	return r.max
}

// Contains 判断分数是否在区间内 [min, max)
// 使用左闭右开区间，避免边界值重叠
// 例如：ScoreRange{0, 10}.Contains(10) 返回 false，ScoreRange{0, 10}.Contains(9.99) 返回 true
func (r ScoreRange) Contains(score float64) bool {
	return score >= r.min && score < r.max
}

// IsValid 检查区间是否有效
// 左闭右开区间要求 min < max
func (r ScoreRange) IsValid() bool {
	return r.min < r.max
}

// ===================== 计分参数 =================

// ScoringParams 计分参数值对象
// 根据不同的计分策略，使用不同的字段
type ScoringParams struct {
	// 计数策略（cnt）专用参数
	CntOptionContents []string
}

// NewScoringParams 创建计分参数
func NewScoringParams() *ScoringParams {
	return &ScoringParams{
		CntOptionContents: make([]string, 0),
	}
}

// WithCntOptionContents 设置计数策略的选项内容列表
func (p *ScoringParams) WithCntOptionContents(contents []string) *ScoringParams {
	if contents == nil {
		p.CntOptionContents = make([]string, 0)
	} else {
		p.CntOptionContents = contents
	}
	return p
}

// GetCntOptionContents 获取计数策略的选项内容列表
func (p *ScoringParams) GetCntOptionContents() []string {
	if p == nil {
		return nil
	}
	return p.CntOptionContents
}

// ToMap 转换为 map[string]interface{}（用于持久化）
// 根据计分策略构建相应的参数结构
func (p *ScoringParams) ToMap(strategy ScoringStrategyCode) map[string]interface{} {
	result := make(map[string]interface{})

	if p == nil {
		return result
	}

	// 根据策略类型处理参数
	switch strategy {
	case ScoringStrategyCnt:
		// 计数策略：直接存储 cnt_option_contents
		// 注意：在应用层已经验证了 cnt 策略必须提供非空的 CntOptionContents
		// 所以这里应该总是有值，但为了防御性编程，仍然检查
		if len(p.CntOptionContents) > 0 {
			result["cnt_option_contents"] = p.CntOptionContents
		}
		// 如果为空数组，不存储该字段（读取时会返回空数组作为默认值）

	case ScoringStrategySum, ScoringStrategyAvg:
		// 求和和平均策略：当前不需要额外参数
		// 如果需要扩展，可以在这里添加

	default:
		// 其他策略：当前不需要额外参数
	}

	return result
}

// FromMap 从 map[string]interface{} 创建（用于从持久化层恢复）
func ScoringParamsFromMap(ctx context.Context, params map[string]interface{}, strategy ScoringStrategyCode) *ScoringParams {
	// 添加日志：记录输入参数
	paramsJSON, _ := json.Marshal(params)
	logger.L(ctx).Infow("ScoringParamsFromMap: input",
		"strategy", strategy,
		"params", string(paramsJSON),
		"params_type", getTypeName(params),
	)

	// 处理 nil 或空 map 的情况（nil map 的 len() 返回 0）
	if len(params) == 0 {
		logger.L(ctx).Debugw("ScoringParamsFromMap: params is nil or empty",
			"strategy", strategy,
		)
		return NewScoringParams()
	}

	result := NewScoringParams()

	// 根据策略类型解析参数
	switch strategy {
	case ScoringStrategyCnt:
		// 从 cnt_option_contents 字段读取
		contents, ok := params["cnt_option_contents"]
		if !ok || contents == nil {
			logger.L(ctx).Warnw("ScoringParamsFromMap: cnt_option_contents not found",
				"strategy", strategy,
				"params_keys", getMapKeys(params),
			)
			break
		}

		// 处理数组类型
		// 优先处理 MongoDB 的 primitive.A 类型
		var contentsArray []interface{}
		switch v := contents.(type) {
		case primitive.A:
			contentsArray = []interface{}(v)
		case []interface{}:
			contentsArray = v
		case []string:
			// 直接是字符串数组
			result.CntOptionContents = v
			logger.L(ctx).Infow("ScoringParamsFromMap: extracted cnt_option_contents (direct string array)",
				"count", len(result.CntOptionContents),
				"contents", result.CntOptionContents,
			)
		default:
			logger.L(ctx).Warnw("ScoringParamsFromMap: cnt_option_contents is not array type",
				"contents_type", getTypeName(contents),
			)
		}

		// 处理 interface{} 数组，转换为字符串数组
		if contentsArray != nil {
			result.CntOptionContents = make([]string, 0, len(contentsArray))
			for _, item := range contentsArray {
				if str, ok := item.(string); ok {
					result.CntOptionContents = append(result.CntOptionContents, str)
				} else {
					logger.L(ctx).Warnw("ScoringParamsFromMap: array item is not string",
						"item_type", getTypeName(item),
						"item_value", item,
					)
				}
			}
			logger.L(ctx).Infow("ScoringParamsFromMap: extracted cnt_option_contents",
				"count", len(result.CntOptionContents),
				"contents", result.CntOptionContents,
			)
		}

	case ScoringStrategySum, ScoringStrategyAvg:
		// 求和和平均策略：当前不需要额外参数

	default:
		// 其他策略：当前不需要额外参数
	}

	resultJSON, _ := json.Marshal(result.GetCntOptionContents())
	logger.L(ctx).Infow("ScoringParamsFromMap: final result",
		"cnt_option_contents", string(resultJSON),
	)

	return result
}

// getTypeName 获取类型的字符串表示
func getTypeName(v interface{}) string {
	if v == nil {
		return "nil"
	}
	return reflect.TypeOf(v).String()
}

// getMapKeys 获取 map 的键列表（用于日志记录）
func getMapKeys(m map[string]interface{}) []string {
	if m == nil {
		return []string{}
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// ===================== 量表类别（主类）=================

// Category 量表主类
// 每个量表只选1个主类；其余信息用标签表达
type Category string

const (
	// CategoryADHD ADHD
	CategoryADHD Category = "adhd"
	// CategoryTicDisorder 抽动障碍
	CategoryTicDisorder Category = "td"
	// CategoryASD 自闭症
	CategoryASD Category = "asd"
	// CategoryOCD 强迫症
	CategoryOCD Category = "ocd"
	// CategorySensoryIntegration 感统
	CategorySensoryIntegration Category = "sii"
	// CategoryExecutiveFunction 执行功能
	CategoryExecutiveFunction Category = "efn"
	// CategoryEmotion 情绪
	CategoryEmotion Category = "emt"
	// CategorySleep 睡眠
	CategorySleep Category = "slp"
)

// NewCategory 创建类别
func NewCategory(value string) Category {
	return Category(value)
}

// AllCategories 所有类别
var AllCategories = []Category{
	CategoryADHD, CategoryTicDisorder, CategoryASD, CategoryOCD,
	CategorySensoryIntegration, CategoryExecutiveFunction, CategoryEmotion, CategorySleep,
}

// String 返回类别的字符串表示
func (c Category) String() string {
	return string(c)
}

// Value 获取类别值
func (c Category) Value() string {
	return string(c)
}

// IsEmpty 判断类别是否为空
func (c Category) IsEmpty() bool {
	return c == ""
}

// IsValid 检查类别是否有效
func (c Category) IsValid() bool {
	if c.IsEmpty() {
		return true // 允许为空（可选字段）
	}
	switch c {
	case CategoryADHD, CategoryTicDisorder, CategoryASD, CategoryOCD,
		CategorySensoryIntegration, CategoryExecutiveFunction, CategoryEmotion, CategorySleep:
		return true
	default:
		return false
	}
}

// IsOpen 检查类别是否开放
func (c Category) IsOpen() bool {
	if c.IsEmpty() {
		return false
	}
	// 开放的类别
	switch c {
	case CategorySensoryIntegration, CategoryExecutiveFunction, CategoryEmotion, CategorySleep:
		return true
	default:
		return false
	}
}

// ===================== 量表阶段 =================

// Stage 量表阶段
type Stage string

const (
	// StageScreening 筛查
	StageScreening Stage = "screening"
	// StageDeepAssessment 深评
	StageDeepAssessment Stage = "deep_assessment"
	// StageFollowUp 随访
	StageFollowUp Stage = "follow_up"
	// StageOutcome 结局
	StageOutcome Stage = "outcome"
)

// NewStage 创建阶段
func NewStage(value string) Stage {
	return Stage(value)
}

// String 返回阶段的字符串表示
func (s Stage) String() string {
	return string(s)
}

// Value 获取阶段值
func (s Stage) Value() string {
	return string(s)
}

// IsEmpty 判断阶段是否为空
func (s Stage) IsEmpty() bool {
	return s == ""
}

// IsValid 检查阶段是否有效
func (s Stage) IsValid() bool {
	if s.IsEmpty() {
		return true // 允许为空（可选字段）
	}
	switch s {
	case StageScreening, StageDeepAssessment, StageFollowUp, StageOutcome:
		return true
	default:
		return false
	}
}

// ===================== 使用年龄 =================

// ApplicableAge 使用年龄
type ApplicableAge string

const (
	// ApplicableAgeInfant 婴幼儿
	ApplicableAgeInfant ApplicableAge = "infant"
	// ApplicableAgePreschool 学龄前
	ApplicableAgePreschool ApplicableAge = "preschool"
	// ApplicableAgeSchoolChild 学龄儿童
	ApplicableAgeSchoolChild ApplicableAge = "school_child"
	// ApplicableAgeAdolescent 青少年
	ApplicableAgeAdolescent ApplicableAge = "adolescent"
	// ApplicableAgeAdult 成人
	ApplicableAgeAdult ApplicableAge = "adult"
)

// NewApplicableAge 创建使用年龄
func NewApplicableAge(value string) ApplicableAge {
	return ApplicableAge(value)
}

// String 返回使用年龄的字符串表示
func (a ApplicableAge) String() string {
	return string(a)
}

// Value 获取使用年龄值
func (a ApplicableAge) Value() string {
	return string(a)
}

// IsEmpty 判断使用年龄是否为空
func (a ApplicableAge) IsEmpty() bool {
	return a == ""
}

// IsValid 检查使用年龄是否有效
func (a ApplicableAge) IsValid() bool {
	if a.IsEmpty() {
		return true // 允许为空（可选字段）
	}
	switch a {
	case ApplicableAgeInfant, ApplicableAgePreschool,
		ApplicableAgeSchoolChild, ApplicableAgeAdolescent, ApplicableAgeAdult:
		return true
	default:
		return false
	}
}

// ===================== 填报人 =================

// Reporter 填报人
type Reporter string

const (
	// ReporterParent 家长评
	ReporterParent Reporter = "parent"
	// ReporterTeacher 教师评
	ReporterTeacher Reporter = "teacher"
	// ReporterSelf 自评
	ReporterSelf Reporter = "self"
	// ReporterClinical 临床评定
	ReporterClinical Reporter = "clinical"
)

// NewReporter 创建填报人
func NewReporter(value string) Reporter {
	return Reporter(value)
}

// String 返回填报人的字符串表示
func (r Reporter) String() string {
	return string(r)
}

// Value 获取填报人值
func (r Reporter) Value() string {
	return string(r)
}

// IsEmpty 判断填报人是否为空
func (r Reporter) IsEmpty() bool {
	return r == ""
}

// IsValid 检查填报人是否有效
func (r Reporter) IsValid() bool {
	if r.IsEmpty() {
		return true // 允许为空（可选字段）
	}
	switch r {
	case ReporterParent, ReporterTeacher, ReporterSelf, ReporterClinical:
		return true
	default:
		return false
	}
}

// ===================== 标签 =================

// Tag 量表标签
// 用于表达除主类外的其他信息，标签值通过后台输入设置，不再固定常量
type Tag string

// NewTag 创建标签
func NewTag(value string) Tag {
	return Tag(value)
}

// String 返回标签的字符串表示
func (t Tag) String() string {
	return string(t)
}

// Value 获取标签值
func (t Tag) Value() string {
	return string(t)
}

// IsEmpty 判断标签是否为空
func (t Tag) IsEmpty() bool {
	return t == ""
}

// Validate 验证标签格式（只验证格式，不验证固定值）
// 标签允许：字母、数字、下划线、中文字符，长度1-50
func (t Tag) Validate() error {
	if t.IsEmpty() {
		return nil // 允许为空
	}

	value := t.String()
	if len(value) == 0 {
		return errors.WithCode(code.ErrInvalidArgument, "标签不能为空")
	}
	if len(value) > 50 {
		return errors.WithCode(code.ErrInvalidArgument, "标签长度不能超过50个字符")
	}

	// 验证字符格式：只允许字母、数字、下划线、中文字符
	matched, _ := regexp.MatchString(`^[\w\p{Han}]+$`, value)
	if !matched {
		return errors.WithCode(code.ErrInvalidArgument, "标签只能包含字母、数字、下划线和中文")
	}

	return nil
}
