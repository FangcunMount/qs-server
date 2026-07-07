package statistics

import (
	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/outboxpolicy"
	"github.com/FangcunMount/qs-server/pkg/event"
)

// FootprintDurableStagingPolicy 是持久化-staging 策略 用于 footprint。
// events. 机制 是 中性 outbox concern; 这个包 仅 负责。
// footprint-特定 默认 禁用 list 和 keeps 稳定 API 用于 callers。
type FootprintDurableStagingPolicy = outboxpolicy.Policy

// 默认DisabledHighFrequencyFootprintEvents 列出footprint events moved 到 scan 投影。
func DefaultDisabledHighFrequencyFootprintEvents() []string {
	return []string{
		eventcatalog.FootprintEntryOpened,
		eventcatalog.FootprintIntakeConfirmed,
		eventcatalog.FootprintTesteeProfileCreated,
		eventcatalog.FootprintCareRelationshipEstablished,
		eventcatalog.FootprintAnswerSheetSubmitted,
		eventcatalog.FootprintAssessmentCreated,
		eventcatalog.FootprintReportGenerated,
	}
}

// NewFootprintDurableStagingPolicy 构建策略 从 禁用 event types。
func NewFootprintDurableStagingPolicy(disabledEventTypes []string) *FootprintDurableStagingPolicy {
	return outboxpolicy.NewPolicy(disabledEventTypes)
}

// InstallFootprintDurableStagingPolicy sets 进程-wide footprint staging 策略。
func InstallFootprintDurableStagingPolicy(policy *FootprintDurableStagingPolicy) {
	outboxpolicy.Install(policy)
}

// FootprintEventAllowed 检查installed footprint staging 策略。
func FootprintEventAllowed(eventType string) bool {
	return outboxpolicy.Allowed(eventType)
}

// FilterFootprintStagingEvents removes footprint events blocked 按 staging 策略。
func FilterFootprintStagingEvents(events []event.DomainEvent) []event.DomainEvent {
	return outboxpolicy.Filter(events)
}
