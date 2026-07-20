package definition

import (
	"fmt"
	"sort"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/conclusion"
)

// ValidationIssue reports one Definition-level invariant violation.
type ValidationIssue struct {
	Field   string
	Code    string
	Message string
}

// Validate checks references and semantic configuration across all Definition layers.
func Validate(def Definition) []ValidationIssue {
	issues := make([]ValidationIssue, 0)
	for _, issue := range ValidateMeasureSpec(def.Measure) {
		issues = append(issues, ValidationIssue{Field: "measure." + issue.Field, Code: issue.Code, Message: issue.Message})
	}
	factorCodes := makeStringSet()
	for _, item := range def.Measure.Factors {
		if item.Code != "" {
			factorCodes[item.Code] = struct{}{}
		}
	}
	issues = append(issues, validateCalibration(def.Calibration, factorCodes)...)
	issues = append(issues, validateExecution(def.Execution, factorCodes)...)
	outcomeCodes, outcomeIssues := validateOutcomes(def.Outcomes)
	issues = append(issues, outcomeIssues...)
	issues = append(issues, validateConclusions(def.Conclusions, factorCodes, outcomeCodes)...)
	issues = append(issues, validateReportMap(def.ReportMap)...)
	return issues
}

func validateExecution(spec ExecutionSpec, factorCodes map[string]struct{}) []ValidationIssue {
	issues := make([]ValidationIssue, 0)
	if spec.Brief2 != nil && spec.SPM != nil {
		issues = append(issues, ValidationIssue{Field: "execution", Code: "execution.multiple", Message: "only one algorithm execution spec may be configured"})
	}
	if brief2 := spec.Brief2; brief2 != nil {
		if _, ok := factorCodes[brief2.PrimaryFactorCode]; brief2.PrimaryFactorCode == "" || !ok {
			issues = append(issues, ValidationIssue{Field: "execution.brief2.primary_factor_code", Code: "brief2.primary_factor.not_found", Message: "brief2 primary factor must be defined"})
		}
		issues = append(issues, validateExecutionFactorCodes("execution.brief2.index_factor_codes", brief2.IndexFactorCodes, factorCodes)...)
		issues = append(issues, validateExecutionFactorCodes("execution.brief2.validity_factor_codes", brief2.ValidityFactorCodes, factorCodes)...)
	}
	if spm := spec.SPM; spm != nil {
		if spm.TimeLimitSeconds <= 0 {
			issues = append(issues, ValidationIssue{Field: "execution.spm.time_limit_seconds", Code: "spm.time_limit.required", Message: "spm time limit must be positive"})
		}
		if _, ok := factorCodes[spm.TotalFactorCode]; spm.TotalFactorCode == "" || !ok {
			issues = append(issues, ValidationIssue{Field: "execution.spm.total_factor_code", Code: "spm.total_factor.not_found", Message: "spm total factor must be defined"})
		}
		seenQuestions := makeStringSet()
		for _, set := range spm.ItemSets {
			if set.Code == "" || len(set.Items) == 0 {
				issues = append(issues, ValidationIssue{Field: "execution.spm.item_sets", Code: "spm.item_set.invalid", Message: "spm item sets require code and items"})
			}
			for _, item := range set.Items {
				if item.QuestionCode == "" || item.CorrectOptionCode == "" {
					issues = append(issues, ValidationIssue{Field: "execution.spm.item_sets", Code: "spm.item.invalid", Message: "spm items require question and correct option codes"})
					continue
				}
				if _, duplicate := seenQuestions[item.QuestionCode]; duplicate {
					issues = append(issues, ValidationIssue{Field: "execution.spm.item_sets", Code: "spm.question.duplicate", Message: fmt.Sprintf("spm question %s is duplicated", item.QuestionCode)})
				}
				seenQuestions[item.QuestionCode] = struct{}{}
			}
		}
		if len(spm.ItemSets) == 0 {
			issues = append(issues, ValidationIssue{Field: "execution.spm.item_sets", Code: "spm.item_sets.required", Message: "spm requires item sets"})
		}
	}
	return issues
}

func validateExecutionFactorCodes(field string, codes []string, factorCodes map[string]struct{}) []ValidationIssue {
	issues := make([]ValidationIssue, 0)
	for _, code := range codes {
		if _, ok := factorCodes[code]; code == "" || !ok {
			issues = append(issues, ValidationIssue{Field: field, Code: "execution.factor.not_found", Message: fmt.Sprintf("execution factor %s is not defined", code)})
		}
	}
	return issues
}

