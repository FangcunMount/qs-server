package request

// QuestionnaireGetRequest 获取问卷请求
type QuestionnaireGetRequest struct {
	Code string `uri:"code" binding:"required" json:"code"`
}

// QuestionnaireValidateCodeRequest 验证问卷代码请求
type QuestionnaireValidateCodeRequest struct {
	Code string `json:"code" binding:"required,min=3,max=50"`
}

// QuestionnaireListRequest 获取问卷列表请求
type QuestionnaireListRequest struct {
	Page     int    `form:"page,default=1" json:"page"`
	PageSize int    `form:"page_size,default=10" json:"page_size"`
	Keyword  string `form:"keyword" json:"keyword"`
	Status   string `form:"status" json:"status"`
}

// QuestionnaireCreateRequest 创建问卷请求
type QuestionnaireCreateRequest struct {
	Code        string `json:"code" binding:"required,min=3,max=50"`
	Title       string `json:"title" binding:"required,min=1,max=200"`
	Description string `json:"description,omitempty"`
	Category    string `json:"category,omitempty"`
}

// QuestionnaireUpdateRequest 更新问卷请求
type QuestionnaireUpdateRequest struct {
	Code        string `uri:"code" binding:"required" json:"code"`
	Title       string `json:"title" binding:"required,min=1,max=200"`
	Description string `json:"description,omitempty"`
	Category    string `json:"category,omitempty"`
}

// QuestionnairePublishRequest 发布问卷请求
type QuestionnairePublishRequest struct {
	Code string `uri:"code" binding:"required" json:"code"`
}

// QuestionnaireArchiveRequest 归档问卷请求
type QuestionnaireArchiveRequest struct {
	Code string `uri:"code" binding:"required" json:"code"`
}
