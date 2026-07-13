package cachegovernance

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/logger"
	signalredis "github.com/FangcunMount/component-base/pkg/signaling/redis"
	"github.com/FangcunMount/qs-server/internal/pkg/cachesignal"
)

// StartCacheSignalWatcher 订阅缓存失效信令并触发本地预热。
func StartCacheSignalWatcher(
	ctx context.Context,
	coordinator Coordinator,
	questionnaireSignaler *signalredis.Signaler[cachesignal.QuestionnaireCacheChangedSignal],
	scaleSignaler *signalredis.Signaler[cachesignal.ScaleCacheChangedSignal],
	typologySignaler *signalredis.Signaler[cachesignal.TypologyModelCacheChangedSignal],
) {
	if coordinator == nil {
		return
	}
	if questionnaireSignaler != nil {
		go func() {
			err := questionnaireSignaler.Watch(ctx, func(msgCtx context.Context, signal cachesignal.QuestionnaireCacheChangedSignal) {
				if signal.Code == "" {
					return
				}
				if err := coordinator.HandleQuestionnairePublished(msgCtx, signal.Code, signal.Version); err != nil {
					logger.L(msgCtx).Warnw("questionnaire cache signal warmup failed",
						"code", signal.Code,
						"version", signal.Version,
						"error", err.Error(),
					)
				}
			})
			if err != nil && ctx.Err() == nil {
				logger.L(ctx).Errorw("questionnaire cache signal watcher stopped", "error", err.Error())
			}
		}()
	}
	if scaleSignaler != nil {
		go func() {
			err := scaleSignaler.Watch(ctx, func(msgCtx context.Context, signal cachesignal.ScaleCacheChangedSignal) {
				if signal.Code == "" {
					return
				}
				if err := coordinator.HandleScalePublished(msgCtx, signal.Code); err != nil {
					logger.L(msgCtx).Warnw("scale cache signal warmup failed",
						"code", signal.Code,
						"error", err.Error(),
					)
				}
			})
			if err != nil && ctx.Err() == nil {
				logger.L(ctx).Errorw("scale cache signal watcher stopped", "error", err.Error())
			}
		}()
	}
	if typologySignaler != nil {
		go func() {
			err := typologySignaler.Watch(ctx, func(msgCtx context.Context, signal cachesignal.TypologyModelCacheChangedSignal) {
				if signal.Code == "" {
					return
				}
				if err := coordinator.HandleTypologyModelPublished(msgCtx, signal.Code); err != nil {
					logger.L(msgCtx).Warnw("typology model cache signal warmup failed",
						"code", signal.Code,
						"error", err.Error(),
					)
				}
			})
			if err != nil && ctx.Err() == nil {
				logger.L(ctx).Errorw("typology model cache signal watcher stopped", "error", err.Error())
			}
		}()
	}
}
