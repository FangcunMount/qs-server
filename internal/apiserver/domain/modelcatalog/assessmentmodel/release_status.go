package assessmentmodel

// ReleaseStatus describes whether an immutable published snapshot may be used
// for new assessments. Archived snapshots remain readable by exact version for
// assessments that already captured the release reference.
type ReleaseStatus string

const (
	ReleaseStatusActive   ReleaseStatus = "active"
	ReleaseStatusArchived ReleaseStatus = "archived"
)

func NormalizeReleaseStatus(value ReleaseStatus) ReleaseStatus {
	switch value {
	case ReleaseStatusActive, ReleaseStatusArchived:
		return value
	default:
		return ReleaseStatusArchived
	}
}

func (s ReleaseStatus) IsActive() bool { return s == ReleaseStatusActive }
