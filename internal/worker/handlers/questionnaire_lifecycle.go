package handlers

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"
)

// ==================== QuestionnairePublishedEventDTO ====================

// QuestionnairePublishedEventDTO 问卷发布事件 DTO（用于 JSON 反序列化）
type QuestionnairePublishedEventDTO struct {
	EventID         string `json:"event_id"`
	EventType       string `json:"event_type"`
	QuestionnaireID uint64 `json:"questionnaire_id"`
	Code            string `json:"code"`
	Version         string `json:"version"`
	Title           string `json:"title"`
	PublishedAt     string `json:"published_at"`
}

// ==================== QuestionnairePublishedHandler ====================

// QuestionnairePublishedHandler 处理问卷发布事件
// 职责：
// - 通知 collection-server 更新缓存
// - 预热问卷缓存
type QuestionnairePublishedHandler struct {
	*BaseHandler
	logger *slog.Logger
	// TODO: 注入 Redis 客户端或 collection-server gRPC 客户端
}

// NewQuestionnairePublishedHandler 创建问卷发布事件处理器
func NewQuestionnairePublishedHandler(logger *slog.Logger) *QuestionnairePublishedHandler {
	return &QuestionnairePublishedHandler{
		BaseHandler: NewBaseHandler("questionnaire.published", "questionnaire_published_handler"),
		logger:      logger,
	}
}

// Handle 处理问卷发布事件
func (h *QuestionnairePublishedHandler) Handle(ctx context.Context, payload []byte) error {
	var dto QuestionnairePublishedEventDTO
	if err := json.Unmarshal(payload, &dto); err != nil {
		h.logger.Error("failed to unmarshal questionnaire published event",
			slog.String("handler", h.Name()),
			slog.String("error", err.Error()),
		)
		return err
	}

	h.logger.Info("processing questionnaire published event",
		slog.String("handler", h.Name()),
		slog.String("event_id", dto.EventID),
		slog.Uint64("questionnaire_id", dto.QuestionnaireID),
		slog.String("code", dto.Code),
		slog.String("version", dto.Version),
		slog.String("title", dto.Title),
	)

	// 1. 更新/预热问卷缓存
	if err := h.warmupQuestionnaireCache(ctx, &dto); err != nil {
		h.logger.Error("failed to warmup questionnaire cache",
			slog.String("handler", h.Name()),
			slog.String("error", err.Error()),
		)
		return err
	}

	h.logger.Info("questionnaire published event processed successfully",
		slog.String("handler", h.Name()),
		slog.String("code", dto.Code),
	)

	return nil
}

// warmupQuestionnaireCache 预热问卷缓存
func (h *QuestionnairePublishedHandler) warmupQuestionnaireCache(ctx context.Context, dto *QuestionnairePublishedEventDTO) error {
	h.logger.Debug("warming up questionnaire cache",
		slog.String("code", dto.Code),
		slog.String("version", dto.Version),
	)

	// TODO: 实现缓存预热
	// 方案1: 直接操作 Redis
	// questionnaireJSON, err := h.questionnaireClient.GetQuestionnaire(ctx, dto.Code, dto.Version)
	// h.redisClient.Set(ctx, fmt.Sprintf("questionnaire:%s:%s", dto.Code, dto.Version), questionnaireJSON, 24*time.Hour)
	//
	// 方案2: 调用 collection-server 的 gRPC 接口
	// h.collectionClient.WarmupCache(ctx, &WarmupCacheRequest{Code: dto.Code, Version: dto.Version})

	return nil
}

// ==================== QuestionnaireUnpublishedEventDTO ====================

// QuestionnaireUnpublishedEventDTO 问卷下架事件 DTO
type QuestionnaireUnpublishedEventDTO struct {
	EventID         string `json:"event_id"`
	EventType       string `json:"event_type"`
	QuestionnaireID uint64 `json:"questionnaire_id"`
	Code            string `json:"code"`
	Version         string `json:"version"`
	UnpublishedAt   string `json:"unpublished_at"`
}

// ==================== QuestionnaireUnpublishedHandler ====================

// QuestionnaireUnpublishedHandler 处理问卷下架事件
// 职责：清除问卷缓存
type QuestionnaireUnpublishedHandler struct {
	*BaseHandler
	logger *slog.Logger
}

