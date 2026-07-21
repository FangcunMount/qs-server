package execute

import (
	"errors"
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
)

var (
	errInputSnapshotIdentityRequired = errors.New("input snapshot identity is required")
	errUnsupportedInputSnapshotRef   = errors.New("previous input snapshot reference is not an isn:v2 identity")
)

// inputSnapshotRefFromResolvedInput builds a verifiable audit reference for a
// resolved input snapshot (EV-R009). Every attempt requires the digest-backed
// isn:v2 form; incomplete material never falls back to a readable label.
func inputSnapshotRefFromResolvedInput(input *evaluationinput.InputSnapshot) (string, error) {
	if identity, ok := evaluationinput.NewInputSnapshotIdentity(input); ok {
		ref := identity.Ref()
		if evaluationinput.IsIdentityRef(ref) {
			return ref, nil
		}
	}
	return "", errInputSnapshotIdentityRequired
}

// validateInputSnapshotRefAcrossAttempts rejects retries whose re-materialized
// input differs from the previous attempt (EV-R009). Force retry may establish
// a different v2 identity, but it never admits an unsupported historical ref.
func validateInputSnapshotRefAcrossAttempts(previous, current string, origin retrygovernance.AttemptOrigin) error {
	if !evaluationinput.IsIdentityRef(current) {
		return errInputSnapshotIdentityRequired
	}
	if previous == "" {
		return nil
	}
	if !evaluationinput.IsIdentityRef(previous) {
		return fmt.Errorf("%w: %s", errUnsupportedInputSnapshotRef, previous)
	}
	if origin == retrygovernance.AttemptOriginForce {
		return nil
	}
	if previous != current {
		return fmt.Errorf("input snapshot drifted between attempts: previous=%s current=%s", previous, current)
	}
	return nil
}
