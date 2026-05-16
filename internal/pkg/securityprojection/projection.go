package securityprojection

import "github.com/FangcunMount/qs-server/internal/pkg/securityplane"

// PrincipalInput is a transport-neutral identity projection input.
type PrincipalInput struct {
	Kind         securityplane.PrincipalKind
	Source       securityplane.PrincipalSource
	UserID       string
	AccountID    string
	TenantDomain string
	OrgID        uint64
	HasOrgID     bool
	SessionID    string
	TokenID      string
	Username     string
	Roles        []string
	AMR          []string
}

// PrincipalFromInput creates the canonical security-plane principal view.
func PrincipalFromInput(in PrincipalInput) securityplane.Principal {
	kind := in.Kind
	if kind == "" {
		kind = securityplane.PrincipalKindUnknown
	}
	source := in.Source
	if source == "" {
		source = securityplane.PrincipalSourceUnknown
	}
	return securityplane.Principal{
		Kind:         kind,
		Source:       source,
		UserID:       in.UserID,
		AccountID:    in.AccountID,
		TenantDomain: in.TenantDomain,
		OrgID:        in.OrgID,
		HasOrgID:     in.HasOrgID,
		SessionID:    in.SessionID,
		TokenID:      in.TokenID,
		Username:     in.Username,
		Roles:        append([]string(nil), in.Roles...),
		AMR:          append([]string(nil), in.AMR...),
	}
}

// OrgScopeFromIdentity creates the canonical IAM domain + QS org scope view.
func OrgScopeFromIdentity(tenantDomain string, orgID uint64, hasOrg bool, casbinDomain string) securityplane.OrgScope {
	return securityplane.NewOrgScope(tenantDomain, orgID, hasOrg, casbinDomain)
}

// ServiceIdentityInput is a transport-neutral service identity projection input.
type ServiceIdentityInput struct {
	ServiceID      string
	Source         securityplane.ServiceIdentitySource
	TargetAudience []string
	CommonName     string
	Namespace      string
}

// ServiceIdentityFromInput creates the canonical security-plane service identity view.
func ServiceIdentityFromInput(in ServiceIdentityInput) securityplane.ServiceIdentity {
	source := in.Source
	if source == "" {
		source = securityplane.ServiceIdentitySourceUnknown
	}
	return securityplane.ServiceIdentity{
		ServiceID:      in.ServiceID,
		Source:         source,
		TargetAudience: append([]string(nil), in.TargetAudience...),
		CommonName:     in.CommonName,
		Namespace:      in.Namespace,
	}
}
