package handlers

import (
	"context"
	"encoding/json"
	"log/slog"
)

// ==================== ScalePublishedEventDTO ====================

// ScalePublishedEventDTO 量表发布事件 DTO
type ScalePublishedEventDTO struct {
	EventID     string `json:"event_id"`
	EventType   string `json:"event_type"`
	ScaleID     uint64 `json:"scale_id"`
	Code        string `json:"code"`
	Version     string `json:"version"`
	Name        string `json:"name"`
	PublishedAt string `json:"published_at"`
}

// ==================== ScalePublishedHandler ====================

// ScalePublishedHandler 处理量表发布事件
// 职责：
// - 更新量表缓存
// - 预加载计算规则到 qs-worker
type ScalePublishedHandler struct {
	*BaseHandler
	logger *slog.Logger
}

// NewScalePublishedHandler 创建量表发布事件处理器
func NewScalePublishedHandler(logger *slog.Logger) *ScalePublishedHandler {
	return &ScalePublishedHandler{
		BaseHandler: NewBaseHandler("scale.published", "scale_published_handler"),
		logger:      logger,
	}
}

// Handle 处理量表发布事件
func (h *ScalePublishedHandler) Handle(ctx context.Context, payload []byte) error {
	var dto ScalePublishedEventDTO
	if err := json.Unmarshal(payload, &dto); err != nil {
		h.logger.Error("failed to unmarshal scale published event",
			slog.String("handler", h.Name()),
			slog.String("error", err.Error()),
		)
		return err
	}

	h.logger.Info("processing scale published event",
		slog.String("handler", h.Name()),
		slog.String("event_id", dto.EventID),
		slog.Uint64("scale_id", dto.ScaleID),
		slog.String("code", dto.Code),
		slog.String("version", dto.Version),
		slog.String("name", dto.Name),
	)

	// 1. 更新量表缓存
	if err := h.warmupScaleCache(ctx, &dto); err != nil {
		h.logger.Error("failed to warmup scale cache",
			slog.String("handler", h.Name()),
			slog.String("error", err.Error()),
		)
		return err
	}

	// 2. 预加载计算规则
	if err := h.preloadCalculationRules(ctx, &dto); err != nil {
		h.logger.Error("failed to preload calculation rules",
			slog.String("handler", h.Name()),
			slog.String("error", err.Error()),
		)
		// 预加载失败不阻塞，规则会在使用时懒加载
	}

	h.logger.Info("scale published event processed successfully",
		slog.String("handler", h.Name()),
		slog.String("code", dto.Code),
	)

	return nil
}

// warmupScaleCache 预热量表缓存
func (h *ScalePublishedHandler) warmupScaleCache(ctx context.Context, dto *ScalePublishedEventDTO) error {
	h.logger.Debug("warming up scale cache",
		slog.String("code", dto.Code),
		slog.String("version", dto.Version),
	)

	// TODO: 实现缓存预热
	// scaleJSON, err := h.scaleClient.GetScale(ctx, dto.Code, dto.Version)
	// h.redisClient.Set(ctx, fmt.Sprintf("scale:%s:%s", dto.Code, dto.Version), scaleJSON, 24*time.Hour)

	return nil
}

// preloadCalculationRules 预加载计算规则
func (h *ScalePublishedHandler) preloadCalculationRules(ctx context.Context, dto *ScalePublishedEventDTO) error {
	h.logger.Debug("preloading calculation rules",
		slog.String("code", dto.Code),
		slog.String("version", dto.Version),
	)

	// TODO: 预加载量表的计算规则到内存或本地缓存
	// rules, err := h.scaleClient.GetCalculationRules(ctx, dto.Code, dto.Version)
	// h.ruleCache.Set(dto.Code, dto.Version, rules)

	return nil
}

// ==================== ScaleUnpublishedEventDTO ====================

// ScaleUnpublishedEventDTO 量表下架事件 DTO
type ScaleUnpublishedEventDTO struct {
	EventID       string `json:"event_id"`
	EventType     string `json:"event_type"`
	ScaleID       uint64 `json:"scale_id"`
	Code          string `json:"code"`
	Version       string `json:"version"`
	UnpublishedAt string `json:"unpublished_at"`
}

