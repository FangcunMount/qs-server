package execute

import (
	"fmt"
	"strconv"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

// inputSnapshotRefFromResolvedInput builds a stable, readable audit reference for a resolved input snapshot.
func inputSnapshotRefFromResolvedInput(a *assessment.Assessment, input *evaluationinput.InputSnapshot) string {
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
