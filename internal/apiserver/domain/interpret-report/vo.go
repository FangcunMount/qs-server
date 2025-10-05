package interpretationreport

import (
	"strconv"

	v1 "github.com/fangcun-mount/qs-server/pkg/meta/v1"
)

// InterpretReportID 解读报告唯一标识
type InterpretReportID struct {
	value v1.ID
}

// NewInterpretReportID 创建解读报告ID
func NewInterpretReportID(value uint64) InterpretReportID {
	return InterpretReportID{value: v1.NewID(value)}
}

// Value 获取ID值
func (id InterpretReportID) Value() v1.ID {
	return id.value
}

// String 获取ID字符串
func (id InterpretReportID) String() string {
	return strconv.FormatUint(id.value.Value(), 10)
}

// InterpretReportStatus 解读报告状态
type InterpretReportStatus uint8

const (
	STATUS_DRAFT     InterpretReportStatus = 0 // 草稿
	STATUS_GENERATED InterpretReportStatus = 1 // 已生成
	STATUS_REVIEWED  InterpretReportStatus = 2 // 已审核
	STATUS_PUBLISHED InterpretReportStatus = 3 // 已发布
	STATUS_ARCHIVED  InterpretReportStatus = 4 // 已归档
)

// Value 获取状态值
func (s InterpretReportStatus) Value() uint8 {
	return uint8(s)
}

// String 获取状态字符串
func (s InterpretReportStatus) String() string {
	switch s {
	case STATUS_DRAFT:
		return "draft"
	case STATUS_GENERATED:
		return "generated"
	case STATUS_REVIEWED:
		return "reviewed"
	case STATUS_PUBLISHED:
		return "published"
	case STATUS_ARCHIVED:
		return "archived"
	default:
		return "unknown"
	}
}

// IsValid 检查状态是否有效
func (s InterpretReportStatus) IsValid() bool {
	return s >= STATUS_DRAFT && s <= STATUS_ARCHIVED
}

// CanTransitionTo 检查是否可以转换到目标状态
func (s InterpretReportStatus) CanTransitionTo(target InterpretReportStatus) bool {
	switch s {
	case STATUS_DRAFT:
		return target == STATUS_GENERATED
	case STATUS_GENERATED:
		return target == STATUS_REVIEWED || target == STATUS_PUBLISHED
	case STATUS_REVIEWED:
		return target == STATUS_PUBLISHED || target == STATUS_ARCHIVED
	case STATUS_PUBLISHED:
		return target == STATUS_ARCHIVED
	case STATUS_ARCHIVED:
		return false // 归档状态不能转换到其他状态
	default:
		return false
	}
}

// InterpretItemType 解读项类型
type InterpretItemType string

const (
	ITEM_TYPE_FACTOR    InterpretItemType = "factor"    // 因子解读
	ITEM_TYPE_TOTAL     InterpretItemType = "total"     // 总分解读
	ITEM_TYPE_DIMENSION InterpretItemType = "dimension" // 维度解读
	ITEM_TYPE_SUMMARY   InterpretItemType = "summary"   // 总结解读
)

// String 获取类型字符串
func (t InterpretItemType) String() string {
	return string(t)
}

// IsValid 检查类型是否有效
func (t InterpretItemType) IsValid() bool {
	return t == ITEM_TYPE_FACTOR || t == ITEM_TYPE_TOTAL ||
		t == ITEM_TYPE_DIMENSION || t == ITEM_TYPE_SUMMARY
}
