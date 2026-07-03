package catalogl1

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/component-base/pkg/signaling"
	signalredis "github.com/FangcunMount/component-base/pkg/signaling/redis"
)

// SignalEvictor 按信令失效 L1。
type SignalEvictor interface {
	EvictOnSignal(code string)
}

// DetailSignalEvictor 按信令失效单桶详情（含 version）。
type DetailSignalEvictor interface {
	EvictOnSignal(code, version string)
}

// StartSignalWatcher 订阅 catalog 变更信令并失效 L1。
func StartSignalWatcher[T signaling.Signal](
	ctx context.Context,
	signaler *signalredis.Signaler[T],
	codeFn func(T) string,
	evict func(code string),
	logLabel string,
) {
	if signaler == nil || evict == nil {
		return
	}
	go func() {
		for {
			err := signaler.Watch(ctx, func(msgCtx context.Context, signal T) {
				code := codeFn(signal)
				if code == "" {
					return
				}
				evict(code)
				logger.L(msgCtx).Debugw(logLabel, "code", code)
			})
			if ctx.Err() != nil {
				return
			}
			logger.L(ctx).Errorw(logLabel+" watcher stopped", "error", err)
			time.Sleep(time.Second)
		}
	}()
}
