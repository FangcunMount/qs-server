package request

// OperatorScopeRole is a company-local role range; OrgID is never client-set.
type OperatorScopeRole struct {
	RoleName string   `json:"role_name"`
	Kind     string   `json:"kind"`
	StoreIDs []string `json:"store_ids"`
}
type ReplaceOperatorScopeRequest struct {
	ExpectedPolicyVersion string `json:"expected_policy_version"`
	// Pointer distinguishes an explicit empty replacement from an omitted field.
	Roles  *[]OperatorScopeRole `json:"roles"`
	Reason string               `json:"reason"`
}
