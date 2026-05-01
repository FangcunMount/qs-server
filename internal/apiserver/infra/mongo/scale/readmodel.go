package scale

import (
	"context"

	domainScale "github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/scalereadmodel"
)

type scaleReadModel struct {
	repo domainScale.Repository
}

// NewScaleReadModel adapts scale Mongo repositories to the read-model port.
func NewScaleReadModel(repo domainScale.Repository) scalereadmodel.ScaleReader {
	return scaleReadModel{repo: repo}
}

func (r scaleReadModel) ListScales(ctx context.Context, filter scalereadmodel.ScaleFilter, page scalereadmodel.PageRequest) ([]scalereadmodel.ScaleSummaryRow, error) {
	items, err := r.repo.FindSummaryList(ctx, page.Page, page.PageSize, scaleFilterToConditions(filter))
	if err != nil {
		return nil, err
	}
	return scaleRowsFromDomain(items), nil
}

func (r scaleReadModel) CountScales(ctx context.Context, filter scalereadmodel.ScaleFilter) (int64, error) {
	return r.repo.CountWithConditions(ctx, scaleFilterToConditions(filter))
}

func scaleFilterToConditions(filter scalereadmodel.ScaleFilter) map[string]interface{} {
	conditions := make(map[string]interface{})
	if filter.Status != "" {
		conditions["status"] = filter.Status
	}
	if filter.Title != "" {
		conditions["title"] = filter.Title
	}
	if filter.Category != "" {
		conditions["category"] = filter.Category
	}
	return conditions
}

func scaleRowsFromDomain(items []*domainScale.MedicalScale) []scalereadmodel.ScaleSummaryRow {
	rows := make([]scalereadmodel.ScaleSummaryRow, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		rows = append(rows, scalereadmodel.ScaleSummaryRow{
			Code:              item.GetCode().String(),
			Title:             item.GetTitle(),
			Description:       item.GetDescription(),
			Category:          item.GetCategory().String(),
			Stages:            stringValues(item.GetStages()),
			ApplicableAges:    stringValues(item.GetApplicableAges()),
			Reporters:         stringValues(item.GetReporters()),
			Tags:              stringValues(item.GetTags()),
			QuestionnaireCode: item.GetQuestionnaireCode().String(),
			Status:            item.GetStatus().String(),
			CreatedBy:         item.GetCreatedBy(),
			CreatedAt:         item.GetCreatedAt(),
			UpdatedBy:         item.GetUpdatedBy(),
			UpdatedAt:         item.GetUpdatedAt(),
		})
	}
	return rows
}

type stringer interface {
	String() string
}

func stringValues[T stringer](values []T) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, value.String())
	}
	return result
}
