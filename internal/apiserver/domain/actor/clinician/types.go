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
