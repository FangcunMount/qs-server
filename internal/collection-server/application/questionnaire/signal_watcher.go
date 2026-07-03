package questionnaire

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/component-base/pkg/signaling"
	signalredis "github.com/FangcunMount/component-base/pkg/signaling/redis"
	"github.com/FangcunMount/qs-server/internal/pkg/cachesignal"
)

// StartCacheSignalWatcher 订阅 questionnaire_cache_changed 并失效 collection L1。
func StartCacheSignalWatcher(
	ctx context.Context,
	signaler *signalredis.Signaler[cachesignal.QuestionnaireCacheChangedSignal],
	cache PublishedDetailCache,
) {
	if signaler == nil || cache == nil {
		return
	}
	go func() {
		for {
			err := signaler.Watch(ctx, func(msgCtx context.Context, signal cachesignal.QuestionnaireCacheChangedSignal) {
				if signal.Code == "" {
					return
				}
				if signal.Version == "" {
					cache.Delete(signal.Code, "")
				} else {
					cache.Delete(signal.Code, signal.Version)
					cache.Delete(signal.Code, "")
				}
				logger.L(msgCtx).Debugw("questionnaire cache signal evicted",
					"code", signal.Code,
					"version", signal.Version,
					"action", signal.Action,
				)
			})
			if ctx.Err() != nil {
				return
			}
			logger.L(ctx).Errorw("questionnaire cache signal watcher stopped", "error", err)
			time.Sleep(time.Second)
		}
	}()
}

var _ signaling.Watcher[cachesignal.QuestionnaireCacheChangedSignal] = (*signalredis.Signaler[cachesignal.QuestionnaireCacheChangedSignal])(nil)
