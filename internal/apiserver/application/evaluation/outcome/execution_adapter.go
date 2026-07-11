package outcome

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// ExecutionFromAssessmentOutcome is the sole application-layer adapter from
// the retiring Assessment-owned result representation to the Outcome-owned
// execution result.
func ExecutionFromAssessmentOutcome(source *assessment.AssessmentOutcome) *domainoutcome.Execution {
	if source == nil {
		return nil
	}
	execution := domainoutcome.NewExecution(modelRefFromAssessment(source.ModelRef), summaryFromAssessment(source.Summary), detailFromAssessment(source.Detail))
	execution.Primary = scoreFromAssessment(source.Primary)
	execution.Level = levelFromAssessment(source.Level)
	execution.Profile = profileFromAssessment(source.Profile)
	execution.Dimensions = dimensionsFromAssessment(source.Dimensions)
	execution.Validity = validityFromAssessment(source.Validity)
	return execution
}

// AssessmentOutcomeFromExecution is the sole compatibility adapter required
// by Assessment lifecycle and legacy score projections until they accept the
// Outcome-owned execution value directly.
func AssessmentOutcomeFromExecution(source *domainoutcome.Execution) *assessment.AssessmentOutcome {
	if source == nil {
		return nil
	}
	result := assessment.NewAssessmentOutcome(modelRefToAssessment(source.ModelRef), summaryToAssessment(source.Summary), detailToAssessment(source.Detail))
	result.Primary = scoreToAssessment(source.Primary)
	result.Level = levelToAssessment(source.Level)
	result.Profile = profileToAssessment(source.Profile)
	result.Dimensions = dimensionsToAssessment(source.Dimensions)
	result.Validity = validityToAssessment(source.Validity)
	return result
}

func ModelRefFromAssessment(ref assessment.EvaluationModelRef) domainoutcome.ModelRef {
	return modelRefFromAssessment(ref)
}

func modelRefFromAssessment(ref assessment.EvaluationModelRef) domainoutcome.ModelRef {
	identity := ref.ExecutionIdentity()
	return domainoutcome.ModelRef{ModelKind: identity.Kind, ModelSubKind: identity.SubKind, ModelAlgorithm: identity.Algorithm, ModelCode: ref.Code().String(), ModelVersion: ref.Version(), ModelTitle: ref.Title()}
}

func modelRefToAssessment(ref domainoutcome.ModelRef) assessment.EvaluationModelRef {
	return assessment.NewEvaluationModelRefWithIdentity(assessment.EvaluationModelKind(ref.Kind()), ref.SubKind(), ref.Algorithm(), meta.ZeroID, ref.Code(), ref.Version(), ref.Title())
}

func summaryFromAssessment(value assessment.ResultSummary) domainoutcome.Summary {
	return domainoutcome.Summary{PrimaryLabel: value.PrimaryLabel, Score: cloneFloat(value.Score), Level: cloneString(value.Level), Tags: append([]string(nil), value.Tags...)}
}
func summaryToAssessment(value domainoutcome.Summary) assessment.ResultSummary {
	return assessment.ResultSummary{PrimaryLabel: value.PrimaryLabel, Score: cloneFloat(value.Score), Level: cloneString(value.Level), Tags: append([]string(nil), value.Tags...)}
}
func detailFromAssessment(value assessment.EvaluationDetail) domainoutcome.Detail {
	return domainoutcome.Detail{Kind: value.Kind, Payload: value.Payload}
}
func detailToAssessment(value domainoutcome.Detail) assessment.EvaluationDetail {
	return assessment.EvaluationDetail{Kind: assessment.EvaluationModelKind(value.Kind), Payload: value.Payload}
}

