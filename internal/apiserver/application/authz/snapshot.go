package authz

import (
	"context"
	"strings"
)

type snapCtxKey struct{}

// AuthorizationMode is the local projection of IAM AuthZ v3 permission modes.
type AuthorizationMode int32

const (
	AuthorizationModeUnspecified         AuthorizationMode = 0
	AuthorizationModeUnconditional       AuthorizationMode = 1
	AuthorizationModeObjectCheckRequired AuthorizationMode = 2
)

// Permission is a single exact AuthZ v3 resource/action capability.
type Permission struct {
	Resource string
	Action   string
	Mode     AuthorizationMode
}

// Snapshot 即 CurrentAuthzSnapshot：IAM GetAuthorizationSnapshot 在单次请求内的授权投影。
// 动作真值以 IAM 为准；不在 QS 内自造与 IAM 冲突的角色真值。
type Snapshot struct {
	DirectRoles         []string
	EffectiveRoles      []string
	Permissions         []Permission
	AuthzVersion        int64
	AuthorizationDomain string
	IAMAppName          string
}

// WithSnapshot 将快照写入 context（供 application 层使用）。
func WithSnapshot(ctx context.Context, s *Snapshot) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, snapCtxKey{}, s)
}

// FromContext 读取授权快照；未注入时 ok 为 false。
func FromContext(ctx context.Context) (*Snapshot, bool) {
	if ctx == nil {
		return nil, false
	}
	v := ctx.Value(snapCtxKey{})
	if v == nil {
		return nil, false
	}
	s, ok := v.(*Snapshot)
	return s, ok && s != nil
}

// SubjectKey 固定为 user:<user_id>，与 IAM AuthZ v3 Subject 契约对齐。
func SubjectKey(userIDStr string) string {
	return "user:" + userIDStr
}

// DirectRoleNames returns the roles assigned directly to the subject.
func (s *Snapshot) DirectRoleNames() []string {
	if s == nil {
		return nil
	}
	return append([]string(nil), s.DirectRoles...)
}

// EffectiveRoleNames returns direct roles plus their inheritance closure.
func (s *Snapshot) EffectiveRoleNames() []string {
	if s == nil {
		return nil
	}
	return append([]string(nil), s.EffectiveRoles...)
}

func actionCovers(have, want string) bool {
	if have == "" || want == "" {
		return false
	}
	if have == "*" {
		return true
	}
	return have == want
}

func resourceCovers(pattern, resource string) bool {
	if pattern == resource {
		return true
	}
	patternParts := strings.Split(pattern, ":")
	resourceParts := strings.Split(resource, ":")
	if len(patternParts) != len(resourceParts) {
		return false
	}
	for i := range patternParts {
		if patternParts[i] != "*" && patternParts[i] != resourceParts[i] {
			return false
		}
	}
	return true
}

// HasResourceAction only recognizes unconditional permissions.
func (s *Snapshot) HasResourceAction(resource, want string) bool {
	if s == nil {
		return false
	}
	for _, p := range s.Permissions {
		res := p.Resource
		act := p.Action
		if p.Mode == AuthorizationModeUnconditional && resourceCovers(res, resource) && actionCovers(act, want) {
			return true
		}
	}
	return false
}

// HasObjectAuthorizationCandidate recognizes both unconditional and object-check permissions.
// It is only a routing guard; IAM Check remains authoritative for object-check entries.
func (s *Snapshot) HasObjectAuthorizationCandidate(resource, action string) bool {
	if s == nil {
		return false
	}
	for _, permission := range s.Permissions {
		if permission.Mode == AuthorizationModeUnspecified {
			continue
		}
		if resourceCovers(permission.Resource, resource) && actionCovers(permission.Action, action) {
			return true
		}
	}
	return false
}

// IsQSAdmin requires the final unconditional QS wildcard grant. A role name
// alone is not an authorization decision.
func (s *Snapshot) IsQSAdmin() bool {
	if s == nil {
		return false
	}
	for _, p := range s.Permissions {
		if p.Mode == AuthorizationModeUnconditional &&
			(p.Resource == "qs:*:*:*" || p.Resource == "*:*:*:*") && p.Action == "*" {
			return true
		}
	}
	return false
}
