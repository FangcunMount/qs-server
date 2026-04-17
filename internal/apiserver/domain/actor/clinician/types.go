package clinician

import "github.com/FangcunMount/qs-server/internal/pkg/meta"

// ID 从业者ID类型。
type ID = meta.ID

// NewID 创建从业者ID。
func NewID(id uint64) ID {
	return meta.FromUint64(id)
}

// Type 从业者类型。
type Type string

const (
	TypeDoctor    Type = "doctor"    // 医生
	TypeCounselor Type = "counselor" // 咨询师
	TypeTherapist Type = "therapist" // 治疗师
	TypeOther     Type = "other"     // 其他
)

// String 返回从业者类型的原始字符串值。
func (t Type) String() string {
	return string(t)
}

// DisplayName 返回从业者类型的中文展示名称。
func (t Type) DisplayName() string {
	switch t {
	case TypeDoctor:
		return "医生"
	case TypeCounselor:
		return "咨询师"
	case TypeTherapist:
		return "治疗师"
	case TypeOther:
		return "其他"
	default:
		return string(t)
	}
}
