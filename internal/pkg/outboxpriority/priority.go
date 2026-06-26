package outboxpriority

import "github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"

// P0 核心报告链路事件，优先 claim。
var P0 = []string{
	eventcatalog.AnswerSheetSubmitted,
	eventcatalog.AssessmentSubmitted,
}

// P1 报告链路后续事件。
var P1 = []string{
	eventcatalog.AssessmentFailed,
	eventcatalog.ReportGenerated,
	eventcatalog.ReportGeneratedV2,
	eventcatalog.AssessmentInterpreted,
	eventcatalog.AssessmentInterpretedV2,
}

// P2 行为足迹类事件（可靠但不应抢占核心链路）。
var P2 = []string{
	eventcatalog.FootprintEntryOpened,
	eventcatalog.FootprintIntakeConfirmed,
	eventcatalog.FootprintTesteeProfileCreated,
	eventcatalog.FootprintCareRelationshipEstablished,
	eventcatalog.FootprintCareRelationshipTransferred,
	eventcatalog.FootprintAnswerSheetSubmitted,
	eventcatalog.FootprintAssessmentCreated,
	eventcatalog.FootprintReportGenerated,
}

// ClaimOrder 返回 claim 优先级顺序：先 P0，再 P1，最后 fallback 全量 pending。
func ClaimOrder(customP0, customP1 []string) [][]string {
	p0 := coalesce(customP0, P0)
	p1 := coalesce(customP1, P1)
	return [][]string{p0, appendSlices(p0, p1), nil}
}

func coalesce(custom, fallback []string) []string {
	if len(custom) > 0 {
		return custom
	}
	return fallback
}

func appendSlices(parts ...[]string) []string {
	total := 0
	for _, part := range parts {
		total += len(part)
	}
	if total == 0 {
		return nil
	}
	merged := make([]string, 0, total)
	seen := make(map[string]struct{}, total)
	for _, part := range parts {
		for _, item := range part {
			if _, ok := seen[item]; ok {
				continue
			}
			seen[item] = struct{}{}
			merged = append(merged, item)
		}
	}
	return merged
}

const (
	BucketP0 = "p0"
	BucketP1 = "p1"
	BucketP2 = "p2"
)

// ReadyIndexBuckets is the claim order for outbox ready-index scheduling.
var ReadyIndexBuckets = []string{BucketP0, BucketP1, BucketP2}

// Bucket returns the ready-index bucket for an event type.
func Bucket(eventType string) string {
	for _, item := range P0 {
		if item == eventType {
			return BucketP0
		}
	}
	for _, item := range P1 {
		if item == eventType {
			return BucketP1
		}
	}
	return BucketP2
}
