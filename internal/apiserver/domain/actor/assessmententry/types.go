package assessmententry

import "github.com/FangcunMount/qs-server/internal/pkg/meta"

// ID 测评入口ID类型。
type ID = meta.ID

// NewID 创建测评入口ID。
func NewID(id uint64) ID {
	return meta.FromUint64(id)
}

// TargetType 入口目标类型。
type TargetType string

const (
	TargetTypeQuestionnaire TargetType = "questionnaire" // 问卷
	TargetTypeScale         TargetType = "scale"         // 量表
)

// String 返回目标类型的原始字符串值。
func (t TargetType) String() string {
	return string(t)
}

// DisplayName 返回目标类型的中文展示名称。
func (t TargetType) DisplayName() string {
	switch t {
	case TargetTypeQuestionnaire:
		return "问卷"
	case TargetTypeScale:
		return "量表"
	default:
		return string(t)
	}
}
