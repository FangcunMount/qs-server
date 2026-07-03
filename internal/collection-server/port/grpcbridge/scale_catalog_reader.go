package grpcbridge

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/collection-server/application/scale"
)

// ScaleCatalogReader 将 infra gRPC 输出转换为 application DTO。
type ScaleCatalogReader struct {
	inner ScaleReader
}

func NewScaleCatalogReader(inner ScaleReader) *ScaleCatalogReader {
	return &ScaleCatalogReader{inner: inner}
}

func (r *ScaleCatalogReader) GetScale(ctx context.Context, code string) (*scale.ScaleResponse, error) {
	if r == nil {
		return nil, nil
	}
	return CallBridge(r.inner,
		func() (*ScaleOutput, error) { return r.inner.GetScale(ctx, code) },
		toScaleResponse,
	)
}

func (r *ScaleCatalogReader) ListScales(ctx context.Context, page, pageSize int32, status, title, category string, stages, applicableAges, reporters, tags []string) (*scale.ListScalesResponse, error) {
	if r == nil {
		return nil, nil
	}
	return CallBridge(r.inner,
		func() (*ListScalesOutput, error) {
			return r.inner.ListScales(ctx, page, pageSize, status, title, category, stages, applicableAges, reporters, tags)
		},
		toListScalesResponse,
	)
}

func (r *ScaleCatalogReader) ListHotScales(ctx context.Context, limit, windowDays int32) (*scale.ListHotScalesResponse, error) {
	if r == nil {
		return nil, nil
	}
	return CallBridge(r.inner,
		func() (*ListHotScalesOutput, error) { return r.inner.ListHotScales(ctx, limit, windowDays) },
		toListHotScalesResponse,
	)
}

func (r *ScaleCatalogReader) GetScaleCategories(ctx context.Context) (*scale.ScaleCategoriesResponse, error) {
	if r == nil {
		return nil, nil
	}
	return CallBridge(r.inner,
		func() (*ScaleCategoriesOutput, error) { return r.inner.GetScaleCategories(ctx) },
		toScaleCategoriesResponse,
	)
}

func toListScalesResponse(out *ListScalesOutput) *scale.ListScalesResponse {
	scales := make([]scale.ScaleSummaryResponse, 0, len(out.Scales))
	for _, item := range out.Scales {
		scales = append(scales, toScaleSummaryFromOutput(item))
	}
	return &scale.ListScalesResponse{
		Scales:   scales,
		Total:    out.Total,
		Page:     out.Page,
		PageSize: out.PageSize,
	}
}

func toListHotScalesResponse(out *ListHotScalesOutput) *scale.ListHotScalesResponse {
	scales := make([]scale.HotScaleSummaryResponse, 0, len(out.Scales))
	for _, item := range out.Scales {
		scales = append(scales, scale.HotScaleSummaryResponse{
			ScaleSummaryResponse: toScaleSummaryFromOutput(item.ScaleSummaryOutput),
			Rank:                 item.Rank,
			SubmissionCount:      item.SubmissionCount,
			HeatScore:            item.HeatScore,
		})
	}
	return &scale.ListHotScalesResponse{
		Scales:     scales,
		Total:      int64(len(scales)),
		Limit:      out.Limit,
		WindowDays: out.WindowDays,
	}
}

func toScaleSummaryFromOutput(s ScaleSummaryOutput) scale.ScaleSummaryResponse {
	return scale.ScaleSummaryResponse{
		Code:                 s.Code,
		Title:                s.Title,
		Description:          s.Description,
		Category:             s.Category,
		Stages:               s.Stages,
		ApplicableAges:       s.ApplicableAges,
		Reporters:            s.Reporters,
		Tags:                 s.Tags,
		QuestionnaireCode:    s.QuestionnaireCode,
		QuestionnaireVersion: s.QuestionnaireVersion,
		Status:               s.Status,
		QuestionCount:        s.QuestionCount,
	}
}

func toScaleResponse(s *ScaleOutput) *scale.ScaleResponse {
	factors := make([]scale.FactorResponse, len(s.Factors))
	for i, factor := range s.Factors {
		factors[i] = toFactorResponse(&factor)
	}
	return &scale.ScaleResponse{
		Code:                 s.Code,
		Title:                s.Title,
		Description:          s.Description,
		Category:             s.Category,
		Stages:               s.Stages,
		ApplicableAges:       s.ApplicableAges,
		Reporters:            s.Reporters,
		Tags:                 s.Tags,
		QuestionnaireCode:    s.QuestionnaireCode,
		QuestionnaireVersion: s.QuestionnaireVersion,
		Status:               s.Status,
		Factors:              factors,
		QuestionCount:        s.QuestionCount,
	}
}

func toFactorResponse(f *FactorOutput) scale.FactorResponse {
	rules := make([]scale.InterpretRuleResponse, len(f.InterpretRules))
	for i, rule := range f.InterpretRules {
		rules[i] = scale.InterpretRuleResponse{
			MinScore:   rule.MinScore,
			MaxScore:   rule.MaxScore,
			RiskLevel:  rule.RiskLevel,
			Conclusion: rule.Conclusion,
			Suggestion: rule.Suggestion,
		}
	}
	return scale.FactorResponse{
		Code:            f.Code,
		Title:           f.Title,
		FactorType:      f.FactorType,
		IsTotalScore:    f.IsTotalScore,
		QuestionCodes:   f.QuestionCodes,
		ScoringStrategy: f.ScoringStrategy,
		ScoringParams:   f.ScoringParams,
		MaxScore:        f.MaxScore,
		RiskLevel:       f.RiskLevel,
		InterpretRules:  rules,
	}
}

func toScaleCategoriesResponse(out *ScaleCategoriesOutput) *scale.ScaleCategoriesResponse {
	categories := make([]scale.CategoryResponse, len(out.Categories))
	for i, cat := range out.Categories {
		categories[i] = scale.CategoryResponse{Value: cat.Value, Label: cat.Label}
	}
	stages := make([]scale.StageResponse, len(out.Stages))
	for i, stage := range out.Stages {
		stages[i] = scale.StageResponse{Value: stage.Value, Label: stage.Label}
	}
	applicableAges := make([]scale.ApplicableAgeResponse, len(out.ApplicableAges))
	for i, age := range out.ApplicableAges {
		applicableAges[i] = scale.ApplicableAgeResponse{Value: age.Value, Label: age.Label}
	}
	reporters := make([]scale.ReporterResponse, len(out.Reporters))
	for i, rep := range out.Reporters {
		reporters[i] = scale.ReporterResponse{Value: rep.Value, Label: rep.Label}
	}
	tags := make([]scale.TagResponse, len(out.Tags))
	for i, tag := range out.Tags {
		tags[i] = scale.TagResponse{Value: tag.Value, Label: tag.Label, Category: tag.Category}
	}
	return &scale.ScaleCategoriesResponse{
		Categories:     categories,
		Stages:         stages,
		ApplicableAges: applicableAges,
		Reporters:      reporters,
		Tags:           tags,
	}
}
