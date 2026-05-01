package scale

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/cachetarget"
	domainScale "github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/scalereadmodel"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
)

// ListPublished 查询已发布量表摘要列表。
func (s *queryService) ListPublished(ctx context.Context, dto ListScalesDTO) (*ScaleSummaryListResult, error) {
	if err := validateScaleListPage(dto.Page, dto.PageSize); err != nil {
		return nil, err
	}

	filter, err := s.normalizeScaleFilter(dto.Filter)
	if err != nil {
		return nil, err
	}
	filter.Status = domainScale.StatusPublished.Value()

	if s.canUsePublishedListCache(filter) {
		if cached, ok := s.listCache.GetPage(ctx, dto.Page, dto.PageSize); ok {
			return scaleSummaryListResultFromCachePage(cached), nil
		}
	}

	result, err := s.listScaleSummaryRows(ctx, filter, dto.Page, dto.PageSize)
	if err != nil {
		return nil, err
	}

	if s.canUsePublishedListCache(filter) {
		go func() {
			_ = s.listCache.Rebuild(context.Background())
		}()
	}
	s.recordHotset(ctx, cachetarget.NewStaticScaleListWarmupTarget())

	return result, nil
}

func validateScaleListPage(page, pageSize int) error {
	if page <= 0 {
		return errors.WithCode(errorCode.ErrInvalidArgument, "页码必须大于0")
	}
	if pageSize <= 0 {
		return errors.WithCode(errorCode.ErrInvalidArgument, "每页数量必须大于0")
	}
	if pageSize > 100 {
		return errors.WithCode(errorCode.ErrInvalidArgument, "每页数量不能超过100")
	}
	return nil
}

func (s *queryService) canUsePublishedListCache(filter scalereadmodel.ScaleFilter) bool {
	return filter.Title == "" && filter.Category == "" && s.listCache != nil
}

func (s *queryService) listScaleSummaryRows(ctx context.Context, filter scalereadmodel.ScaleFilter, page, pageSize int) (*ScaleSummaryListResult, error) {
	items, err := s.reader.ListScales(ctx, filter, scalereadmodel.PageRequest{Page: page, PageSize: pageSize})
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "获取量表列表失败")
	}

	total, err := s.reader.CountScales(ctx, filter)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "获取量表总数失败")
	}

	return toSummaryRowsResult(ctx, items, total, s.identitySvc), nil
}
