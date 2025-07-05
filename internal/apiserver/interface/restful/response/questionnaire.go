package response

import (
	"github.com/yshujie/questionnaire-scale/internal/apiserver/application/dto"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/interface/restful/mapper"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/interface/restful/viewmodel"
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

// NewQuestionnaireResponse 创建问卷响应
func NewQuestionnaireResponse(dto *dto.QuestionnaireDTO) *QuestionnaireResponse {
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
		Questions:   mapper.NewQuestionMapper().ToViewModels(dto.Questions),
	}

	return response
}

// NewQuestionnaireListResponse 创建问卷列表响应
func NewQuestionnaireListResponse(dtos []*dto.QuestionnaireDTO, total int64, page, pageSize int) *QuestionnaireListResponse {
	if dtos == nil {
		return nil
	}

	questionnaires := make([]QuestionnaireResponse, len(dtos))
	for i, dto := range dtos {
		questionnaires[i] = *NewQuestionnaireResponse(dto)
	}

	return &QuestionnaireListResponse{
		Questionnaires: questionnaires,
		TotalCount:     total,
		Page:           page,
		PageSize:       pageSize,
	}
}
