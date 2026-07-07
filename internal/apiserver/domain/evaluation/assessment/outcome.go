package assessment

// DimensionKind 划分维度结果 独立于 scale 因子 semantics。
type DimensionKind string

const (
	DimensionKindFactor  DimensionKind = "factor"
	DimensionKindPole    DimensionKind = "pole"
	DimensionKindTrait   DimensionKind = "trait"
	DimensionKindIndex   DimensionKind = "index"
	DimensionKindAbility DimensionKind = "ability"
)

// OutcomeScoreKind 划分主 或 维度分 value。
type OutcomeScoreKind string

const (
	OutcomeScoreKindRawTotal     OutcomeScoreKind = "raw_total"
	OutcomeScoreKindMatchPercent OutcomeScoreKind = "match_percent"
	OutcomeScoreKindTScore       OutcomeScoreKind = "t_score"
	OutcomeScoreKindPercentile   OutcomeScoreKind = "percentile"
)

// ProfileKind 划分画像结果 独立于 模型家族。
type ProfileKind string

const (
	ProfileKindPersonalityType  ProfileKind = "personality_type"
	ProfileKindPersonalityTrait ProfileKind = "personality_trait"
	ProfileKindAbilityProfile   ProfileKind = "ability_profile"
)

// ProfileResult 记录portrait-style 结果s such 作为 人格类型 或 ability 画像。
type ProfileResult struct {
	Kind        ProfileKind
	Code        string
	Name        string
	Summary     string
	Traits      []string
	Strengths   []string
	Weaknesses  []string
	Suggestions []string
}

// OutcomeScoreValue 是规范 score re呈现 on 测评结果。
type OutcomeScoreValue struct {
	Kind  OutcomeScoreKind
	Value float64
	Label string
	Max   *float64
}

// OutcomeResultLevel 是规范 等级 re呈现 on 测评结果。
type OutcomeResultLevel struct {
	Code     string
	Label    string
	Severity string
}

// DimensionResult 记录一个scored 维度 on 测评结果。
type DimensionResult struct {
	Code           string
	Name           string
	Kind           DimensionKind
	Role           string
	ParentCode     string
	HierarchyLevel int
	SortOrder      int
	Score          *OutcomeScoreValue
	DerivedScores  []OutcomeScoreValue
	Level          *OutcomeResultLevel
	Description    string
	Suggestion     string
}

// ValidityResult 记录可选 有效ity checks 用于 测评结果。
type ValidityResult struct {
	Code    string
	Label   string
	Passed  bool
	Message string
}

// AssessmentOutcome 是规范 执行结果 用于 全部模型家族。
type AssessmentOutcome struct {
	ModelRef   EvaluationModelRef
	Summary    ResultSummary
	Detail     EvaluationDetail
	Primary    *OutcomeScoreValue
	Level      *OutcomeResultLevel
	Profile    *ProfileResult
	Dimensions []DimensionResult
	Validity   []ValidityResult
}

// NewAssessmentOutcome 创建规范 测评结果。
func NewAssessmentOutcome(
	modelRef EvaluationModelRef,
	summary ResultSummary,
	detail EvaluationDetail,
) *AssessmentOutcome {
	if detail.Kind == "" {
		detail.Kind = modelRef.Kind()
	}
	return &AssessmentOutcome{
		ModelRef:   modelRef,
		Summary:    summary,
		Detail:     detail,
		Dimensions: make([]DimensionResult, 0),
		Validity:   make([]ValidityResult, 0),
	}
}

