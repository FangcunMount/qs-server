package actor

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/staff"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// StaffMapper 员工映射器
type StaffMapper struct{}

// NewStaffMapper 创建员工映射器
func NewStaffMapper() *StaffMapper {
	return &StaffMapper{}
}

// ToPO 将领域对象转换为持久化对象
func (m *StaffMapper) ToPO(domain *staff.Staff) *StaffPO {
	if domain == nil {
		return nil
	}

	// 转换角色列表
	roles := make([]string, len(domain.Roles()))
	for i, role := range domain.Roles() {
		roles[i] = string(role)
	}

	po := &StaffPO{
		OrgID:     domain.OrgID(),
		IAMUserID: domain.IAMUserID(),
		Roles:     roles,
		Name:      domain.Name(),
		Email:     domain.Email(),
		Phone:     domain.Phone(),
		IsActive:  domain.IsActive(),
	}

	// 设置ID（如果已存在）
	if domain.ID() > 0 {
		po.ID = meta.ID(domain.ID())
	}

	return po
}

// ToDomain 将持久化对象转换为领域对象
func (m *StaffMapper) ToDomain(po *StaffPO) *staff.Staff {
	if po == nil {
		return nil
	}

	// 创建员工
	domain := staff.NewStaff(po.OrgID, po.IAMUserID, po.Name)

	// 设置ID
	domain.SetID(staff.ID(po.ID))

	// 转换角色列表
	roles := make([]staff.Role, len(po.Roles))
	for i, roleStr := range po.Roles {
		roles[i] = staff.Role(roleStr)
	}

	// 从仓储恢复状态
	domain.RestoreFromRepository(
		roles,
		po.Email,
		po.Phone,
		po.IsActive,
	)

	return domain
}

// ToDomains 批量转换为领域对象
func (m *StaffMapper) ToDomains(pos []*StaffPO) []*staff.Staff {
	if pos == nil {
		return nil
	}

	domains := make([]*staff.Staff, len(pos))
	for i, po := range pos {
		domains[i] = m.ToDomain(po)
	}
	return domains
}

// SyncID 同步ID到领域对象
func (m *StaffMapper) SyncID(po *StaffPO, domain *staff.Staff) {
	if po != nil && domain != nil {
		domain.SetID(staff.ID(po.ID))
	}
}
