package actor

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/clinician"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/relation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
)

// RelationMapper 关系映射器。
type RelationMapper struct{}

// NewRelationMapper 创建关系映射器。
func NewRelationMapper() *RelationMapper {
	return &RelationMapper{}
}

// ToPO 转换为持久化对象。
func (m *RelationMapper) ToPO(item *domain.ClinicianTesteeRelation) *ClinicianRelationPO {
	if item == nil {
		return nil
	}

	po := &ClinicianRelationPO{
		OrgID:        item.OrgID(),
		ClinicianID:  item.ClinicianID(),
		TesteeID:     item.TesteeID(),
		RelationType: string(item.RelationType()),
		SourceType:   string(item.SourceType()),
		SourceID:     item.SourceID(),
		IsActive:     item.IsActive(),
		BoundAt:      item.BoundAt(),
		UnboundAt:    item.UnboundAt(),
	}
	if item.ID() > 0 {
		po.ID = item.ID()
	}
	return po
}

// ToDomain 转换为领域对象。
func (m *RelationMapper) ToDomain(po *ClinicianRelationPO) *domain.ClinicianTesteeRelation {
	if po == nil {
		return nil
	}

	item := domain.NewClinicianTesteeRelation(
		po.OrgID,
		clinician.ID(po.ClinicianID),
		testee.ID(po.TesteeID),
		domain.RelationType(po.RelationType),
		domain.SourceType(po.SourceType),
		po.SourceID,
		po.IsActive,
		po.BoundAt,
		po.UnboundAt,
	)
	item.SetID(po.ID)
	return item
}

// ToDomains 批量转换为领域对象。
func (m *RelationMapper) ToDomains(pos []*ClinicianRelationPO) []*domain.ClinicianTesteeRelation {
	items := make([]*domain.ClinicianTesteeRelation, 0, len(pos))
	for _, po := range pos {
		items = append(items, m.ToDomain(po))
	}
	return items
}

// SyncID 同步回领域对象。
func (m *RelationMapper) SyncID(po *ClinicianRelationPO, item *domain.ClinicianTesteeRelation) {
	if po != nil && item != nil {
		item.SetID(po.ID)
	}
}
