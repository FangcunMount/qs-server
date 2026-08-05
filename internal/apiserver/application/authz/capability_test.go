package authz

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/pkg/securityplane"
)

func TestDecideCapabilityOutcomes(t *testing.T) {
	admin := &Snapshot{Roles: []string{"qs:admin"}}
	reader := &Snapshot{Permissions: []Permission{{Resource: "qs:questionnaires", Action: "read|list"}}}

	tests := []struct {
		name       string
		snapshot   *Snapshot
		capability Capability
		allowed    bool
		outcome    securityplane.CapabilityOutcome
	}{
		{
			name:       "missing snapshot",
			capability: CapabilityReadQuestionnaires,
			outcome:    securityplane.CapabilityOutcomeMissingSnapshot,
		},
		{
			name:       "allowed by admin",
			snapshot:   admin,
			capability: CapabilityOrgAdmin,
			allowed:    true,
			outcome:    securityplane.CapabilityOutcomeAllowed,
		},
		{
			name:       "allowed by resource action",
			snapshot:   reader,
			capability: CapabilityReadQuestionnaires,
			allowed:    true,
			outcome:    securityplane.CapabilityOutcomeAllowed,
		},
		{
			name:       "denied known capability",
			snapshot:   reader,
			capability: CapabilityManageAssessmentModels,
			outcome:    securityplane.CapabilityOutcomeDenied,
		},
		{
			name:       "unknown capability",
			snapshot:   reader,
			capability: Capability("unknown"),
			outcome:    securityplane.CapabilityOutcomeUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision := DecideCapability(tt.snapshot, tt.capability)
			if decision.Allowed != tt.allowed {
				t.Fatalf("allowed = %v, want %v: %#v", decision.Allowed, tt.allowed, decision)
			}
			if decision.Outcome != tt.outcome {
				t.Fatalf("outcome = %q, want %q: %#v", decision.Outcome, tt.outcome, decision)
			}
		})
	}
}

func TestDecideAnyCapability(t *testing.T) {
	snap := &Snapshot{Permissions: []Permission{{Resource: "qs:assessment_models", Action: "read"}}}

	decision := DecideAnyCapability(snap, CapabilityManageQuestionnaires, CapabilityReadAssessmentModels)
	if !decision.Allowed || decision.Outcome != securityplane.CapabilityOutcomeAllowed {
		t.Fatalf("decision = %#v, want allowed", decision)
	}

	denied := DecideAnyCapability(snap, CapabilityManageQuestionnaires, CapabilityManageAssessmentModels)
	if denied.Allowed || denied.Outcome != securityplane.CapabilityOutcomeDenied {
		t.Fatalf("denied = %#v, want denied", denied)
	}
}

func TestNormTableCapabilities(t *testing.T) {
	t.Parallel()
	snapshot := &Snapshot{Permissions: []Permission{{Resource: "qs:modelcatalog:collection:norm_tables", Action: "read|list|import"}}}
	if !DecideCapability(snapshot, CapabilityReadNormTables).Allowed {
		t.Fatal("read_norm_tables should be allowed")
	}
	if !DecideCapability(snapshot, CapabilityManageNormTables).Allowed {
		t.Fatal("manage_norm_tables should be allowed")
	}
}
