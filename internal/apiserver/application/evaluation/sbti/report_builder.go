package sbti

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
	return assessment.EvaluationModelKindSBTI
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
			return ResultDetail{}, fmt.Errorf("sbti result detail is nil")
		}
		return *payload, nil
	default:
		return ResultDetail{}, fmt.Errorf("unsupported sbti result detail payload: %T", outcome.Result.Detail.Payload)
	}
}

func reportModelName(detail ResultDetail) string {
	if detail.TypeName == "" {
		return "SBTI 趣味人格测评"
	}
	return "SBTI 趣味人格测评 - " + detail.TypeName
}

func reportModelCode(outcome evaluationresult.Outcome, detail ResultDetail) string {
	if outcome.Result != nil && !outcome.Result.ModelRef.Code().IsEmpty() {
		return outcome.Result.ModelRef.Code().String()
	}
	if detail.TypeCode != "" {
		return detail.TypeCode
	}
	return "SBTI_FUN"
}

func reportConclusion(detail ResultDetail) string {
	title := strings.TrimSpace(detail.TypeCode + " " + detail.TypeName)
	if detail.OneLiner != "" {
		title += " - " + detail.OneLiner
	}
	if detail.Similarity > 0 {
		title += fmt.Sprintf("（匹配度 %.0f%%）", detail.Similarity*100)
	}
	return strings.TrimSpace(title)
}

func reportDimensions(detail ResultDetail) []domainReport.DimensionInterpret {
	if len(detail.Dimensions) == 0 {
		return nil
	}
	maxScore := 6.0
	dimensions := make([]domainReport.DimensionInterpret, 0, len(detail.Dimensions))
	for _, dim := range detail.Dimensions {
		description := strings.TrimSpace(fmt.Sprintf("%s：%s 档，原始分 %.0f/6", dim.Name, dim.Level, dim.RawScore))
		if dim.Model != "" {
			description = dim.Model + " / " + description
		}
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
	suggestions := make([]domainReport.Suggestion, 0, 5)
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
	add(detail.Outcome.Commentary)
	if detail.Source.Attribution != "" {
		add(fmt.Sprintf("来源与授权：%s；License: %s；非商业使用: %t。", detail.Source.Attribution, detail.Source.License, detail.Source.NonCommercial))
	}
	return suggestions
}

func reportModelExtra(detail ResultDetail) *domainReport.ModelExtra {
	extra := &domainReport.ModelExtra{
		Kind:           "sbti",
		TypeCode:       detail.TypeCode,
		TypeName:       detail.TypeName,
		OneLiner:       detail.OneLiner,
		ImageURL:       detail.ImageURL,
		MatchPercent:   detail.Similarity * 100,
		IsSpecial:      detail.Outcome.IsSpecial,
		SpecialTrigger: detail.SpecialTrigger,
		Commentary:     detail.Outcome.Commentary,
	}
	if detail.Rarity.Percent > 0 || detail.Rarity.Label != "" || detail.Rarity.OneInX > 0 {
		extra.Rarity = &domainReport.ModelRarity{
			Percent: detail.Rarity.Percent,
			Label:   detail.Rarity.Label,
			OneInX:  detail.Rarity.OneInX,
		}
	}
	if extra.IsEmpty() {
		return nil
	}
	return extra
}
