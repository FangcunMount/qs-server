package questionnaire

// ReleaseStatus is the lifecycle of an immutable questionnaire snapshot.
// Archived releases are not selectable for new assessments, but remain
// available to exact-version readers used by existing assessments.
type ReleaseStatus string

const (
	ReleaseStatusActive   ReleaseStatus = "active"
	ReleaseStatusArchived ReleaseStatus = "archived"
)

func NormalizeReleaseStatus(value ReleaseStatus, legacyActive bool) ReleaseStatus {
	switch value {
	case ReleaseStatusActive, ReleaseStatusArchived:
		return value
	default:
		if legacyActive {
			return ReleaseStatusActive
		}
		return ReleaseStatusArchived
	}
}

func (s ReleaseStatus) IsActive() bool { return s == ReleaseStatusActive }
