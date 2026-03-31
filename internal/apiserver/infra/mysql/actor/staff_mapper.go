package actor

import (
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/operator"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// OperatorMapper 操作者映射器
type OperatorMapper struct{}

// NewOperatorMapper 创建操作者映射器
func NewOperatorMapper() *OperatorMapper {
	return &OperatorMapper{}
}

// ToPO 将领域对象转换为持久化对象
func (m *OperatorMapper) ToPO(item *domain.Operator) *OperatorPO {
	if item == nil {
		return nil
	}

	// 转换角色列表
	roles := make([]string, len(item.Roles()))
	for i, role := range item.Roles() {
		roles[i] = string(role)
	}

	po := &OperatorPO{
		OrgID:    item.OrgID(),
		UserID:   item.UserID(),
		Roles:    roles,
		Name:     item.Name(),
		Email:    item.Email(),
		Phone:    item.Phone(),
		IsActive: item.IsActive(),
	}

	// 设置ID（如果已存在）
	if item.ID() > 0 {
		po.ID = meta.ID(item.ID())
	}

	return po
}

// ToDomain 将持久化对象转换为领域对象
func (m *OperatorMapper) ToDomain(po *OperatorPO) *domain.Operator {
	if po == nil {
		return nil
	}

	// 创建操作者
	item := domain.NewOperator(po.OrgID, po.UserID, po.Name)

	// 设置ID
	item.SetID(domain.ID(po.ID))

	// 转换角色列表
	roles := make([]domain.Role, len(po.Roles))
	for i, roleStr := range po.Roles {
		roles[i] = domain.Role(roleStr)
	}

	// 从仓储恢复状态
	item.RestoreFromRepository(
		roles,
		po.Email,
		po.Phone,
		po.IsActive,
	)

	return item
}

// ToDomains 批量转换为领域对象
func (m *OperatorMapper) ToDomains(pos []*OperatorPO) []*domain.Operator {
	if pos == nil {
		return nil
	}

	domains := make([]*domain.Operator, len(pos))
	for i, po := range pos {
		domains[i] = m.ToDomain(po)
	}
	return domains
}

// SyncID 同步ID到领域对象
func (m *OperatorMapper) SyncID(po *OperatorPO, item *domain.Operator) {
	if po != nil && item != nil {
		item.SetID(domain.ID(po.ID))
	}
}
