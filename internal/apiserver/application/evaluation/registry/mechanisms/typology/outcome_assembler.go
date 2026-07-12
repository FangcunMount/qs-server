package typology

import (
	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	outcometypology "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome/typology"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
)

func executionFromPersonalityType(modelRef assessment.EvaluationModelRef, detail outcometypology.PersonalityTypeDetail) *domainoutcome.Execution {
	score := detail.MatchPercent
	if score == 0 && detail.Similarity > 0 {
		score = detail.Similarity * 100
	}
	execution := domainoutcome.NewExecution(evaloutcome.ModelRefFromAssessment(modelRef), domainoutcome.Summary{PrimaryLabel: detail.TypeCode, Score: scorePointer(score), Tags: []string{detail.TypeName, detail.OneLiner}}, domainoutcome.Detail{Kind: modelRef.Kind(), Payload: detail})
	execution.Primary = &domainoutcome.ScoreValue{Kind: domainoutcome.ScoreKindMatchPercent, Value: score, Label: detail.TypeCode}
	execution.Level = &domainoutcome.ResultLevel{Code: detail.TypeCode, Label: detail.TypeName, Severity: "none"}
	execution.Profile = &domainoutcome.ProfileResult{Kind: domainoutcome.ProfileKindPersonalityType, Code: detail.TypeCode, Name: detail.TypeName}
	return execution
}

func executionFromTraitProfile(modelRef assessment.EvaluationModelRef, detail outcometypology.TraitProfileDetail) *domainoutcome.Execution {
	primary := "trait_profile"
	if len(detail.Traits) > 0 {
		primary = detail.Traits[0].Code
	}
	execution := domainoutcome.NewExecution(evaloutcome.ModelRefFromAssessment(modelRef), domainoutcome.Summary{PrimaryLabel: primary}, domainoutcome.Detail{Kind: modelRef.Kind(), Payload: detail})
	if len(detail.Traits) > 0 {
		execution.Profile = &domainoutcome.ProfileResult{Kind: domainoutcome.ProfileKindPersonalityTrait, Code: detail.Traits[0].Code, Name: detail.Traits[0].Name}
	}
	return execution
}

func scorePointer(score float64) *float64 {
	if score == 0 {
		return nil
	}
	return &score
}
