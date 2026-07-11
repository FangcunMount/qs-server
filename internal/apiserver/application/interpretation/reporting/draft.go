package reporting

import (
	interpinput "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/input"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
)

func draftWithInputSummary(input interpinput.InterpretationInput, draft *report.Draft) *report.Draft {
	if draft == nil {
		return nil
	}
	content := draft.Content()
	if !input.Model.IsEmpty() {
		content.Model = input.Model
	}
	content.PrimaryScore = input.Result.Primary
	content.Level = input.Result.Level
	return report.NewDraft(content)
}
