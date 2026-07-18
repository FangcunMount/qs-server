package systemgovernance

import "github.com/FangcunMount/qs-server/internal/apiserver/cache/governance/target"

// ActionRegistry 暴露governance action 描述符。
type ActionRegistry struct {
	actions []ActionDescriptor
}

// NewActionRegistry 返回v1 action 目录。
func NewActionRegistry(enabled ...map[string]bool) *ActionRegistry {
	flags := map[string]bool{}
	if len(enabled) > 0 && enabled[0] != nil {
		flags = enabled[0]
	}
	return &ActionRegistry{actions: defaultActions(flags)}
}

// List 返回全部action 描述符。
func (r *ActionRegistry) List() []ActionDescriptor {
	if r == nil {
		return defaultActions(nil)
	}
	out := make([]ActionDescriptor, len(r.actions))
	copy(out, r.actions)
	return out
}

// Get 返回一个action 描述符。
func (r *ActionRegistry) Get(actionID string) (ActionDescriptor, bool) {
	for _, item := range r.List() {
		if item.ID == actionID {
			return item, true
		}
	}
	return ActionDescriptor{}, false
}

func defaultActions(enabled map[string]bool) []ActionDescriptor {
	return []ActionDescriptor{
		{
			ID:                   "cache.reload_policy",
			Domain:               DomainCache,
			Label:                "Reload cache policy",
			RiskLevel:            "medium",
			Enabled:              true,
			RequiresConfirmation: true,
			InputSchema: map[string]interface{}{
				"type": "object", "required": []string{"expected_version"},
				"properties": map[string]interface{}{
					"expected_version": map[string]interface{}{"type": "integer", "minimum": 1},
				},
			},
		},
		{
			ID:                   "cache.manual_warmup",
			Domain:               DomainCache,
			Label:                "Manual cache warmup",
			RiskLevel:            "low",
			Enabled:              true,
			RequiresConfirmation: true,
			InputSchema: map[string]interface{}{
				"type":     "object",
				"required": []string{"targets"},
				"properties": map[string]interface{}{
					"targets": map[string]interface{}{
						"type": "array",
						"items": map[string]interface{}{
							"type":     "object",
							"required": []string{"kind", "scope"},
							"properties": map[string]interface{}{
								"kind": map[string]interface{}{
									"type": "string",
									"enum": warmupKindEnum(),
								},
								"scope": map[string]interface{}{"type": "string"},
							},
						},
					},
				},
			},
		},
		{
			ID:                   "cache.repair_complete",
			Domain:               DomainCache,
			Label:                "Repair complete hook",
			RiskLevel:            "low",
			Enabled:              true,
			RequiresConfirmation: true,
			InputSchema: map[string]interface{}{
				"type":     "object",
				"required": []string{"repair_kind"},
				"properties": map[string]interface{}{
					"repair_kind":         map[string]interface{}{"type": "string"},
					"org_ids":             map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "integer"}},
					"questionnaire_codes": map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
					"plan_ids":            map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "integer"}},
				},
			},
		},
		{
			ID:        "events.replay_pending",
			Domain:    DomainEvents,
			Label:     "Replay pending outbox events",
			RiskLevel: "high",
			Enabled:   false,
			Planned:   true,
		},
		{
			ID: "resilience.release_lock", Domain: DomainResilience, Label: "Relinquish leader lease", RiskLevel: "high",
			Enabled: enabled["resilience.release_lock"], Planned: !enabled["resilience.release_lock"], RequiresConfirmation: true,
			InputSchema: map[string]interface{}{"type": "object", "required": []string{"component", "instance_id", "workload"}},
		},
		{
			ID: "resilience.tune_rate_limit", Domain: DomainResilience, Label: "Tune rate limit parameters", RiskLevel: "medium",
			Enabled: enabled["resilience.tune_rate_limit"], Planned: !enabled["resilience.tune_rate_limit"], RequiresConfirmation: true,
			InputSchema: map[string]interface{}{"type": "object", "required": []string{"mode", "component", "budget", "expected_version"}},
		},
	}
}

func warmupKindEnum() []string {
	return []string{
		string(cachetarget.WarmupKindStaticScale),
		string(cachetarget.WarmupKindStaticQuestionnaire),
		string(cachetarget.WarmupKindStaticTypologyModel),
		string(cachetarget.WarmupKindQueryStatsOverview),
	}
}
