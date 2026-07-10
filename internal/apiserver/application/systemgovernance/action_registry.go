package systemgovernance

import "github.com/FangcunMount/qs-server/internal/apiserver/cachetarget"

// ActionRegistry 暴露governance action 描述符。
type ActionRegistry struct {
	actions []ActionDescriptor
}

// NewActionRegistry 返回v1 action 目录。
func NewActionRegistry() *ActionRegistry {
	return &ActionRegistry{actions: defaultActions()}
}

// List 返回全部action 描述符。
func (r *ActionRegistry) List() []ActionDescriptor {
	if r == nil {
		return defaultActions()
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

func defaultActions() []ActionDescriptor {
	return []ActionDescriptor{
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
			ID:        "resilience.drain_queue",
			Domain:    DomainResilience,
			Label:     "Drain in-memory queue",
			RiskLevel: "high",
			Enabled:   false,
			Planned:   true,
		},
		{
			ID:        "resilience.release_lock",
			Domain:    DomainResilience,
			Label:     "Release distributed lock",
			RiskLevel: "high",
			Enabled:   false,
			Planned:   true,
		},
		{
			ID:        "resilience.tune_rate_limit",
			Domain:    DomainResilience,
			Label:     "Tune rate limit parameters",
			RiskLevel: "medium",
			Enabled:   false,
			Planned:   true,
		},
	}
}

func warmupKindEnum() []string {
	return []string{
		string(cachetarget.WarmupKindStaticScale),
		string(cachetarget.WarmupKindStaticQuestionnaire),
		string(cachetarget.WarmupKindStaticTypologyModel),
		string(cachetarget.WarmupKindQueryStatsOverview),
		string(cachetarget.WarmupKindQueryStatsSystem),
		string(cachetarget.WarmupKindQueryStatsQuestionnaire),
		string(cachetarget.WarmupKindQueryStatsPlan),
	}
}
