package authz

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/pkg/securityplane"
)

func TestSnapshotViewFromSnapshotCopiesFields(t *testing.T) {
	snap := &Snapshot{
		Roles: []string{"qs:operator"},
		Permissions: []Permission{
			{Resource: "qs:questionnaires", Action: "read|list"},
		},
		AuthzVersion: 7,
		CasbinDomain: "tenant:42",
		IAMAppName:   "qs-server",
	}

	view := SnapshotViewFromSnapshot(snap)
	snap.Roles[0] = "mutated"
	snap.Permissions[0].Resource = "mutated"

	if got := view.RoleNames(); len(got) != 1 || got[0] != "qs:operator" {
		t.Fatalf("roles = %#v, want [qs:operator]", got)
	}
	if got := view.PermissionViews(); len(got) != 1 || got[0].Resource != "qs:questionnaires" {
		t.Fatalf("permissions = %#v, want qs:questionnaires", got)
	}
	if view.AuthzVersion != 7 || view.CasbinDomain != "tenant:42" || view.IAMAppName != "qs-server" {
		t.Fatalf("view metadata = %#v", view)
	}
}

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
			capability: CapabilityManageScales,
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
			if SnapshotSatisfiesCapability(tt.snapshot, tt.capability) != tt.allowed {
				t.Fatalf("compat bool drifted for %s", tt.capability)
			}
		})
	}
}

func TestDecideAnyCapability(t *testing.T) {
	snap := &Snapshot{Permissions: []Permission{{Resource: "qs:scales", Action: "read"}}}

	decision := DecideAnyCapability(snap, CapabilityManageQuestionnaires, CapabilityReadScales)
	if !decision.Allowed || decision.Outcome != securityplane.CapabilityOutcomeAllowed {
		t.Fatalf("decision = %#v, want allowed", decision)
	}

	denied := DecideAnyCapability(snap, CapabilityManageQuestionnaires, CapabilityManageScales)
	if denied.Allowed || denied.Outcome != securityplane.CapabilityOutcomeDenied {
		t.Fatalf("denied = %#v, want denied", denied)
	}
}
