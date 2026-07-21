package evaluation

// ModelIdentityResponse 已发布模型引用（测评/报告层）。
type ModelIdentityResponse struct {
	// 测评层 Kind；类型学规范值为 typology。
	Kind            string `json:"kind" example:"typology" enums:"scale,typology,behavioral_rating,cognitive"`
	SubKind         string `json:"sub_kind,omitempty" example:"typology"`
	Algorithm       string `json:"algorithm,omitempty"`
	Code            string `json:"code"`
	Version         string `json:"version,omitempty"`
	Title           string `json:"title,omitempty"`
	ProductChannel  string `json:"product_channel,omitempty"`
	AlgorithmFamily string `json:"algorithm_family,omitempty"`
	DecisionKind    string `json:"decision_kind,omitempty"`
}

// ScoreValueResponse 主分投影。
type ScoreValueResponse struct {
	Kind  string   `json:"kind"`
	Value float64  `json:"value"`
	Label string   `json:"label,omitempty"`
	Max   *float64 `json:"max,omitempty"`
}

// ResultLevelResponse outcome 等级投影。
type ResultLevelResponse struct {
	Code     string `json:"code"`
	Label    string `json:"label"`
	Severity string `json:"severity,omitempty"`
}

// AssessmentSummaryResponse 测评摘要响应（含 model/primary_score/level）。
type AssessmentSummaryResponse struct {
	ID                   string `json:"id"`
	QuestionnaireCode    string `json:"questionnaire_code"`
	QuestionnaireVersion string `json:"questionnaire_version"`
	// 关联答卷 ID；仅用于测评摘要展示与历史追溯。新提交通过 assessment-readiness 取得 assessment_id。
	AnswerSheetID string                `json:"answer_sheet_id,omitempty"`
	Model         ModelIdentityResponse `json:"model"`
	PrimaryScore  *ScoreValueResponse   `json:"primary_score,omitempty"`
	Level         *ResultLevelResponse  `json:"level,omitempty"`
	OriginType    string                `json:"origin_type"`
	Status        string                `json:"status"`
	CreatedAt     string                `json:"created_at"`
	SubmittedAt   string                `json:"submitted_at,omitempty"`
	InterpretedAt string                `json:"interpreted_at,omitempty"`
}

// AssessmentDetailResponse 测评详情响应（含 model/primary_score/level）。
type AssessmentDetailResponse struct {
	ID                   string                `json:"id"`
	OrgID                string                `json:"org_id"`
	TesteeID             string                `json:"testee_id"`
	QuestionnaireCode    string                `json:"questionnaire_code"`
	QuestionnaireVersion string                `json:"questionnaire_version"`
	AnswerSheetID        string                `json:"answer_sheet_id,omitempty"`
	Model                ModelIdentityResponse `json:"model"`
	PrimaryScore         *ScoreValueResponse   `json:"primary_score,omitempty"`
	Level                *ResultLevelResponse  `json:"level,omitempty"`
	OriginType           string                `json:"origin_type"`
	OriginID             string                `json:"origin_id,omitempty"`
	Status               string                `json:"status"`
	CreatedAt            string                `json:"created_at"`
	SubmittedAt          string                `json:"submitted_at,omitempty"`
	InterpretedAt        string                `json:"interpreted_at,omitempty"`
	FailedAt             string                `json:"failed_at,omitempty"`
	FailureReason        string                `json:"failure_reason,omitempty"`
}

// AssessmentReportResponse 测评报告响应（含 model/primary_score/level）。
type AssessmentReportResponse struct {
	AssessmentID string                       `json:"assessment_id"`
	Model        ModelIdentityResponse        `json:"model"`
	PrimaryScore *ScoreValueResponse          `json:"primary_score,omitempty"`
	Level        *ResultLevelResponse         `json:"level,omitempty"`
	Conclusion   string                       `json:"conclusion"`
	Dimensions   []DimensionInterpretResponse `json:"dimensions"`
	Suggestions  []SuggestionResponse         `json:"suggestions"`
	ModelExtra   *ModelExtraResponse          `json:"model_extra,omitempty"`
	CreatedAt    string                       `json:"created_at"`
}

