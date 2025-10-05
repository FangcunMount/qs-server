package request

import (
	"github.com/fangcun-mount/qs-server/internal/apiserver/interface/restful/viewmodel"
)

// CreateQuestionnaireRequest 创建问卷请求
type CreateQuestionnaireRequest struct {
	Title       string `json:"title" valid:"required~标题不能为空"`
	Description string `json:"description"`
	ImgUrl      string `json:"img_url"`
}

// EditQuestionnaireBasicInfoRequest 编辑问卷基本信息请求
type EditQuestionnaireBasicInfoRequest struct {
	Title       string `json:"title" valid:"required~标题不能为空"`
	Description string `json:"description"`
	ImgUrl      string `json:"img_url"`
}

// EditQuestionnaireQuestionsRequest 编辑问卷问题请求
type EditQuestionnaireQuestionsRequest struct {
	Questions []viewmodel.QuestionDTO `json:"questions" valid:"required~问题列表不能为空"`
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
