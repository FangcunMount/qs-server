package response

// OperatorDataScopeResponse describes the range of one role assignment.
type OperatorDataScopeResponse struct {
	OrgID    string   `json:"org_id"`
	Kind     string   `json:"kind" enums:"stores,all_stores"`
	StoreIDs []string `json:"store_ids"`
}

type OperatorAssignmentScopeResponse struct {
	ID         string `json:"assignment_id"`
	RoleID     string `json:"role_id"`
	RoleName   string `json:"role_name"`
	Protection string `json:"management_protection"`
	// Null means unconfigured, never company-wide access.
	Scope *OperatorDataScopeResponse `json:"scope"`
}

type OperatorScopeConfigurationResponse struct {
	OperatorID              string                            `json:"operator_id"`
	PolicyVersion           string                            `json:"policy_version"`
	Assignments             []OperatorAssignmentScopeResponse `json:"assignments"`
	UnconfiguredAssignments []OperatorAssignmentScopeResponse `json:"unconfigured_assignments"`
	ProtectedAccess         bool                              `json:"protected_access"`
}

type OperatorScopeUpdateResponse struct {
	PolicyVersion     string `json:"policy_version"`
	ProjectionPending bool   `json:"projection_pending"`
}
