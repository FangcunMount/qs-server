package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"time"
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
		// 1. apiserver repository 层在 FindByCode 时负责缓存读写和失效
		// 2. worker 事件处理主要用于通知其他服务（如 collection-server、search-service）
		// 3. 首次访问时自动加载到缓存，无需预热
		//
		// 如需实现缓存预热（Eager Loading），需要：
		// - 方案A: Worker添加MongoDB客户端，直接查询并写入Redis
		// - 方案B: 通过gRPC调用apiserver的查询接口触发缓存加载
		// - 方案C: 在apiserver的PublishScale方法中同步写入缓存
		//
		// 权衡：预热能减少首次访问延迟，但增加系统复杂度。
		// 对于低频访问的量表，懒加载更经济；高频量表可考虑预热。

		// 生成小程序码（通过 gRPC 调用 apiserver）
		if deps.InternalClient != nil {
			resp, err := deps.InternalClient.GenerateScaleQRCode(ctx, data.Code)
			if err != nil {
				deps.Logger.Warn("failed to generate scale QR code",
					slog.String("event_id", env.ID),
					slog.String("code", data.Code),
					slog.String("error", err.Error()),
				)
				// 生成二维码失败不影响事件处理，只记录警告
			} else if resp.Success {
				deps.Logger.Info("scale QR code generated",
					slog.String("event_id", env.ID),
					slog.String("code", data.Code),
					slog.String("qrcode_url", resp.QrcodeUrl),
				)
			} else {
				deps.Logger.Warn("scale QR code generation failed",
					slog.String("event_id", env.ID),
					slog.String("code", data.Code),
					slog.String("message", resp.Message),
				)
			}
		}

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

		// 缓存失效已由 Repository 层自动处理，此处无需重复失效
		// 事件主要用于通知其他服务（如 collection-server、search-service）

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

		// 缓存失效已由 Repository 层自动处理，此处无需重复失效
		// 事件主要用于通知其他服务（如 collection-server、search-service）

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

		// 缓存失效已由 Repository 层自动处理，此处无需重复失效
		// 事件主要用于通知其他服务（如 collection-server、search-service）

		return nil
	}
}

