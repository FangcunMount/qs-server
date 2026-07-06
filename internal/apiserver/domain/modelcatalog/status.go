package assessmentmodel

// ModelStatus is the lifecycle status of a draft assessment model.
type ModelStatus string

const (
	ModelStatusDraft     ModelStatus = "draft"
	ModelStatusPublished ModelStatus = "published"
	ModelStatusArchived  ModelStatus = "archived"
)

func (s ModelStatus) String() string { return string(s) }

func (s ModelStatus) IsDraft() bool     { return s == ModelStatusDraft }
func (s ModelStatus) IsPublished() bool { return s == ModelStatusPublished }
func (s ModelStatus) IsArchived() bool  { return s == ModelStatusArchived }

func ParseModelStatus(value string) (ModelStatus, bool) {
	switch ModelStatus(value) {
	case ModelStatusDraft, ModelStatusPublished, ModelStatusArchived:
		return ModelStatus(value), true
	default:
		return "", false
	}
}
