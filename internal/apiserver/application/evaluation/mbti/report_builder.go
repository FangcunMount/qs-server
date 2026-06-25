package mbti

import (
	"context"
	"fmt"
	"strings"

	evaluationresult "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/result"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/report"
)

type ReportBuilder struct{}

var _ evaluationresult.ReportBuilder = ReportBuilder{}

func NewReportBuilder() ReportBuilder {
	return ReportBuilder{}
}

func (ReportBuilder) Kind() assessment.EvaluationModelKind {
	return assessment.EvaluationModelKindMBTI
}

func (ReportBuilder) ReportType() domainReport.ReportType {
	return domainReport.ReportTypeStandard
}

func (ReportBuilder) Build(_ context.Context, outcome evaluationresult.Outcome) (*domainReport.InterpretReport, error) {
	if outcome.Assessment == nil {
		return nil, fmt.Errorf("assessment is required")
	}
	if outcome.Result == nil {
		return nil, fmt.Errorf("evaluation result is required")
	}
	detail, err := detailFromOutcome(outcome)
	if err != nil {
		return nil, err
	}
	return domainReport.NewInterpretReport(
		domainReport.ID(outcome.Assessment.ID()),
		reportModelName(detail),
		reportModelCode(outcome, detail),
		outcome.Result.TotalScore,
		domainReport.RiskLevel(outcome.Result.RiskLevel),
		reportConclusion(detail),
		reportDimensions(detail),
		reportSuggestions(detail),
		reportModelExtra(detail),
	), nil
}

func detailFromOutcome(outcome evaluationresult.Outcome) (ResultDetail, error) {
	switch payload := outcome.Result.Detail.Payload.(type) {
	case ResultDetail:
		return payload, nil
	case *ResultDetail:
		if payload == nil {
			return ResultDetail{}, fmt.Errorf("mbti result detail is nil")
		}
		return *payload, nil
	default:
		return ResultDetail{}, fmt.Errorf("unsupported mbti result detail payload: %T", outcome.Result.Detail.Payload)
	}
}

func reportModelName(detail ResultDetail) string {
	if detail.TypeName == "" {
		return "MBTI 人格类型测评"
	}
	return "MBTI 人格类型测评 - " + detail.TypeName
}

func reportModelCode(outcome evaluationresult.Outcome, detail ResultDetail) string {
	if outcome.Result != nil && !outcome.Result.ModelRef.Code().IsEmpty() {
		return outcome.Result.ModelRef.Code().String()
	}
	if detail.TypeCode != "" {
		return detail.TypeCode
	}
	return "MBTI_OEJTS"
}

func reportConclusion(detail ResultDetail) string {
	title := strings.TrimSpace(detail.TypeCode + " " + detail.TypeName)
	if detail.OneLiner != "" {
		title += " - " + detail.OneLiner
	}
	return strings.TrimSpace(title)
}

func reportDimensions(detail ResultDetail) []domainReport.DimensionInterpret {
	if len(detail.Dimensions) == 0 {
		return nil
	}
	maxScore := 40.0
	dimensions := make([]domainReport.DimensionInterpret, 0, len(detail.Dimensions))
	for _, dim := range detail.Dimensions {
		description := fmt.Sprintf("%s：倾向 %s（原始分 %.0f，偏好强度 %.0f%%）",
			dim.Name, dim.Preference, dim.RawScore, dim.Strength)
		dimensions = append(dimensions, domainReport.NewDimensionInterpret(
			domainReport.FactorCode(dim.Code),
			dim.Name,
			dim.RawScore,
			&maxScore,
			domainReport.RiskLevelNone,
			description,
			"",
		))
	}
	return dimensions
}

func reportSuggestions(detail ResultDetail) []domainReport.Suggestion {
	suggestions := make([]domainReport.Suggestion, 0, 8)
	add := func(content string) {
		content = strings.TrimSpace(content)
		if content == "" {
			return
		}
		suggestions = append(suggestions, domainReport.Suggestion{
			Category: domainReport.SuggestionCategoryGeneral,
			Content:  content,
		})
	}
	add(detail.Profile.Summary)
	for _, s := range detail.Profile.Strengths {
		add("优势：" + s)
	}
	for _, s := range detail.Profile.Weaknesses {
		add("注意：" + s)
	}
	for _, s := range detail.Profile.Suggestions {
		add("建议：" + s)
	}
	if detail.Source.Attribution != "" {
		add(fmt.Sprintf("来源与授权：%s；License: %s；非商业使用: %t。",
			detail.Source.Attribution, detail.Source.License, detail.Source.NonCommercial))
	}
	return suggestions
}

func reportModelExtra(detail ResultDetail) *domainReport.ModelExtra {
	extra := &domainReport.ModelExtra{
		Kind:         "mbti",
		TypeCode:     detail.TypeCode,
		TypeName:     detail.TypeName,
		OneLiner:     detail.OneLiner,
		ImageURL:     detail.ImageURL,
		MatchPercent: detail.MatchPercent,
		Commentary:   detail.Profile.Summary,
	}
	if extra.IsEmpty() {
		return nil
	}
	return extra
}
