package modelcatalog

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	appauthz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/securityplane"
)

// ActorContext is the transport-neutral security context for model-catalog
// application commands. REST and gRPC adapters construct it from their own
// authentication mechanisms before invoking a use case.
type ActorContext struct {
	Principal securityplane.Principal
	Scope     securityplane.OrgScope
}

// Action identifies one model-catalog use-case permission boundary.
type Action string

const (
	ActionManageCatalog    Action = "manage_catalog"
	ActionEditDefinition   Action = "edit_definition"
	ActionPublishCatalog   Action = "publish_catalog"
	ActionReadCatalog      Action = "read_catalog"
	ActionResolvePublished Action = "resolve_published"
)

// Resource identifies the model targeted by an application command.
type Resource struct {
	Code string
	Kind domain.Kind
}

// Authorizer keeps permission decisions at the application boundary.
type Authorizer interface {
	Authorize(ctx context.Context, actor ActorContext, action Action, resource Resource) error
}

// SnapshotAuthorizer evaluates the IAM snapshot injected into context by a
// transport adapter. It deliberately does not depend on Gin or JWT details.
type SnapshotAuthorizer struct{}

func (SnapshotAuthorizer) Authorize(ctx context.Context, actor ActorContext, action Action, _ Resource) error {
	if actor.Principal.Kind == securityplane.PrincipalKindUnknown {
		return errors.WithCode(code.ErrPermissionDenied, "authenticated actor is required")
	}
	if action != ActionResolvePublished && !actor.Scope.HasOrgID {
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
