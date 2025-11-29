package request

// ============= Assessment 相关请求 =============

// CreateAssessmentRequest 创建测评请求
type CreateAssessmentRequest struct {
	TesteeID             uint64  `json:"testee_id" valid:"required"`             // 受试者ID
	QuestionnaireID      uint64  `json:"questionnaire_id" valid:"required"`      // 问卷ID
	QuestionnaireCode    string  `json:"questionnaire_code" valid:"required"`    // 问卷编码
	QuestionnaireVersion string  `json:"questionnaire_version" valid:"required"` // 问卷版本
	AnswerSheetID        uint64  `json:"answer_sheet_id" valid:"required"`       // 答卷ID
	MedicalScaleID       *uint64 `json:"medical_scale_id"`                       // 量表ID（可选）
	MedicalScaleCode     *string `json:"medical_scale_code"`                     // 量表编码（可选）
	MedicalScaleName     *string `json:"medical_scale_name"`                     // 量表名称（可选）
	OriginType           string  `json:"origin_type" valid:"required"`           // 来源类型：adhoc/plan/screening
	OriginID             *string `json:"origin_id"`                              // 来源ID（可选）
}

// SubmitAssessmentRequest 提交测评请求
type SubmitAssessmentRequest struct {
	AssessmentID uint64 `json:"assessment_id" valid:"required"` // 测评ID
}

// ListAssessmentsRequest 查询测评列表请求
type ListAssessmentsRequest struct {
	Page     int    `form:"page" json:"page"`           // 页码
	PageSize int    `form:"page_size" json:"page_size"` // 每页数量
	Status   string `form:"status" json:"status"`       // 状态筛选
	TesteeID uint64 `form:"testee_id" json:"testee_id"` // 受试者ID筛选
}

// GetStatisticsRequest 获取统计数据请求
type GetStatisticsRequest struct {
	StartTime *string `form:"start_time" json:"start_time"` // 开始时间（可选，格式：2006-01-02）
	EndTime   *string `form:"end_time" json:"end_time"`     // 结束时间（可选，格式：2006-01-02）
	ScaleCode *string `form:"scale_code" json:"scale_code"` // 量表编码筛选（可选）
}

// ============= Score 相关请求 =============

// GetFactorTrendRequest 获取因子趋势请求
type GetFactorTrendRequest struct {
	TesteeID   uint64 `form:"testee_id" json:"testee_id" valid:"required"`     // 受试者ID
	FactorCode string `form:"factor_code" json:"factor_code" valid:"required"` // 因子编码
	Limit      int    `form:"limit" json:"limit"`                              // 返回记录数限制
}

// ============= Report 相关请求 =============

// ListReportsRequest 查询报告列表请求
type ListReportsRequest struct {
	TesteeID uint64 `form:"testee_id" json:"testee_id"` // 受试者ID
	Page     int    `form:"page" json:"page"`           // 页码
	PageSize int    `form:"page_size" json:"page_size"` // 每页数量
}

// ExportReportRequest 导出报告请求
type ExportReportRequest struct {
	Format string `form:"format" json:"format"` // 导出格式：pdf/html
}

// ============= Evaluation 相关请求 =============

// BatchEvaluateRequest 批量评估请求
type BatchEvaluateRequest struct {
	AssessmentIDs []uint64 `json:"assessment_ids" valid:"required"` // 测评ID列表
}
