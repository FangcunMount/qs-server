package actor

import (
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/clinician"
)

// ClinicianMapper 从业者映射器。
type ClinicianMapper struct{}

// NewClinicianMapper 创建从业者映射器。
func NewClinicianMapper() *ClinicianMapper {
	return &ClinicianMapper{}
}

// ToPO 转换为持久化对象。
func (m *ClinicianMapper) ToPO(item *domain.Clinician) *ClinicianPO {
	if item == nil {
		return nil
	}

	var employeeCode *string
	if item.EmployeeCode() != "" {
		value := item.EmployeeCode()
		employeeCode = &value
	}

	po := &ClinicianPO{
		OrgID:         item.OrgID(),
		OperatorID:    item.OperatorID(),
		Name:          item.Name(),
		Department:    item.Department(),
		Title:         item.Title(),
		ClinicianType: string(item.ClinicianType()),
		EmployeeCode:  employeeCode,
		IsActive:      item.IsActive(),
	}
	if item.ID() > 0 {
		po.ID = item.ID()
	}
	return po
}

// ToDomain 转换为领域对象。
func (m *ClinicianMapper) ToDomain(po *ClinicianPO) *domain.Clinician {
	if po == nil {
		return nil
	}

	employeeCode := ""
	if po.EmployeeCode != nil {
		employeeCode = *po.EmployeeCode
	}

	item := domain.NewClinician(
		po.OrgID,
		po.OperatorID,
		po.Name,
		po.Department,
		po.Title,
		domain.Type(po.ClinicianType),
		employeeCode,
		po.IsActive,
	)
	item.SetID(po.ID)
	return item
}

// ToDomains 批量转换为领域对象。
func (m *ClinicianMapper) ToDomains(pos []*ClinicianPO) []*domain.Clinician {
	items := make([]*domain.Clinician, 0, len(pos))
	for _, po := range pos {
		items = append(items, m.ToDomain(po))
	}
	return items
}

// SyncID 同步回领域对象。
func (m *ClinicianMapper) SyncID(po *ClinicianPO, item *domain.Clinician) {
	if po != nil && item != nil {
		item.SetID(po.ID)
	}
}
