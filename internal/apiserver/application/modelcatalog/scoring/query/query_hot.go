package query

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/scoring/legacyadapter"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/scoring/shared"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/scalereadmodel"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/scalereadmodel/hotrank"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
)

// ListHotPublished 查询热门已发布量表摘要列表。
func (s *queryService) ListHotPublished(ctx context.Context, dto shared.ListHotScalesDTO) (*shared.HotScaleListResult, error) {
	limit := normalizeHotScaleLimit(dto.Limit)
	windowDays := normalizeHotScaleWindowDays(dto.WindowDays)

	if cached, ok := s.loadHotScaleListCache(ctx, limit, windowDays); ok {
		return cached, nil
	}

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

	result := shared.ToHotScaleRowsListResult(ctx, hotItems, limit, windowDays, s.identitySvc)
	s.storeHotScaleListCache(ctx, limit, windowDays, result)
	return result, nil
}

func (s *queryService) loadHotScaleListCache(ctx context.Context, limit, windowDays int) (*shared.HotScaleListResult, bool) {
	if s == nil || s.hotListCache == nil {
		return nil, false
	}
	data, ok := s.hotListCache.Get(ctx, limit, windowDays)
	if !ok || len(data) == 0 {
		return nil, false
	}
	var result shared.HotScaleListResult
	if err := json.Unmarshal(data, &result); err != nil {
		logger.L(ctx).Warnw("failed to decode hot scale list cache",
			"limit", limit,
			"window_days", windowDays,
			"error", err,
		)
		return nil, false
	}
	if len(result.Items) == 0 {
		return nil, false
	}
	return &result, true
}

func (s *queryService) storeHotScaleListCache(ctx context.Context, limit, windowDays int, result *shared.HotScaleListResult) {
	if s == nil || s.hotListCache == nil || result == nil || len(result.Items) == 0 {
		return
	}
	data, err := json.Marshal(result)
	if err != nil {
		logger.L(ctx).Warnw("failed to encode hot scale list cache",
			"limit", limit,
			"window_days", windowDays,
			"error", err,
		)
		return
	}
	if err := s.hotListCache.Set(ctx, limit, windowDays, data); err != nil {
		logger.L(ctx).Warnw("failed to store hot scale list cache",
			"limit", limit,
			"window_days", windowDays,
			"error", err,
		)
	}
}

func (s *queryService) loadHotScaleRank(ctx context.Context, limit, windowDays int) ([]shared.HotScaleSummaryRow, error) {
	if s == nil || s.hotRank == nil {
		return []shared.HotScaleSummaryRow{}, nil
	}
	rankItems, err := s.hotRank.Top(ctx, hotrank.Query{
		WindowDays: windowDays,
		Limit:      hotRankCandidateLimit(limit),
	})
	if err != nil {
		return nil, err
	}

	result := make([]shared.HotScaleSummaryRow, 0, limit)
	seen := make(map[string]struct{}, len(rankItems))
	for _, rankItem := range rankItems {
		questionnaireCode := strings.TrimSpace(rankItem.QuestionnaireCode)
		if questionnaireCode == "" {
			continue
		}
		item, ok, err := s.publishedScaleRowByQuestionnaireCode(ctx, questionnaireCode)
		if err != nil {
			logger.L(ctx).Warnw("failed to resolve hot scale by questionnaire code",
				"questionnaire_code", questionnaireCode,
				"error", err,
			)
			continue
		}
		if !ok || !isPublishedOpenScaleRow(item) {
			continue
		}
		code := item.Code
		if _, ok := seen[code]; ok {
			continue
		}
		seen[code] = struct{}{}
		result = append(result, shared.HotScaleSummaryRow{
			Scale:           item,
			SubmissionCount: rankItem.Score,
		})
		if len(result) >= limit {
			break
		}
	}

	return result, nil
}

func (s *queryService) loadHotScaleFallback(ctx context.Context, limit int, existing []shared.HotScaleSummaryRow) ([]shared.HotScaleSummaryRow, error) {
	seen := make(map[string]struct{}, len(existing))
	for _, item := range existing {
		if item.Scale.Code == "" {
			continue
		}
		seen[item.Scale.Code] = struct{}{}
	}

	rows, err := s.reader.ListScales(ctx, scalereadmodel.ScaleFilter{Status: scalereadmodel.ScaleStatusPublished, PublishedOnly: true}, scalereadmodel.PageRequest{Page: 1, PageSize: 100})
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "获取热门量表兜底列表失败")
	}

	result := make([]shared.HotScaleSummaryRow, 0, limit-len(existing))
	for _, row := range rows {
		if !isPublishedOpenScaleRow(row) {
			continue
		}
		code := row.Code
		if _, ok := seen[code]; ok {
			continue
		}
		seen[code] = struct{}{}
		result = append(result, shared.HotScaleSummaryRow{Scale: row})
		if len(existing)+len(result) >= limit {
			break
		}
	}
	return result, nil
}

func (s *queryService) publishedScaleRowByQuestionnaireCode(ctx context.Context, questionnaireCode string) (scalereadmodel.ScaleSummaryRow, bool, error) {
	if s == nil || s.readerV2 == nil {
		return scalereadmodel.ScaleSummaryRow{}, false, domain.ErrNotFound
	}
	snapshot, err := s.readerV2.FindPublishedModelByQuestionnaire(ctx, questionnaireCode, "")
	if err != nil {
		return scalereadmodel.ScaleSummaryRow{}, false, err
	}
	result, err := legacyadapter.ScaleResultFromPublishedModel(snapshot)
	if err != nil {
		return scalereadmodel.ScaleSummaryRow{}, false, err
	}
	return scaleSummaryRowFromResult(result), true, nil
}

func scaleSummaryRowFromResult(result *shared.ScaleResult) scalereadmodel.ScaleSummaryRow {
	if result == nil {
		return scalereadmodel.ScaleSummaryRow{}
	}
	return scalereadmodel.ScaleSummaryRow{
		Code:              result.Code,
		ScaleVersion:      result.ScaleVersion,
		Title:             result.Title,
		Description:       result.Description,
		Category:          result.Category,
		Stages:            append([]string(nil), result.Stages...),
		ApplicableAges:    append([]string(nil), result.ApplicableAges...),
		Reporters:         append([]string(nil), result.Reporters...),
		Tags:              append([]string(nil), result.Tags...),
		QuestionnaireCode: result.QuestionnaireCode,
		QuestionCount:     result.QuestionCount,
		Status:            result.Status,
		CreatedAt:         result.CreatedAt,
		UpdatedAt:         result.UpdatedAt,
	}
}

func isPublishedOpenScaleRow(row scalereadmodel.ScaleSummaryRow) bool {
	return row.Status == scalereadmodel.ScaleStatusPublished && isOpenScaleCategory(row.Category)
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
