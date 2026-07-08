package run

// AnalyticsProjectorCheckpointStatusProcessing is the legacy analytics in-flight status.
const AnalyticsProjectorCheckpointStatusProcessing = "processing"

// AnalyticsProjectorCheckpointStatusCompleted is the legacy analytics terminal success status.
const AnalyticsProjectorCheckpointStatusCompleted = "completed"

// AnalyticsProjectorCheckpointStatusPending is the legacy analytics deferred status.
const AnalyticsProjectorCheckpointStatusPending = "pending"

// UnifiedStatusForAnalytics maps legacy analytics checkpoint status to evaluation-run vocabulary.
func UnifiedStatusForAnalytics(status string) string {
	switch status {
	case AnalyticsProjectorCheckpointStatusProcessing:
		return StatusRunning.String()
	case AnalyticsProjectorCheckpointStatusCompleted:
		return StatusSucceeded.String()
	case AnalyticsProjectorCheckpointStatusPending:
		return StatusPending.String()
	default:
		return status
	}
}

// AnalyticsStatusFromUnified maps unified checkpoint status back to analytics vocabulary.
func AnalyticsStatusFromUnified(status string) string {
	switch Status(status) {
	case StatusRunning:
		return AnalyticsProjectorCheckpointStatusProcessing
	case StatusSucceeded:
		return AnalyticsProjectorCheckpointStatusCompleted
	case StatusPending:
		return AnalyticsProjectorCheckpointStatusPending
	default:
		return status
	}
}
