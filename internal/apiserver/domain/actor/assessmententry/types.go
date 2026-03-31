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