func validateCalibration(calibration Calibration, factorCodes map[string]struct{}) []ValidationIssue {
	issues := make([]ValidationIssue, 0)
	seen := makeStringSet()
	for _, ref := range calibration.NormRefs {
		key := ref.FactorCode + "@" + ref.NormTableVersion
		if ref.FactorCode == "" {
			issues = append(issues, ValidationIssue{Field: "calibration.norm_refs", Code: "norm_ref.factor.required", Message: "norm ref factor_code is required"})
		}
		if ref.NormTableVersion == "" {
			issues = append(issues, ValidationIssue{Field: "calibration.norm_refs", Code: "norm_ref.version.required", Message: "norm ref table version is required"})
		}
		if _, ok := factorCodes[ref.FactorCode]; ref.FactorCode != "" && !ok {
			issues = append(issues, ValidationIssue{Field: "calibration.norm_refs", Code: "norm_ref.factor.not_found", Message: fmt.Sprintf("norm ref factor %s is not defined", ref.FactorCode)})
		}
		if _, ok := seen[key]; key != "@" && ok {
			issues = append(issues, ValidationIssue{Field: "calibration.norm_refs", Code: "norm_ref.duplicate", Message: fmt.Sprintf("norm ref %s is duplicated", key)})
		}
		seen[key] = struct{}{}
	}
	return issues
}

func validateOutcomes(outcomes []conclusion.Outcome) (map[string]struct{}, []ValidationIssue) {
	codes := makeStringSet()
	issues := make([]ValidationIssue, 0)
	for _, item := range outcomes {
		if item.Code == "" {
			issues = append(issues, ValidationIssue{Field: "outcomes", Code: "outcome.code.required", Message: "outcome code is required"})
			continue
		}
		if _, ok := codes[item.Code]; ok {
			issues = append(issues, ValidationIssue{Field: "outcomes", Code: "outcome.code.duplicate", Message: fmt.Sprintf("outcome code %s is duplicated", item.Code)})
		}
		codes[item.Code] = struct{}{}
	}
	return codes, issues
}

func validateConclusions(items []conclusion.Conclusion, factorCodes, outcomeCodes map[string]struct{}) []ValidationIssue {
	issues := make([]ValidationIssue, 0)
	for _, item := range items {
		switch typed := item.(type) {
		case conclusion.RiskConclusion:
			issues = append(issues, validateFactorConclusion("risk", typed.FactorCode, "", typed.Rules, factorCodes, outcomeCodes)...)
		case conclusion.NormConclusion:
			issues = append(issues, validateFactorConclusion("norm", typed.FactorCode, typed.ScoreBasis, typed.Rules, factorCodes, outcomeCodes)...)
		case conclusion.AbilityConclusion:
			issues = append(issues, validateFactorConclusion("ability", typed.FactorCode, typed.ScoreBasis, typed.Rules, factorCodes, outcomeCodes)...)
		case conclusion.TypeConclusion:
			issues = append(issues, validateTypeConclusion(typed, factorCodes, outcomeCodes)...)
		default:
			issues = append(issues, ValidationIssue{Field: "conclusions", Code: "conclusion.kind.unsupported", Message: fmt.Sprintf("unsupported conclusion type %T", item)})
		}
	}
	return issues
}

func validateFactorConclusion(kind, factorCode string, basis conclusion.ScoreBasis, rules []conclusion.ScoreRangeOutcome, factorCodes, outcomeCodes map[string]struct{}) []ValidationIssue {
	issues := make([]ValidationIssue, 0)
	prefix := "conclusions." + kind
	if _, ok := factorCodes[factorCode]; factorCode == "" || !ok {
		issues = append(issues, ValidationIssue{Field: prefix + ".factor_code", Code: "conclusion.factor.not_found", Message: fmt.Sprintf("conclusion factor %s is not defined", factorCode)})
	}
	if kind != "risk" && !validScoreBasis(basis) {
		issues = append(issues, ValidationIssue{Field: prefix + ".score_basis", Code: "conclusion.score_basis.invalid", Message: "conclusion score basis is invalid"})
	}
	if len(rules) == 0 {
		issues = append(issues, ValidationIssue{Field: prefix + ".rules", Code: "conclusion.rules.required", Message: "conclusion requires at least one score range rule"})
		return issues
	}
	for _, rule := range rules {
		if rule.MinScore > rule.MaxScore {
			issues = append(issues, ValidationIssue{Field: prefix + ".rules", Code: "conclusion.range.invalid", Message: "conclusion range min_score must not exceed max_score"})
		}
		if rule.OutcomeCode == "" {
			issues = append(issues, ValidationIssue{Field: prefix + ".rules", Code: "conclusion.outcome_code.required", Message: "score range rule requires outcome_code; display text belongs in outcomes registry"})
			continue
		}
		if _, ok := outcomeCodes[rule.OutcomeCode]; !ok {
			issues = append(issues, ValidationIssue{Field: prefix + ".rules", Code: "conclusion.outcome.not_found", Message: fmt.Sprintf("outcome %s is not defined", rule.OutcomeCode)})
		}
	}
	issues = append(issues, validateScoreRangeCoverage(prefix+".rules", rules)...)
	return issues
}

