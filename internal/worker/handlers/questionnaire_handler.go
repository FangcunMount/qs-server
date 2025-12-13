package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	redis "github.com/redis/go-redis/v9"
)

func init() {
	Register("questionnaire_published_handler", func(deps *Dependencies) HandlerFunc {
		return handleQuestionnairePublished(deps)
	})
	Register("questionnaire_unpublished_handler", func(deps *Dependencies) HandlerFunc {
		return handleQuestionnaireUnpublished(deps)
	})
	Register("questionnaire_archived_handler", func(deps *Dependencies) HandlerFunc {
		return handleQuestionnaireArchived(deps)
	})
}

// ==================== Payload 定义 ====================

// QuestionnairePublishedPayload 问卷发布事件数据
type QuestionnairePublishedPayload struct {
	Code        string    `json:"code"`
	Version     string    `json:"version"`
	Title       string    `json:"title"`
	PublishedAt time.Time `json:"published_at"`
}

// QuestionnaireUnpublishedPayload 问卷下架事件数据
type QuestionnaireUnpublishedPayload struct {
	Code          string    `json:"code"`
	Version       string    `json:"version"`
	UnpublishedAt time.Time `json:"unpublished_at"`
}

// QuestionnaireArchivedPayload 问卷归档事件数据
type QuestionnaireArchivedPayload struct {
	Code       string    `json:"code"`
	Version    string    `json:"version"`
	ArchivedAt time.Time `json:"archived_at"`
}

// ==================== 辅助函数 ====================

const questionnaireCachePrefix = "questionnaire:"

func deleteQuestionnaireCache(ctx context.Context, redisCache redis.UniversalClient, logger *slog.Logger, code, version string) {
	if redisCache == nil {
		return
	}

	cacheKey := fmt.Sprintf("%s%s:%s", questionnaireCachePrefix, code, version)
	if err := redisCache.Del(ctx, cacheKey).Err(); err != nil {
		logger.Warn("failed to delete questionnaire cache",
			slog.String("cache_key", cacheKey),
			slog.String("error", err.Error()),
		)
	} else {
		logger.Info("questionnaire cache deleted", slog.String("cache_key", cacheKey))
	}
}

func clearQuestionnaireCachesByCode(ctx context.Context, redisCache redis.UniversalClient, logger *slog.Logger, code string) {
	if redisCache == nil {
		return
	}

	pattern := fmt.Sprintf("%s%s:*", questionnaireCachePrefix, code)
	iter := redisCache.Scan(ctx, 0, pattern, 100).Iterator()
	for iter.Next(ctx) {
		key := iter.Val()
		if err := redisCache.Del(ctx, key).Err(); err != nil {
			logger.Warn("failed to delete cache key", slog.String("key", key), slog.String("error", err.Error()))
		}
	}
	if err := iter.Err(); err != nil {
		logger.Warn("cache scan error", slog.String("pattern", pattern), slog.String("error", err.Error()))
	} else {
		logger.Info("questionnaire caches cleared", slog.String("pattern", pattern))
	}
}

// ==================== Handler 实现 ====================

func handleQuestionnairePublished(deps *Dependencies) HandlerFunc {
	return func(ctx context.Context, eventType string, payload []byte) error {
		var data QuestionnairePublishedPayload
		env, err := ParseEventData(payload, &data)
		if err != nil {
			return fmt.Errorf("failed to parse questionnaire published event: %w", err)
		}

		deps.Logger.Info("processing questionnaire published",
			slog.String("event_id", env.ID),
			slog.String("code", data.Code),
			slog.String("version", data.Version),
			slog.String("title", data.Title),
		)

		// 缓存预热策略：
		// 与量表相同，采用 Lazy Loading 模式。问卷数据结构较大（包含题目、选项等），
		// 主动预热会占用较多内存和Redis空间。
		//
		// 建议场景：
		// - 高并发问卷（如入校筛查）：可在发布时预热，避免瞬时峰值导致的cache stampede
		// - 低频问卷：按需加载更经济
		//
		// 实现参考 handleScalePublished 的注释，实施方案相同。
		// 缓存key建议格式：questionnaire:{code}:{version}

		return nil
	}
}

func handleQuestionnaireUnpublished(deps *Dependencies) HandlerFunc {
	return func(ctx context.Context, eventType string, payload []byte) error {
		var data QuestionnaireUnpublishedPayload
		env, err := ParseEventData(payload, &data)
		if err != nil {
			return fmt.Errorf("failed to parse questionnaire unpublished event: %w", err)
		}

		deps.Logger.Info("processing questionnaire unpublished",
			slog.String("event_id", env.ID),
			slog.String("code", data.Code),
			slog.String("version", data.Version),
		)

		deleteQuestionnaireCache(ctx, deps.RedisCache, deps.Logger, data.Code, data.Version)

		return nil
	}
}

func handleQuestionnaireArchived(deps *Dependencies) HandlerFunc {
	return func(ctx context.Context, eventType string, payload []byte) error {
		var data QuestionnaireArchivedPayload
		env, err := ParseEventData(payload, &data)
		if err != nil {
			return fmt.Errorf("failed to parse questionnaire archived event: %w", err)
		}

		deps.Logger.Info("processing questionnaire archived",
			slog.String("event_id", env.ID),
			slog.String("code", data.Code),
			slog.String("version", data.Version),
		)

		clearQuestionnaireCachesByCode(ctx, deps.RedisCache, deps.Logger, data.Code)

		return nil
	}
}
