package main

import (
	"context"
	"testing"

	stop "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirementstop"
)

func TestControlledRuntimeCannotAdoptSavedOrForeignOwners(t *testing.T) {
	ctx := context.Background()
	for _, s := range []*lifecycleServiceController{nil, {}, {local: new(stop.Lease)}, {remote: new(stop.RemoteController)}} {
		if s.controlledResume(ctx) == nil {
			t.Fatal("unissued original lease resumed")
		}
		if _, e := s.observeControlled(ctx, new(lifecycleAPITransition)); e == nil {
			t.Fatal("saved CID became runtime producer")
		}
	}
	h := new(lifecycleFixedHost)
	for _, o := range []*lifecycleControlledRuntime{nil, {}, {local: new(stop.DependentRuntimeObservation), remote: new(stop.RemoteRuntimeObservation)}} {
		if o.validate(h) == nil {
			t.Fatal("unissued runtime minted acceptance")
		}
	}
	if _, e := h.observeControlledAfterDataComparison(ctx, lifecycleRequest{}); e == nil {
		t.Fatal("missing frozen whole baseline resumed")
	}
	if _, e := h.purgeAcceptedRemoteMaterials(ctx, lifecycleRequest{}, new(lifecycleControlledRuntime)); e == nil {
		t.Fatal("unaccepted host purged remote materials")
	}
}

// Narrow regression for integration order only. The exact production calls are
// inspected; it cannot stand in for root Window/DDL/service/broker acceptance.
func TestControlledRuntimeActualCallerConsumesRetainedPreBProof(t *testing.T) {
	for _, c := range []struct{ file, function, effect string }{
		{"lifecycle_native_acceptance.go", "verifyNativeAcceptance", "observeControlledAfterDataComparison"},
		{"lifecycle_controlled_runtime.go", "observeControlledAfterDataComparison", "controlledResume"},
	} {
		t.Run(c.function, func(t *testing.T) {
			calls := preBComparisonProductionCalls(t, c.file, c.function)
			proof, effect := -1, -1
			for i, name := range calls {
				if name == "verifyCompleteDataBeforeInternalResume" {
					t.Fatal("controlled runtime rescanned stopped rows after B startup")
				}
				if name == "verifyPreBDataComparison" {
					proof = i
				}
				if name == c.effect {
					effect = i
				}
			}
			if proof < 0 || effect <= proof {
				t.Fatal("controlled internal resume bypassed original completed pre-B comparison")
			}
		})
	}
}

func TestControlledAIResumePrecedesDependentResumeAndFinalOnlyVerifies(t *testing.T) {
	calls := preBComparisonProductionCalls(t, "lifecycle_controlled_runtime.go", "observeControlledAfterDataComparison")
	proof, fence, ai, dependents := -1, -1, -1, -1
	for i, name := range calls {
		switch name {
		case "verifyPreBDataComparison":
			proof = i
		case "CheckWholeWriterFence":
			if fence < 0 {
				fence = i
			}
		case "Resume":
			ai = i
		case "controlledResume":
			dependents = i
		}
	}
	if proof < 0 || fence <= proof || ai <= fence || dependents <= ai {
		t.Fatal("AI resume bypassed completed comparison/full fence or followed dependents")
	}
	for _, name := range preBComparisonProductionCalls(t, "lifecycle_fixed_host.go", "ResumeAcceptedEntrypoints") {
		if name == "Resume" {
			t.Fatal("ordinary final resume may start AI again")
		}
	}
	h := new(lifecycleFixedHost)
	if h.registerAIStoppedMaterials(t.Context(), lifecycleRequest{}, new(lifecycleBatchMaterials)) == nil {
		t.Fatal("missing catalog minted journal handoff")
	}
}
