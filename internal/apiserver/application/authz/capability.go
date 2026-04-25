package authz

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/pkg/securityplane"
)

// Capability 与 REST capability_middleware 对齐，基于 IAM resource/action 判定。
type Capability string

const (
	CapabilityOrgAdmin              Capability = "org_admin"
	CapabilityReadQuestionnaires    Capability = "read_questionnaires"
	CapabilityManageQuestionnaires  Capability = "manage_questionnaires"
	CapabilityReadScales            Capability = "read_scales"
	CapabilityManageScales          Capability = "manage_scales"
	CapabilityReadAnswersheets      Capability = "read_answersheets"
	CapabilityManageEvaluationPlans Capability = "manage_evaluation_plans"
	CapabilityEvaluateAssessments   Capability = "evaluate_assessments"
)

func hasAnyResourceAction(s *Snapshot, resource string, actions []string) bool {
	for _, a := range actions {
		if s.HasResourceAction(resource, a) {
			return true
		}
	}
	return false
}

// SnapshotViewFromSnapshot projects a request authz snapshot into the Security Control Plane model.
func SnapshotViewFromSnapshot(s *Snapshot) securityplane.AuthzSnapshotView {
	if s == nil {
		return securityplane.AuthzSnapshotView{}
	}
	permissions := make([]securityplane.AuthzPermissionView, 0, len(s.Permissions))
	for _, p := range s.Permissions {
		permissions = append(permissions, securityplane.AuthzPermissionView{
			Resource: p.Resource,
			Action:   p.Action,
		})
	}
	return securityplane.AuthzSnapshotView{
		Roles:        append([]string(nil), s.Roles...),
		Permissions:  permissions,
		AuthzVersion: s.AuthzVersion,
		CasbinDomain: s.CasbinDomain,
		IAMAppName:   s.IAMAppName,
	}
}

// DecideCapability explains whether an IAM snapshot satisfies one capability.
func DecideCapability(s *Snapshot, c Capability) securityplane.CapabilityDecision {
	if s == nil {
		return securityplane.CapabilityDecision{
			Capability: string(c),
			Allowed:    false,
			Outcome:    securityplane.CapabilityOutcomeMissingSnapshot,
			Reason:     "authorization snapshot required",
		}
	}
	if !isKnownCapability(c) {
		return securityplane.CapabilityDecision{
			Capability: string(c),
			Allowed:    false,
			Outcome:    securityplane.CapabilityOutcomeUnknown,
			Reason:     fmt.Sprintf("capability %s is not registered", c),
		}
	}
	if capabilityAllowed(s, c) {
		return securityplane.CapabilityDecision{
			Capability: string(c),
			Allowed:    true,
			Outcome:    securityplane.CapabilityOutcomeAllowed,
			Reason:     fmt.Sprintf("capability %s allowed by IAM authorization", c),
		}
	}
	return securityplane.CapabilityDecision{
		Capability: string(c),
		Allowed:    false,
		Outcome:    securityplane.CapabilityOutcomeDenied,
		Reason:     fmt.Sprintf("capability %s denied by IAM authorization", c),
	}
}

// DecideAnyCapability explains whether an IAM snapshot satisfies at least one capability.
func DecideAnyCapability(s *Snapshot, capabilities ...Capability) securityplane.CapabilityDecision {
	if s == nil {
		return securityplane.CapabilityDecision{
			Capability: fmt.Sprint(capabilities),
			Allowed:    false,
			Outcome:    securityplane.CapabilityOutcomeMissingSnapshot,
			Reason:     "authorization snapshot required",
		}
	}
	hasKnown := false
	for _, c := range capabilities {
		decision := DecideCapability(s, c)
		if decision.Outcome != securityplane.CapabilityOutcomeUnknown {
			hasKnown = true
		}
		if decision.Allowed {
			return decision
		}
	}
	if !hasKnown {
		return securityplane.CapabilityDecision{
			Capability: fmt.Sprint(capabilities),
			Allowed:    false,
			Outcome:    securityplane.CapabilityOutcomeUnknown,
			Reason:     fmt.Sprintf("capabilities %v are not registered", capabilities),
		}
	}
	return securityplane.CapabilityDecision{
		Capability: fmt.Sprint(capabilities),
		Allowed:    false,
		Outcome:    securityplane.CapabilityOutcomeDenied,
		Reason:     fmt.Sprintf("capabilities %v denied by IAM authorization", capabilities),
	}
}

// SnapshotSatisfiesCapability 判断 IAM 快照是否满足动作级能力（不依赖 JWT roles）。
func SnapshotSatisfiesCapability(s *Snapshot, c Capability) bool {
	return DecideCapability(s, c).Allowed
}

func isKnownCapability(c Capability) bool {
	switch c {
	case CapabilityOrgAdmin:
	case CapabilityReadQuestionnaires:
	case CapabilityManageQuestionnaires:
	case CapabilityReadScales:
	case CapabilityManageScales:
	case CapabilityReadAnswersheets:
	case CapabilityManageEvaluationPlans:
	case CapabilityEvaluateAssessments:
	default:
		return false
	}
	return true
}

func capabilityAllowed(s *Snapshot, c Capability) bool {
	switch c {
	case CapabilityOrgAdmin:
		return s.IsQSAdmin()
	case CapabilityReadQuestionnaires:
		if s.IsQSAdmin() {
			return true
		}
		return hasAnyResourceAction(s, "qs:questionnaires", []string{"read", "list"})
	case CapabilityManageQuestionnaires:
		if s.IsQSAdmin() {
			return true
		}
		return hasAnyResourceAction(s, "qs:questionnaires", []string{"create", "update", "delete", "publish", "unpublish", "archive", "statistics"})
	case CapabilityReadScales:
		if s.IsQSAdmin() {
			return true
		}
		return hasAnyResourceAction(s, "qs:scales", []string{"read", "list"})
	case CapabilityManageScales:
		if s.IsQSAdmin() {
			return true
		}
		return hasAnyResourceAction(s, "qs:scales", []string{"create", "update", "delete", "publish", "unpublish", "archive"})
	case CapabilityReadAnswersheets:
		if s.IsQSAdmin() {
			return true
		}
		return hasAnyResourceAction(s, "qs:answersheets", []string{"read", "list", "statistics"})
	case CapabilityManageEvaluationPlans:
		if s.IsQSAdmin() {
			return true
		}
		planActs := []string{"create", "update", "pause", "resume", "cancel", "enroll", "terminate", "statistics"}
		taskActs := []string{"schedule", "open", "complete", "expire", "cancel", "read", "list"}
		return hasAnyResourceAction(s, "qs:evaluation_plans", planActs) &&
			hasAnyResourceAction(s, "qs:evaluation_plan_tasks", taskActs)
	case CapabilityEvaluateAssessments:
		if s.IsQSAdmin() {
			return true
		}
		return s.HasResourceAction("qs:assessments", "retry") ||
			s.HasResourceAction("qs:assessments", "batch_evaluate")
	default:
		return false
	}
}
