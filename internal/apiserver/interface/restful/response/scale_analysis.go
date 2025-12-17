package response

import "time"

// ScaleAnalysisResponse 量表趋势分析响应
type ScaleAnalysisResponse struct {
	Scales []ScaleTrendResponse `json:"scales"` // 量表趋势列表
}

// ScaleTrendResponse 量表趋势响应
type ScaleTrendResponse struct {
	ScaleID   string              `json:"scale_id"`   // 量表ID
	ScaleCode string              `json:"scale_code"` // 量表编码
	ScaleName string              `json:"scale_name"` // 量表名称
	Tests     []ScaleTestResponse `json:"tests"`      // 测评历史记录（按时间升序排列）
}

// ScaleTestResponse 量表测评记录响应
type ScaleTestResponse struct {
	AssessmentID string                `json:"assessment_id"` // 测评ID
	TestDate     time.Time             `json:"test_date"`     // 测评日期
	TotalScore   float64               `json:"total_score"`   // 总分
	RiskLevel    string                `json:"risk_level"`    // 风险等级：normal/medium/high
	Result       string                `json:"result"`        // 结果描述，如："轻度焦虑"
	Factors      []ScaleFactorResponse `json:"factors"`       // 各因子得分
}

// ScaleFactorResponse 因子得分响应
type ScaleFactorResponse struct {
	FactorCode string   `json:"factor_code"`          // 因子编码
	FactorName string   `json:"factor_name"`          // 因子名称
	RawScore   float64  `json:"raw_score"`            // 原始分
	TScore     *float64 `json:"t_score,omitempty"`    // T分
	Percentile *float64 `json:"percentile,omitempty"` // 百分位
	RiskLevel  string   `json:"risk_level,omitempty"` // 风险等级：normal/medium/high
}
