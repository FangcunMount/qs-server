package operator

import domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/operator"

// toOperatorResult 将领域对象转换为 DTO
// 这是一个包内共享的辅助函数
func toOperatorResult(s *domain.Operator) *OperatorResult {
	if s == nil {
		return nil
	}

	// 转换角色列表
	roles := make([]string, len(s.Roles()))
	for i, role := range s.Roles() {
		roles[i] = string(role)
	}

	return &OperatorResult{
		ID:       uint64(s.ID()),
		OrgID:    s.OrgID(),
		UserID:   s.UserID(),
		Roles:    roles,
		Name:     s.Name(),
		Email:    s.Email(),
		Phone:    s.Phone(),
		IsActive: s.IsActive(),
	}
}
