package response

import (
	"fmt"
	"time"

	assessment "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/engine"
)

// ============= Assessment 相关响应 =============

// AssessmentResponse 测评响应
type AssessmentResponse struct {
	ID                   string   `json:"id"`                     // 测评ID
	OrgID                string   `json:"org_id"`                 // 组织ID
	TesteeID             string   `json:"testee_id"`              // 受试者ID
	QuestionnaireCode    string   `json:"questionnaire_code"`     // 问卷编码（唯一标识）
	QuestionnaireVersion string   `json:"questionnaire_version"`  // 问卷版本
	AnswerSheetID        string   `json:"answer_sheet_id"`        // 答卷ID
	MedicalScaleID       *string  `json:"medical_scale_id"`       // 量表ID
	MedicalScaleCode     *string  `json:"medical_scale_code"`     // 量表编码
	MedicalScaleName     *string  `json:"medical_scale_name"`     // 量表名称
	OriginType           string   `json:"origin_type"`            // 来源类型
	OriginID             *string  `json:"origin_id"`              // 来源ID
	Status               string   `json:"status"`                 // 状态
	TotalScore           *float64 `json:"total_score,omitempty"`  // 总分
	RiskLevel            *string  `json:"risk_level,omitempty"`   // 风险等级
	SubmittedAt          *string  `json:"submitted_at,omitempty"` // 提交时间
	InterpretedAt        *string  `json:"interpreted_at,omitempty"`
	FailedAt             *string  `json:"failed_at,omitempty"`
	FailureReason        *string  `json:"failure_reason,omitempty"`
}

// AssessmentListResponse 测评列表响应
type AssessmentListResponse struct {
	Items      []*AssessmentResponse `json:"items"`       // 测评列表
	Total      int                   `json:"total"`       // 总数
	Page       int                   `json:"page"`        // 当前页
	PageSize   int                   `json:"page_size"`   // 每页数量
	TotalPages int                   `json:"total_pages"` // 总页数
}

// AssessmentStatisticsResponse 测评统计响应
type AssessmentStatisticsResponse struct {
	TotalCount       int                  `json:"total_count"`       // 总测评数
	PendingCount     int                  `json:"pending_count"`     // 待提交数
	SubmittedCount   int                  `json:"submitted_count"`   // 已提交数
	InterpretedCount int                  `json:"interpreted_count"` // 已解读数
	FailedCount      int                  `json:"failed_count"`      // 失败数
	AverageScore     *float64             `json:"average_score"`     // 平均分
	RiskDistribution map[string]int       `json:"risk_distribution"` // 风险等级分布
	ScaleStats       []*ScaleStatResponse `json:"scale_stats"`       // 按量表统计
}

// ScaleStatResponse 量表统计响应
type ScaleStatResponse struct {
	ScaleCode    string   `json:"scale_code"`    // 量表编码
	ScaleName    string   `json:"scale_name"`    // 量表名称
	Count        int      `json:"count"`         // 测评数
	AverageScore *float64 `json:"average_score"` // 平均分
}

// BatchEvaluationResponse 批量评估响应
type BatchEvaluationResponse struct {
	TotalCount   int      `json:"total_count"`   // 总数
	SuccessCount int      `json:"success_count"` // 成功数
	FailedCount  int      `json:"failed_count"`  // 失败数
	FailedIDs    []string `json:"failed_ids"`    // 失败的测评ID列表
}

// ============= Score 相关响应 =============

// ScoreResponse 得分响应
type ScoreResponse struct {
	AssessmentID string             `json:"assessment_id"` // 测评ID
	TotalScore   float64            `json:"total_score"`   // 总分
	RiskLevel    string             `json:"risk_level"`    // 整体风险等级
	FactorScores []*FactorScoreItem `json:"factor_scores"` // 因子得分列表
}

