package relation

import (
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/clinician"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
)

// ClinicianTesteeRelation 表示从业者与受试者之间的业务关系。
type ClinicianTesteeRelation struct {
	id           ID
	orgID        int64
	clinicianID  clinician.ID
	testeeID     testee.ID
	relationType RelationType
	sourceType   SourceType
	sourceID     *uint64
	isActive     bool
	boundAt      time.Time
	unboundAt    *time.Time
}

// NewClinicianTesteeRelation 创建关系。
func NewClinicianTesteeRelation(
	orgID int64,
	clinicianID clinician.ID,
	testeeID testee.ID,
	relationType RelationType,
	sourceType SourceType,
	sourceID *uint64,
	isActive bool,
	boundAt time.Time,
	unboundAt *time.Time,
) *ClinicianTesteeRelation {
	var copiedSourceID *uint64
	if sourceID != nil {
		value := *sourceID
		copiedSourceID = &value
	}

	return &ClinicianTesteeRelation{
		orgID:        orgID,
		clinicianID:  clinicianID,
		testeeID:     testeeID,
		relationType: relationType,
		sourceType:   sourceType,
		sourceID:     copiedSourceID,
		isActive:     isActive,
		boundAt:      boundAt,
		unboundAt:    unboundAt,
	}
}

// ID 获取关系ID。
func (a *ClinicianTesteeRelation) ID() ID {
	return a.id
}

// OrgID 获取机构ID。
func (a *ClinicianTesteeRelation) OrgID() int64 {
	return a.orgID
}

// ClinicianID 获取从业者ID。
func (a *ClinicianTesteeRelation) ClinicianID() clinician.ID {
	return a.clinicianID
}

// TesteeID 获取受试者ID。
func (a *ClinicianTesteeRelation) TesteeID() testee.ID {
	return a.testeeID
}

// RelationType 获取关系类型。
func (a *ClinicianTesteeRelation) RelationType() RelationType {
	return a.relationType
}

// SourceType 获取来源类型。
func (a *ClinicianTesteeRelation) SourceType() SourceType {
	return a.sourceType
}

// SourceID 获取来源对象ID。
func (a *ClinicianTesteeRelation) SourceID() *uint64 {
	if a.sourceID == nil {
		return nil
	}
	value := *a.sourceID
	return &value
}

// IsActive 是否激活。
func (a *ClinicianTesteeRelation) IsActive() bool {
	return a.isActive
}

// BoundAt 获取绑定时间。
func (a *ClinicianTesteeRelation) BoundAt() time.Time {
	return a.boundAt
}

// UnboundAt 获取解绑时间。
func (a *ClinicianTesteeRelation) UnboundAt() *time.Time {
	return a.unboundAt
}

// SetID 设置ID。
func (a *ClinicianTesteeRelation) SetID(id ID) {
	a.id = id
}

// Unbind 解绑关系。
func (a *ClinicianTesteeRelation) Unbind(now time.Time) {
	a.isActive = false
	a.unboundAt = &now
}