// ModelExtraResponse 人格等模型的报告扩展。
type ModelExtraResponse struct {
	Kind           string               `json:"kind,omitempty"`
	TypeCode       string               `json:"type_code,omitempty"`
	TypeName       string               `json:"type_name,omitempty"`
	OneLiner       string               `json:"one_liner,omitempty"`
	ImageURL       string               `json:"image_url,omitempty"`
	MatchPercent   float64              `json:"match_percent,omitempty"`
	IsSpecial      bool                 `json:"is_special,omitempty"`
	SpecialTrigger string               `json:"special_trigger,omitempty"`
	Commentary     string               `json:"commentary,omitempty"`
	Rarity         *ModelRarityResponse `json:"rarity,omitempty"`
}

// ModelRarityResponse 理论稀有度投影。
type ModelRarityResponse struct {
	Percent float64 `json:"percent,omitempty"`
	Label   string  `json:"label,omitempty"`
	OneInX  int32   `json:"one_in_x,omitempty"`
}

// ListAssessmentsResponse 测评列表响应。
type ListAssessmentsResponse struct {
	Items      []AssessmentSummaryResponse `json:"items"`
	Total      int32                       `json:"total"`
	Page       int32                       `json:"page"`
	PageSize   int32                       `json:"page_size"`
	TotalPages int32                       `json:"total_pages"`
}

// AssessmentStatusResponse 测评状态响应（用于长轮询）
type AssessmentStatusResponse struct {
	Status          string   `json:"status" enums:"processing,interpreted,failed"` // 对外报告状态：处理中、已生成、失败
	Stage           string   `json:"stage,omitempty"`
	Message         string   `json:"message,omitempty"`
	Reason          string   `json:"reason,omitempty"`
	NextPollAfterMs int      `json:"next_poll_after_ms,omitempty"`
	TotalScore      *float64 `json:"total_score,omitempty"`
	RiskLevel       *string  `json:"risk_level,omitempty"`
	UpdatedAt       int64    `json:"updated_at"` // Unix timestamp
}

// FactorScoreResponse 因子得分响应
type FactorScoreResponse struct {
	FactorCode   string  `json:"factor_code"`
	FactorName   string  `json:"factor_name"`
	RawScore     float64 `json:"raw_score"`
	RiskLevel    string  `json:"risk_level,omitempty"`
	IsTotalScore bool    `json:"is_total_score"`
}

// SuggestionResponse 建议响应
type SuggestionResponse struct {
	Category   string  `json:"category"`
	Content    string  `json:"content"`
	FactorCode *string `json:"factor_code,omitempty"`
}

// DimensionInterpretResponse 维度解读响应
type DimensionInterpretResponse struct {
	FactorCode    string                 `json:"factor_code"`
	FactorName    string                 `json:"factor_name"`
	RawScore      float64                `json:"raw_score"`
	MaxScore      *float64               `json:"max_score,omitempty"`
	RiskLevel     string                 `json:"risk_level"`
	DerivedScores []ScoreValueResponse   `json:"derived_scores,omitempty"`
	Level         *ResultLevelResponse   `json:"level,omitempty"`
	NormReference *NormReferenceResponse `json:"norm_reference,omitempty"`
	Description   string                 `json:"description"`
	Suggestion    string                 `json:"suggestion,omitempty"`
}

// NormReferenceResponse 是生成维度常模分时实际命中的常模表与分组。
// 年龄与性别为空表示命中了不区分人口学信息的通用常模。
type NormReferenceResponse struct {
	ScoreKind    string  `json:"score_kind" example:"t_score"`
	Benchmark    float64 `json:"benchmark" example:"50"`
	TableVersion string  `json:"table_version,omitempty"`
	FormVariant  string  `json:"form_variant,omitempty"`
	MinAgeMonths int32   `json:"min_age_months,omitempty"`
	MaxAgeMonths int32   `json:"max_age_months,omitempty"`
	Gender       string  `json:"gender,omitempty"`
}

