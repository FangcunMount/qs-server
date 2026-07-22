package scoring

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
)

// FactorScoringReportInput is factor-scoring mechanism report assembly input.
type FactorScoringReportInput struct {
	AssessmentID        report.ID
	PresentationProfile *report.PresentationProfile
	Scale               *ReportModel
	TotalScore          float64
	RiskLevel           report.RiskLevel
	Conclusion          string
	Suggestion          string
	FactorScores        []FactorReportScore
}

// BuildFactorScoringDraft assembles the same factor-scoring content without
// creating the legacy report representation.
func BuildFactorScoringDraft(composer report.DraftBuilder, input FactorScoringReportInput) (*report.Draft, error) {
	if composer == nil {
		return nil, report.ErrInvalidArgument
	}
	resolved, err := resolveFactorScoringInput(input)
	if err != nil {
		return nil, err
	}
	return composer.BuildDraft(generateReportInput(resolved))
}

func resolveFactorScoringInput(input FactorScoringReportInput) (ReportInput, error) {
	factorScores := make([]FactorReportScore, 0, len(input.FactorScores))
	visible, configured := factorScoreVisibility(input.PresentationProfile)
	for _, fs := range input.FactorScores {
		if configured && !visible[fs.FactorCode] {
			factorScores = append(factorScores, fs)
			continue
		}
		if fs.Conclusion == "" && fs.Suggestion == "" {
			var err error
			fs.Conclusion, fs.Suggestion, err = interpretScaleFactor(input.Scale, fs)
			if err != nil {
				return ReportInput{}, err
			}
		}
		factorScores = append(factorScores, fs)
	}

	conclusion, suggestion := input.Conclusion, input.Suggestion
	if conclusion == "" && suggestion == "" {
		for _, fs := range factorScores {
			if fs.IsTotalScore {
				conclusion, suggestion = fs.Conclusion, fs.Suggestion
				break
			}
		}
	}

	return ReportInput{
		AssessmentID: input.AssessmentID,
		Model:        input.Scale,
		TotalScore:   input.TotalScore,
		RiskLevel:    input.RiskLevel,
		Conclusion:   conclusion,
		Suggestion:   suggestion,
		FactorScores: factorScores,
	}, nil
}

func factorScoreVisibility(profile *report.PresentationProfile) (map[string]bool, bool) {
	if profile == nil || !profile.Configured() {
		return nil, false
	}
	return profile.VisibleSet(), true
}

func generateReportInput(input ReportInput) report.GenerateReportInput {
	reportInput := report.GenerateReportInput{
		AssessmentID: input.AssessmentID,
		TotalScore:   input.TotalScore,
		RiskLevel:    input.RiskLevel,
		Conclusion:   input.Conclusion,
		Suggestion:   input.Suggestion,
	}
	if input.Model != nil {
		reportInput.ModelName = input.Model.Title
		reportInput.ModelCode = input.Model.Code
	}
	reportInput.FactorScores = factorScoreInputs(input.FactorScores, input.Model)
	return reportInput
}

func factorScoreInputs(
	factorScores []FactorReportScore,
	model *ReportModel,
) []report.FactorScoreInput {
	factorMeta := make(map[string]FactorReportModel)
	if model != nil {
		for _, f := range model.Factors {
			factorMeta[f.Code] = f
		}
	}
	inputs := make([]report.FactorScoreInput, 0, len(factorScores))
	for _, fs := range factorScores {
		meta, ok := factorMeta[fs.FactorCode]
		factorName := fs.FactorName
		var maxScore *float64
		if ok {
			if factorName == "" {
				factorName = meta.Title
			}
			maxScore = meta.MaxScore
		}
		if factorName == "" {
			factorName = fs.FactorCode
		}
		inputs = append(inputs, report.FactorScoreInput{
			FactorCode:     report.FactorCode(fs.FactorCode),
			FactorName:     factorName,
			RawScore:       fs.RawScore,
			MaxScore:       maxScore,
			RiskLevel:      fs.RiskLevel,
			DerivedScores:  fs.DerivedScores,
			Level:          fs.Level,
			NormReference:  fs.NormReference,
			Description:    fs.Conclusion,
			Suggestion:     fs.Suggestion,
			IsTotalScore:   fs.IsTotalScore,
			Role:           fs.Role,
			ParentCode:     fs.ParentCode,
			HierarchyLevel: fs.HierarchyLevel,
			SortOrder:      fs.SortOrder,
		})
	}
	return inputs
}
