package response

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/application/survey/questionnaire"
	"github.com/FangcunMount/qs-server/internal/apiserver/interface/restful/viewmodel"
)

// QuestionnaireResponse 问卷响应
type QuestionnaireResponse struct {
	Code        string                  `json:"code"`
	Title       string                  `json:"title"`
	Description string                  `json:"description"`
	ImgUrl      string                  `json:"img_url"`
	Version     string                  `json:"version"`
	Status      string                  `json:"status"`
	Type        string                  `json:"type"`
	Questions   []viewmodel.QuestionDTO `json:"questions,omitempty"`
}

// QuestionnaireListResponse 问卷列表响应
type QuestionnaireListResponse struct {
	Questionnaires []QuestionnaireResponse `json:"questionnaires"`
	TotalCount     int64                   `json:"total_count"`
	Page           int                     `json:"page"`
	PageSize       int                     `json:"page_size"`
}

// QuestionnaireSummaryResponse 问卷摘要响应（不包含问题详情）
type QuestionnaireSummaryResponse struct {
	Code        string `json:"code"`
	Title       string `json:"title"`
	Description string `json:"description"`
	ImgUrl      string `json:"img_url"`
	Version     string `json:"version"`
	Status      string `json:"status"`
	Type        string `json:"type"`
}

// QuestionnaireSummaryListResponse 问卷摘要列表响应
type QuestionnaireSummaryListResponse struct {
	Questionnaires []QuestionnaireSummaryResponse `json:"questionnaires"`
	TotalCount     int64                          `json:"total_count"`
}

// NewQuestionnaireResponseFromResult 从应用层 Result 创建问卷响应
func NewQuestionnaireResponseFromResult(result *questionnaire.QuestionnaireResult) *QuestionnaireResponse {
	if result == nil {
		return nil
	}

	questions := make([]viewmodel.QuestionDTO, 0, len(result.Questions))
	for _, q := range result.Questions {
		options := make([]viewmodel.OptionDTO, 0, len(q.Options))
		for _, opt := range q.Options {
			options = append(options, viewmodel.OptionDTO{
				Code:    opt.Value,
				Content: opt.Label,
				Score:   float64(opt.Score),
			})
		}

		// 转换 show_controller
		var showController *viewmodel.ShowControllerDTO
		if q.ShowController != nil {
			conditions := make([]viewmodel.ShowControllerConditionDTO, 0, len(q.ShowController.GetQuestions()))
			for _, cond := range q.ShowController.GetQuestions() {
				optionCodes := make([]string, 0, len(cond.SelectOptionCodes))
				for _, code := range cond.SelectOptionCodes {
					optionCodes = append(optionCodes, code.Value())
				}
				conditions = append(conditions, viewmodel.ShowControllerConditionDTO{
					Code:             cond.Code.Value(),
					SelectOptionCodes: optionCodes,
				})
			}
			showController = &viewmodel.ShowControllerDTO{
				Rule:     q.ShowController.GetRule(),
				Questions: conditions,
			}
		}

		questions = append(questions, viewmodel.QuestionDTO{
			Code:           q.Code,
			Stem:           q.Stem,
			Type:           q.Type,
			Tips:           q.Description,
			Options:        options,
			ShowController: showController,
		})
	}

	return &QuestionnaireResponse{
		Code:        result.Code,
		Title:       result.Title,
		Description: result.Description,
		ImgUrl:      result.ImgUrl,
		Version:     result.Version,
		Status:      result.Status,
		Type:        result.Type,
		Questions:   questions,
	}
}

// NewQuestionnaireListResponseFromResult 从应用层 ListResult 创建列表响应
func NewQuestionnaireListResponseFromResult(result *questionnaire.QuestionnaireListResult) *QuestionnaireListResponse {
	if result == nil {
		return &QuestionnaireListResponse{
			Questionnaires: []QuestionnaireResponse{},
			TotalCount:     0,
		}
	}

	questionnaires := make([]QuestionnaireResponse, 0, len(result.Items))
	for _, item := range result.Items {
		if resp := NewQuestionnaireResponseFromResult(item); resp != nil {
			questionnaires = append(questionnaires, *resp)
		}
	}

	return &QuestionnaireListResponse{
		Questionnaires: questionnaires,
		TotalCount:     result.Total,
	}
}

// NewQuestionnaireSummaryListResponse 从应用层 SummaryListResult 创建摘要列表响应
func NewQuestionnaireSummaryListResponse(result *questionnaire.QuestionnaireSummaryListResult) *QuestionnaireSummaryListResponse {
	if result == nil {
		return &QuestionnaireSummaryListResponse{
			Questionnaires: []QuestionnaireSummaryResponse{},
			TotalCount:     0,
		}
	}

	questionnaires := make([]QuestionnaireSummaryResponse, 0, len(result.Items))
	for _, item := range result.Items {
		questionnaires = append(questionnaires, QuestionnaireSummaryResponse{
			Code:        item.Code,
			Title:       item.Title,
			Description: item.Description,
			ImgUrl:      item.ImgUrl,
			Version:     item.Version,
			Status:      item.Status,
			Type:        item.Type,
		})
	}

	return &QuestionnaireSummaryListResponse{
		Questionnaires: questionnaires,
		TotalCount:     result.Total,
	}
}
