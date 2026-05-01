package scale

import (
	"context"
	"strings"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/logger"
	domainScale "github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/scalereadmodel"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
)

// ListHotPublished 查询热门已发布量表摘要列表。
func (s *queryService) ListHotPublished(ctx context.Context, dto ListHotScalesDTO) (*HotScaleListResult, error) {
	limit := normalizeHotScaleLimit(dto.Limit)
	windowDays := normalizeHotScaleWindowDays(dto.WindowDays)

	hotItems, err := s.loadHotScaleRank(ctx, limit, windowDays)
	if err != nil {
		logger.L(ctx).Warnw("failed to load hot scale rank from redis",
			"window_days", windowDays,
			"limit", limit,
			"error", err,
		)
	}

	if len(hotItems) < limit {
		fallback, err := s.loadHotScaleFallback(ctx, limit, hotItems)
		if err != nil {
			return nil, err
		}
		hotItems = append(hotItems, fallback...)
	}
	if len(hotItems) > limit {
		hotItems = hotItems[:limit]
	}

	return toHotScaleListResult(ctx, hotItems, limit, windowDays, s.identitySvc), nil
}

func (s *queryService) loadHotScaleRank(ctx context.Context, limit, windowDays int) ([]domainScale.HotScaleSummary, error) {
	if s == nil || s.hotRank == nil {
		return []domainScale.HotScaleSummary{}, nil
	}
	rankItems, err := s.hotRank.Top(ctx, domainScale.ScaleHotRankQuery{
		WindowDays: windowDays,
		Limit:      hotRankCandidateLimit(limit),
	})
	if err != nil {
		return nil, err
	}

	result := make([]domainScale.HotScaleSummary, 0, limit)
	seen := make(map[string]struct{}, len(rankItems))
	for _, rankItem := range rankItems {
		questionnaireCode := strings.TrimSpace(rankItem.QuestionnaireCode)
		if questionnaireCode == "" {
			continue
		}
		item, err := s.repo.FindByQuestionnaireCode(ctx, questionnaireCode)
		if err != nil {
			logger.L(ctx).Warnw("failed to resolve hot scale by questionnaire code",
				"questionnaire_code", questionnaireCode,
				"error", err,
			)
			continue
		}
		if item == nil || !item.IsPublished() || !item.GetCategory().IsOpen() {
			continue
		}
		code := item.GetCode().String()
		if _, ok := seen[code]; ok {
			continue
		}
		seen[code] = struct{}{}
		result = append(result, domainScale.HotScaleSummary{
			Scale:           item,
			SubmissionCount: rankItem.Score,
		})
		if len(result) >= limit {
			break
		}
	}

	return result, nil
}

func (s *queryService) loadHotScaleFallback(ctx context.Context, limit int, existing []domainScale.HotScaleSummary) ([]domainScale.HotScaleSummary, error) {
	seen := make(map[string]struct{}, len(existing))
	for _, item := range existing {
		if item.Scale == nil {
			continue
		}
		seen[item.Scale.GetCode().String()] = struct{}{}
	}

	rows, err := s.reader.ListScales(ctx, scalereadmodel.ScaleFilter{Status: domainScale.StatusPublished.Value()}, scalereadmodel.PageRequest{Page: 1, PageSize: 100})
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "获取热门量表兜底列表失败")
	}

	result := make([]domainScale.HotScaleSummary, 0, limit-len(existing))
	for _, row := range rows {
		item, err := s.repo.FindByCode(ctx, row.Code)
		if err != nil {
			logger.L(ctx).Warnw("failed to resolve fallback hot scale",
				"scale_code", row.Code,
				"error", err,
			)
			continue
		}
		if item == nil || !item.IsPublished() || !item.GetCategory().IsOpen() {
			continue
		}
		code := item.GetCode().String()
		if _, ok := seen[code]; ok {
			continue
		}
		seen[code] = struct{}{}
		result = append(result, domainScale.HotScaleSummary{Scale: item})
		if len(existing)+len(result) >= limit {
			break
		}
	}
	return result, nil
}

func hotRankCandidateLimit(limit int) int {
	candidateLimit := limit * 4
	if candidateLimit < 20 {
		return 20
	}
	if candidateLimit > 100 {
		return 100
	}
	return candidateLimit
}

func normalizeHotScaleLimit(limit int) int {
	if limit <= 0 {
		return defaultHotScaleLimit
	}
	if limit < minHotScaleLimit {
		return minHotScaleLimit
	}
	if limit > maxHotScaleLimit {
		return maxHotScaleLimit
	}
	return limit
}

func normalizeHotScaleWindowDays(windowDays int) int {
	if windowDays <= 0 {
		return defaultHotScaleWindowDays
	}
	if windowDays > maxHotScaleWindowDays {
		return maxHotScaleWindowDays
	}
	return windowDays
}
