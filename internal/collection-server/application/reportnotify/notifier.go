package reportnotify

import "github.com/FangcunMount/qs-server/internal/pkg/reportstatus"

// StatusEvent 是传输无关的报告状态变更事件。
type StatusEvent = reportstatus.ChangedSignal

// Notifier 向 HTTP wait-report 与 WebSocket 等传输层广播报告状态唤醒。
type Notifier interface {
	Subscribe(assessmentID string) (<-chan StatusEvent, func())
	Notify(signal StatusEvent)
	ActiveSubscriptions() int
}
