package main

import (
	"context"
	"strings"
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

func TestFinalControlledReadComparesStableInstancesAndRefreshesReadiness(t *testing.T) {
	old := stop.DependentRuntimeInstance{ContainerID: strings.Repeat("a", 64), ImageID: "sha256:" + strings.Repeat("b", 64), ProgramSHA256: strings.Repeat("c", 64), StateSHA256: strings.Repeat("d", 64), ReadySHA256: strings.Repeat("e", 64)}
	prior := stop.DependentRuntimeSnapshot{Instances: []stop.DependentRuntimeInstance{old}}
	for name, change := range map[string]func(*stop.DependentRuntimeInstance){
		"current-readiness": func(x *stop.DependentRuntimeInstance) { x.ReadySHA256 = strings.Repeat("f", 64) },
		"new-cid":           func(x *stop.DependentRuntimeInstance) { x.ContainerID = strings.Repeat("f", 64) },
		"new-image":         func(x *stop.DependentRuntimeInstance) { x.ImageID = "sha256:" + strings.Repeat("f", 64) },
		"wrong-program":     func(x *stop.DependentRuntimeInstance) { x.ProgramSHA256 = strings.Repeat("f", 64) },
		"restarted-process": func(x *stop.DependentRuntimeInstance) { x.StateSHA256 = strings.Repeat("f", 64) },
	} {
		t.Run(name, func(t *testing.T) {
			actual := old
			change(&actual)
			if lifecycleSameRuntimeInstances(prior, stop.DependentRuntimeSnapshot{Instances: []stop.DependentRuntimeInstance{actual}}) != (name == "current-readiness") {
				t.Fatal("final read lost original process/source binding or rejected fresh readiness")
			}
		})
	}
	if lifecycleSameRuntimeInstances(stop.DependentRuntimeSnapshot{}, stop.DependentRuntimeSnapshot{}) || lifecycleSameRuntimeInstances(prior, stop.DependentRuntimeSnapshot{}) {
		t.Fatal("absent final runtime became evidence")
	}
	if _, err := new(lifecycleFixedHost).observeFinalControlledRuntime(t.Context(), lifecycleRequest{}, new(lifecycleControlledRuntime)); err == nil {
		t.Fatal("unissued final runtime became purge permission")
	}
}

func TestFinalControlledReadOccursBeforePurgeAndNeverStartsServicesAgain(t *testing.T) {
	calls := preBComparisonProductionCalls(t, "lifecycle_controlled_runtime.go", "purgeAcceptedRemoteMaterials")
	read, purge := -1, -1
	for i, name := range calls {
		if name == "observeFinalControlledRuntime" {
			read = i
		}
		if name == "PurgeOwnedMaterials" {
			purge = i
		}
	}
	if read < 0 || purge <= read {
		t.Fatal("final live read happened after D terminal/material deletion")
	}
	calls = preBComparisonProductionCalls(t, "lifecycle_controlled_runtime.go", "observeFinalControlledRuntime")
	fences, reads, apiReads := 0, 0, 0
	for _, name := range calls {
		switch name {
		case "CheckWholeWriterFence":
			fences++
		case "observeControlled":
			reads++
		case "observeAcceptance":
			apiReads++
		case "controlledResume", "ResumeDependents", "RestoreDependents":
			t.Fatal("final read restarted services")
		}
	}
	if fences != 2 || reads != 1 || apiReads != 1 {
		t.Fatal("final actual read or its full fences were omitted")
	}
	for _, name := range preBComparisonProductionCalls(t, "lifecycle_fixed_host.go", "ResumeAcceptedEntrypoints") {
		if name == "ResumeDependents" || name == "Do" || name == "observeControlled" || name == "observeAcceptance" {
			t.Fatal("post-zero completion attempted deleted-channel/journal runtime work")
		}
	}
	if new(lifecycleFixedHost).ResumeAcceptedEntrypoints(t.Context(), lifecycleRequest{}) == nil {
		t.Fatal("completion was unconditional without native zero/terminal/window")
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
