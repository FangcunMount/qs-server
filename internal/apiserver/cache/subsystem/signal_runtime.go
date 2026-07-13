package cachebootstrap

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/component-base/pkg/signaling"
	signalredis "github.com/FangcunMount/component-base/pkg/signaling/redis"
	cachegov "github.com/FangcunMount/qs-server/internal/apiserver/cache/governance"
	cacheobserve "github.com/FangcunMount/qs-server/internal/pkg/cache/observe"
	cachesignal "github.com/FangcunMount/qs-server/internal/pkg/cache/signal"
	redis "github.com/redis/go-redis/v9"
)

type signalRuntime struct {
	questionnaire *signalredis.Signaler[cachesignal.QuestionnaireCacheChangedSignal]
	scale         *signalredis.Signaler[cachesignal.ScaleCacheChangedSignal]
	typology      *signalredis.Signaler[cachesignal.TypologyModelCacheChangedSignal]
	service       string
}

func newSignalRuntime(client redis.UniversalClient, opts SignalOptions, service string) *signalRuntime {
	runtime := &signalRuntime{service: service}
	if !opts.Enabled || client == nil {
		return runtime
	}
	redisOptions := opts.redisOptions()
	runtime.questionnaire = signalredis.NewSignaler[cachesignal.QuestionnaireCacheChangedSignal](client, redisOptions)
	runtime.scale = signalredis.NewSignaler[cachesignal.ScaleCacheChangedSignal](client, redisOptions)
	runtime.typology = signalredis.NewSignaler[cachesignal.TypologyModelCacheChangedSignal](client, redisOptions)
	return runtime
}

func (o SignalOptions) redisOptions() signalredis.Options {
	opts := signalredis.DefaultOptions()
	opts.Prefix = "qs:signal"
	if o.Prefix != "" {
		opts.Prefix = o.Prefix
	}
	if o.Channel != "" {
		opts.Channel = o.Channel
	}
	if o.BufferSize > 0 {
		opts.BufferSize = o.BufferSize
	}
	return opts
}

func (r *signalRuntime) Start(ctx context.Context, coordinator cachegov.Coordinator) {
	if r == nil {
		return
	}
	cachegov.StartCacheSignalWatcher(ctx, coordinator, r.questionnaire, r.scale, r.typology)
}

func (r *signalRuntime) NotifyQuestionnaireCacheChanged(ctx context.Context, code, version, action string) {
	if r == nil || r.questionnaire == nil || code == "" {
		return
	}
	signal := cachesignal.QuestionnaireCacheChangedSignal{
		Code: code, Version: version, Action: action, OccurredAt: time.Now().UTC(),
	}
	cacheobserve.IncSignalNotify(signal.SignalName(), r.service)
	if err := r.questionnaire.Notify(ctx, signal); err != nil {
		cacheobserve.IncSignalNotifyFailed(signal.SignalName(), r.service)
		logger.L(ctx).Warnw("questionnaire cache signal notify failed",
			"code", code, "version", version, "error", err.Error())
	}
}

func (r *signalRuntime) NotifyScaleCacheChanged(ctx context.Context, code, action string) {
	if r == nil || r.scale == nil || code == "" {
		return
	}
	signal := cachesignal.ScaleCacheChangedSignal{
		Code: code, Action: action, OccurredAt: time.Now().UTC(),
	}
	cacheobserve.IncSignalNotify(signal.SignalName(), r.service)
	if err := r.scale.Notify(ctx, signal); err != nil {
		cacheobserve.IncSignalNotifyFailed(signal.SignalName(), r.service)
		logger.L(ctx).Warnw("scale cache signal notify failed",
			"code", code, "error", err.Error())
	}
}

func (r *signalRuntime) NotifyTypologyModelCacheChanged(ctx context.Context, code, action string) {
	if r == nil || r.typology == nil || code == "" {
		return
	}
	signal := cachesignal.TypologyModelCacheChangedSignal{
		Code: code, Action: action, OccurredAt: time.Now().UTC(),
	}
	cacheobserve.IncSignalNotify(signal.SignalName(), r.service)
	if err := r.typology.Notify(ctx, signal); err != nil {
		cacheobserve.IncSignalNotifyFailed(signal.SignalName(), r.service)
		logger.L(ctx).Warnw("typology model cache signal notify failed",
			"code", code, "error", err.Error())
	}
}

var (
	_ SignalNotifier                                                  = (*signalRuntime)(nil)
	_ signaling.Notifier[cachesignal.QuestionnaireCacheChangedSignal] = (*signalredis.Signaler[cachesignal.QuestionnaireCacheChangedSignal])(nil)
	_ signaling.Watcher[cachesignal.QuestionnaireCacheChangedSignal]  = (*signalredis.Signaler[cachesignal.QuestionnaireCacheChangedSignal])(nil)
)