// FactorScoreItem 因子得分项
type FactorScoreItem struct {
	FactorCode   string   `json:"factor_code"`    // 因子编码
	FactorName   string   `json:"factor_name"`    // 因子名称
	RawScore     float64  `json:"raw_score"`      // 原始分
	MaxScore     *float64 `json:"max_score,omitempty"` // 最大分
	RiskLevel    string   `json:"risk_level"`     // 风险等级
	Conclusion   string   `json:"conclusion"`     // 结论
	Suggestion   string   `json:"suggestion"`     // 建议
	IsTotalScore bool     `json:"is_total_score"` // 是否为总分因子
}

// FactorTrendResponse 因子趋势响应
type FactorTrendResponse struct {
	TesteeID   string            `json:"testee_id"`   // 受试者ID
	FactorCode string            `json:"factor_code"` // 因子编码
	FactorName string            `json:"factor_name"` // 因子名称
	DataPoints []*TrendDataPoint `json:"data_points"` // 数据点列表
}

// TrendDataPoint 趋势数据点
type TrendDataPoint struct {
	AssessmentID string  `json:"assessment_id"` // 测评ID
	RawScore     float64 `json:"raw_score"`     // 得分
	RiskLevel    string  `json:"risk_level"`    // 风险等级
}

// HighRiskFactorsResponse 高风险因子响应
type HighRiskFactorsResponse struct {
	AssessmentID    string             `json:"assessment_id"`     // 测评ID
	HasHighRisk     bool               `json:"has_high_risk"`     // 是否存在高风险
	HighRiskFactors []*FactorScoreItem `json:"high_risk_factors"` // 高风险因子列表
	NeedsUrgentCare bool               `json:"needs_urgent_care"` // 是否需要紧急关注
}

// ============= Report 相关响应 =============

// ReportResponse 报告响应
type ReportResponse struct {
	AssessmentID string           `json:"assessment_id"` // 测评ID
	ScaleName    string           `json:"scale_name"`    // 量表名称
	ScaleCode    string           `json:"scale_code"`    // 量表编码
	TotalScore   float64          `json:"total_score"`   // 总分
	RiskLevel    string           `json:"risk_level"`    // 风险等级
	Conclusion   string           `json:"conclusion"`    // 总结论
	Dimensions   []*DimensionItem `json:"dimensions"`    // 维度解读列表
	Suggestions  []string         `json:"suggestions"`   // 建议列表
	CreatedAt    string           `json:"created_at"`    // 创建时间
}

// DimensionItem 维度解读项
type DimensionItem struct {
	FactorCode  string   `json:"factor_code"`         // 因子编码
	FactorName  string   `json:"factor_name"`         // 因子名称
	RawScore    float64  `json:"raw_score"`           // 原始分
	MaxScore    *float64 `json:"max_score,omitempty"` // 最大分
	RiskLevel   string   `json:"risk_level"`          // 风险等级
	Description string   `json:"description"`         // 解读描述
}

// ReportListResponse 报告列表响应
type ReportListResponse struct {
	Items      []*ReportResponse `json:"items"`       // 报告列表
	Total      int               `json:"total"`       // 总数
	Page       int               `json:"page"`        // 当前页
	PageSize   int               `json:"page_size"`   // 每页数量
	TotalPages int               `json:"total_pages"` // 总页数
}

// ReportExportResponse 报告导出响应
type ReportExportResponse struct {
	FileName    string `json:"file_name"`    // 文件名
	ContentType string `json:"content_type"` // 内容类型
	DownloadURL string `json:"download_url"` // 下载地址
}

// ============= 转换函数 =============