// ListAssessmentsRequest 测评列表请求
type ListAssessmentsRequest struct {
	Status         string `form:"status"`
	Page           int32  `form:"page"`
	PageSize       int32  `form:"page_size"`
	ScaleCode      string `form:"scale_code"`
	RiskLevel      string `form:"risk_level"`
	DateFrom       string `form:"date_from"`
	DateTo         string `form:"date_to"`
	AssessmentKind string `form:"assessment_kind"`
}

// TrendPointResponse 趋势数据点响应
type TrendPointResponse struct {
	AssessmentID string  `json:"assessment_id"`
	Score        float64 `json:"score"`
	RiskLevel    string  `json:"risk_level"`
	CreatedAt    string  `json:"created_at"`
}

// AssessmentTrendSnapshotResponse 趋势快照响应
type AssessmentTrendSnapshotResponse struct {
	AssessmentID         string  `json:"assessment_id"`
	ScaleCode            string  `json:"scale_code,omitempty"`
	ScaleName            string  `json:"scale_name,omitempty"`
	QuestionnaireVersion string  `json:"questionnaire_version,omitempty"`
	SubmittedAt          string  `json:"submitted_at,omitempty"`
	TotalScore           float64 `json:"total_score,omitempty"`
	RiskLevel            string  `json:"risk_level,omitempty"`
}

// AssessmentTrendTimelinePointResponse 趋势时间线数据点
type AssessmentTrendTimelinePointResponse struct {
	AssessmentID string  `json:"assessment_id"`
	SubmittedAt  string  `json:"submitted_at,omitempty"`
	TotalScore   float64 `json:"total_score,omitempty"`
	RiskLevel    string  `json:"risk_level,omitempty"`
	FillerLabel  string  `json:"filler_label,omitempty"`
}

// AssessmentFactorChangeResponse 因子变化响应
type AssessmentFactorChangeResponse struct {
	FactorCode    string  `json:"factor_code"`
	FactorName    string  `json:"factor_name"`
	CurrentScore  float64 `json:"current_score"`
	PreviousScore float64 `json:"previous_score"`
	Delta         float64 `json:"delta"`
	RiskLevel     string  `json:"risk_level,omitempty"`
}

// AssessmentFactorTrendPointResponse 因子趋势数据点
type AssessmentFactorTrendPointResponse struct {
	AssessmentID string  `json:"assessment_id"`
	SubmittedAt  string  `json:"submitted_at,omitempty"`
	Score        float64 `json:"score"`
	RiskLevel    string  `json:"risk_level,omitempty"`
}

// AssessmentFactorTrendResponse 因子趋势响应
type AssessmentFactorTrendResponse struct {
	FactorCode string                               `json:"factor_code"`
	FactorName string                               `json:"factor_name"`
	Points     []AssessmentFactorTrendPointResponse `json:"points"`
}

// AssessmentTrendMetaResponse 趋势元信息
type AssessmentTrendMetaResponse struct {
	ComparableCount    int32  `json:"comparable_count"`
	HasMultipleFillers bool   `json:"has_multiple_fillers"`
	DisplayMode        string `json:"display_mode"`
	Note               string `json:"note,omitempty"`
}

// AssessmentTrendSummaryResponse 测评趋势摘要响应
type AssessmentTrendSummaryResponse struct {
	Current       *AssessmentTrendSnapshotResponse       `json:"current,omitempty"`
	Previous      *AssessmentTrendSnapshotResponse       `json:"previous,omitempty"`
	Timeline      []AssessmentTrendTimelinePointResponse `json:"timeline,omitempty"`
	FactorChanges []AssessmentFactorChangeResponse       `json:"factor_changes,omitempty"`
	FactorTrends  []AssessmentFactorTrendResponse        `json:"factor_trends,omitempty"`
	Meta          AssessmentTrendMetaResponse            `json:"meta"`
}

// GetFactorTrendRequest 获取因子趋势请求
type GetFactorTrendRequest struct {
	FactorCode string `form:"factor_code" binding:"required"`
	Limit      int32  `form:"limit"`
}
