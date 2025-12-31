package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"time"
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

		// 生成小程序码（通过 gRPC 调用 apiserver）
		if deps.InternalClient != nil {
			resp, err := deps.InternalClient.GenerateQuestionnaireQRCode(ctx, data.Code, data.Version)
			if err != nil {
				deps.Logger.Warn("failed to generate questionnaire QR code",
					slog.String("event_id", env.ID),
					slog.String("code", data.Code),
					slog.String("error", err.Error()),
				)
				// 生成二维码失败不影响事件处理，只记录警告
			} else if resp.Success {
				deps.Logger.Info("questionnaire QR code generated",
					slog.String("event_id", env.ID),
					slog.String("code", data.Code),
					slog.String("qrcode_url", resp.QrcodeUrl),
				)
			} else {
				deps.Logger.Warn("questionnaire QR code generation failed",
					slog.String("event_id", env.ID),
					slog.String("code", data.Code),
					slog.String("message", resp.Message),
				)
			}
		}

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

		// 缓存失效已由 Repository 层自动处理，此处无需重复失效
		// 事件主要用于通知其他服务（如 collection-server、search-service）

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

		// 缓存失效已由 Repository 层自动处理，此处无需重复失效
		// 事件主要用于通知其他服务（如 collection-server、search-service）

		return nil
	}
}
