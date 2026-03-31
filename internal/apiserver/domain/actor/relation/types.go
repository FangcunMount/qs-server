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
	RelationTypePrimary      RelationType = "primary"      // 主诊
	RelationTypeAttending    RelationType = "attending"    // 随诊
	RelationTypeCreator      RelationType = "creator"      // 创建
	RelationTypeCollaborator RelationType = "collaborator" // 协作
)

// SourceType 关系来源类型。
type SourceType string

const (
	SourceTypeAssessmentEntry SourceType = "assessment_entry" // 测评入口
	SourceTypeManual          SourceType = "manual"           // 手动
	SourceTypeImport          SourceType = "import"           // 导入
	SourceTypeTransfer        SourceType = "transfer"         // 转诊
)
