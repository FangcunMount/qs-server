package scale

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/scalelistcache"
)

func scaleSummaryListResultFromCachePage(page *scalelistcache.Page) *ScaleSummaryListResult {
	if page == nil {
		return nil
	}

	items := make([]*ScaleSummaryResult, 0, len(page.Items))
	for _, item := range page.Items {
		items = append(items, &ScaleSummaryResult{
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
	return &ScaleSummaryListResult{
		Items: items,
		Total: page.Total,
	}
}

func logScaleListCacheError(ctx context.Context, err error) {
	if err == nil {
		return
	}
	logger.L(ctx).Warnw("failed to rebuild scale list cache", "error", err)
}