// ==================== ScaleUnpublishedHandler ====================

// ScaleUnpublishedHandler 处理量表下架事件
type ScaleUnpublishedHandler struct {
	*BaseHandler
	logger *slog.Logger
}

// NewScaleUnpublishedHandler 创建量表下架事件处理器
func NewScaleUnpublishedHandler(logger *slog.Logger) *ScaleUnpublishedHandler {
	return &ScaleUnpublishedHandler{
		BaseHandler: NewBaseHandler("scale.unpublished", "scale_unpublished_handler"),
		logger:      logger,
	}
}

// Handle 处理量表下架事件
func (h *ScaleUnpublishedHandler) Handle(ctx context.Context, payload []byte) error {
	var dto ScaleUnpublishedEventDTO
	if err := json.Unmarshal(payload, &dto); err != nil {
		h.logger.Error("failed to unmarshal scale unpublished event",
			slog.String("handler", h.Name()),
			slog.String("error", err.Error()),
		)
		return err
	}

	h.logger.Info("processing scale unpublished event",
		slog.String("handler", h.Name()),
		slog.String("event_id", dto.EventID),
		slog.String("code", dto.Code),
		slog.String("version", dto.Version),
	)

	// 清除缓存和计算规则
	h.invalidateScaleCache(ctx, &dto)
	h.invalidateCalculationRules(ctx, &dto)

	h.logger.Info("scale unpublished event processed successfully",
		slog.String("handler", h.Name()),
		slog.String("code", dto.Code),
	)

	return nil
}

// invalidateScaleCache 清除量表缓存
func (h *ScaleUnpublishedHandler) invalidateScaleCache(ctx context.Context, dto *ScaleUnpublishedEventDTO) {
	h.logger.Debug("invalidating scale cache",
		slog.String("code", dto.Code),
		slog.String("version", dto.Version),
	)
	// TODO: h.redisClient.Del(ctx, fmt.Sprintf("scale:%s:%s", dto.Code, dto.Version))
}

// invalidateCalculationRules 清除计算规则缓存
func (h *ScaleUnpublishedHandler) invalidateCalculationRules(ctx context.Context, dto *ScaleUnpublishedEventDTO) {
	h.logger.Debug("invalidating calculation rules",
		slog.String("code", dto.Code),
		slog.String("version", dto.Version),
	)
	// TODO: h.ruleCache.Delete(dto.Code, dto.Version)
}

// ==================== ScaleUpdatedEventDTO ====================

// ScaleUpdatedEventDTO 量表更新事件 DTO
type ScaleUpdatedEventDTO struct {
	EventID   string `json:"event_id"`
	EventType string `json:"event_type"`
	ScaleID   uint64 `json:"scale_id"`
	Code      string `json:"code"`
	Version   string `json:"version"`
	UpdatedAt string `json:"updated_at"`
}

// ==================== ScaleUpdatedHandler ====================

// ScaleUpdatedHandler 处理量表更新事件
// 职责：重新加载计算规则（当因子、解读规则变化时）
type ScaleUpdatedHandler struct {
	*BaseHandler
	logger *slog.Logger
}

// NewScaleUpdatedHandler 创建量表更新事件处理器
func NewScaleUpdatedHandler(logger *slog.Logger) *ScaleUpdatedHandler {
	return &ScaleUpdatedHandler{
		BaseHandler: NewBaseHandler("scale.updated", "scale_updated_handler"),
		logger:      logger,
	}
}

