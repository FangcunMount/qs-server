package assessment

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/logger"
	evalerrors "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/apperrors"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationreadmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/safeconv"
)

type myAssessmentListWorkflow struct {
	service *submissionService
}

func (w myAssessmentListWorkflow) List(ctx context.Context, dto ListMyAssessmentsDTO) (*AssessmentListResult, error) {
	s := w.service
	l := logger.L(ctx)
	startTime := time.Now()

	l.Debugw("查询我的测评列表",
		"action", "list_my_assessments",
		"testee_id", dto.TesteeID,
		"page", dto.Page,
		"page_size", dto.PageSize,
		"status", dto.Status,
		"scale_code", dto.ScaleCode,
		"risk_level", dto.RiskLevel,
	)

	if dto.TesteeID == 0 {
		l.Warnw("受试者ID为空",
			"action", "list_my_assessments",
			"result", "invalid_params",
		)
		return nil, evalerrors.InvalidArgument("受试者ID不能为空")
	}

	page, pageSize := normalizePagination(dto.Page, dto.PageSize)
	cacheKey := newMyAssessmentListCacheKey(dto, page, pageSize)
	cacheHelper := myAssessmentListCacheHelper{cache: s.listCache}
	if cached, ok := cacheHelper.Get(ctx, cacheKey); ok {
		return cached, nil
	}

	l.Debugw("开始查询测评列表",
		"testee_id", dto.TesteeID,
		"page", page,
		"page_size", pageSize,
		"status", dto.Status,
		"scale_code", dto.ScaleCode,
		"risk_level", dto.RiskLevel,
		"date_from", cacheKey.dateFrom,
		"date_to", cacheKey.dateTo,
	)
	items, total, err := myAssessmentQuery{reader: s.reader}.List(ctx, dto, page, pageSize)
	if err != nil {
		l.Errorw("查询测评列表失败",
			"testee_id", dto.TesteeID,
			"action", "list_my_assessments",
			"result", "failed",
			"error", err.Error(),
		)
		return nil, evalerrors.Database(err, "查询测评列表失败")
	}

	totalInt, err := safeconv.Int64ToInt(total)
	if err != nil {
		return nil, evalerrors.DatabaseMessage("测评总数超出安全范围")
	}
	l.Debugw("查询我的测评列表成功",
		"action", "list_my_assessments",
		"result", "success",
		"testee_id", dto.TesteeID,
		"total_count", totalInt,
		"page_count", len(items),
		"duration_ms", time.Since(startTime).Milliseconds(),
	)

	result := &AssessmentListResult{
		Items:      items,
		Total:      totalInt,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: (totalInt + pageSize - 1) / pageSize,
	}
	cacheHelper.Set(ctx, cacheKey, result)
	return result, nil
}

type myAssessmentQuery struct {
	reader evaluationreadmodel.AssessmentReader
}

func (q myAssessmentQuery) List(
	ctx context.Context,
	dto ListMyAssessmentsDTO,
	page int,
	pageSize int,
) ([]*AssessmentResult, int64, error) {
	if q.reader == nil {
		return nil, 0, evalerrors.ModuleNotConfigured("assessment read model is not configured")
	}
	rows, total, err := q.reader.ListAssessments(ctx, evaluationreadmodel.AssessmentFilter{
		TesteeID:  &dto.TesteeID,
		Statuses:  normalizeMyAssessmentStatuses(dto.Status),
		ScaleCode: dto.ScaleCode,
		RiskLevel: dto.RiskLevel,
		DateFrom:  dto.DateFrom,
		DateTo:    dto.DateTo,
	}, evaluationreadmodel.PageRequest{Page: page, PageSize: pageSize})
	if err != nil {
		return nil, 0, err
	}
	items, err := assessmentRowsToResults(rows)
	return items, total, err
}

func normalizeMyAssessmentStatuses(raw string) []string {
	switch raw {
	case "":
		return nil
	case "pending":
		return []string{assessment.StatusPending.String(), assessment.StatusSubmitted.String()}
	case "done":
		return []string{assessment.StatusInterpreted.String()}
	default:
		return []string{raw}
	}
}
