package iambridge

import "context"

// OperatorAssignmentFact preserves foreign-company and unconfigured facts for
// conflict/protection checks; callers must not flatten them into role names.
type OperatorAssignmentFact struct {
	AssignmentID         string
	RoleID               string
	RoleName             string
	ManagementProtection string
	Scope                *OperatorAssignmentScope
}
type OperatorAssignmentScope struct {
	OrgID    int64
	Kind     string
	StoreIDs []uint64
}
type OperatorAssignmentFacts struct {
	Assignments   []OperatorAssignmentFact
	PolicyVersion int64
}
type OperatorScopedRole struct {
	RoleName string
	Scope    OperatorAssignmentScope
}

type OperatorScopeGateway interface {
	ReplaceOperatorScopedRoles(context.Context, int64, int64, []OperatorScopedRole, int64, string, string) (int64, error)
	LoadOperatorAssignmentFacts(context.Context, int64) (OperatorAssignmentFacts, error)
}
