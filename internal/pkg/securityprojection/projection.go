package securityprojection

import "github.com/FangcunMount/qs-server/internal/pkg/securityplane"

// PrincipalInput is a transport-neutral identity projection input.
type PrincipalInput struct {
	Kind      securityplane.PrincipalKind
	Source    securityplane.PrincipalSource
	UserID    string
	AccountID string
	TenantID  string
	SessionID string
	TokenID   string
	Username  string
	Roles     []string
	AMR       []string
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
		Kind:      kind,
		Source:    source,
		UserID:    in.UserID,
		AccountID: in.AccountID,
		TenantID:  in.TenantID,
		SessionID: in.SessionID,
		TokenID:   in.TokenID,
		Username:  in.Username,
		Roles:     append([]string(nil), in.Roles...),
		AMR:       append([]string(nil), in.AMR...),
	}
}

// TenantScopeFromTenantID creates the canonical tenant/org scope view.
func TenantScopeFromTenantID(tenantID, casbinDomain string) securityplane.TenantScope {
	return securityplane.NewTenantScope(tenantID, casbinDomain)
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