// AssessmentOutcomeFromEvaluationResult 适配旧版 评估 结果。
//
// Deprecated: 仅用于表征和 ApplyEvaluation 适配；应用写路径必须直接使用 AssessmentOutcome。
func AssessmentOutcomeFromEvaluationResult(result *EvaluationResult) *AssessmentOutcome {
	if result == nil {
		return nil
	}
	outcome := NewAssessmentOutcome(result.ModelRef, result.Summary, result.Detail)
	if result.Summary.Score != nil {
		outcome.Primary = &OutcomeScoreValue{
			Kind:  OutcomeScoreKindRawTotal,
			Value: *result.Summary.Score,
		}
	} else if result.TotalScore != 0 {
		outcome.Primary = &OutcomeScoreValue{
			Kind:  OutcomeScoreKindRawTotal,
			Value: result.TotalScore,
		}
	}
	if result.Summary.Level != nil && *result.Summary.Level != "" {
		outcome.Level = &OutcomeResultLevel{Code: *result.Summary.Level}
	} else if result.RiskLevel != "" {
		outcome.Level = &OutcomeResultLevel{Code: string(result.RiskLevel)}
	}
	if result.Summary.PrimaryLabel != "" && outcome.Level != nil && outcome.Level.Label == "" {
		outcome.Level.Label = result.Summary.PrimaryLabel
	}
	outcome.Dimensions = dimensionResultsFromFactorScores(result.FactorScores)
	return outcome
}

// ToEvaluationResult 投影结果 为 旧写模型。
func (o *AssessmentOutcome) ToEvaluationResult() *EvaluationResult {
	if o == nil {
		return nil
	}
	result := NewModelEvaluationResult(o.ModelRef, o.Summary, o.Detail)
	if o.Primary != nil {
		result.TotalScore = o.Primary.Value
		if result.Summary.Score == nil {
			score := o.Primary.Value
			result.Summary.Score = &score
		}
	}
	if o.Level != nil && o.Level.Code != "" {
		if IsRiskLevelCode(o.Level.Code) {
			result.RiskLevel = RiskLevel(o.Level.Code)
			level := o.Level.Code
			if result.Summary.Level == nil {
				result.Summary.Level = &level
			}
		}
		if result.Summary.PrimaryLabel == "" {
			if o.Level.Label != "" {
				result.Summary.PrimaryLabel = o.Level.Label
			} else if !IsRiskLevelCode(o.Level.Code) {
				result.Summary.PrimaryLabel = o.Level.Code
			}
		}
	}
	if scores, ok := o.Detail.Payload.([]FactorScoreResult); ok && len(scores) > 0 {
		result.FactorScores = scores
	} else if len(o.Dimensions) > 0 {
		result.FactorScores = factorScoreResultsFromDimensions(o.Dimensions)
	}
	if result.Conclusion == "" && o.Summary.PrimaryLabel != "" {
		result.Conclusion = o.Summary.PrimaryLabel
	}
	return result
}

func dimensionResultsFromFactorScores(scores []FactorScoreResult) []DimensionResult {
	results := make([]DimensionResult, 0, len(scores))
	for _, score := range scores {
		dim := DimensionResult{
			Code: score.FactorCode.String(),
			Name: score.FactorName,
			Kind: DimensionKindFactor,
			Score: &OutcomeScoreValue{
				Kind:  OutcomeScoreKindRawTotal,
				Value: score.RawScore,
			},
			Description: score.Conclusion,
			Suggestion:  score.Suggestion,
		}
		if score.RiskLevel != "" {
			dim.Level = &OutcomeResultLevel{Code: string(score.RiskLevel)}
		}
		results = append(results, dim)
	}
	return results
}

func factorScoreResultsFromDimensions(dimensions []DimensionResult) []FactorScoreResult {
	results := make([]FactorScoreResult, 0, len(dimensions))
	for _, dim := range dimensions {
		if dim.Score == nil {
			continue
		}
		risk := RiskLevelNone
		if dim.Level != nil && IsRiskLevelCode(dim.Level.Code) {
			risk = RiskLevel(dim.Level.Code)
		}
		results = append(results, FactorScoreResult{
			FactorCode: NewFactorCode(dim.Code),
			FactorName: dim.Name,
			RawScore:   dim.Score.Value,
			RiskLevel:  risk,
			Conclusion: dim.Description,
			Suggestion: dim.Suggestion,
		})
	}
	return results
}
