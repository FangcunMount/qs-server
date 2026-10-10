package main

import (
	"context"
	"errors"
	"reflect"
	"testing"

	fence "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirementfence"
	stop "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirementstop"
)

// These callback tests exercise the exact production ordering/error join. They
// never create a native Lease, Window, root management channel or service proof.
func TestPartialStopOnlyRecoversIssuedSidesAndKeepsUnknownErrors(t *testing.T) {
	localUnknown := errors.New("offline-local-stop-unknown")
	remoteUnknown := errors.New("offline-remote-stop-unknown")
	localRestoreFailed := errors.New("offline-local-restore-failed")
	remoteRestoreFailed := errors.New("offline-remote-restore-failed")
	for _, tc := range []struct {
		name                              string
		localStopErr, remoteStopErr       error
		localBaseline                     bool
		localRestoreErr, remoteRestoreErr error
		wantCalls                         []string
	}{
		{"local_failure_before_baseline", localUnknown, nil, false, nil, nil, []string{"local-stop"}},
		{"local_failure_after_baseline", localUnknown, nil, true, nil, nil, []string{"local-stop", "local-restore"}},
		{"remote_failure_after_local_stopped", nil, remoteUnknown, true, nil, nil, []string{"local-stop", "remote-stop", "local-restore", "remote-restore"}},
		{"both_partial_unknown_local_restore_refused", nil, remoteUnknown, true, localRestoreFailed, nil, []string{"local-stop", "remote-stop", "local-restore", "remote-restore"}},
		{"both_restore_failures_are_kept", nil, remoteUnknown, true, localRestoreFailed, remoteRestoreFailed, []string{"local-stop", "remote-stop", "local-restore", "remote-restore"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls []string
			var localRestore, remoteRestore func() error
			stopErr := lifecycleRunServiceStops(func() error {
				calls = append(calls, "local-stop")
				if tc.localBaseline {
					localRestore = func() error { calls = append(calls, "local-restore"); return tc.localRestoreErr }
				}
				return tc.localStopErr
			}, func() error {
				calls = append(calls, "remote-stop")
				remoteRestore = func() error { calls = append(calls, "remote-restore"); return tc.remoteRestoreErr }
				return tc.remoteStopErr
			})
			if stopErr == nil || tc.localStopErr != nil && !errors.Is(stopErr, tc.localStopErr) || tc.localStopErr == nil && !errors.Is(stopErr, tc.remoteStopErr) {
				t.Fatal("stop failure or unknown responsibility lost")
			}
			restoreErr := lifecycleRecoverIssuedServiceSides(localRestore, remoteRestore)
			if !reflect.DeepEqual(calls, tc.wantCalls) {
				t.Fatal("unissued side recovered or failed side skipped")
			}
			if (tc.localRestoreErr == nil && tc.remoteRestoreErr == nil) != (restoreErr == nil) ||
				tc.localRestoreErr != nil && !errors.Is(restoreErr, tc.localRestoreErr) ||
				tc.remoteRestoreErr != nil && !errors.Is(restoreErr, tc.remoteRestoreErr) {
				t.Fatal("partial recovery failure was hidden or prevented the other side")
			}
		})
	}
}

func TestPartialStopSuccessfulOrderAndNoCallbackCanGrantLease(t *testing.T) {
	var calls []string
	if err := lifecycleRunServiceStops(func() error { calls = append(calls, "local"); return nil }, func() error { calls = append(calls, "remote"); return nil }); err != nil || !reflect.DeepEqual(calls, []string{"local", "remote"}) {
		t.Fatal("successful stop order changed")
	}
	if lifecycleRunServiceStops(nil, func() error { t.Fatal("remote ran without local producer"); return nil }) == nil || lifecycleRunServiceStops(func() error { t.Fatal("local ran without remote producer"); return nil }, nil) == nil {
		t.Fatal("absent producer accepted")
	}
	if lifecycleEffectsPreflight(context.Background()) == nil {
		t.Fatal("routing callbacks activated missing production adapters")
	}
}

func TestPartialStopRejectsImportedWindowAndRetainsOriginalOwners(t *testing.T) {
	r, identity := recoveryServiceTestIdentity()
	w := new(fence.MaintenanceWindow)
	local, issuer := new(stop.Lease), new(stop.BudgetIssuer)
	v := &lifecycleServiceController{window: w, approval: new(stop.Approval), local: local, issuer: issuer, identity: identity, stopAttempted: true}
	for _, attempted := range []bool{false, true} {
		v.partialRecoveryAttempted = attempted
		if v.RestorePartialStop(context.Background(), r, w) == nil || v.local != local || v.issuer != issuer || v.window != w || v.remoteStopAttempted {
			t.Fatal("flags/imported opaque pointers granted recovery or changed original ownership")
		}
		if v.partialRecoveryAttempted != attempted {
			t.Fatal("rejected imported budget consumed actual recovery attempt")
		}
	}
	h := new(lifecycleFixedHost)
	if h.RestoreStoppedServices(context.Background(), r, w) == nil {
		t.Fatal("zero Window produced a successful service recovery receipt")
	}
}
