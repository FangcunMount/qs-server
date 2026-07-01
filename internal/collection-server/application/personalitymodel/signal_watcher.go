package personalitymodel

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/component-base/pkg/signaling"
	signalredis "github.com/FangcunMount/component-base/pkg/signaling/redis"
	"github.com/FangcunMount/qs-server/internal/pkg/cachesignal"
)

// StartCacheSignalWatcher 订阅 personality_model_cache_changed 并失效 collection L1。
func StartCacheSignalWatcher(
	ctx context.Context,
	signaler *signalredis.Signaler[cachesignal.PersonalityModelCacheChangedSignal],
	cache CatalogCache,
) {
	if signaler == nil || cache == nil {
		return
	}
	go func() {
		for {
			err := signaler.Watch(ctx, func(msgCtx context.Context, signal cachesignal.PersonalityModelCacheChangedSignal) {
				if signal.Code == "" {
					return
				}
				EvictCatalogOnSignal(cache, signal.Code)
				logger.L(msgCtx).Debugw("personality model cache signal evicted",
					"code", signal.Code,
					"action", signal.Action,
				)
			})
			if ctx.Err() != nil {
				return
			}
			logger.L(ctx).Errorw("personality model cache signal watcher stopped", "error", err)
			time.Sleep(time.Second)
		}
	}()
}

var _ signaling.Watcher[cachesignal.PersonalityModelCacheChangedSignal] = (*signalredis.Signaler[cachesignal.PersonalityModelCacheChangedSignal])(nil)
