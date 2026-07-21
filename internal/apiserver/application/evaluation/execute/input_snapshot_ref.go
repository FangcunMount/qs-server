package execute

import (
	"fmt"
	"strconv"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

// inputSnapshotRefFromResolvedInput builds a verifiable audit reference for a
// resolved input snapshot (EV-R009). New writes use the digest-backed
// "isn:v1:" form; readable legacy forms remain only as a fallback for
// degenerate snapshots and for reading historical rows.
func inputSnapshotRefFromResolvedInput(a *assessment.Assessment, input *evaluationinput.InputSnapshot) string {
	if identity, ok := evaluationinput.NewInputSnapshotIdentity(input); ok {
		return identity.Ref()
	}
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
// carry no content proof, so only isn:v1 pairs are comparable.
func validateInputSnapshotRefAcrossAttempts(previous, current string) error {
	if previous == "" || current == "" {
		return nil
	}
	if !evaluationinput.IsIdentityRef(previous) || !evaluationinput.IsIdentityRef(current) {
		return nil
	}
	if previous != current {
		return fmt.Errorf("input snapshot drifted between attempts: previous=%s current=%s", previous, current)
	}
	return nil
}