// Handle 处理量表更新事件
func (h *ScaleUpdatedHandler) Handle(ctx context.Context, payload []byte) error {
	var dto ScaleUpdatedEventDTO
	if err := json.Unmarshal(payload, &dto); err != nil {
		h.logger.Error("failed to unmarshal scale updated event",
			slog.String("handler", h.Name()),
			slog.String("error", err.Error()),
		)
		return err
	}

	h.logger.Info("processing scale updated event",
		slog.String("handler", h.Name()),
		slog.String("event_id", dto.EventID),
		slog.String("code", dto.Code),
		slog.String("version", dto.Version),
	)

	// 重新加载计算规则
	if err := h.reloadCalculationRules(ctx, &dto); err != nil {
		h.logger.Error("failed to reload calculation rules",
			slog.String("handler", h.Name()),
			slog.String("error", err.Error()),
		)
		return err
	}

	h.logger.Info("scale updated event processed successfully",
		slog.String("handler", h.Name()),
		slog.String("code", dto.Code),
	)

	return nil
}

// reloadCalculationRules 重新加载计算规则
func (h *ScaleUpdatedHandler) reloadCalculationRules(ctx context.Context, dto *ScaleUpdatedEventDTO) error {
	h.logger.Debug("reloading calculation rules",
		slog.String("code", dto.Code),
		slog.String("version", dto.Version),
	)

	// TODO:
	// 1. 清除旧的计算规则缓存
	// 2. 重新加载新的计算规则
	// h.ruleCache.Delete(dto.Code, dto.Version)
	// rules, err := h.scaleClient.GetCalculationRules(ctx, dto.Code, dto.Version)
	// h.ruleCache.Set(dto.Code, dto.Version, rules)

	return nil
}

// ==================== ScaleArchivedEventDTO ====================

// ScaleArchivedEventDTO 量表归档事件 DTO
type ScaleArchivedEventDTO struct {
	EventID    string `json:"event_id"`
	EventType  string `json:"event_type"`
	ScaleID    uint64 `json:"scale_id"`
	Code       string `json:"code"`
	Version    string `json:"version"`
	ArchivedAt string `json:"archived_at"`
}

// ==================== ScaleArchivedHandler ====================

// ScaleArchivedHandler 处理量表归档事件
// 职责：清除缓存和计算规则
type ScaleArchivedHandler struct {
	*BaseHandler
	logger *slog.Logger
}

// NewScaleArchivedHandler 创建量表归档事件处理器
func NewScaleArchivedHandler(logger *slog.Logger) *ScaleArchivedHandler {
	return &ScaleArchivedHandler{
		BaseHandler: NewBaseHandler("scale.archived", "scale_archived_handler"),
		logger:      logger,
	}
}

// Handle 处理量表归档事件
func (h *ScaleArchivedHandler) Handle(ctx context.Context, payload []byte) error {
	var dto ScaleArchivedEventDTO
	if err := json.Unmarshal(payload, &dto); err != nil {
		h.logger.Error("failed to unmarshal scale archived event",
			slog.String("handler", h.Name()),
			slog.String("error", err.Error()),
		)
		return err
	}

	h.logger.Info("processing scale archived event",
		slog.String("handler", h.Name()),
		slog.String("event_id", dto.EventID),
		slog.String("code", dto.Code),
		slog.String("version", dto.Version),
	)

	// 清除缓存和计算规则
	h.invalidateScaleCache(ctx, &dto)
	h.invalidateCalculationRules(ctx, &dto)

	h.logger.Info("scale archived event processed successfully",
		slog.String("handler", h.Name()),
		slog.String("code", dto.Code),
	)

	return nil
}

// invalidateScaleCache 清除量表缓存
func (h *ScaleArchivedHandler) invalidateScaleCache(ctx context.Context, dto *ScaleArchivedEventDTO) {
	h.logger.Debug("invalidating scale cache (archived)",
		slog.String("code", dto.Code),
	)
	// TODO: 清除所有版本的缓存
	// h.redisClient.Del(ctx, fmt.Sprintf("scale:%s:*", dto.Code))
}

// invalidateCalculationRules 清除计算规则缓存
func (h *ScaleArchivedHandler) invalidateCalculationRules(ctx context.Context, dto *ScaleArchivedEventDTO) {
	h.logger.Debug("invalidating calculation rules (archived)",
		slog.String("code", dto.Code),
	)
	// TODO: 清除所有版本的规则
	// h.ruleCache.DeleteAll(dto.Code)
}
