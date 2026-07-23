package lifecycle

import (
	"context"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

// Action 标识已提交的目录生命周期转换
type Action string

const (
	ActionPublished   Action = "publish"   // 发布
	ActionUnpublished Action = "unpublish" // 取消发布
	ActionArchived    Action = "archive"   // 归档
)

// Effect 包含最佳努力工作，仅在聚合及其已发布的头部成功持久化后执行
// 影响仅应用于评估模型，而不是问卷。
type Effect interface {
	// Supports 支持
	Supports(domain.Identity) bool
	// AfterTransition 转换后
	AfterTransition(context.Context, *domain.AssessmentModel, Action)
}

// EffectsRegistry 解析效果按模型身份。定义
// 策略解析和生命周期侧效果故意保持分离。
type EffectsRegistry struct {
	effects []Effect
}

// NewEffectsRegistry 创建效果注册表
func NewEffectsRegistry(effects ...Effect) EffectsRegistry {
	result := EffectsRegistry{effects: make([]Effect, 0, len(effects))}
	for _, effect := range effects {
		if effect != nil {
			result.effects = append(result.effects, effect)
		}
	}
	return result
}

// AfterTransition 转换后
func (r EffectsRegistry) AfterTransition(ctx context.Context, model *domain.AssessmentModel, action Action) {
	if model == nil {
		return
	}
	identity := domain.Identity{Kind: model.Kind, SubKind: domain.CanonicalSubKindFor(model.Kind), Algorithm: model.Algorithm}
	for _, effect := range r.effects {
		if effect.Supports(identity) {
			effect.AfterTransition(ctx, model, action)
		}
	}
}

// EffectFunc 组合根效果的适配器
type EffectFunc struct {
	Match func(domain.Identity) bool
	Run   func(context.Context, *domain.AssessmentModel, Action)
}

// Supports 支持
func (f EffectFunc) Supports(identity domain.Identity) bool {
	return f.Match != nil && f.Match(identity)
}

// AfterTransition 转换后
func (f EffectFunc) AfterTransition(ctx context.Context, model *domain.AssessmentModel, action Action) {
	if f.Run != nil {
		f.Run(ctx, model, action)
	}
}
