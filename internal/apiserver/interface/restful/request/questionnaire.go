package request

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/interface/restful/viewmodel"
)

// CreateQuestionnaireRequest 创建问卷请求
type CreateQuestionnaireRequest struct {
	Title       string `json:"title" valid:"required~标题不能为空"`
	Description string `json:"description"`
	ImgUrl      string `json:"img_url"`
}

// UpdateQuestionnaireBasicInfoRequest 更新问卷基本信息请求
type UpdateQuestionnaireBasicInfoRequest struct {
	Title       string `json:"title" valid:"required~标题不能为空"`
	Description string `json:"description"`
	ImgUrl      string `json:"img_url"`
}

// AddQuestionRequest 添加问题请求
type AddQuestionRequest struct {
	Code        string                `json:"code" valid:"required"`
	Stem        string                `json:"stem" valid:"required"`
	Type        string                `json:"type" valid:"required"`
	Options     []viewmodel.OptionDTO `json:"options"`
	Required    bool                  `json:"required"`
	Description string                `json:"description"`
}

// UpdateQuestionRequest 更新问题请求
type UpdateQuestionRequest struct {
	Code        string                `json:"code" valid:"required"`
	Stem        string                `json:"stem" valid:"required"`
	Type        string                `json:"type" valid:"required"`
	Options     []viewmodel.OptionDTO `json:"options"`
	Required    bool                  `json:"required"`
	Description string                `json:"description"`
}

// ReorderQuestionsRequest 重排问题请求
type ReorderQuestionsRequest struct {
	QuestionCodes []string `json:"question_codes" valid:"required"`
}

// BatchUpdateQuestionsRequest 批量更新问题请求
type BatchUpdateQuestionsRequest struct {
	Questions []viewmodel.QuestionDTO `json:"questions" valid:"required"`
}

// EditQuestionnaireBasicInfoRequest 编辑问卷基本信息请求（旧）
type EditQuestionnaireBasicInfoRequest struct {
	Title       string `json:"title" valid:"required~标题不能为空"`
	Description string `json:"description"`
	ImgUrl      string `json:"img_url"`
}

// EditQuestionnaireQuestionsRequest 编辑问卷问题请求（旧）
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
