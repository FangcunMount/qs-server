package cachesignal

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/component-base/pkg/signaling"
	signalredis "github.com/FangcunMount/component-base/pkg/signaling/redis"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane"
)

// Notifier best-effort 发送缓存失效唤醒信号。
type Notifier struct {
	questionnaire signaling.Notifier[QuestionnaireCacheChangedSignal]
	scale         signaling.Notifier[ScaleCacheChangedSignal]
	typology      signaling.Notifier[TypologyModelCacheChangedSignal]
	service       string
}

type Config struct {
	Signaling SignalingOptions
	Service   string
}

func NewNotifier(opsHandle *cacheplane.Handle, cfg Config) (*Notifier, error) {
	n := &Notifier{service: cfg.Service}
	if !cfg.Signaling.Enabled || opsHandle == nil || opsHandle.Client == nil {
		return n, nil
	}
	standalone, err := AsStandaloneClient(opsHandle.Client)
	if err != nil {
		return nil, err
	}
	qSignaler, err := NewQuestionnaireSignaler(standalone, cfg.Signaling)
	if err != nil {
		return nil, err
	}
	sSignaler, err := NewScaleSignaler(standalone, cfg.Signaling)
	if err != nil {
		return nil, err
	}
	pSignaler, err := NewTypologyModelSignaler(standalone, cfg.Signaling)
	if err != nil {
		return nil, err
	}
	n.questionnaire = qSignaler
	n.scale = sSignaler
	n.typology = pSignaler
	return n, nil
}

func (n *Notifier) QuestionnaireSignaler() *signalredis.Signaler[QuestionnaireCacheChangedSignal] {
	if n == nil {
		return nil
	}
	if s, ok := n.questionnaire.(*signalredis.Signaler[QuestionnaireCacheChangedSignal]); ok {
		return s
	}
	return nil
}

func (n *Notifier) ScaleSignaler() *signalredis.Signaler[ScaleCacheChangedSignal] {
	if n == nil {
		return nil
	}
	if s, ok := n.scale.(*signalredis.Signaler[ScaleCacheChangedSignal]); ok {
		return s
	}
	return nil
}

func (n *Notifier) TypologyModelSignaler() *signalredis.Signaler[TypologyModelCacheChangedSignal] {
	if n == nil {
		return nil
	}
	if s, ok := n.typology.(*signalredis.Signaler[TypologyModelCacheChangedSignal]); ok {
		return s
	}
	return nil
}

func (n *Notifier) NotifyQuestionnaireCacheChanged(ctx context.Context, code, version, action string) {
	if n == nil || n.questionnaire == nil || code == "" {
		return
	}
	signal := QuestionnaireCacheChangedSignal{
		Code:       code,
		Version:    version,
		Action:     action,
		OccurredAt: time.Now().UTC(),
	}
	IncNotify(signal.SignalName(), n.service)
	if err := n.questionnaire.Notify(ctx, signal); err != nil {
		IncNotifyFailed(signal.SignalName(), n.service)
		logger.L(ctx).Warnw("questionnaire cache signal notify failed",
			"code", code,
			"version", version,
			"error", err.Error(),
		)
	}
}

func (n *Notifier) NotifyScaleCacheChanged(ctx context.Context, code, action string) {
	if n == nil || n.scale == nil || code == "" {
		return
	}
	signal := ScaleCacheChangedSignal{
		Code:       code,
		Action:     action,
		OccurredAt: time.Now().UTC(),
	}
	IncNotify(signal.SignalName(), n.service)
	if err := n.scale.Notify(ctx, signal); err != nil {
		IncNotifyFailed(signal.SignalName(), n.service)
		logger.L(ctx).Warnw("scale cache signal notify failed",
			"code", code,
			"error", err.Error(),
		)
	}
}

func (n *Notifier) NotifyTypologyModelCacheChanged(ctx context.Context, code, action string) {
	if n == nil || n.typology == nil || code == "" {
		return
	}
	signal := TypologyModelCacheChangedSignal{
		Code:       code,
		Action:     action,
		OccurredAt: time.Now().UTC(),
	}
	IncNotify(signal.SignalName(), n.service)
	if err := n.typology.Notify(ctx, signal); err != nil {
		IncNotifyFailed(signal.SignalName(), n.service)
		logger.L(ctx).Warnw("typology model cache signal notify failed",
			"code", code,
			"error", err.Error(),
		)
	}
}
