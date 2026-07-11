package modelcatalog

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	appauthz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/securityplane"
)

// ActorContext 是评估模型目录应用命令的传输中性安全上下文
type ActorContext struct {
	Principal securityplane.Principal // 主体
	Scope     securityplane.OrgScope  // 范围
}

// Action 标识评估模型目录应用用例权限边界
type Action string

const (
	ActionManageCatalog    Action = "manage_catalog"    // 管理评估模型目录
	ActionEditDefinition   Action = "edit_definition"   // 编辑评估模型定义
	ActionPublishCatalog   Action = "publish_catalog"   // 发布评估模型
	ActionReadCatalog      Action = "read_catalog"      // 读取评估模型
	ActionResolvePublished Action = "resolve_published" // 解析已发布评估模型
)

// Resource 标识应用命令的目标模型
type Resource struct {
	Code string      // 评估模型代码
	Kind domain.Kind // 评估模型类型
}

// Authorizer 保持应用边界内的权限决策
type Authorizer interface {
	Authorize(ctx context.Context, actor ActorContext, action Action, resource Resource) error
}

// SnapshotAuthorizer 评估IAM快照注入上下文的传输适配器
type SnapshotAuthorizer struct{}

// Authorize 授权模型目录应用用例
func (SnapshotAuthorizer) Authorize(ctx context.Context, actor ActorContext, action Action, _ Resource) error {
	if actor.Principal.Kind == securityplane.PrincipalKindUnknown {
		return errors.WithCode(code.ErrPermissionDenied, "authenticated actor is required")
	}
	if action != ActionResolvePublished && !actor.Scope.HasOrgID && !IsTrustedServiceActor(actor) {
		return errors.WithCode(code.ErrPermissionDenied, "resolved organization scope is required")
	}
	if action == ActionResolvePublished && actor.Principal.Kind != securityplane.PrincipalKindService {
		return errors.WithCode(code.ErrPermissionDenied, "published model resolution requires a service actor")
	}
	snapshot, ok := appauthz.FromContext(ctx)
	if !ok {
		return errors.WithCode(code.ErrPermissionDenied, "authorization snapshot required")
	}
	if decision := appauthz.DecideCapability(snapshot, capabilityForAction(action)); !decision.Allowed {
		return errors.WithCode(code.ErrPermissionDenied, "%s", decision.Reason)
	}
	return nil
}

// IsTrustedServiceActor reports whether an actor originates from a trusted
// internal service channel.
func IsTrustedServiceActor(actor ActorContext) bool {
	if actor.Principal.Kind != securityplane.PrincipalKindService {
		return false
	}
	return actor.Principal.Source == securityplane.PrincipalSourceServiceAuth ||
		actor.Principal.Source == securityplane.PrincipalSourceMTLS
}

// capabilityForAction 根据动作获取能力
func capabilityForAction(action Action) appauthz.Capability {
	switch action {
	case ActionManageCatalog:
		return appauthz.CapabilityManageAssessmentModels
	case ActionEditDefinition:
		return appauthz.CapabilityEditAssessmentModelDefinitions
	case ActionPublishCatalog:
		return appauthz.CapabilityPublishAssessmentModels
	case ActionReadCatalog:
		return appauthz.CapabilityReadAssessmentModels
	case ActionResolvePublished:
		return appauthz.CapabilityResolvePublishedAssessmentModels
	default:
		return ""
	}
}
