package staff

import domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/staff"

// toStaffResult 将领域对象转换为 DTO
// 这是一个包内共享的辅助函数
func toStaffResult(s *domain.Staff) *StaffResult {
	if s == nil {
		return nil
	}

	// 转换角色列表
	roles := make([]string, len(s.Roles()))
	for i, role := range s.Roles() {
		roles[i] = string(role)
	}

	return &StaffResult{
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
