package port

// Factor 因子接口
type Factor interface {
	Code() string
	Title() string
	IsTotalScore() bool
	Type() FactorType
	CalculationRule() CalculationRule
	InterpretRules() []InterpretRule
}

// CalculationRule 计算规则接口
type CalculationRule interface {
	FormulaType() FormulaType
	SourceCodes() []string
}

// InterpretRule 解读规则接口
type InterpretRule interface {
	ScoreRange() ScoreRange
	Content() string
}

// ScoreRange 分数范围接口
type ScoreRange interface {
	MinScore() float64
	MaxScore() float64
}

// FactorType 因子类型
type FactorType string

// String 返回因子类型字符串
func (t FactorType) String() string {
	return string(t)
}

// FormulaType 公式类型
type FormulaType string

// String 返回公式类型字符串
func (t FormulaType) String() string {
	return string(t)
}

// NewFactor 创建因子
func NewFactor(
	code string,
	title string,
	isTotalScore bool,
	factorType FactorType,
	calculationRule CalculationRule,
	interpretRules []InterpretRule,
) Factor {
	return &factorImpl{
		code:            code,
		title:           title,
		isTotalScore:    isTotalScore,
		factorType:      factorType,
		calculationRule: calculationRule,
		interpretRules:  interpretRules,
	}
}

// NewCalculationRule 创建计算规则
func NewCalculationRule(formulaType FormulaType, sourceCodes []string) CalculationRule {
	return &calculationRuleImpl{
		formulaType: formulaType,
		sourceCodes: sourceCodes,
	}
}

// NewInterpretRule 创建解读规则
func NewInterpretRule(scoreRange ScoreRange, content string) InterpretRule {
	return &interpretRuleImpl{
		scoreRange: scoreRange,
		content:    content,
	}
}

// NewScoreRange 创建分数范围
func NewScoreRange(minScore, maxScore float64) ScoreRange {
	return &scoreRangeImpl{
		minScore: minScore,
		maxScore: maxScore,
	}
}

// factorImpl 因子实现
type factorImpl struct {
	code            string
	title           string
	isTotalScore    bool
	factorType      FactorType
	calculationRule CalculationRule
	interpretRules  []InterpretRule
}

func (f *factorImpl) Code() string                     { return f.code }
func (f *factorImpl) Title() string                    { return f.title }
func (f *factorImpl) IsTotalScore() bool               { return f.isTotalScore }
func (f *factorImpl) Type() FactorType                 { return f.factorType }
func (f *factorImpl) CalculationRule() CalculationRule { return f.calculationRule }
func (f *factorImpl) InterpretRules() []InterpretRule  { return f.interpretRules }

// calculationRuleImpl 计算规则实现
type calculationRuleImpl struct {
	formulaType FormulaType
	sourceCodes []string
}

func (r *calculationRuleImpl) FormulaType() FormulaType { return r.formulaType }
func (r *calculationRuleImpl) SourceCodes() []string    { return r.sourceCodes }

// interpretRuleImpl 解读规则实现
type interpretRuleImpl struct {
	scoreRange ScoreRange
	content    string
}

func (r *interpretRuleImpl) ScoreRange() ScoreRange { return r.scoreRange }
func (r *interpretRuleImpl) Content() string        { return r.content }

// scoreRangeImpl 分数范围实现
type scoreRangeImpl struct {
	minScore float64
	maxScore float64
}

func (r *scoreRangeImpl) MinScore() float64 { return r.minScore }
func (r *scoreRangeImpl) MaxScore() float64 { return r.maxScore }
