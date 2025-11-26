package response

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/application/dto"
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
	Questions   []viewmodel.QuestionDTO `json:"questions,omitempty"`
}

// QuestionnaireListResponse 问卷列表响应
type QuestionnaireListResponse struct {
	Questionnaires []QuestionnaireResponse `json:"questionnaires"`
	TotalCount     int64                   `json:"total_count"`
	Page           int                     `json:"page"`
	PageSize       int                     `json:"page_size"`
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

		questions = append(questions, viewmodel.QuestionDTO{
			Code:    q.Code,
			Stem:    q.Stem,
			Type:    q.Type,
			Tips:    q.Description,
			Options: options,
		})
	}

	return &QuestionnaireResponse{
		Code:        result.Code,
		Title:       result.Title,
		Description: result.Description,
		ImgUrl:      result.ImgUrl,
		Version:     result.Version,
		Status:      result.Status,
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

// NewQuestionnaireResponse 创建问卷响应（兼容旧版本）
// 注意：Questions 需要在 handler 层单独映射，避免循环依赖
func NewQuestionnaireResponse(dto *dto.QuestionnaireDTO, questions []viewmodel.QuestionDTO) *QuestionnaireResponse {
	if dto == nil {
		return nil
	}

	response := &QuestionnaireResponse{
		Code:        dto.Code,
		Title:       dto.Title,
		Description: dto.Description,
		ImgUrl:      dto.ImgUrl,
		Version:     dto.Version,
		Status:      dto.Status,
		Questions:   questions,
	}

	return response
}

// NewQuestionnaireListResponse 创建问卷列表响应（兼容旧版本）
// 注意：Questions 需要在 handler 层单独映射，避免循环依赖
func NewQuestionnaireListResponse(dtos []*dto.QuestionnaireDTO, total int64, page, pageSize int) *QuestionnaireListResponse {
	if dtos == nil {
		return nil
	}

	questionnaires := make([]QuestionnaireResponse, len(dtos))
	for i, dto := range dtos {
		// 列表视图不需要详细的 Questions，传入 nil
		questionnaires[i] = *NewQuestionnaireResponse(dto, nil)
	}

	return &QuestionnaireListResponse{
		Questionnaires: questionnaires,
		TotalCount:     total,
		Page:           page,
		PageSize:       pageSize,
	}
}
