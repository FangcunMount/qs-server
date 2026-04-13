package relation

import "github.com/FangcunMount/qs-server/internal/pkg/meta"

// ID 关系ID类型。
type ID = meta.ID

// NewID 创建关系ID。
func NewID(id uint64) ID {
	return meta.FromUint64(id)
}

// RelationType 从业者与受试者关系类型。
type RelationType string

const (
	RelationTypeAssigned     RelationType = "assigned"     // 已分配，授予访问权
	RelationTypeCreator      RelationType = "creator"      // 创建来源，不授予访问权
	RelationTypePrimary      RelationType = "primary"      // 兼容预留：主诊
	RelationTypeAttending    RelationType = "attending"    // 兼容预留：随诊
	RelationTypeCollaborator RelationType = "collaborator" // 兼容预留：协作
)

// SourceType 关系来源类型。
type SourceType string

const (
	SourceTypeAssessmentEntry SourceType = "assessment_entry" // 测评入口
	SourceTypeManual          SourceType = "manual"           // 手动
	SourceTypeImport          SourceType = "import"           // 导入
	SourceTypeTransfer        SourceType = "transfer"         // 转诊
)