// validateScoreRangeCoverage enforces the publish contract for half-open ranges
// [min, max), with the last range treated as max-inclusive for upper bound coverage.
// Adjacent ranges must meet exactly at endpoints; overlaps and gaps are rejected.
func validateScoreRangeCoverage(field string, rules []conclusion.ScoreRangeOutcome) []ValidationIssue {
	type scoredRange struct {
		min, max float64
		index    int
	}
	ordered := make([]scoredRange, 0, len(rules))
	for i, rule := range rules {
		if rule.MinScore > rule.MaxScore {
			continue
		}
		ordered = append(ordered, scoredRange{min: rule.MinScore, max: rule.MaxScore, index: i})
	}
	if len(ordered) < 2 {
		return nil
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].min == ordered[j].min {
			return ordered[i].max < ordered[j].max
		}
		return ordered[i].min < ordered[j].min
	})

	issues := make([]ValidationIssue, 0)
	for i := 0; i < len(ordered); i++ {
		for j := i + 1; j < len(ordered); j++ {
			left, right := ordered[i], ordered[j]
			// Half-open overlap: [a,b) and [c,d) overlap iff a < d && c < b.
			if left.min < right.max && right.min < left.max {
				issues = append(issues, ValidationIssue{
					Field:   field,
					Code:    "conclusion.range.overlap",
					Message: fmt.Sprintf("score ranges overlap between rules %d and %d; use adjacent [min,max) boundaries", left.index, right.index),
				})
			}
		}
	}
	for i := 0; i+1 < len(ordered); i++ {
		left, right := ordered[i], ordered[i+1]
		if left.max < right.min {
			issues = append(issues, ValidationIssue{
				Field:   field,
				Code:    "conclusion.range.gap",
				Message: fmt.Sprintf("score ranges leave a gap between %.4g and %.4g", left.max, right.min),
			})
		}
	}
	return issues
}

func validateTypeConclusion(item conclusion.TypeConclusion, factorCodes, outcomeCodes map[string]struct{}) []ValidationIssue {
	issues := make([]ValidationIssue, 0)
	for _, code := range item.FactorCodes {
		if _, ok := factorCodes[code]; code == "" || !ok {
			issues = append(issues, ValidationIssue{Field: "conclusions.type.factor_codes", Code: "conclusion.factor.not_found", Message: fmt.Sprintf("conclusion factor %s is not defined", code)})
		}
	}
	if !validTypeDecision(item.Decision.Kind) {
		issues = append(issues, ValidationIssue{Field: "conclusions.type.decision.kind", Code: "type_conclusion.decision.invalid", Message: "type conclusion decision kind is invalid"})
	}
	for _, pole := range item.Decision.Poles {
		if _, ok := factorCodes[pole.FactorCode]; pole.FactorCode == "" || !ok {
			issues = append(issues, ValidationIssue{Field: "conclusions.type.decision.poles", Code: "conclusion.factor.not_found", Message: fmt.Sprintf("pole factor %s is not defined", pole.FactorCode)})
		}
	}
	for _, rule := range item.SpecialRules {
		if rule.Code == "" || rule.Kind == "" || rule.Phase == "" {
			issues = append(issues, ValidationIssue{Field: "conclusions.type.special_rules", Code: "type_conclusion.special_rule.invalid", Message: "type special rule code, kind and phase are required"})
		}
		if rule.OutcomeCode != "" {
			if _, ok := outcomeCodes[rule.OutcomeCode]; !ok {
				issues = append(issues, ValidationIssue{Field: "conclusions.type.special_rules", Code: "conclusion.outcome.not_found", Message: fmt.Sprintf("outcome %s is not defined", rule.OutcomeCode)})
			}
		}
	}
	for _, profile := range item.Profiles {
		if _, ok := outcomeCodes[profile.OutcomeCode]; profile.OutcomeCode == "" || !ok {
			issues = append(issues, ValidationIssue{Field: "conclusions.type.profiles", Code: "conclusion.outcome.not_found", Message: fmt.Sprintf("outcome %s is not defined", profile.OutcomeCode)})
		}
	}
	return issues
}

func validateReportMap(reportMap ReportMap) []ValidationIssue {
	issues := make([]ValidationIssue, 0)
	seen := makeStringSet()
	for _, section := range reportMap.Sections {
		if section.Code == "" {
			issues = append(issues, ValidationIssue{Field: "report_map.sections", Code: "report_section.code.required", Message: "report section code is required"})
			continue
		}
		if _, ok := seen[section.Code]; ok {
			issues = append(issues, ValidationIssue{Field: "report_map.sections", Code: "report_section.code.duplicate", Message: fmt.Sprintf("report section %s is duplicated", section.Code)})
		}
		seen[section.Code] = struct{}{}
	}
	return issues
}

func validScoreBasis(value conclusion.ScoreBasis) bool {
	switch value {
	case conclusion.ScoreBasisRaw, conclusion.ScoreBasisTScore, conclusion.ScoreBasisPercentile, conclusion.ScoreBasisStandardScore:
		return true
	default:
		return false
	}
}

func validTypeDecision(kind binding.DecisionKind) bool {
	switch kind {
	case binding.DecisionKindPoleComposition, binding.DecisionKindTraitProfile, binding.DecisionKindNearestPattern, binding.DecisionKindDominantFactor:
		return true
	default:
		return false
	}
}

func makeStringSet() map[string]struct{} { return make(map[string]struct{}) }
