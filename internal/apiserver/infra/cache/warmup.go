package cache

import (
	"context"
	"fmt"

	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
)

// WarmupService 缓存预热服务
type WarmupService struct {
	scaleRepo scale.Repository
}

// NewWarmupService 创建缓存预热服务
func NewWarmupService(scaleRepo scale.Repository) *WarmupService {
	return &WarmupService{
		scaleRepo: scaleRepo,
	}
}

// WarmupScaleCache 预热量表缓存
// hotScaleCodes: 热点量表编码列表（如 ["SDS", "SAS", "Conners"]）
func (s *WarmupService) WarmupScaleCache(ctx context.Context, hotScaleCodes []string) error {
	l := logger.L(ctx)
	l.Infow("开始预热量表缓存", "count", len(hotScaleCodes))

	// 检查是否为缓存装饰器
	cachedRepo, ok := s.scaleRepo.(*CachedScaleRepository)
	if !ok {
		l.Debugw("量表 Repository 未使用缓存装饰器，跳过预热")
		return nil
	}

	if err := cachedRepo.WarmupCache(ctx, hotScaleCodes); err != nil {
		return fmt.Errorf("预热量表缓存失败: %w", err)
	}

	l.Infow("量表缓存预热完成", "count", len(hotScaleCodes))
	return nil
}

// WarmupDefaultScales 预热默认热点量表
// 根据业务实际情况配置常用量表编码
func (s *WarmupService) WarmupDefaultScales(ctx context.Context) error {
	// 默认热点量表编码（可根据实际业务调整）
	defaultHotScales := []string{
		"SDS",     // 抑郁自评量表
		"SAS",     // 焦虑自评量表
		"Conners", // Conners 量表
		// 可根据实际使用情况添加更多
	}

	return s.WarmupScaleCache(ctx, defaultHotScales)
}
