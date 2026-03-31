package authz

import (
	"context"
	"strings"
)

type snapCtxKey struct{}

// Permission 表示 IAM 授权快照中的一条 (resource, action)；action 可能为 Casbin 中的组合串如 "read|update"。
type Permission struct {
	Resource string
	Action   string
}

// Snapshot 即 CurrentAuthzSnapshot：IAM GetAuthorizationSnapshot 在单次请求内的授权投影。
// 动作真值以 IAM 为准；不在 QS 内自造与 IAM 冲突的角色真值。
type Snapshot struct {
	Roles         []string
	Permissions   []Permission
	AuthzVersion  int64
	CasbinDomain  string
	IAMAppName    string
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

// SubjectKey 固定为 user:<user_id>，与 IAM Assignment / Casbin sub 对齐。
func SubjectKey(userIDStr string) string {
	return "user:" + userIDStr
}

func (s *Snapshot) hasRole(name string) bool {
	if s == nil || name == "" {
		return false
	}
	for _, r := range s.Roles {
		if r == name {
			return true
		}
	}
	return false
}

func actionCovers(have, want string) bool {
	if have == "" || want == "" {
		return false
	}
	// Casbin 管理员策略常见为 object=qs:*, action=.*
	if have == ".*" || have == "*" {
		return true
	}
	for _, part := range strings.Split(have, "|") {
		if strings.TrimSpace(part) == want {
			return true
		}
	}
	return false
}

// HasResourceAction 判断快照中是否允许对 resource 执行 want 动作（支持 qs:* / .* 通配）。
func (s *Snapshot) HasResourceAction(resource, want string) bool {
	if s == nil {
		return false
	}
	for _, p := range s.Permissions {
		res := p.Resource
		act := p.Action
		if res == "qs:*" && actionCovers(act, want) {
			return true
		}
		if res == resource && actionCovers(act, want) {
			return true
		}
	}
	return false
}

// IsQSAdmin 使用 IAM 快照：qs:admin 角色或 qs:* + 通配动作（如 .*）。
func (s *Snapshot) IsQSAdmin() bool {
	if s == nil {
		return false
	}
	if s.hasRole("qs:admin") {
		return true
	}
	for _, p := range s.Permissions {
		if p.Resource == "qs:*" && actionCovers(p.Action, "read") {
			return true
		}
	}
	return false
}
