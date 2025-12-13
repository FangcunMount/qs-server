package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	redis "github.com/redis/go-redis/v9"
)

func init() {
	Register("scale_published_handler", func(deps *Dependencies) HandlerFunc {
		return handleScalePublished(deps)
	})
	Register("scale_unpublished_handler", func(deps *Dependencies) HandlerFunc {
		return handleScaleUnpublished(deps)
	})
	Register("scale_updated_handler", func(deps *Dependencies) HandlerFunc {
		return handleScaleUpdated(deps)
	})
	Register("scale_archived_handler", func(deps *Dependencies) HandlerFunc {
		return handleScaleArchived(deps)
	})
}

// ==================== Payload 定义 ====================

// ScalePublishedPayload 量表发布事件数据
type ScalePublishedPayload struct {
	ScaleID     uint64    `json:"scale_id"`
	Code        string    `json:"code"`
	Version     string    `json:"version"`
	Name        string    `json:"name"`
	PublishedAt time.Time `json:"published_at"`
}

// ScaleUnpublishedPayload 量表下架事件数据
type ScaleUnpublishedPayload struct {
	ScaleID       uint64    `json:"scale_id"`
	Code          string    `json:"code"`
	Version       string    `json:"version"`
	UnpublishedAt time.Time `json:"unpublished_at"`
}

// ScaleUpdatedPayload 量表更新事件数据
type ScaleUpdatedPayload struct {
	ScaleID   uint64    `json:"scale_id"`
	Code      string    `json:"code"`
	Version   string    `json:"version"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ScaleArchivedPayload 量表归档事件数据
type ScaleArchivedPayload struct {
	ScaleID    uint64    `json:"scale_id"`
	Code       string    `json:"code"`
	Version    string    `json:"version"`
	ArchivedAt time.Time `json:"archived_at"`
}

// ==================== 辅助函数 ====================

const (
	scaleCachePrefix = "scale:"
	scaleRulePrefix  = "scale:rule:"
)

func deleteScaleCache(ctx context.Context, redisCache redis.UniversalClient, logger *slog.Logger, code, version string) {
	if redisCache == nil {
		return
	}

	cacheKey := fmt.Sprintf("%s%s:%s", scaleCachePrefix, code, version)
	if err := redisCache.Del(ctx, cacheKey).Err(); err != nil {
		logger.Warn("failed to delete scale cache",
			slog.String("cache_key", cacheKey),
			slog.String("error", err.Error()),
		)
	}
}

func clearScaleCachesByCode(ctx context.Context, redisCache redis.UniversalClient, logger *slog.Logger, code string) {
	if redisCache == nil {
		return
	}

	patterns := []string{
		fmt.Sprintf("%s%s:*", scaleCachePrefix, code),
		fmt.Sprintf("%s%s:*", scaleRulePrefix, code),
	}

	for _, pattern := range patterns {
		iter := redisCache.Scan(ctx, 0, pattern, 100).Iterator()
		for iter.Next(ctx) {
			key := iter.Val()
			if err := redisCache.Del(ctx, key).Err(); err != nil {
				logger.Warn("failed to delete cache key", slog.String("key", key), slog.String("error", err.Error()))
			}
		}
		if err := iter.Err(); err != nil {
			logger.Warn("cache scan error", slog.String("pattern", pattern), slog.String("error", err.Error()))
		}
	}

	logger.Info("scale caches cleared", slog.String("code", code))
}

// ==================== Handler 实现 ====================

func handleScalePublished(deps *Dependencies) HandlerFunc {
	return func(ctx context.Context, eventType string, payload []byte) error {
		var data ScalePublishedPayload
		env, err := ParseEventData(payload, &data)
		if err != nil {
			return fmt.Errorf("failed to parse scale published event: %w", err)
		}

		deps.Logger.Info("processing scale published",
			slog.String("event_id", env.ID),
			slog.Uint64("scale_id", data.ScaleID),
			slog.String("code", data.Code),
			slog.String("version", data.Version),
			slog.String("name", data.Name),
		)

		// 缓存策略说明：
		// 当前采用 Lazy Loading（懒加载）+ Cache-Aside 模式：
		// 1. apiserver repository 层在 FindByCode 时负责缓存读写
		// 2. worker 只负责缓存失效（unpublish时删除）
		// 3. 首次访问时自动加载到缓存，无需预热
		//
		// 如需实现缓存预热（Eager Loading），需要：
		// - 方案A: Worker添加MongoDB客户端，直接查询并写入Redis
		// - 方案B: 通过gRPC调用apiserver的查询接口触发缓存加载
		// - 方案C: 在apiserver的PublishScale方法中同步写入缓存
		//
		// 权衡：预热能减少首次访问延迟，但增加系统复杂度。
		// 对于低频访问的量表，懒加载更经济；高频量表可考虑预热。

		return nil
	}
}

func handleScaleUnpublished(deps *Dependencies) HandlerFunc {
	return func(ctx context.Context, eventType string, payload []byte) error {
		var data ScaleUnpublishedPayload
		env, err := ParseEventData(payload, &data)
		if err != nil {
			return fmt.Errorf("failed to parse scale unpublished event: %w", err)
		}

		deps.Logger.Info("processing scale unpublished",
			slog.String("event_id", env.ID),
			slog.Uint64("scale_id", data.ScaleID),
			slog.String("code", data.Code),
			slog.String("version", data.Version),
		)

		deleteScaleCache(ctx, deps.RedisCache, deps.Logger, data.Code, data.Version)

		return nil
	}
}

func handleScaleUpdated(deps *Dependencies) HandlerFunc {
	return func(ctx context.Context, eventType string, payload []byte) error {
		var data ScaleUpdatedPayload
		env, err := ParseEventData(payload, &data)
		if err != nil {
			return fmt.Errorf("failed to parse scale updated event: %w", err)
		}

		deps.Logger.Info("processing scale updated",
			slog.String("event_id", env.ID),
			slog.Uint64("scale_id", data.ScaleID),
			slog.String("code", data.Code),
			slog.String("version", data.Version),
		)

		// 缓存更新策略：
		// 量表更新时需要使缓存失效，否则会读取到旧数据。
		// 如果缓存key包含版本号（如 scale:{code}:{version}），
		// 则更新版本号时自动隔离，无需删除旧缓存。
		//
		// 当前实现：删除该版本的缓存，下次访问时重新加载
		if deps.RedisCache != nil {
			deleteScaleCache(ctx, deps.RedisCache, deps.Logger, data.Code, data.Version)
		}

		// 可选优化：删除后立即预热新数据（参考 handleScalePublished 注释）

		return nil
	}
}

func handleScaleArchived(deps *Dependencies) HandlerFunc {
	return func(ctx context.Context, eventType string, payload []byte) error {
		var data ScaleArchivedPayload
		env, err := ParseEventData(payload, &data)
		if err != nil {
			return fmt.Errorf("failed to parse scale archived event: %w", err)
		}

		deps.Logger.Info("processing scale archived",
			slog.String("event_id", env.ID),
			slog.Uint64("scale_id", data.ScaleID),
			slog.String("code", data.Code),
			slog.String("version", data.Version),
		)

		clearScaleCachesByCode(ctx, deps.RedisCache, deps.Logger, data.Code)

		return nil
	}
}
