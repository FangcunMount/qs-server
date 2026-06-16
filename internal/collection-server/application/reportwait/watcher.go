package reportwait

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/component-base/pkg/signaling"
	signalredis "github.com/FangcunMount/component-base/pkg/signaling/redis"
	"github.com/FangcunMount/qs-server/internal/pkg/reportstatus"
)

const watcherService = "collection-server"

// StartSignalWatcher 订阅 report_status_changed 并转发到 WaitHub。
func StartSignalWatcher(
	ctx context.Context,
	signaler *signalredis.Signaler[reportstatus.ChangedSignal],
	waitHub WaitHub,
) {
	if signaler == nil || waitHub == nil {
		return
	}
	go func() {
		for {
			err := signaler.Watch(ctx, func(msgCtx context.Context, signal reportstatus.ChangedSignal) {
				reportstatus.IncWatchReceived(signal.SignalName(), watcherService)
				if signal.AssessmentID == "" {
					return
				}
				waitHub.Notify(signal)
				logger.L(msgCtx).Debugw("wait-report signal received",
					"assessment_id", signal.AssessmentID,
					"status", signal.Status,
				)
			})
			if ctx.Err() != nil {
				return
			}
			reportstatus.IncWatchReconnect(reportstatus.SignalNameReportStatusChanged, watcherService)
			logger.L(ctx).Errorw("report status signal watcher stopped", "error", err)
			time.Sleep(time.Second)
		}
	}()
}

var _ signaling.Watcher[reportstatus.ChangedSignal] = (*signalredis.Signaler[reportstatus.ChangedSignal])(nil)
