package request

import "github.com/yshujie/questionnaire-scale/internal/apiserver/interface/restful/viewmodel"

// QuestionnaireCreateRequest 创建问卷请求
type CreateQuestionnaireRequest struct {
	Title       string `json:"title" valid:"required"`
	Description string `json:"description" valid:"required"`
	ImgUrl      string `json:"img_url"`
}

// EditQuestionnaireBasicInfoRequest 编辑问卷请求
type EditQuestionnaireBasicInfoRequest struct {
	Title       string `json:"title" valid:"required"`
	Description string `json:"description" valid:"required"`
	ImgUrl      string `json:"img_url" valid:"required"`
}

// EditQuestionnaireQuestionsRequest 编辑问卷问题请求
type EditQuestionnaireQuestionsRequest struct {
	Questions []viewmodel.QuestionDTO `json:"questions" valid:"required"`
}

// QueryQuestionnaireRequest 问卷ID请求
type QueryQuestionnaireRequest struct {
	Code string `json:"code" valid:"required"`
}

// QueryQuestionnaireListRequest 问卷列表请求
type QueryQuestionnaireListRequest struct {
	Page       int               `json:"page" valid:"required"`
	PageSize   int               `json:"page_size" valid:"required"`
	Conditions map[string]string `json:"conditions"`
}
