package query

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/scoring/shared"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/scalelistcache"
)

func scaleSummaryListResultFromCachePage(page *scalelistcache.Page) *shared.ScaleSummaryListResult {
	if page == nil {
		return nil
	}

	items := make([]*shared.ScaleSummaryResult, 0, len(page.Items))
	for _, item := range page.Items {
		items = append(items, &shared.ScaleSummaryResult{
			Code:              item.Code,
			Title:             item.Title,
			Description:       item.Description,
			Category:          item.Category,
			Stages:            item.Stages,
			ApplicableAges:    item.ApplicableAges,
			Reporters:         item.Reporters,
			Tags:              item.Tags,
			QuestionnaireCode: item.QuestionnaireCode,
			Status:            item.Status,
			CreatedBy:         item.CreatedBy,
			CreatedAt:         item.CreatedAt,
			UpdatedBy:         item.UpdatedBy,
			UpdatedAt:         item.UpdatedAt,
		})
	}
	return &shared.ScaleSummaryListResult{
		Items: items,
		Total: page.Total,
	}
}
