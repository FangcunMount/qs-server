package cognitive

import domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"

func publishValidationError(model *domain.AssessmentModel) error {
	if model == nil {
		return invalidArgument("模型不能为空")
	}
	result := model.ValidateForPublish()
	if result.Passed() {
		return nil
	}
	return invalidArgument("%s", result.Issues[0].Message)
}
