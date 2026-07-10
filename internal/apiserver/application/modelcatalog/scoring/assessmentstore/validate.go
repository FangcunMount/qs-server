package assessmentstore

import (
	"fmt"
	"sort"
	"strings"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/scale"
)

// ValidateScaleForPublish checks scale-specific publish rules on the definition envelope.
func ValidateScaleForPublish(model *domain.AssessmentModel) error {
	if model == nil {
		return fmt.Errorf("model: assessment model is nil")
	}
	snapshot, err := scaleSnapshotFromDefinitionPayload(model.Definition)
	if err != nil {
		return err
	}

	var errs []string
	if model.Title == "" {
		errs = append(errs, "title: 量表标题不能为空")
	}
	if model.Code == "" {
		errs = append(errs, "code: 量表编码不能为空")
	}
	if len(snapshot.Factors) == 0 {
		errs = append(errs, "factors: 量表必须至少包含一个因子")
	}
	if !hasTotalScoreFactor(snapshot.Factors) {
		errs = append(errs, "factors: 量表必须包含一个总分因子")
	}
	for _, factor := range snapshot.Factors {
		errs = append(errs, validatePublishFactor(factor)...)
	}
	if bindingQuestionnaireCode(model, snapshot) == "" {
		errs = append(errs, "questionnaireCode: 量表必须关联一个问卷")
	}
	if bindingQuestionnaireVersion(model, snapshot) == "" {
		errs = append(errs, "questionnaireVersion: 量表必须指定关联问卷的版本")
	}
	if len(errs) > 0 {
		if len(errs) == 1 {
			return fmt.Errorf("%s", errs[0])
		}
		return fmt.Errorf("验证失败：%s", strings.Join(errs, "; "))
	}
	return nil
}

func scaleSnapshotFromDefinitionPayload(payload domain.DefinitionPayload) (*scalesnapshot.ScaleSnapshot, error) {
	if payload.Format != "" && payload.Format != domain.PayloadFormatAssessmentScaleV1 {
		return nil, fmt.Errorf("unsupported scale definition payload format %s", payload.Format)
	}
	if len(payload.Data) == 0 {
		return nil, fmt.Errorf("scale definition payload is empty")
	}
	return scalesnapshot.ParsePublishedPayload(payload.Data)
}

func hasTotalScoreFactor(factors []scalesnapshot.FactorSnapshot) bool {
	for _, factor := range factors {
		if factor.IsTotalScore {
			return true
		}
	}
	return false
}

func validatePublishFactor(factor scalesnapshot.FactorSnapshot) []string {
	var errs []string
	factorCode := factor.Code
	if factorCode == "" {
		factorCode = "<empty>"
		errs = append(errs, "factor[<empty>].code: 因子编码不能为空")
	}
	if factor.Title == "" {
		errs = append(errs, fmt.Sprintf("factor[%s].title: 因子标题不能为空", factorCode))
	}
	strategy := factor.ScoringStrategy
	if strategy == "" {
		strategy = "sum"
	}
	if !isValidScoringStrategy(strategy) {
		errs = append(errs, fmt.Sprintf("factor[%s].scoringStrategy: 计分策略无效", factorCode))
	}
	if factor.MaxScore != nil && *factor.MaxScore <= 0 {
		errs = append(errs, fmt.Sprintf("factor[%s].scoringSpec: max score must be greater than 0", factorCode))
	}
	if strategy == "cnt" && len(factor.ScoringParams.CntOptionContents) == 0 {
		errs = append(errs, fmt.Sprintf("factor[%s].scoringSpec: cnt scoring strategy requires cnt_option_contents", factorCode))
	}
	if questionErr := validatePublishQuestionCodes(factor); questionErr != "" {
		errs = append(errs, fmt.Sprintf("factor[%s].questionCodes: %s", factorCode, questionErr))
	}
	if !factor.IsTotalScore && len(factor.QuestionCodes) == 0 {
		errs = append(errs, fmt.Sprintf("factor[%s].questionCodes: 非总分因子必须包含至少一个题目", factorCode))
	}
	if len(factor.InterpretRules) == 0 {
		errs = append(errs, fmt.Sprintf("factor[%s].interpretRules: 因子必须包含至少一个解读规则", factorCode))
	}
	errs = append(errs, validatePublishInterpretRules(factorCode, factor.InterpretRules)...)
	return errs
}

func validatePublishQuestionCodes(factor scalesnapshot.FactorSnapshot) string {
	if !factor.IsTotalScore && len(factor.QuestionCodes) == 0 {
		return "non-total-score factor requires question codes"
	}
	seen := make(map[string]struct{}, len(factor.QuestionCodes))
	for _, code := range factor.QuestionCodes {
		if code == "" {
			return "question code cannot be empty"
		}
		if _, ok := seen[code]; ok {
			return fmt.Sprintf("duplicate question code: %s", code)
		}
		seen[code] = struct{}{}
	}
	return ""
}

func validatePublishInterpretRules(factorCode string, rules []scalesnapshot.InterpretRuleSnapshot) []string {
	if len(rules) == 0 {
		return nil
	}
	ordered := append([]scalesnapshot.InterpretRuleSnapshot(nil), rules...)
	sort.SliceStable(ordered, func(i, j int) bool {
		return ordered[i].Min < ordered[j].Min
	})
	var errs []string
	for i, rule := range ordered {
		if rule.Min >= rule.Max || !isValidRiskLevel(rule.RiskLevel) {
			errs = append(errs, fmt.Sprintf("factor[%s].interpretRules[%d]: 解读规则无效（分数区间或风险等级不正确）", factorCode, i))
		}
		if i == 0 {
			continue
		}
		previous := ordered[i-1]
		if previous.Max > rule.Min {
			errs = append(errs, fmt.Sprintf(
				"factor[%s].interpretRules: interpretation rules overlap: [%.2f, %.2f) and [%.2f, %.2f)",
				factorCode,
				previous.Min, previous.Max,
				rule.Min, rule.Max,
			))
		}
	}
	return errs
}

func bindingQuestionnaireCode(model *domain.AssessmentModel, snapshot *scalesnapshot.ScaleSnapshot) string {
	if model != nil && model.Binding.QuestionnaireCode != "" {
		return model.Binding.QuestionnaireCode
	}
	if snapshot != nil {
		return snapshot.QuestionnaireCode
	}
	return ""
}

func bindingQuestionnaireVersion(model *domain.AssessmentModel, snapshot *scalesnapshot.ScaleSnapshot) string {
	if model != nil && model.Binding.QuestionnaireVersion != "" {
		return model.Binding.QuestionnaireVersion
	}
	if snapshot != nil {
		return snapshot.QuestionnaireVersion
	}
	return ""
}

func isValidScoringStrategy(strategy string) bool {
	switch strategy {
	case "sum", "avg", "cnt":
		return true
	default:
		return false
	}
}

func isValidRiskLevel(level string) bool {
	switch level {
	case "none", "low", "medium", "high", "severe":
		return true
	default:
		return false
	}
}