func scoreFromAssessment(value *assessment.OutcomeScoreValue) *domainoutcome.ScoreValue {
	if value == nil {
		return nil
	}
	return &domainoutcome.ScoreValue{Kind: domainoutcome.ScoreKind(value.Kind), Value: value.Value, Label: value.Label, Max: cloneFloat(value.Max)}
}
func scoreToAssessment(value *domainoutcome.ScoreValue) *assessment.OutcomeScoreValue {
	if value == nil {
		return nil
	}
	return &assessment.OutcomeScoreValue{Kind: assessment.OutcomeScoreKind(value.Kind), Value: value.Value, Label: value.Label, Max: cloneFloat(value.Max)}
}
func levelFromAssessment(value *assessment.OutcomeResultLevel) *domainoutcome.ResultLevel {
	if value == nil {
		return nil
	}
	return &domainoutcome.ResultLevel{Code: value.Code, Label: value.Label, Severity: value.Severity}
}
func levelToAssessment(value *domainoutcome.ResultLevel) *assessment.OutcomeResultLevel {
	if value == nil {
		return nil
	}
	return &assessment.OutcomeResultLevel{Code: value.Code, Label: value.Label, Severity: value.Severity}
}

func profileFromAssessment(value *assessment.ProfileResult) *domainoutcome.ProfileResult {
	if value == nil {
		return nil
	}
	return &domainoutcome.ProfileResult{Kind: domainoutcome.ProfileKind(value.Kind), Code: value.Code, Name: value.Name, Summary: value.Summary, Traits: append([]string(nil), value.Traits...), Strengths: append([]string(nil), value.Strengths...), Weaknesses: append([]string(nil), value.Weaknesses...), Suggestions: append([]string(nil), value.Suggestions...)}
}
func profileToAssessment(value *domainoutcome.ProfileResult) *assessment.ProfileResult {
	if value == nil {
		return nil
	}
	return &assessment.ProfileResult{Kind: assessment.ProfileKind(value.Kind), Code: value.Code, Name: value.Name, Summary: value.Summary, Traits: append([]string(nil), value.Traits...), Strengths: append([]string(nil), value.Strengths...), Weaknesses: append([]string(nil), value.Weaknesses...), Suggestions: append([]string(nil), value.Suggestions...)}
}

func dimensionsFromAssessment(values []assessment.DimensionResult) []domainoutcome.DimensionResult {
	result := make([]domainoutcome.DimensionResult, 0, len(values))
	for _, value := range values {
		scores := make([]domainoutcome.ScoreValue, 0, len(value.DerivedScores))
		for _, score := range value.DerivedScores {
			scores = append(scores, *scoreFromAssessment(&score))
		}
		result = append(result, domainoutcome.DimensionResult{Code: value.Code, Name: value.Name, Kind: domainoutcome.DimensionKind(value.Kind), Role: value.Role, ParentCode: value.ParentCode, HierarchyLevel: value.HierarchyLevel, SortOrder: value.SortOrder, Score: scoreFromAssessment(value.Score), DerivedScores: scores, Level: levelFromAssessment(value.Level), Description: value.Description, Suggestion: value.Suggestion})
	}
	return result
}
func dimensionsToAssessment(values []domainoutcome.DimensionResult) []assessment.DimensionResult {
	result := make([]assessment.DimensionResult, 0, len(values))
	for _, value := range values {
		scores := make([]assessment.OutcomeScoreValue, 0, len(value.DerivedScores))
		for _, score := range value.DerivedScores {
			scores = append(scores, *scoreToAssessment(&score))
		}
		result = append(result, assessment.DimensionResult{Code: value.Code, Name: value.Name, Kind: assessment.DimensionKind(value.Kind), Role: value.Role, ParentCode: value.ParentCode, HierarchyLevel: value.HierarchyLevel, SortOrder: value.SortOrder, Score: scoreToAssessment(value.Score), DerivedScores: scores, Level: levelToAssessment(value.Level), Description: value.Description, Suggestion: value.Suggestion})
	}
	return result
}

func validityFromAssessment(values []assessment.ValidityResult) []domainoutcome.ValidityResult {
	result := make([]domainoutcome.ValidityResult, 0, len(values))
	for _, value := range values {
		result = append(result, domainoutcome.ValidityResult{Code: value.Code, Label: value.Label, Passed: value.Passed, Message: value.Message})
	}
	return result
}
func validityToAssessment(values []domainoutcome.ValidityResult) []assessment.ValidityResult {
	result := make([]assessment.ValidityResult, 0, len(values))
	for _, value := range values {
		result = append(result, assessment.ValidityResult{Code: value.Code, Label: value.Label, Passed: value.Passed, Message: value.Message})
	}
	return result
}

func cloneFloat(value *float64) *float64 {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}
func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}
