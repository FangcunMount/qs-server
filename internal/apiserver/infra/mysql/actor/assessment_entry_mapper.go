package actor

import (
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/assessmententry"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/clinician"
)

// AssessmentEntryMapper 测评入口映射器。
type AssessmentEntryMapper struct{}

// NewAssessmentEntryMapper 创建测评入口映射器。
func NewAssessmentEntryMapper() *AssessmentEntryMapper {
	return &AssessmentEntryMapper{}
}

// ToPO 转换为持久化对象。
func (m *AssessmentEntryMapper) ToPO(item *domain.AssessmentEntry) *AssessmentEntryPO {
	if item == nil {
		return nil
	}

	var targetVersion *string
	if item.TargetVersion() != "" {
		value := item.TargetVersion()
		targetVersion = &value
	}

	po := &AssessmentEntryPO{
		OrgID:         item.OrgID(),
		ClinicianID:   item.ClinicianID(),
		Token:         item.Token(),
		TargetType:    string(item.TargetType()),
		TargetCode:    item.TargetCode(),
		TargetVersion: targetVersion,
		IsActive:      item.IsActive(),
		ExpiresAt:     item.ExpiresAt(),
	}
	if item.ID() > 0 {
		po.ID = item.ID()
	}
	return po
}

// ToDomain 转换为领域对象。
func (m *AssessmentEntryMapper) ToDomain(po *AssessmentEntryPO) *domain.AssessmentEntry {
	if po == nil {
		return nil
	}

	targetVersion := ""
	if po.TargetVersion != nil {
		targetVersion = *po.TargetVersion
	}

	item := domain.NewAssessmentEntry(
		po.OrgID,
		clinician.ID(po.ClinicianID),
		po.Token,
		domain.TargetType(po.TargetType),
		po.TargetCode,
		targetVersion,
		po.IsActive,
		po.ExpiresAt,
	)
	item.SetID(po.ID)
	return item
}

// ToDomains 批量转换为领域对象。
func (m *AssessmentEntryMapper) ToDomains(pos []*AssessmentEntryPO) []*domain.AssessmentEntry {
	items := make([]*domain.AssessmentEntry, 0, len(pos))
	for _, po := range pos {
		items = append(items, m.ToDomain(po))
	}
	return items
}

// SyncID 同步回领域对象。
func (m *AssessmentEntryMapper) SyncID(po *AssessmentEntryPO, item *domain.AssessmentEntry) {
	if po != nil && item != nil {
		item.SetID(po.ID)
	}
}
