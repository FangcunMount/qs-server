package response

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/application/survey/answersheet"
	"github.com/FangcunMount/qs-server/internal/apiserver/interface/restful/viewmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// ============= Response 结构 =============

// AnswerSheetResponse 答卷响应
type AnswerSheetResponse struct {
	ID                meta.ID               `json:"id"`
	QuestionnaireCode string                `json:"questionnaire_code"`
	QuestionnaireVer  string                `json:"questionnaire_ver"`
	Title             string                `json:"title"`
	Score             float64               `json:"score"`
	FillerID          meta.ID               `json:"filler_id"`
	FillerName        string                `json:"filler_name"`
	Answers           []viewmodel.AnswerDTO `json:"answers"`
	FilledAt          string                `json:"filled_at"`
}

// AnswerSheetListResponse 答卷列表响应
type AnswerSheetListResponse struct {
	Total int64                    `json:"total"`
	Items []AnswerSheetSummaryItem `json:"items"`
}

// AnswerSheetSummaryItem 答卷摘要项
type AnswerSheetSummaryItem struct {
	ID                meta.ID `json:"id"`
	QuestionnaireCode string  `json:"questionnaire_code"`
	QuestionnaireVer  string  `json:"questionnaire_ver"`
	Title             string  `json:"title"`
	Score             float64 `json:"score"`
	FillerID          meta.ID `json:"filler_id"`
	FilledAt          string  `json:"filled_at"`
}

// AnswerSheetStatisticsResponse 答卷统计响应
type AnswerSheetStatisticsResponse struct {
	QuestionnaireCode string  `json:"questionnaire_code"`
	TotalCount        int64   `json:"total_count"`
	AverageScore      float64 `json:"average_score"`
	MaxScore          float64 `json:"max_score"`
	MinScore          float64 `json:"min_score"`
}

// ============= 转换函数 =============

// NewAnswerSheetResponse 从应用层 Result 创建响应
func NewAnswerSheetResponse(result *answersheet.AnswerSheetResult) *AnswerSheetResponse {
	if result == nil {
		return nil
	}

	answers := make([]viewmodel.AnswerDTO, 0, len(result.Answers))
	for _, a := range result.Answers {
		answers = append(answers, viewmodel.AnswerDTO{
			QuestionCode: a.QuestionCode,
			QuestionType: a.QuestionType,
			Value:        a.Value,
			Score:        a.Score,
		})
	}

	return &AnswerSheetResponse{
		ID:                meta.ID(result.ID),
		QuestionnaireCode: result.QuestionnaireCode,
		QuestionnaireVer:  result.QuestionnaireVer,
		Title:             result.QuestionnaireTitle,
		Score:             result.Score,
		FillerID:          meta.ID(result.FillerID),
		FillerName:        result.FillerName,
		Answers:           answers,
		FilledAt:          result.FilledAt.Format("2006-01-02 15:04:05"),
	}
}

// NewAnswerSheetListResponse 从应用层 Result 创建列表响应
func NewAnswerSheetListResponse(result *answersheet.AnswerSheetListResult) *AnswerSheetListResponse {
	if result == nil {
		return &AnswerSheetListResponse{
			Total: 0,
			Items: []AnswerSheetSummaryItem{},
		}
	}

	items := make([]AnswerSheetSummaryItem, 0, len(result.Items))
	for _, item := range result.Items {
		items = append(items, AnswerSheetSummaryItem{
			ID:                meta.ID(item.ID),
			QuestionnaireCode: item.QuestionnaireCode,
			QuestionnaireVer:  item.QuestionnaireVer,
			Title:             item.QuestionnaireTitle,
			Score:             item.Score,
			FillerID:          meta.ID(item.FillerID),
			FilledAt:          item.FilledAt.Format("2006-01-02 15:04:05"),
		})
	}

	return &AnswerSheetListResponse{
		Total: result.Total,
		Items: items,
	}
}

// NewAnswerSheetSummaryListResponse 从应用层 SummaryListResult 创建摘要列表响应
func NewAnswerSheetSummaryListResponse(result *answersheet.AnswerSheetSummaryListResult) *AnswerSheetListResponse {
	if result == nil {
		return &AnswerSheetListResponse{
			Total: 0,
			Items: []AnswerSheetSummaryItem{},
		}
	}

	items := make([]AnswerSheetSummaryItem, 0, len(result.Items))
	for _, item := range result.Items {
		items = append(items, AnswerSheetSummaryItem{
			ID:                meta.ID(item.ID),
			QuestionnaireCode: item.QuestionnaireCode,
			Title:             item.QuestionnaireTitle,
			Score:             item.Score,
			FillerID:          meta.ID(item.FillerID),
			FilledAt:          item.FilledAt.Format("2006-01-02 15:04:05"),
		})
	}

	return &AnswerSheetListResponse{
		Total: result.Total,
		Items: items,
	}
}

// NewAnswerSheetStatisticsResponse 从应用层 Statistics 创建响应
func NewAnswerSheetStatisticsResponse(stats *answersheet.AnswerSheetStatistics) *AnswerSheetStatisticsResponse {
	if stats == nil {
		return nil
	}

	return &AnswerSheetStatisticsResponse{
		QuestionnaireCode: stats.QuestionnaireCode,
		TotalCount:        stats.TotalCount,
		AverageScore:      stats.AverageScore,
		MaxScore:          stats.MaxScore,
		MinScore:          stats.MinScore,
	}
}

// ============= 旧的结构（兼容性保留）=============

// SaveAnswerSheetResponse 保存答卷响应
type SaveAnswerSheetResponse struct {
	ID meta.ID `json:"id"`
}

// GetAnswerSheetResponse 获取答卷响应
type GetAnswerSheetResponse struct {
	ID                meta.ID               `json:"id"`
	QuestionnaireCode string                `json:"questionnaire_code"`
	Version           string                `json:"questionnaire_version"`
	Title             string                `json:"title"`
	Score             float64               `json:"score"`
	WriterID          meta.ID               `json:"writer_id"`
	WriterName        string                `json:"writer_name"`
	TesteeID          meta.ID               `json:"testee_id"`
	TesteeName        string                `json:"testee_name"`
	Answers           []viewmodel.AnswerDTO `json:"answers"`
	CreatedAt         string                `json:"created_at"`
	UpdatedAt         string                `json:"updated_at"`
}

// AnswerSheetItem 答卷列表项
type AnswerSheetItem struct {
	ID                meta.ID `json:"id"`
	QuestionnaireCode string  `json:"questionnaire_code"`
	Version           string  `json:"questionnaire_version"`
	Title             string  `json:"title"`
	Score             float64 `json:"score"`
	WriterID          meta.ID `json:"writer_id"`
	TesteeID          meta.ID `json:"testee_id"`
}

// ListAnswerSheetsResponse 获取答卷列表响应
type ListAnswerSheetsResponse struct {
	Total int64             `json:"total"`
	Items []AnswerSheetItem `json:"items"`
}
