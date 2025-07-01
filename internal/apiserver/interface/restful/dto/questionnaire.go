package dto

// QuestionnaireCreateRequest 创建问卷请求
type QuestionnaireCreateRequest struct {
	Title       string `json:"title" valid:"required"`
	Description string `json:"description" valid:"required"`
	ImgUrl      string `json:"img_url"`
}

// QuestionnaireIDRequest 问卷ID请求
type QuestionnaireIDRequest struct {
	ID uint64 `json:"id" valid:"required"`
}

// QuestionnaireResponse 问卷响应
type QuestionnaireResponse struct {
	ID          uint64 `json:"id"`
	Code        string `json:"code"`
	Title       string `json:"title"`
	Description string `json:"description"`
	ImgUrl      string `json:"img_url"`
	Version     uint8  `json:"version"`
	Status      uint8  `json:"status"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

// QuestionnaireListResponse 问卷列表响应
type QuestionnaireListResponse struct {
	Questionnaires []*QuestionnaireResponse `json:"questionnaires"`
	TotalCount     int64                    `json:"total_count"`
	Page           int                      `json:"page"`
	PageSize       int                      `json:"page_size"`
}

// QuestionnaireEditRequest 编辑问卷请求
type QuestionnaireEditRequest struct {
	ID      uint64 `json:"id" valid:"required"`
	Title   string `json:"title" valid:"required"`
	ImgUrl  string `json:"img_url" valid:"required"`
	Version uint8  `json:"version" valid:"required"`
}

// QuestionnairePublishRequest 发布问卷请求
type QuestionnairePublishRequest struct {
	ID uint64 `json:"id" valid:"required"`
}

// QuestionnaireUnpublishRequest 下架问卷请求
type QuestionnaireUnpublishRequest struct {
	ID uint64 `json:"id" valid:"required"`
}
