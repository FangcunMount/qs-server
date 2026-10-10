package compatibilityretirementbackup

import (
	"context"
	"testing"

	fence "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirementfence"
)

func TestHostRecoveredReadbackCannotReconstructNativeOwner(t *testing.T) {
	r := TargetRecoveryRequest{}
	w := new(fence.MaintenanceWindow)
	b := TargetRecoveryBorrowed{}
	for name, v := range map[string]*TargetRecoveryVerification{"nil": nil, "zero": {}, "self_only": {}} {
		t.Run(name, func(t *testing.T) {
			if name == "self_only" {
				v.self = v
			}
			if v.VerifyHostRecoveredTargets(context.Background(), b, r, w) == nil {
				t.Fatal("metadata/zero native recovery produced host readback")
			}
		})
	}
	for name, v := range map[string]*TargetLifecycleResult{"nil": nil, "zero": {}, "self_only": {}} {
		t.Run(name, func(t *testing.T) {
			if name == "self_only" {
				v.self = v
			}
			if v.VerifyHostRecoveredTargets(context.Background(), b, r, w) == nil {
				t.Fatal("saved lifecycle result became live host readback")
			}
		})
	}
	for _, ctx := range []context.Context{nil, context.Background()} {
		if verifyHostRecoveredTargetPlan(ctx, nil, b, r, w) == nil {
			t.Fatal("missing actual borrowed/native plan accepted")
		}
	}
}
