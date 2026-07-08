package reportstatus

import (
	"context"
	"strconv"
	"time"

	"github.com/FangcunMount/component-base/pkg/log"
	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/component-base/pkg/signaling"
	signalredis "github.com/FangcunMount/component-base/pkg/signaling/redis"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane"
)

// Reporter best-effort 写入 Redis 状态并发送唤醒信号。
type Reporter struct {
	cache    *Cache
	signaler signaling.Notifier[ChangedSignal]
	ttl      time.Duration
	service  string
}

func NewReporter(opsHandle *cacheplane.Handle, cfg Config) (*Reporter, error) {
	cache := NewCache(opsHandle)
	var signaler signaling.Notifier[ChangedSignal]
	if cfg.Signaling.Enabled && opsHandle != nil && opsHandle.Client != nil {
		standalone, err := AsStandaloneClient(opsHandle.Client)
		if err != nil {
			log.Warnf("report status signaling disabled: %v", err)
		} else {
			s, err := NewSignaler(standalone, cfg.Signaling)
			if err != nil {
				log.Warnf("report status signaling disabled: %v", err)
			} else {
				signaler = s
			}
		}
	}
	return &Reporter{
		cache:    cache,
		signaler: signaler,
		ttl:      cfg.normalizedTTL(),
		service:  cfg.Service,
	}, nil
}

func AssessmentKey(id uint64) string {
	return strconv.FormatUint(id, 10)
}

func (r *Reporter) SetQueued(ctx context.Context, assessmentID, answerSheetID string) {
	r.set(ctx, func() error {
		return r.cache.SetQueued(ctx, assessmentID, answerSheetID, r.ttl)
	})
}

func (r *Reporter) SetProcessing(ctx context.Context, assessmentID, answerSheetID, stage string) {
	r.set(ctx, func() error {
		return r.cache.SetProcessing(ctx, assessmentID, answerSheetID, stage, r.ttl)
	})
}

func (r *Reporter) SetCompleted(ctx context.Context, assessmentID, answerSheetID, reportID string) {
	if r == nil {
		return
	}
	if err := r.cache.SetCompleted(ctx, assessmentID, answerSheetID, reportID, r.ttl); err != nil {
		r.logSetError(ctx, "completed", assessmentID, err)
	}
	r.notify(ctx, ChangedSignal{
		AssessmentID:  assessmentID,
		AnswerSheetID: answerSheetID,
		ReportID:      reportID,
		Status:        "completed",
		Stage:         "completed",
		Message:       "报告已生成",
		OccurredAt:    time.Now().UTC(),
	})
}

func (r *Reporter) SetFailed(ctx context.Context, assessmentID, answerSheetID, reason, message string) {
	if r == nil {
		return
	}
	if err := r.cache.SetFailed(ctx, assessmentID, answerSheetID, reason, message, r.ttl); err != nil {
		r.logSetError(ctx, "failed", assessmentID, err)
	}
	r.notify(ctx, ChangedSignal{
		AssessmentID:  assessmentID,
		AnswerSheetID: answerSheetID,
		Status:        "failed",
		Stage:         "failed",
		Reason:        reason,
		Message:       message,
		OccurredAt:    time.Now().UTC(),
	})
}

func (r *Reporter) Cache() *Cache {
	if r == nil {
		return nil
	}
	return r.cache
}

func (r *Reporter) Signaler() *signalredis.Signaler[ChangedSignal] {
	if r == nil {
		return nil
	}
	if s, ok := r.signaler.(*signalredis.Signaler[ChangedSignal]); ok {
		return s
	}
	return nil
}

func (r *Reporter) set(ctx context.Context, fn func() error) {
	if r == nil || r.cache == nil {
		return
	}
	if err := fn(); err != nil {
		r.logSetError(ctx, "update", "", err)
	}
}

func (r *Reporter) notify(ctx context.Context, signal ChangedSignal) {
	if r == nil || r.signaler == nil {
		return
	}
	IncNotify(signal.SignalName(), r.service)
	if err := r.signaler.Notify(ctx, signal); err != nil {
		IncNotifyFailed(signal.SignalName(), r.service)
		logger.L(ctx).Warnw("report status signal notify failed",
			"assessment_id", signal.AssessmentID,
			"status", signal.Status,
			"error", err.Error(),
		)
	}
}

func (r *Reporter) logSetError(ctx context.Context, action, assessmentID string, err error) {
	logger.L(ctx).Warnw("report status cache update failed",
		"action", action,
		"assessment_id", assessmentID,
		"error", err.Error(),
	)
}
