package operator

import (
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/operator"
	actorreadmodel "github.com/FangcunMount/qs-server/internal/apiserver/port/actorreadmodel"
)

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
	effectiveRoles := roleStrings(s.EffectiveRoles())

	return &OperatorResult{
		ID:                     s.ID().Uint64(),
		OrgID:                  s.OrgID(),
		UserID:                 s.UserID(),
		Roles:                  roles,
		EffectiveRoles:         effectiveRoles,
		InheritedRoles:         inheritedRoleNames(roles, effectiveRoles),
		AuthzPolicyVersion:     s.AuthzPolicyVersion(),
		AuthzProjectionPending: s.AuthzProjectionPending(),
		Name:                   s.Name(),
		Email:                  s.Email(),
		Phone:                  s.Phone(),
		IsActive:               s.IsActive(),
	}
}

func toOperatorResultFromRow(row *actorreadmodel.OperatorRow) *OperatorResult {
	if row == nil {
		return nil
	}
	return &OperatorResult{
		ID:                     row.ID,
		OrgID:                  row.OrgID,
		UserID:                 row.UserID,
		Roles:                  append([]string(nil), row.Roles...),
		EffectiveRoles:         append([]string(nil), row.EffectiveRoles...),
		InheritedRoles:         inheritedRoleNames(row.Roles, row.EffectiveRoles),
		AuthzPolicyVersion:     row.AuthzPolicyVersion,
		AuthzProjectionPending: row.AuthzProjectionPending,
		Name:                   row.Name,
		Email:                  row.Email,
		Phone:                  row.Phone,
		IsActive:               row.IsActive,
	}
}

func roleStrings(roles []domain.Role) []string {
	result := make([]string, len(roles))
	for index, role := range roles {
		result[index] = string(role)
	}
	return result
}

func inheritedRoleNames(direct, effective []string) []string {
	directSet := make(map[string]struct{}, len(direct))
	for _, role := range direct {
		directSet[role] = struct{}{}
	}
	result := make([]string, 0, len(effective))
	for _, role := range effective {
		if _, directRole := directSet[role]; !directRole {
			result = append(result, role)
		}
	}
	return result
}
