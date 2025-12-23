package scale

import (
	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
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
	scoringStrategy ScoringStrategyCode
	scoringParams   *ScoringParams

	// 最大分
	maxScore *float64

	// 解读规则
	interpretRules []InterpretationRule
}

// ===================== Factor 构造相关 =================

// FactorOption 因子构造选项
type FactorOption func(*Factor)

// NewFactor 创建因子
func NewFactor(factorCode FactorCode, title string, opts ...FactorOption) (*Factor, error) {
	if factorCode.IsEmpty() {
		return nil, errors.WithCode(code.ErrInvalidArgument, "factor code cannot be empty")
	}
	if title == "" {
		return nil, errors.WithCode(code.ErrInvalidArgument, "factor title cannot be empty")
	}

	f := &Factor{
		code:            factorCode,
		title:           title,
		factorType:      FactorTypePrimary,
		scoringStrategy: ScoringStrategySum,
		scoringParams:   NewScoringParams(),
		isShow:          true, // 默认显示
	}

	for _, opt := range opts {
		opt(f)
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

// WithIsShow 设置是否显示
func WithIsShow(isShow bool) FactorOption {
	return func(f *Factor) {
		f.isShow = isShow
	}
}

// WithQuestionCodes 设置关联的题目编码
func WithQuestionCodes(codes []meta.Code) FactorOption {
	return func(f *Factor) {
		f.questionCodes = codes
	}
}

// WithScoringStrategy 设置计分策略
func WithScoringStrategy(strategy ScoringStrategyCode) FactorOption {
	return func(f *Factor) {
		f.scoringStrategy = strategy
	}
}

// WithScoringParams 设置计分参数
func WithScoringParams(params *ScoringParams) FactorOption {
	return func(f *Factor) {
		if params == nil {
			f.scoringParams = NewScoringParams()
		} else {
			f.scoringParams = params
		}
	}
}

// WithInterpretRules 设置解读规则
func WithInterpretRules(rules []InterpretationRule) FactorOption {
	return func(f *Factor) {
		f.interpretRules = rules
	}
}

// WithMaxScore 设置最大分
func WithMaxScore(maxScore *float64) FactorOption {
	return func(f *Factor) {
		f.maxScore = maxScore
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

// IsShow 是否显示
func (f *Factor) IsShow() bool {
	return f.isShow
}

// GetQuestionCodes 获取关联的题目编码
func (f *Factor) GetQuestionCodes() []meta.Code {
	return f.questionCodes
}

// GetScoringStrategy 获取计分策略
func (f *Factor) GetScoringStrategy() ScoringStrategyCode {
	return f.scoringStrategy
}

// GetScoringParams 获取计分参数
func (f *Factor) GetScoringParams() *ScoringParams {
	if f.scoringParams == nil {
		return NewScoringParams()
	}
	return f.scoringParams
}

// GetInterpretRules 获取解读规则
func (f *Factor) GetInterpretRules() []InterpretationRule {
	return f.interpretRules
}

// GetMaxScore 获取最大分
func (f *Factor) GetMaxScore() *float64 {
	return f.maxScore
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
	for _, rule := range f.interpretRules {
		if rule.Matches(score) {
			return &rule
		}
	}
	return nil
}

// ===================== 包内私有方法（供领域服务调用）=================

// updateTitle 更新因子标题
func (f *Factor) updateTitle(title string) error {
	if title == "" {
		return errors.WithCode(code.ErrInvalidArgument, "factor title cannot be empty")
	}
	f.title = title
	return nil
}

// updateQuestionCodes 更新关联的题目编码
func (f *Factor) updateQuestionCodes(codes []meta.Code) {
	f.questionCodes = codes
}

// updateScoringStrategy 更新计分策略
func (f *Factor) updateScoringStrategy(strategy ScoringStrategyCode, params *ScoringParams) {
	f.scoringStrategy = strategy
	if params == nil {
		f.scoringParams = NewScoringParams()
	} else {
		f.scoringParams = params
	}
}

// updateInterpretRules 更新解读规则
func (f *Factor) updateInterpretRules(rules []InterpretationRule) {
	f.interpretRules = rules
}

// addInterpretRule 添加解读规则
func (f *Factor) addInterpretRule(rule InterpretationRule) {
	f.interpretRules = append(f.interpretRules, rule)
}