// NewAssessmentResponse 从应用层 Result 创建响应
func NewAssessmentResponse(result *assessment.AssessmentResult) *AssessmentResponse {
	if result == nil {
		return nil
	}

	// 转换 ID 字段为字符串
	idStr := fmt.Sprintf("%d", result.ID)
	orgIDStr := fmt.Sprintf("%d", result.OrgID)
	testeeIDStr := fmt.Sprintf("%d", result.TesteeID)
	answerSheetIDStr := fmt.Sprintf("%d", result.AnswerSheetID)

	var medicalScaleIDStr *string
	if result.MedicalScaleID != nil {
		s := fmt.Sprintf("%d", *result.MedicalScaleID)
		medicalScaleIDStr = &s
	}

	resp := &AssessmentResponse{
		ID:                   idStr,
		OrgID:                orgIDStr,
		TesteeID:             testeeIDStr,
		QuestionnaireCode:    result.QuestionnaireCode,
		QuestionnaireVersion: result.QuestionnaireVersion,
		AnswerSheetID:        answerSheetIDStr,
		MedicalScaleID:       medicalScaleIDStr,
		MedicalScaleCode:     result.MedicalScaleCode,
		MedicalScaleName:     result.MedicalScaleName,
		OriginType:           result.OriginType,
		OriginID:             result.OriginID,
		Status:               result.Status,
		TotalScore:           result.TotalScore,
		RiskLevel:            result.RiskLevel,
	}

	// 格式化时间
	if result.SubmittedAt != nil {
		t := result.SubmittedAt.Format(time.RFC3339)
		resp.SubmittedAt = &t
	}
	if result.InterpretedAt != nil {
		t := result.InterpretedAt.Format(time.RFC3339)
		resp.InterpretedAt = &t
	}
	if result.FailedAt != nil {
		t := result.FailedAt.Format(time.RFC3339)
		resp.FailedAt = &t
	}
	resp.FailureReason = result.FailureReason

	return resp
}

// NewAssessmentListResponse 从应用层 Result 创建列表响应
func NewAssessmentListResponse(result *assessment.AssessmentListResult) *AssessmentListResponse {
	if result == nil {
		return nil
	}

	items := make([]*AssessmentResponse, 0, len(result.Items))
	for _, item := range result.Items {
		items = append(items, NewAssessmentResponse(item))
	}

	return &AssessmentListResponse{
		Items:      items,
		Total:      result.Total,
		Page:       result.Page,
		PageSize:   result.PageSize,
		TotalPages: result.TotalPages,
	}
}

// NewAssessmentStatisticsResponse 从应用层 Result 创建统计响应
func NewAssessmentStatisticsResponse(result *assessment.AssessmentStatistics) *AssessmentStatisticsResponse {
	if result == nil {
		return nil
	}

	scaleStats := make([]*ScaleStatResponse, 0, len(result.ScaleStats))
	for _, s := range result.ScaleStats {
		scaleStats = append(scaleStats, &ScaleStatResponse{
			ScaleCode:    s.ScaleCode,
			ScaleName:    s.ScaleName,
			Count:        s.Count,
			AverageScore: s.AverageScore,
		})
	}

	return &AssessmentStatisticsResponse{
		TotalCount:       result.TotalCount,
		PendingCount:     result.PendingCount,
		SubmittedCount:   result.SubmittedCount,
		InterpretedCount: result.InterpretedCount,
		FailedCount:      result.FailedCount,
		AverageScore:     result.AverageScore,
		RiskDistribution: result.RiskDistribution,
		ScaleStats:       scaleStats,
	}
}

// NewBatchEvaluationResponse 从应用层 Result 创建批量评估响应
func NewBatchEvaluationResponse(result *engine.BatchResult) *BatchEvaluationResponse {
	if result == nil {
		return nil
	}

	// 转换失败的 ID 列表为字符串
	failedIDs := make([]string, 0, len(result.FailedIDs))
	for _, id := range result.FailedIDs {
		failedIDs = append(failedIDs, fmt.Sprintf("%d", id))
	}

	return &BatchEvaluationResponse{
		TotalCount:   result.TotalCount,
		SuccessCount: result.SuccessCount,
		FailedCount:  result.FailedCount,
		FailedIDs:    failedIDs,
	}
}

