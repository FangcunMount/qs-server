package response

import "github.com/yshujie/questionnaire-scale/internal/apiserver/interface/restful/viewmodel"

// QuestionnaireListResponse 问卷列表响应
type QuestionnaireListResponse struct {
	Questionnaires []QuestionnaireBasicInfoResponse `json:"questionnaires"`
	TotalCount     int64                            `json:"total_count"`
	Page           int                              `json:"page"`
	PageSize       int                              `json:"page_size"`
}

// QuestionnaireBasicResponse 问卷基础信息响应
type QuestionnaireBasicInfoResponse struct {
	Code        string `json:"code"`
	Title       string `json:"title"`
	Description string `json:"description"`
	ImgUrl      string `json:"img_url"`
	Version     string `json:"version"`
	Status      uint8  `json:"status"`
}

// QuestionnaireResponse 问卷详细信息详情
type QuestionnaireResponse struct {
	Questionnaire QuestionnaireBasicInfoResponse `json:"questionnaire"`
	Questions     []viewmodel.QuestionDTO        `json:"questions"`
}

// QuestionnaireQuestionsResponse 问卷问题响应
type QuestionnaireQuestionsResponse struct {
	Code      string                  `json:"code"`
	Questions []viewmodel.QuestionDTO `json:"questions"`
}