// NewQuestionnaireUnpublishedHandler 创建问卷下架事件处理器
func NewQuestionnaireUnpublishedHandler(logger *slog.Logger) *QuestionnaireUnpublishedHandler {
	return &QuestionnaireUnpublishedHandler{
		BaseHandler: NewBaseHandler("questionnaire.unpublished", "questionnaire_unpublished_handler"),
		logger:      logger,
	}
}

// Handle 处理问卷下架事件
func (h *QuestionnaireUnpublishedHandler) Handle(ctx context.Context, payload []byte) error {
	var dto QuestionnaireUnpublishedEventDTO
	if err := json.Unmarshal(payload, &dto); err != nil {
		h.logger.Error("failed to unmarshal questionnaire unpublished event",
			slog.String("handler", h.Name()),
			slog.String("error", err.Error()),
		)
		return err
	}

	h.logger.Info("processing questionnaire unpublished event",
		slog.String("handler", h.Name()),
		slog.String("event_id", dto.EventID),
		slog.String("code", dto.Code),
		slog.String("version", dto.Version),
	)

	// 清除问卷缓存
	if err := h.invalidateQuestionnaireCache(ctx, &dto); err != nil {
		h.logger.Error("failed to invalidate questionnaire cache",
			slog.String("handler", h.Name()),
			slog.String("error", err.Error()),
		)
		return err
	}

	h.logger.Info("questionnaire unpublished event processed successfully",
		slog.String("handler", h.Name()),
		slog.String("code", dto.Code),
	)

	return nil
}

// invalidateQuestionnaireCache 清除问卷缓存
func (h *QuestionnaireUnpublishedHandler) invalidateQuestionnaireCache(ctx context.Context, dto *QuestionnaireUnpublishedEventDTO) error {
	h.logger.Debug("invalidating questionnaire cache",
		slog.String("code", dto.Code),
		slog.String("version", dto.Version),
	)

	// TODO: 实现缓存清除
	// h.redisClient.Del(ctx, fmt.Sprintf("questionnaire:%s:%s", dto.Code, dto.Version))

	return nil
}

// ==================== QuestionnaireArchivedHandler ====================

// QuestionnaireArchivedEventDTO 问卷归档事件 DTO
type QuestionnaireArchivedEventDTO struct {
	EventID         string `json:"event_id"`
	EventType       string `json:"event_type"`
	QuestionnaireID uint64 `json:"questionnaire_id"`
	Code            string `json:"code"`
	Version         string `json:"version"`
	ArchivedAt      string `json:"archived_at"`
}

// QuestionnaireArchivedHandler 处理问卷归档事件
type QuestionnaireArchivedHandler struct {
	*BaseHandler
	logger *slog.Logger
}

// NewQuestionnaireArchivedHandler 创建问卷归档事件处理器
func NewQuestionnaireArchivedHandler(logger *slog.Logger) *QuestionnaireArchivedHandler {
	return &QuestionnaireArchivedHandler{
		BaseHandler: NewBaseHandler("questionnaire.archived", "questionnaire_archived_handler"),
		logger:      logger,
	}
}

// Handle 处理问卷归档事件
func (h *QuestionnaireArchivedHandler) Handle(ctx context.Context, payload []byte) error {
	var dto QuestionnaireArchivedEventDTO
	if err := json.Unmarshal(payload, &dto); err != nil {
		h.logger.Error("failed to unmarshal questionnaire archived event",
			slog.String("handler", h.Name()),
			slog.String("error", err.Error()),
		)
		return err
	}

	h.logger.Info("processing questionnaire archived event",
		slog.String("handler", h.Name()),
		slog.String("event_id", dto.EventID),
		slog.String("code", dto.Code),
	)

	// 清除所有版本的缓存
	h.logger.Debug("invalidating all questionnaire cache versions",
		slog.String("code", dto.Code),
	)

	// TODO: 实现缓存清除
	// h.redisClient.Del(ctx, fmt.Sprintf("questionnaire:%s:*", dto.Code))

	h.logger.Info("questionnaire archived event processed successfully",
		slog.String("handler", h.Name()),
		slog.String("code", dto.Code),
	)

	return nil
}

// 确保编译器检查时间包被使用（用于 TODO 注释中的 time.Hour）
var _ = time.Hour
