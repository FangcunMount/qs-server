package interpretationassets

// ProjectionVersion belongs to assessment_models record metadata, outside the
// canonical Definition, public authoring contract and frozen ReportInput.
type ProjectionVersion int

const (
	ProjectionUnassigned  ProjectionVersion = 0
	ProjectionLegacyV1    ProjectionVersion = 1
	ProjectionCanonicalV2 ProjectionVersion = 2
)

func (v ProjectionVersion) IsKnown() bool {
	return v == ProjectionLegacyV1 || v == ProjectionCanonicalV2
}