// NewScoreResponse 从应用层 Result 创建得分响应
func NewScoreResponse(result *assessment.ScoreResult) *ScoreResponse {
	if result == nil {
		return nil
	}

	factorScores := make([]*FactorScoreItem, 0, len(result.FactorScores))
	for _, f := range result.FactorScores {
		factorScores = append(factorScores, &FactorScoreItem{
			FactorCode:   f.FactorCode,
			FactorName:   f.FactorName,
			RawScore:     f.RawScore,
			MaxScore:     f.MaxScore,
			RiskLevel:    f.RiskLevel,
			Conclusion:   f.Conclusion,
			Suggestion:   f.Suggestion,
			IsTotalScore: f.IsTotalScore,
		})
	}

	return &ScoreResponse{
		AssessmentID: fmt.Sprintf("%d", result.AssessmentID),
		TotalScore:   result.TotalScore,
		RiskLevel:    result.RiskLevel,
		FactorScores: factorScores,
	}
}

// NewFactorTrendResponse 从应用层 Result 创建因子趋势响应
func NewFactorTrendResponse(result *assessment.FactorTrendResult) *FactorTrendResponse {
	if result == nil {
		return nil
	}

	dataPoints := make([]*TrendDataPoint, 0, len(result.DataPoints))
	for _, dp := range result.DataPoints {
		dataPoints = append(dataPoints, &TrendDataPoint{
			AssessmentID: fmt.Sprintf("%d", dp.AssessmentID),
			RawScore:     dp.RawScore,
			RiskLevel:    dp.RiskLevel,
		})
	}

	return &FactorTrendResponse{
		TesteeID:   fmt.Sprintf("%d", result.TesteeID),
		FactorCode: result.FactorCode,
		FactorName: result.FactorName,
		DataPoints: dataPoints,
	}
}

// NewHighRiskFactorsResponse 从应用层 Result 创建高风险因子响应
func NewHighRiskFactorsResponse(result *assessment.HighRiskFactorsResult) *HighRiskFactorsResponse {
	if result == nil {
		return nil
	}

	factors := make([]*FactorScoreItem, 0, len(result.HighRiskFactors))
	for _, f := range result.HighRiskFactors {
		factors = append(factors, &FactorScoreItem{
			FactorCode:   f.FactorCode,
			FactorName:   f.FactorName,
			RawScore:     f.RawScore,
			RiskLevel:    f.RiskLevel,
			Conclusion:   f.Conclusion,
			Suggestion:   f.Suggestion,
			IsTotalScore: f.IsTotalScore,
		})
	}

	return &HighRiskFactorsResponse{
		AssessmentID:    fmt.Sprintf("%d", result.AssessmentID),
		HasHighRisk:     result.HasHighRisk,
		HighRiskFactors: factors,
		NeedsUrgentCare: result.NeedsUrgentCare,
	}
}

// NewReportResponse 从应用层 Result 创建报告响应
func NewReportResponse(result *assessment.ReportResult) *ReportResponse {
	if result == nil {
		return nil
	}

	dimensions := make([]*DimensionItem, 0, len(result.Dimensions))
	for _, d := range result.Dimensions {
		dimensions = append(dimensions, &DimensionItem{
			FactorCode:  d.FactorCode,
			FactorName:  d.FactorName,
			RawScore:    d.RawScore,
			MaxScore:    d.MaxScore,
			RiskLevel:   d.RiskLevel,
			Description: d.Description,
		})
	}

	return &ReportResponse{
		AssessmentID: fmt.Sprintf("%d", result.AssessmentID),
		ScaleName:    result.ScaleName,
		ScaleCode:    result.ScaleCode,
		TotalScore:   result.TotalScore,
		RiskLevel:    result.RiskLevel,
		Conclusion:   result.Conclusion,
		Dimensions:   dimensions,
		Suggestions:  result.Suggestions,
		CreatedAt:    result.CreatedAt.Format(time.RFC3339),
	}
}

// NewReportListResponse 从应用层 Result 创建报告列表响应
func NewReportListResponse(result *assessment.ReportListResult) *ReportListResponse {
	if result == nil {
		return nil
	}

	items := make([]*ReportResponse, 0, len(result.Items))
	for _, item := range result.Items {
		items = append(items, NewReportResponse(item))
	}

	return &ReportListResponse{
		Items:      items,
		Total:      result.Total,
		Page:       result.Page,
		PageSize:   result.PageSize,
		TotalPages: result.TotalPages,
	}
}
