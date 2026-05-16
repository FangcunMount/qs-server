package securityplane

// PrincipalKind identifies what kind of authenticated subject is represented.
type PrincipalKind string

const (
	PrincipalKindUnknown PrincipalKind = "unknown"
	PrincipalKindUser    PrincipalKind = "user"
	PrincipalKindService PrincipalKind = "service"
)

// PrincipalSource identifies the transport or credential source that produced a principal view.
type PrincipalSource string

const (
	PrincipalSourceUnknown     PrincipalSource = "unknown"
	PrincipalSourceHTTPJWT     PrincipalSource = "http_jwt"
	PrincipalSourceGRPCJWT     PrincipalSource = "grpc_jwt"
	PrincipalSourceServiceAuth PrincipalSource = "service_auth"
	PrincipalSourceMTLS        PrincipalSource = "mtls"
)

// Principal is the read-only identity view used by the Security Control Plane.
type Principal struct {
	Kind         PrincipalKind
	Source       PrincipalSource
	UserID       string
	AccountID    string
	TenantDomain string // IAM authorization domain (e.g. fangcun, platform).
	OrgID        uint64 // QS business organization scope when resolved.
	HasOrgID     bool
	SessionID    string
	TokenID      string
	Username     string
	Roles        []string
	AMR          []string
}

// RoleNames returns a defensive copy of role names.
func (p Principal) RoleNames() []string {
	return append([]string(nil), p.Roles...)
}

// AuthenticationMethods returns a defensive copy of AMR values.
func (p Principal) AuthenticationMethods() []string {
	return append([]string(nil), p.AMR...)
}

// OrgScope is the read-only IAM authorization domain and QS business org projection.
type OrgScope struct {
	TenantDomain   string
	OrgID          uint64
	HasOrgID       bool
	CasbinDomain   string
	RawScopeSource string
}

// NewOrgScope creates the canonical security-plane org scope view.
func NewOrgScope(tenantDomain string, orgID uint64, hasOrg bool, casbinDomain string) OrgScope {
	return OrgScope{
		TenantDomain: tenantDomain,
		OrgID:        orgID,
		HasOrgID:     hasOrg && orgID > 0,
		CasbinDomain: casbinDomain,
	}
}

// AuthzPermissionView is the transport-agnostic view of one IAM resource/action permission.
type AuthzPermissionView struct {
	Resource string
	Action   string
}

// AuthzSnapshotView is a read-only projection of the IAM authorization snapshot.
type AuthzSnapshotView struct {
	Roles        []string
	Permissions  []AuthzPermissionView
	AuthzVersion int64
	CasbinDomain string
	IAMAppName   string
}

// RoleNames returns a defensive copy of snapshot roles.
func (s AuthzSnapshotView) RoleNames() []string {
	return append([]string(nil), s.Roles...)
}

// PermissionViews returns a defensive copy of snapshot permissions.
func (s AuthzSnapshotView) PermissionViews() []AuthzPermissionView {
	return append([]AuthzPermissionView(nil), s.Permissions...)
}

// CapabilityOutcome is a bounded capability decision result.
type CapabilityOutcome string

const (
	CapabilityOutcomeAllowed         CapabilityOutcome = "allowed"
	CapabilityOutcomeDenied          CapabilityOutcome = "denied"
	CapabilityOutcomeMissingSnapshot CapabilityOutcome = "missing_snapshot"
	CapabilityOutcomeUnknown         CapabilityOutcome = "unknown_capability"
	CapabilityOutcomeInvalidScope    CapabilityOutcome = "invalid_scope"
)

// CapabilityDecision is the read-only explanation of one capability check.
type CapabilityDecision struct {
	Capability string
	Allowed    bool
	Outcome    CapabilityOutcome
	Reason     string
}

// ServiceIdentitySource identifies where a service identity came from.
type ServiceIdentitySource string

const (
	ServiceIdentitySourceUnknown     ServiceIdentitySource = "unknown"
	ServiceIdentitySourceServiceAuth ServiceIdentitySource = "service_auth"
	ServiceIdentitySourceMTLS        ServiceIdentitySource = "mtls"
)

// ServiceIdentity is the read-only service principal view for service auth and mTLS.
type ServiceIdentity struct {
	ServiceID      string
	Source         ServiceIdentitySource
	TargetAudience []string
	CommonName     string
	Namespace      string
}

// Audiences returns a defensive copy of target audiences.
func (s ServiceIdentity) Audiences() []string {
	return append([]string(nil), s.TargetAudience...)
}
