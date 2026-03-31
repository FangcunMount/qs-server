package authz

// Capability 与 REST capability_middleware 对齐，基于 IAM resource/action 判定。
type Capability string

const (
	CapabilityOrgAdmin              Capability = "org_admin"
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

// SnapshotSatisfiesCapability 判断 IAM 快照是否满足动作级能力（不依赖 JWT roles）。
func SnapshotSatisfiesCapability(s *Snapshot, c Capability) bool {
	if s == nil {
		return false
	}
	switch c {
	case CapabilityOrgAdmin:
		return s.IsQSAdmin()
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
