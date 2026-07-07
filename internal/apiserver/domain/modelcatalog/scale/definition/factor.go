package definition

import (
	"slices"

	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// Factor 因子实体
// 因子是量表的组成部分，代表一个测量维度
type Factor struct {
	// 基本信息
	code       FactorCode
	title      string
	factorType FactorType

	// 是否为总分因子
	isTotalScore bool

	// 是否显示（用于报告中的维度展示）
	isShow bool

	// 关联的题目编码列表
	questionCodes []meta.Code

	// 计分策略配置
	scoringSpec ScoringSpec

	// 解读规则
	interpretRules InterpretationRules

	optionErr error
}

// FactorSnapshot 是 Factor 的只读快照，供查询/跨层转换使用。
type FactorSnapshot struct {
	Code            FactorCode
	Title           string
	FactorType      FactorType
	IsTotalScore    bool
	IsShow          bool
	QuestionCodes   []meta.Code
	ScoringStrategy ScoringStrategyCode
	ScoringParams   *ScoringParams
	MaxScore        *float64
	InterpretRules  []InterpretationRule
}

// ===================== Factor 构造相关 =================

// FactorOption 因子构造选项
type FactorOption func(*Factor)

// NewFactor 创建因子
func NewFactor(factorCode FactorCode, title string, opts ...FactorOption) (*Factor, error) {
	if factorCode.IsEmpty() {
		return nil, newError(ErrorKindInvalidArgument, "factor code cannot be empty")
	}
	if title == "" {
		return nil, newError(ErrorKindInvalidArgument, "factor title cannot be empty")
	}

	f := &Factor{
		code:           factorCode,
		title:          title,
		factorType:     FactorTypePrimary,
		scoringSpec:    defaultScoringSpec(),
		interpretRules: MustInterpretationRules(nil),
		isShow:         true, // 默认显示
	}

	for _, opt := range opts {
		opt(f)
	}
	if f.optionErr != nil {
		return nil, f.optionErr
	}
	if err := f.validate(); err != nil {
		return nil, err
	}

	return f, nil
}

// With*** 构造选项

// WithFactorType 设置因子类型
func WithFactorType(ft FactorType) FactorOption {
	return func(f *Factor) {
		f.factorType = ft
	}
}

// WithIsTotalScore 设置是否为总分因子
func WithIsTotalScore(isTotalScore bool) FactorOption {
	return func(f *Factor) {
		f.isTotalScore = isTotalScore
	}
}

// WithIsS如何设置是否显示
func WithIsShow(isShow bool) FactorOption {
	return func(f *Factor) {
		f.isShow = isShow
	}
}

// WithQuestionCodes 设置关联的题目编码
func WithQuestionCodes(codes []meta.Code) FactorOption {
	return func(f *Factor) {
		f.questionCodes = slices.Clone(codes)
	}
}

// WithScoringStrategy 设置计分策略
func WithScoringStrategy(strategy ScoringStrategyCode) FactorOption {
	return func(f *Factor) {
		f.scoringSpec = f.scoringSpec.withStrategy(strategy)
	}
}

// WithScoringParams 设置计分参数
func WithScoringParams(params *ScoringParams) FactorOption {
	return func(f *Factor) {
		f.scoringSpec = f.scoringSpec.withParams(params)
	}
}

// WithInterpretRules 设置解读规则
func WithInterpretRules(rules []InterpretationRule) FactorOption {
	return func(f *Factor) {
		interpretRules, err := NewInterpretationRules(rules)
		if err != nil {
			f.optionErr = err
			return
		}
		f.interpretRules = interpretRules
	}
}

// WithMaxScore 设置最大分
func WithMaxScore(maxScore *float64) FactorOption {
	return func(f *Factor) {
		f.scoringSpec = f.scoringSpec.withMaxScore(maxScore)
	}
}

func WithScoringSpec(spec ScoringSpec) FactorOption {
	return func(f *Factor) {
		f.scoringSpec = spec
	}
}

// ===================== Getter 方法 =================

// GetCode 获取因子编码
func (f *Factor) GetCode() FactorCode {
	return f.code
}

// GetTitle 获取因子标题
func (f *Factor) GetTitle() string {
	return f.title
}

// GetFactorType 获取因子类型
func (f *Factor) GetFactorType() FactorType {
	return f.factorType
}

// IsTotalScore 是否为总分因子
func (f *Factor) IsTotalScore() bool {
	return f.isTotalScore
}

// IsS如何是否显示
func (f *Factor) IsShow() bool {
	return f.isShow
}

// GetQuestionCodes 获取关联的题目编码
func (f *Factor) GetQuestionCodes() []meta.Code {
	return slices.Clone(f.questionCodes)
}

// GetScoringStrategy 获取计分策略
func (f *Factor) GetScoringStrategy() ScoringStrategyCode {
	return f.scoringSpec.Strategy()
}

// GetScoringParams 获取计分参数
func (f *Factor) GetScoringParams() *ScoringParams {
	return f.scoringSpec.Params()
}

// GetInterpretRules 获取解读规则
func (f *Factor) GetInterpretRules() []InterpretationRule {
	return f.interpretRules.Items()
}

// GetMaxScore 获取最大分
func (f *Factor) GetMaxScore() *float64 {
	return f.scoringSpec.MaxScore()
}

func (f *Factor) GetScoringSpec() ScoringSpec {
	return f.scoringSpec
}

func (f *Factor) Snapshot() FactorSnapshot {
	return FactorSnapshot{
		Code:            f.code,
		Title:           f.title,
		FactorType:      f.factorType,
		IsTotalScore:    f.isTotalScore,
		IsShow:          f.isShow,
		QuestionCodes:   f.GetQuestionCodes(),
		ScoringStrategy: f.GetScoringStrategy(),
		ScoringParams:   f.GetScoringParams(),
		MaxScore:        f.GetMaxScore(),
		InterpretRules:  f.GetInterpretRules(),
	}
}

// ===================== 业务方法 =================

// QuestionCount 获取因子包含的题目数量
func (f *Factor) QuestionCount() int {
	return len(f.questionCodes)
}

// ContainsQuestion 判断因子是否包含指定题目
func (f *Factor) ContainsQuestion(questionCode meta.Code) bool {
	for _, qc := range f.questionCodes {
		if qc == questionCode {
			return true
		}
	}
	return false
}

// FindInterpretRule 根据分数查找匹配的解读规则
func (f *Factor) FindInterpretRule(score float64) *InterpretationRule {
	rule, ok := f.interpretRules.Match(score)
	if !ok {
		return nil
	}
	return &rule
}

// ===================== 包内私有方法（供领域服务调用）=================

// updateInterpretRules 更新解读规则
func (f *Factor) updateInterpretRules(rules []InterpretationRule) error {
	interpretRules, err := NewInterpretationRules(rules)
	if err != nil {
		return err
	}
	f.interpretRules = interpretRules
	return nil
}

// addInterpretRule 添加解读规则
func (f *Factor) addInterpretRule(rule InterpretationRule) error {
	interpretRules, err := f.interpretRules.WithAppended(rule)
	if err != nil {
		return err
	}
	f.interpretRules = interpretRules
	return nil
}

func (f *Factor) validate() error {
	if !f.factorType.IsValid() {
		return newError(ErrorKindInvalidArgument, "invalid factor type: %s", f.factorType)
	}
	if err := f.scoringSpec.Validate(); err != nil {
		return err
	}
	if err := validateFactorQuestionCodes(f.isTotalScore, f.questionCodes); err != nil {
		return err
	}
	return nil
}

func validateFactorQuestionCodes(isTotalScore bool, codes []meta.Code) error {
	if !isTotalScore && len(codes) == 0 {
		return newError(ErrorKindInvalidArgument, "non-total-score factor requires question codes")
	}
	seen := make(map[string]struct{}, len(codes))
	for _, code := range codes {
		if code.IsEmpty() {
			return newError(ErrorKindInvalidArgument, "question code cannot be empty")
		}
		value := code.Value()
		if _, ok := seen[value]; ok {
			return newError(ErrorKindInvalidArgument, "duplicate question code: %s", value)
		}
		seen[value] = struct{}{}
	}
	return nil
}
