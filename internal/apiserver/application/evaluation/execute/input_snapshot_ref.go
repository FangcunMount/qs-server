package execute

import (
	"fmt"
	"strconv"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
)

// inputSnapshotRefFromResolvedInput builds a verifiable audit reference for a
// resolved input snapshot (EV-R009). New writes use the digest-backed
// "isn:v2:" form; readable legacy forms remain only as a fallback for
// degenerate snapshots and for reading historical rows.
func inputSnapshotRefFromResolvedInput(a *assessment.Assessment, input *evaluationinput.InputSnapshot) string {
	if identity, ok := evaluationinput.NewInputSnapshotIdentity(input); ok {
		return identity.Ref()
	}
	return fallbackInputSnapshotRef(a, input)
}

func inputSnapshotRefForAttempt(
	a *assessment.Assessment,
	input *evaluationinput.InputSnapshot,
	previous string,
	origin retrygovernance.AttemptOrigin,
) string {
	if evaluationinput.IsV1IdentityRef(previous) && origin != retrygovernance.AttemptOriginForce {
		if identity, ok := evaluationinput.NewLegacyV1InputSnapshotIdentity(input); ok {
			return identity.Ref()
		}
	}
	return inputSnapshotRefFromResolvedInput(a, input)
}

func fallbackInputSnapshotRef(a *assessment.Assessment, input *evaluationinput.InputSnapshot) string {
	if input != nil && input.Model != nil {
		code := input.Model.Code
		version := input.Model.Version
		if code != "" {
			if version != "" {
				return fmt.Sprintf("model:%s@%s", code, version)
			}
			return fmt.Sprintf("model:%s", code)
		}
	}
	if a != nil {
		if ref := a.AnswerSheetRef(); !ref.IsEmpty() {
			return "answersheet:" + strconv.FormatUint(ref.ID().Uint64(), 10)
		}
	}
	return ""
}

// validateInputSnapshotRefAcrossAttempts rejects retries whose re-materialized
// input differs from the previous attempt (EV-R009). Legacy readable refs
// carry no content proof; v1/v2 identity refs are both comparable.
func validateInputSnapshotRefAcrossAttempts(previous, current string, origin retrygovernance.AttemptOrigin) error {
	if previous == "" || current == "" {
		return nil
	}
	if !evaluationinput.IsIdentityRef(previous) || !evaluationinput.IsIdentityRef(current) {
		return nil
	}
	if origin == retrygovernance.AttemptOriginForce {
		return nil
	}
	if previous != current {
		return fmt.Errorf("input snapshot drifted between attempts: previous=%s current=%s", previous, current)
	}
	return nil
}
