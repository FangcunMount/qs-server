package assessmentmodel

// Status 是后台可编辑测评模型的生命周期状态。
type Status string

const (
	StatusDraft     Status = "draft"
	StatusPublished Status = "published"
	StatusArchived  Status = "archived"
)

func (s Status) String() string { return string(s) }

func (s Status) IsDraft() bool     { return s == StatusDraft }
func (s Status) IsPublished() bool { return s == StatusPublished }
func (s Status) IsArchived() bool  { return s == StatusArchived }

func ParseStatus(value string) (Status, bool) {
	switch Status(value) {
	case StatusDraft, StatusPublished, StatusArchived:
		return Status(value), true
	default:
		return "", false
	}
}
