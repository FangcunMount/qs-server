package dto

// QuestionnaireCreateRequest 创建问卷请求
type CreateQuestionnaireRequest struct {
	Title       string `json:"title" valid:"required"`
	Description string `json:"description" valid:"required"`
	ImgUrl      string `json:"img_url"`
}

// QueryQuestionnaireRequest 问卷ID请求
type QueryQuestionnaireRequest struct {
	Code string `json:"code" valid:"required"`
}

// QueryQuestionnaireListRequest 问卷列表请求
type QueryQuestionnaireListRequest struct {
	Page     int `json:"page" valid:"required"`
	PageSize int `json:"page_size" valid:"required"`
}

// EditQuestionnaireBasicInfoRequest 编辑问卷请求
type EditQuestionnaireBasicInfoRequest struct {
	Code        string `json:"code" valid:"required"`
	Title       string `json:"title" valid:"required"`
	Description string `json:"description" valid:"required"`
	ImgUrl      string `json:"img_url" valid:"required"`
}

// QuestionnairePublishRequest 发布问卷请求
type PublishQuestionnaireRequest struct {
	Code string `json:"code" valid:"required"`
}

// QuestionnaireUnpublishRequest 下架问卷请求
type UnpublishQuestionnaireRequest struct {
	Code string `json:"code" valid:"required"`
}

// QuestionnaireResponse 问卷响应
type QuestionnaireBasicInfoResponse struct {
	Code        string `json:"code"`
	Title       string `json:"title"`
	Description string `json:"description"`
	ImgUrl      string `json:"img_url"`
	Version     string `json:"version"`
	Status      uint8  `json:"status"`
}

// QuestionnaireListResponse 问卷列表响应
type QuestionnaireListResponse struct {
	Questionnaires []*QuestionnaireBasicInfoResponse `json:"questionnaires"`
	TotalCount     int64                             `json:"total_count"`
	Page           int                               `json:"page"`
	PageSize       int                               `json:"page_size"`
}
