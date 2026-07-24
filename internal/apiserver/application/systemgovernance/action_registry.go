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
	manualActionsEnabled := true
	if value, configured := enabled["retry.manual_actions"]; configured {
		manualActionsEnabled = value
	}
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
			ID:                   "events.replay_pending",
			Domain:               DomainEvents,
			Label:                "Replay pending outbox events",
			RiskLevel:            "high",
			Enabled:              manualActionsEnabled,
			RequiresConfirmation: true,
			InputSchema:          replayPendingSchema(),
		},
		governedRetryAction("evaluation.retry", DomainEvents, "Retry evaluation", "medium", manualActionsEnabled),
		governedRetryAction("evaluation.force_retry", DomainEvents, "Force retry terminal evaluation", "high", manualActionsEnabled),
		governedRetryAction("interpretation.retry", DomainEvents, "Retry interpretation", "medium", manualActionsEnabled),
		governedRetryAction("interpretation.force_retry", DomainEvents, "Force retry terminal interpretation", "high", manualActionsEnabled),
		reportTemplateAction("interpretation.report_template_publish", "Publish report template version", "draft", manualActionsEnabled),
		reportTemplateAction("interpretation.report_template_disable", "Disable report template version", "published", manualActionsEnabled),
		readmissionAction(manualActionsEnabled),
		catalogRepairAction(manualActionsEnabled),
		{
			ID: "events.replay_delivery", Domain: DomainEvents, Label: "Replay transport dead letter", RiskLevel: "high",
			Enabled: manualActionsEnabled, RequiresConfirmation: true, InputSchema: replayDeliverySchema(),
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

func catalogRepairAction(enabled bool) ActionDescriptor {
	return ActionDescriptor{
		ID: "interpretation.catalog_repair", Domain: DomainActions, Label: "Repair interpretation report catalog",
		RiskLevel: "high", Enabled: enabled, RequiresConfirmation: true,
		InputSchema: map[string]interface{}{
			"type":     "object",
			"required": []string{"dry_run_id", "expected_catalog_version", "expected_source", "reason"},
			"properties": map[string]interface{}{
				"dry_run_id":               map[string]interface{}{"type": "string", "minLength": 1},
				"expected_catalog_version": map[string]interface{}{"type": "string", "minLength": 1},
				"expected_source":          map[string]interface{}{"type": "string", "enum": []string{"artifact", "archive"}},
				"reason":                   map[string]interface{}{"type": "string", "minLength": 1},
			},
		},
	}
}

func readmissionAction(enabled bool) ActionDescriptor {
	return ActionDescriptor{
		ID: "interpretation.readmit_outcome", Domain: DomainActions, Label: "Readmit committed interpretation outcome",
		RiskLevel: "high", Enabled: enabled, RequiresConfirmation: true,
		InputSchema: map[string]interface{}{
			"type":     "object",
			"required": []string{"failure_fingerprint", "expected_reason", "expected_outcome_version", "reason"},
			"properties": map[string]interface{}{
				"failure_fingerprint":      map[string]interface{}{"type": "string", "minLength": 1},
				"expected_reason":          map[string]interface{}{"type": "string", "minLength": 1},
				"expected_outcome_version": map[string]interface{}{"type": "string", "minLength": 1},
				"reason":                   map[string]interface{}{"type": "string", "minLength": 1},
			},
		},
	}
}

func reportTemplateAction(id, label, expectedStatus string, enabled bool) ActionDescriptor {
	return ActionDescriptor{
		ID: id, Domain: DomainActions, Label: label, RiskLevel: "high",
		Enabled: enabled, RequiresConfirmation: true,
		InputSchema: map[string]interface{}{
			"type":     "object",
			"required": []string{"template_id", "template_version", "expected_status", "reason"},
			"properties": map[string]interface{}{
				"template_id":      map[string]interface{}{"type": "string", "minLength": 1},
				"template_version": map[string]interface{}{"type": "string", "minLength": 1},
				"expected_status":  map[string]interface{}{"type": "string", "enum": []string{expectedStatus}},
				"reason":           map[string]interface{}{"type": "string", "minLength": 1},
			},
		},
	}
}

func replayDeliverySchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object", "required": []string{"targets", "reason"},
		"properties": map[string]interface{}{
			"reason": map[string]interface{}{"type": "string", "minLength": 1},
			"targets": map[string]interface{}{
				"type": "array", "minItems": 1, "maxItems": 100,
				"items": map[string]interface{}{
					"type": "object", "required": []string{"id", "expected_delivery_attempts"},
					"properties": map[string]interface{}{
						"id":                         map[string]interface{}{"type": "integer", "minimum": 1},
						"expected_delivery_attempts": map[string]interface{}{"type": "integer", "minimum": 1},
					},
				},
			},
		},
	}
}

func governedRetryAction(id string, domain Domain, label, risk string, enabled bool) ActionDescriptor {
	return ActionDescriptor{
		ID: id, Domain: domain, Label: label, RiskLevel: risk, Enabled: enabled, RequiresConfirmation: true,
		InputSchema: map[string]interface{}{
			"type": "object", "required": []string{"resource_id", "expected_attempt", "reason"},
			"properties": map[string]interface{}{
				"resource_id":      map[string]interface{}{"type": "string"},
				"expected_attempt": map[string]interface{}{"type": "integer", "minimum": 1},
				"reason":           map[string]interface{}{"type": "string", "minLength": 1},
			},
		},
	}
}

func replayPendingSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object", "required": []string{"store", "targets", "reason"},
		"properties": map[string]interface{}{
			"store":  map[string]interface{}{"type": "string"},
			"reason": map[string]interface{}{"type": "string", "minLength": 1},
			"targets": map[string]interface{}{
				"type": "array", "minItems": 1, "maxItems": 100,
				"items": map[string]interface{}{"type": "object", "required": []string{"event_id", "expected_attempt_count"}},
			},
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
